package telegram

import (
	"bytes"
	"context"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"wobble/internal/chat"
	"wobble/internal/realtime"
	"wobble/internal/storage"
	"wobble/internal/tenant"
	"wobble/internal/ticket"
)

type WebhookHandler struct {
	client     Client
	dedupStore DedupStore
	ticketRepo ticket.Repository
	chatRepo   chat.Repository
	tenantRepo tenant.Repository
	publisher  realtime.Publisher
	storage    storage.Service
	urlSigner  *storage.URLSigner
	chatID     int64
}

func NewWebhookHandler(
	client Client,
	dedupStore DedupStore,
	ticketRepo ticket.Repository,
	chatRepo chat.Repository,
	tenantRepo tenant.Repository,
	publisher realtime.Publisher,
	chatID int64,
	storage storage.Service,
	urlSigner *storage.URLSigner,
) *WebhookHandler {
	return &WebhookHandler{
		client:     client,
		dedupStore: dedupStore,
		ticketRepo: ticketRepo,
		chatRepo:   chatRepo,
		tenantRepo: tenantRepo,
		publisher:  publisher,
		storage:    storage,
		urlSigner:  urlSigner,
		chatID:     chatID,
	}
}

func (h *WebhookHandler) getTenantDisplayName(ctx context.Context, tenantID int64) string {
	if h.tenantRepo != nil {
		if ten, err := h.tenantRepo.GetByID(ctx, tenantID); err == nil && ten != nil {
			if ten.TenantName != "" {
				return ten.TenantName
			}
			if ten.AppName != "" {
				return ten.AppName
			}
		}
	}
	return "Aplikasi"
}

// RegisterRoutes mendaftarkan webhook endpoint
func (h *WebhookHandler) RegisterRoutes(r *gin.Engine, verifySecret gin.HandlerFunc) {
	r.POST("/webhook/telegram", verifySecret, h.HandleWebhook)
}

// HandleWebhook menangani webhook update dari Telegram Bot API
func (h *WebhookHandler) HandleWebhook(c *gin.Context) {
	var update Update
	if err := c.ShouldBindJSON(&update); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": false, "message": "invalid update json"})
		return
	}

	// 1. Deduplikasi Update ID
	if update.UpdateID > 0 && h.dedupStore != nil {
		isNew, err := h.dedupStore.RecordUpdateID(c.Request.Context(), update.UpdateID)
		if err != nil {
			slog.Error("failed to record update dedup", "update_id", update.UpdateID, "err", err)
		} else if !isNew {
			// Update sudah pernah diproses, balas 200 segera
			c.JSON(http.StatusOK, gin.H{"ok": true, "dedup": true})
			return
		}
	}

	// 2. Balas HTTP 200 OK ke Telegram terlebih dahulu
	c.JSON(http.StatusOK, gin.H{"ok": true})

	// 3. Proses update di goroutine terpisah agar respons webhook instan
	go func(u Update) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("recovered from panic in telegram webhook worker", "panic", r, "update_id", u.UpdateID)
			}
		}()
		h.processUpdate(context.Background(), u)
	}(update)
}

func (h *WebhookHandler) processUpdate(ctx context.Context, u Update) {
	if u.CallbackQuery != nil {
		h.handleCallbackQuery(ctx, u.CallbackQuery)
		return
	}

	if u.Message != nil {
		h.handleMessage(ctx, u.Message)
		return
	}
}

