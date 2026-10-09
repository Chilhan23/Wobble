package chat

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"unicode/utf8"

	"github.com/google/uuid"

	"wobble/internal/ai"
	"wobble/internal/ticket"
)

var (
	ErrTicketResolved = errors.New("cannot send message to a resolved ticket")
	ErrEmptyMessage   = errors.New("message content cannot be empty")
)

type Notifier interface {
	NotifyNewTicket(ctx context.Context, t *ticket.Ticket, summary string) (cardMessageID int64, err error)
	SendToTopic(ctx context.Context, threadID int64, m *Message) (telegramMessageID int64, err error)
}

type Publisher interface {
	Publish(roomKey string, event string, data any)
}

type AIClient interface {
	GenerateReply(ctx context.Context, systemPrompt string, history []ai.MessageHistory, newMsg string) (string, error)
	Enabled() bool
}

type URLSigner interface {
	SignURL(messageID int64) string
}

type Service interface {
	SendUserMessage(ctx context.Context, tck *ticket.Ticket, req SendMessageRequest) (*MessageDTO, error)
	SendUserAttachment(ctx context.Context, tck *ticket.Ticket, att *AttachmentInput, caption string, clientMsgID *uuid.UUID) (*MessageDTO, error)
	ListMessages(ctx context.Context, ticketID int64, afterID int64, limit int) ([]MessageDTO, error)
}

type service struct {
	chatRepo     Repository
	ticketRepo   ticket.Repository
	publisher    Publisher
	notifier     Notifier
	aiClient     AIClient
	urlSigner    URLSigner
	historyLimit int
	systemPrompt string
	ticketLocks  sync.Map // per-ticket mutex untuk serialisasi AI calls
}

func NewService(
	chatRepo Repository,
	ticketRepo ticket.Repository,
	publisher Publisher,
	notifier Notifier,
	aiClient AIClient,
	urlSigner URLSigner,
	historyLimit int,
	systemPrompt string,
) Service {
	if historyLimit < 1 {
		historyLimit = 10
	}
	if systemPrompt == "" {
		systemPrompt = "Anda adalah asisten AI ramah dan profesional untuk Helpdesk SIMRS (Sistem Informasi Manajemen Rumah Sakit). Bantu pengguna menyelesaikan masalah aplikasi dengan singkat, jelas, dan santun. Jika kendala membutuhkan perbaikan data atau sistem internal, sarankan pengguna menunggu bantuan dari tim IT Support / Programmer."
	}
	return &service{
		chatRepo:     chatRepo,
		ticketRepo:   ticketRepo,
		publisher:    publisher,
		notifier:     notifier,
		aiClient:     aiClient,
		urlSigner:    urlSigner,
		historyLimit: historyLimit,
		systemPrompt: systemPrompt,
	}
}

