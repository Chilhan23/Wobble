package worker

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm"

	"wobble/internal/chat"
	"wobble/internal/realtime"
	"wobble/internal/ticket"
)

type Notifier interface {
	SendToTopic(ctx context.Context, threadID int64, m *chat.Message) (telegramMessageID int64, err error)
	NotifyTicketResolved(ctx context.Context, t *ticket.Ticket) error
}

type Manager struct {
	db         *gorm.DB
	chatRepo   chat.Repository
	ticketRepo ticket.Repository
	notifier   Notifier
	publisher  realtime.Publisher
}

func NewManager(
	db *gorm.DB,
	chatRepo chat.Repository,
	ticketRepo ticket.Repository,
	notifier Notifier,
	publisher realtime.Publisher,
) *Manager {
	return &Manager{
		db:         db,
		chatRepo:   chatRepo,
		ticketRepo: ticketRepo,
		notifier:   notifier,
		publisher:  publisher,
	}
}

// Start meluncurkan seluruh background worker (Outbox, Auto-Close, dan Data Cleanup)
func (m *Manager) Start(ctx context.Context) {
	go m.runOutboxWorker(ctx, 5*time.Second)
	go m.runAutoCloseWorker(ctx, 1*time.Minute)
	go m.runCleanupWorker(ctx, 1*time.Hour)
}

func (m *Manager) runOutboxWorker(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	m.ProcessOutbox(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.ProcessOutbox(ctx)
		}
	}
}

func (m *Manager) ProcessOutbox(ctx context.Context) {
	pending, err := m.chatRepo.ListPendingOutbox(ctx, 20)
	if err != nil {
		slog.Error("failed to list pending outbox", "err", err)
		return
	}

	for _, msg := range pending {
		// 1. Pesan pending lebih dari 10 menit ditandai failed
		if time.Since(msg.CreatedAt) > 10*time.Minute {
			_ = m.chatRepo.UpdateDelivery(ctx, msg.ID, chat.DeliveryStatusFailed, nil)
			continue
		}

		// 2. Kirim ulang pesan ke topic Telegram
		if m.notifier == nil {
			continue
		}

		tck, err := m.ticketRepo.GetByID(ctx, msg.TicketID)
		if err != nil || tck == nil || tck.TelegramThreadID == nil || *tck.TelegramThreadID == 0 {
			continue
		}

		tgMsgID, sendErr := m.notifier.SendToTopic(ctx, *tck.TelegramThreadID, &msg)
		if sendErr == nil && tgMsgID > 0 {
			_ = m.chatRepo.UpdateDelivery(ctx, msg.ID, chat.DeliveryStatusSent, &tgMsgID)
		}
	}
}

func (m *Manager) runAutoCloseWorker(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	m.ProcessAutoClose(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.ProcessAutoClose(ctx)
		}
	}
}

func (m *Manager) ProcessAutoClose(ctx context.Context) {
	// 1. Auto-close tiket open idle > 12 jam
	var openStaleTickets []ticket.Ticket
	openCutoff := time.Now().Add(-12 * time.Hour)
	err := m.db.WithContext(ctx).
		Where("status = ? AND COALESCE(last_user_message_at, created_at) < ?", ticket.StatusOpen, openCutoff).
		Find(&openStaleTickets).Error

	if err == nil {
		for _, t := range openStaleTickets {
			m.autoResolveTicket(ctx, &t, "Tiket ditutup otomatis oleh sistem karena tidak ada aktivitas selama 12 jam.")
			if m.notifier != nil {
				_ = m.notifier.NotifyTicketResolved(ctx, &t)
			}
		}
	}

	// 2. Auto-close tiket escalated idle > 72 jam (berdasarkan waktu update terakhir agar tidak menutup tiket saat programmer aktif)
	var escalatedStaleTickets []ticket.Ticket
	escalatedCutoff := time.Now().Add(-72 * time.Hour)
	err = m.db.WithContext(ctx).
		Where("status = ? AND updated_at < ?", ticket.StatusEscalated, escalatedCutoff).
		Find(&escalatedStaleTickets).Error

	if err == nil {
		for _, t := range escalatedStaleTickets {
			m.autoResolveTicket(ctx, &t, "Tiket ditutup otomatis oleh sistem karena tidak ada aktivitas selama 72 jam.")
			if m.notifier != nil {
				_ = m.notifier.NotifyTicketResolved(ctx, &t)
			}
		}
	}
}

func (m *Manager) autoResolveTicket(ctx context.Context, t *ticket.Ticket, reason string) {
	if err := m.ticketRepo.Resolve(ctx, t.ID); err != nil {
		return
	}

	systemName := "System"
	sysMsg := &chat.Message{
		TicketID:       t.ID,
		SenderType:     chat.SenderTypeSystem,
		SenderName:     &systemName,
		Message:        reason,
		DeliveryStatus: chat.DeliveryStatusSent,
	}
	_, _ = m.chatRepo.Create(ctx, sysMsg)

	if m.publisher != nil {
		m.publisher.Publish(t.PublicID.String(), "ticket_resolved", map[string]any{
			"status":  "resolved",
			"message": reason,
		})
	}
}

func (m *Manager) runCleanupWorker(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	m.ProcessCleanup(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.ProcessCleanup(ctx)
		}
	}
}

func (m *Manager) ProcessCleanup(ctx context.Context) {
	// Hapus riwayat telegram_updates yang lebih dari 7 hari
	cutoff := time.Now().Add(-7 * 24 * time.Hour)
	res := m.db.WithContext(ctx).Exec("DELETE FROM telegram_updates WHERE received_at < ?", cutoff)
	if res.Error != nil {
		slog.Error("failed to cleanup stale telegram updates", "err", res.Error)
	} else if res.RowsAffected > 0 {
		slog.Info(fmt.Sprintf("cleaned up %d stale telegram updates", res.RowsAffected))
	}
}