func (h *WebhookHandler) handleCallbackQuery(ctx context.Context, cb *CallbackQuery) {
	if cb.Message == nil || cb.Message.Chat.ID != h.chatID {
		_ = h.client.AnswerCallbackQuery(ctx, cb.ID, "Invalid supergroup callback")
		return
	}

	if !strings.HasPrefix(cb.Data, "claim:") {
		_ = h.client.AnswerCallbackQuery(ctx, cb.ID, "")
		return
	}

	rawUUID := strings.TrimPrefix(cb.Data, "claim:")
	publicID, err := uuid.Parse(rawUUID)
	if err != nil {
		_ = h.client.AnswerCallbackQuery(ctx, cb.ID, "Format tiket tidak valid")
		return
	}

	tck, err := h.ticketRepo.GetByPublicID(ctx, publicID)
	if err != nil {
		_ = h.client.AnswerCallbackQuery(ctx, cb.ID, "Tiket tidak ditemukan")
		return
	}

	if tck.Status == ticket.StatusResolved {
		_ = h.client.AnswerCallbackQuery(ctx, cb.ID, "Tiket ini sudah ditutup/selesai")
		return
	}

	programmerName := cb.From.DisplayName()
	tgUserID := cb.From.ID

	// Atomic claim di database
	claimedTicket, err := h.ticketRepo.Claim(ctx, tck.ID, programmerName, tgUserID)
	if err != nil {
		_ = h.client.AnswerCallbackQuery(ctx, cb.ID, "Tiket sudah diklaim oleh programmer lain")
		return
	}

	appName := h.getTenantDisplayName(ctx, claimedTicket.TenantID)

	// Buat Forum Topic di supergroup Telegram dengan identitas App/Tenant
	topicName := fmt.Sprintf("[%s] [%s] %s - %s", appName, claimedTicket.ModuleName, claimedTicket.UserName, claimedTicket.TicketCode)
	threadID, err := h.client.CreateForumTopic(ctx, h.chatID, topicName)
	if err != nil {
		slog.Error("failed to create forum topic, reverting claim", "ticket", claimedTicket.TicketCode, "err", err)
		_ = h.ticketRepo.RevertClaim(ctx, claimedTicket.ID)
		_ = h.client.AnswerCallbackQuery(ctx, cb.ID, "Gagal membuat topik, silakan coba lagi")
		return
	}

	// Simpan Thread ID dengan penanganan error
	if err := h.ticketRepo.SetThread(ctx, claimedTicket.ID, threadID); err != nil {
		slog.Error("failed to save thread id, reverting claim and closing topic", "ticket", claimedTicket.TicketCode, "err", err)
		_ = h.client.CloseForumTopic(ctx, h.chatID, threadID)
		_ = h.ticketRepo.RevertClaim(ctx, claimedTicket.ID)
		_ = h.client.AnswerCallbackQuery(ctx, cb.ID, "Gagal menyimpan thread tiket, silakan coba lagi")
		return
	}

	// Edit kartu di General: hapus tombol dan beri tanda klaim
	if cb.Message != nil {
		updatedCardText := fmt.Sprintf(
			"🎫 <b>TIKET BANTUAN</b>\n"+
				"<b>System:</b> %s\n"+
				"<b>Kode:</b> <code>%s</code>\n"+
				"<b>User:</b> %s\n"+
				"<b>Modul:</b> %s\n\n"+
				"✅ <b>DIKLAIM OLEH:</b> %s",
			html.EscapeString(appName),
			html.EscapeString(claimedTicket.TicketCode),
			html.EscapeString(claimedTicket.UserName),
			html.EscapeString(claimedTicket.ModuleName),
			html.EscapeString(programmerName),
		)
		_ = h.client.EditMessageText(ctx, h.chatID, cb.Message.MessageID, updatedCardText, nil)
	}

	// Kirim sapaan ke topik baru
	greeting := fmt.Sprintf(
		"👋 <b>Topik Tiket Dimulai</b>\n"+
			"<b>System:</b> %s\n"+
			"<b>Programmer:</b> %s\n"+
			"<b>Pengguna:</b> %s\n"+
			"<b>Modul:</b> %s\n\n"+
			"<i>Percakapan langsung dengan pengguna dimulai di topik ini. Ketik <code>/close</code> atau <code>/selesai</code> jika kendala telah selesai.</i>",
		html.EscapeString(appName),
		html.EscapeString(programmerName),
		html.EscapeString(claimedTicket.UserName),
		html.EscapeString(claimedTicket.ModuleName),
	)
	_, _ = h.client.SendMessage(ctx, h.chatID, &threadID, greeting, nil)

	// Replay riwayat pesan user ke dalam topik
	history, _ := h.chatRepo.ListByTicket(ctx, claimedTicket.ID, 0, 10)
	for _, m := range history {
		if m.SenderType == chat.SenderTypeUser {
			sender := "User"
			if m.SenderName != nil && *m.SenderName != "" {
				sender = *m.SenderName
			}
			replayText := fmt.Sprintf("👤 <b>%s:</b>\n%s", html.EscapeString(sender), html.EscapeString(m.Message))
			_, _ = h.client.SendMessage(ctx, h.chatID, &threadID, replayText, nil)
		}
	}

	// Broadcast WS event ke browser user (AI otomatis dimatikan)
	if h.publisher != nil {
		h.publisher.Publish(claimedTicket.PublicID.String(), "ticket_claimed", map[string]any{
			"programmer_name": programmerName,
			"status":          "escalated",
		})
	}

	_ = h.client.AnswerCallbackQuery(ctx, cb.ID, "Tiket berhasil diklaim!")
}