func (s *service) getTicketLock(ticketID int64) *sync.Mutex {
	lock, _ := s.ticketLocks.LoadOrStore(ticketID, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

func (s *service) getSignedURL(m *Message) string {
	if s.urlSigner != nil && m.AttachmentPath != nil && *m.AttachmentPath != "" {
		return s.urlSigner.SignURL(m.ID)
	}
	return ""
}

func (s *service) SendUserMessage(ctx context.Context, tck *ticket.Ticket, req SendMessageRequest) (*MessageDTO, error) {
	if tck.Status == ticket.StatusResolved {
		return nil, ErrTicketResolved
	}
	if req.Message == "" {
		return nil, ErrEmptyMessage
	}

	userName := tck.UserName
	msg := &Message{
		TicketID:       tck.ID,
		SenderType:     SenderTypeUser,
		SenderName:     &userName,
		Message:        req.Message,
		DeliveryStatus: DeliveryStatusSent,
		ClientMsgID:    req.ClientMsgID,
	}

	if tck.Status == ticket.StatusEscalated {
		msg.DeliveryStatus = DeliveryStatusPending
	}

	savedMsg, err := s.chatRepo.Create(ctx, msg)
	if err != nil {
		return nil, fmt.Errorf("failed to save user message: %w", err)
	}

	// Sentuh timeline pesan terakhir user
	_ = s.ticketRepo.TouchLastUserMessage(ctx, tck.ID)

	dto := savedMsg.ToDTO(s.getSignedURL(savedMsg))

	// Publish WS event
	if s.publisher != nil {
		s.publisher.Publish(tck.PublicID.String(), "helpdesk_new_message", dto)
	}

	// 1. Kirim Kartu General pada pesan user pertama secara asinkron dan atomik
	summary := req.Message
	if utf8.RuneCountInString(summary) > 150 {
		summary = string([]rune(summary)[:150]) + "..."
	}
	s.ensureTelegramCardAsync(tck.ID, summary)

	// 2. Alur Routing Pesan:
	// - Jika escalated: relay ke topic Telegram
	// - Jika open: panggil AI secara async
	if tck.Status == ticket.StatusEscalated {
		if s.notifier != nil && tck.TelegramThreadID != nil {
			go func(threadID int64, m *Message) {
				tgMsgID, err := s.notifier.SendToTopic(context.Background(), threadID, m)
				if err == nil && tgMsgID > 0 {
					_ = s.chatRepo.UpdateDelivery(context.Background(), m.ID, DeliveryStatusSent, &tgMsgID)
				} else {
					slog.Error("failed to forward message to telegram topic", "thread_id", threadID, "err", err)
				}
			}(*tck.TelegramThreadID, savedMsg)
		}
	} else if tck.Status == ticket.StatusOpen {
		go s.processAIReply(context.Background(), tck, savedMsg.ID, req.Message)
	}

	return &dto, nil
}

func (s *service) SendUserAttachment(ctx context.Context, tck *ticket.Ticket, att *AttachmentInput, caption string, clientMsgID *uuid.UUID) (*MessageDTO, error) {
	if tck.Status == ticket.StatusResolved {
		return nil, ErrTicketResolved
	}
	if att == nil || att.RelativePath == "" {
		return nil, errors.New("attachment payload is required")
	}

	userName := tck.UserName
	msg := &Message{
		TicketID:       tck.ID,
		SenderType:     SenderTypeUser,
		SenderName:     &userName,
		Message:        caption,
		AttachmentType: &att.Type,
		AttachmentPath: &att.RelativePath,
		AttachmentMIME: &att.MIMEType,
		AttachmentSize: &att.Size,
		DeliveryStatus: DeliveryStatusSent,
		ClientMsgID:    clientMsgID,
	}

	if tck.Status == ticket.StatusEscalated {
		msg.DeliveryStatus = DeliveryStatusPending
	}

	savedMsg, err := s.chatRepo.Create(ctx, msg)
	if err != nil {
		return nil, fmt.Errorf("failed to save attachment message: %w", err)
	}

	_ = s.ticketRepo.TouchLastUserMessage(ctx, tck.ID)

	dto := savedMsg.ToDTO(s.getSignedURL(savedMsg))

	if s.publisher != nil {
		s.publisher.Publish(tck.PublicID.String(), "helpdesk_new_message", dto)
	}

	summary := "[Lampiran] " + caption
	if caption == "" {
		summary = fmt.Sprintf("[Lampiran %s]", att.Type)
	}
	s.ensureTelegramCardAsync(tck.ID, summary)

	if tck.Status == ticket.StatusEscalated && s.notifier != nil && tck.TelegramThreadID != nil {
		go func(threadID int64, m *Message) {
			tgMsgID, err := s.notifier.SendToTopic(context.Background(), threadID, m)
			if err == nil && tgMsgID > 0 {
				_ = s.chatRepo.UpdateDelivery(context.Background(), m.ID, DeliveryStatusSent, &tgMsgID)
			}
		}(*tck.TelegramThreadID, savedMsg)
	}

	return &dto, nil
}

func (s *service) ensureTelegramCardAsync(ticketID int64, summary string) {
	if s.notifier == nil {
		return
	}
	go func() {
		mu := s.getTicketLock(ticketID)
		mu.Lock()
		defer mu.Unlock()

		ctx := context.Background()
		curr, err := s.ticketRepo.GetByID(ctx, ticketID)
		if err != nil || curr.TGCardMessageID != nil || curr.Status == ticket.StatusResolved {
			return
		}

		cardID, err := s.notifier.NotifyNewTicket(ctx, curr, summary)
		if err == nil && cardID > 0 {
			_ = s.ticketRepo.SetCardMessageID(ctx, ticketID, cardID)
		} else if err != nil {
			slog.Error("failed to send telegram ticket card", "ticket", curr.TicketCode, "err", err)
		}
	}()
}

func (s *service) processAIReply(ctx context.Context, tck *ticket.Ticket, currentMsgID int64, newMsg string) {
	mu := s.getTicketLock(tck.ID)
	mu.Lock()
	defer mu.Unlock()

	// Pastikan status tiket masih open
	currTicket, err := s.ticketRepo.GetByID(ctx, tck.ID)
	if err != nil || currTicket.Status != ticket.StatusOpen {
		return // Jangan proses AI jika tiket sudah diklaim / resolved
	}

	if s.publisher != nil {
		s.publisher.Publish(tck.PublicID.String(), "ai_typing", map[string]bool{"typing": true})
		defer s.publisher.Publish(tck.PublicID.String(), "ai_typing", map[string]bool{"typing": false})
	}

	var replyText string
	var replySender SenderType = SenderTypeAI
	aiName := "TechNova Assistant"

	if s.aiClient != nil && s.aiClient.Enabled() {
		// Ambil riwayat percakapan terbaru sebelum pesan aktif
		rawHistory, _ := s.chatRepo.ListRecentByTicket(ctx, tck.ID, currentMsgID, s.historyLimit)
		history := make([]ai.MessageHistory, 0, len(rawHistory))
		for _, m := range rawHistory {
			if m.SenderType != SenderTypeUser && m.SenderType != SenderTypeAI {
				continue
			}
			role := "user"
			if m.SenderType == SenderTypeAI {
				role = "assistant"
			}
			history = append(history, ai.MessageHistory{
				Role:    role,
				Content: m.Message,
			})
		}

		generated, genErr := s.aiClient.GenerateReply(ctx, s.systemPrompt, history, newMsg)
		if genErr != nil {
			slog.Warn("ai generation failed, falling back to system message", "ticket", tck.TicketCode, "err", genErr)
			replyText = "Tim IT Support akan segera membantu kendala Anda. Mohon ditunggu."
			replySender = SenderTypeSystem
			aiName = "System"
		} else {
			replyText = generated
		}
	} else {
		replyText = "Tim IT Support akan segera membantu kendala Anda. Mohon ditunggu."
		replySender = SenderTypeSystem
		aiName = "System"
	}

	// Simpan balasan AI / System ke database
	aiMsg := &Message{
		TicketID:       tck.ID,
		SenderType:     replySender,
		SenderName:     &aiName,
		Message:        replyText,
		DeliveryStatus: DeliveryStatusSent,
	}

	savedReply, err := s.chatRepo.Create(ctx, aiMsg)
	if err != nil {
		slog.Error("failed to save ai reply message", "ticket", tck.TicketCode, "err", err)
		return
	}

	// Broadcast ke WebSocket
	if s.publisher != nil {
		s.publisher.Publish(tck.PublicID.String(), "helpdesk_new_message", savedReply.ToDTO(""))
	}
}

func (s *service) ListMessages(ctx context.Context, ticketID int64, afterID int64, limit int) ([]MessageDTO, error) {
	msgs, err := s.chatRepo.ListByTicket(ctx, ticketID, afterID, limit)
	if err != nil {
		return nil, err
	}

	dtos := make([]MessageDTO, len(msgs))
	for i, m := range msgs {
		dtos[i] = m.ToDTO(s.getSignedURL(&m))
	}
	return dtos, nil
}