func (h *WebhookHandler) handleMessage(ctx context.Context, msg *Message) {
	// 1. Abaikan bot
	if msg.From != nil && msg.From.IsBot {
		return
	}

	// 2. Periksa Chat ID: hanya proses pesan dari supergroup yang berwenang
	if msg.Chat.ID != h.chatID {
		return
	}

	// 3. ATURAN ROUTING MUTLAK: Pesan di General Chat (tanpa thread_id) TIDAK PERNAH diteruskan ke user
	if msg.MessageThreadID == 0 {
		return
	}

	// 4. Ambil tiket berdasarkan Forum Topic ID
	tck, err := h.ticketRepo.GetByThreadID(ctx, msg.MessageThreadID)
	if err != nil || tck == nil || tck.Status == ticket.StatusResolved {
		return
	}

	// 5. Command /close atau /selesai
	if msg.IsCommand("/close") || msg.IsCommand("/selesai") {
		_ = h.ticketRepo.Resolve(ctx, tck.ID)

		if h.publisher != nil {
			h.publisher.Publish(tck.PublicID.String(), "ticket_resolved", map[string]any{
				"status":  "resolved",
				"message": "Tiket telah diselesaikan oleh tim IT support.",
			})
		}

		_, _ = h.client.SendMessage(ctx, h.chatID, &msg.MessageThreadID, "✅ <i>Tiket telah ditutup. Pengguna diminta mengisi rating CSAT di layar web.</i>", nil)
		_ = h.client.CloseForumTopic(ctx, h.chatID, msg.MessageThreadID)

		// Edit kartu di General jika ada
		if tck.TGCardMessageID != nil && *tck.TGCardMessageID > 0 {
			appName := h.getTenantDisplayName(ctx, tck.TenantID)
			cardText := fmt.Sprintf(
				"🎫 <b>TIKET BANTUAN</b>\n"+
					"<b>System:</b> %s\n"+
					"<b>Kode:</b> <code>%s</code>\n"+
					"<b>User:</b> %s\n"+
					"<b>Modul:</b> %s\n\n"+
					"🔒 <b>STATUS:</b> DITUTUP / SELESAI",
				html.EscapeString(appName),
				html.EscapeString(tck.TicketCode),
				html.EscapeString(tck.UserName),
				html.EscapeString(tck.ModuleName),
			)
			_ = h.client.EditMessageText(ctx, h.chatID, *tck.TGCardMessageID, cardText, nil)
		}
		return
	}

	// 6. Pesan balasan teks/caption atau media dari programmer di topik
	var attPath *string
	var attMIME *string
	var attSize *int64
	var attType *chat.AttachmentType

	if h.storage != nil {
		var fileID string
		if len(msg.Photo) > 0 {
			fileID = msg.Photo[len(msg.Photo)-1].FileID
		} else if msg.Video != nil {
			fileID = msg.Video.FileID
		} else if msg.Document != nil {
			fileID = msg.Document.FileID
		}

		if fileID != "" {
			tgFile, fErr := h.client.GetFile(ctx, fileID)
			if fErr == nil && tgFile != nil && tgFile.FilePath != "" {
				fileBytes, dErr := h.client.DownloadFile(ctx, tgFile.FilePath)
				if dErr == nil && len(fileBytes) > 0 {
					saved, sErr := h.storage.Save(tck.PublicID, bytes.NewReader(fileBytes), 10*1024*1024, 30*1024*1024)
					if sErr == nil && saved != nil {
						attPath = &saved.RelativePath
						attMIME = &saved.MIMEType
						attSize = &saved.Size
						t := chat.AttachmentType(saved.Type)
						attType = &t
					}
				}
			}
		}
	}

	content := msg.Text
	if content == "" {
		content = msg.Caption
	}
	if content == "" && attPath == nil {
		return
	}

	senderName := msg.From.DisplayName()
	savedMsg, err := h.chatRepo.Create(ctx, &chat.Message{
		TicketID:       tck.ID,
		SenderType:     chat.SenderTypeProgrammer,
		SenderName:     &senderName,
		Message:        content,
		AttachmentPath: attPath,
		AttachmentMIME: attMIME,
		AttachmentSize: attSize,
		AttachmentType: attType,
		DeliveryStatus: chat.DeliveryStatusSent,
	})
	if err != nil {
		slog.Error("failed to save programmer reply from telegram", "ticket", tck.TicketCode, "err", err)
		return
	}

	// Perbarui updated_at tiket agar auto-close escalated tidak menutup tiket yang aktif
	_ = h.ticketRepo.TouchActivity(ctx, tck.ID)

	// Broadcast ke WebSocket browser user
	if h.publisher != nil {
		attURL := ""
		if savedMsg.AttachmentPath != nil && h.urlSigner != nil {
			attURL = h.urlSigner.SignURL(savedMsg.ID)
		}
		h.publisher.Publish(tck.PublicID.String(), "helpdesk_new_message", savedMsg.ToDTO(attURL))
	}
}
