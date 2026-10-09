package chat

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"wobble/internal/storage"
)

var (
	ErrNotFound = errors.New("message not found")
	ErrConflict = errors.New("message conflict")
)

type Repository interface {
	Create(ctx context.Context, m *Message) (*Message, error)
	ListByTicket(ctx context.Context, ticketID int64, afterID int64, limit int) ([]Message, error)
	GetByID(ctx context.Context, id int64) (*Message, error)
	GetAttachmentInfo(ctx context.Context, messageID int64) (*storage.AttachmentInfo, error)
	UpdateDelivery(ctx context.Context, id int64, status DeliveryStatus, telegramMessageID *int64) error
	ListPendingOutbox(ctx context.Context, limit int) ([]Message, error)
	ListRecentByTicket(ctx context.Context, ticketID int64, beforeID int64, limit int) ([]Message, error)
}

type repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) Repository {
	return &repository{db: db}
}

func (r *repository) Create(ctx context.Context, m *Message) (*Message, error) {
	if m.DeliveryStatus == "" {
		m.DeliveryStatus = DeliveryStatusSent
	}

	err := r.db.WithContext(ctx).Create(m).Error
	if err != nil {
		// Idempotensi widget: cek apakah client_msg_id sudah ada untuk tiket ini
		if m.ClientMsgID != nil {
			var existing Message
			findErr := r.db.WithContext(ctx).
				Where("ticket_id = ? AND client_msg_id = ?", m.TicketID, m.ClientMsgID).
				First(&existing).Error
			if findErr == nil {
				return &existing, nil
			}
		}
		return nil, fmt.Errorf("failed to create message: %w", err)
	}
	return m, nil
}

func (r *repository) ListByTicket(ctx context.Context, ticketID int64, afterID int64, limit int) ([]Message, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	var msgs []Message
	query := r.db.WithContext(ctx).
		Where("ticket_id = ?", ticketID)

	if afterID > 0 {
		query = query.Where("id > ?", afterID)
	}

	err := query.Order("id ASC").Limit(limit).Find(&msgs).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list messages: %w", err)
	}
	return msgs, nil
}

func (r *repository) ListRecentByTicket(ctx context.Context, ticketID int64, beforeID int64, limit int) ([]Message, error) {
	if limit <= 0 || limit > 100 {
		limit = 10
	}

	var msgs []Message
	query := r.db.WithContext(ctx).Where("ticket_id = ?", ticketID)
	if beforeID > 0 {
		query = query.Where("id < ?", beforeID)
	}

	err := query.Order("id DESC").Limit(limit).Find(&msgs).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list recent messages: %w", err)
	}

	// Balik urutan ke ASC kronologis agar pembacaan konteks AI sesuai
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}

	return msgs, nil
}

func (r *repository) GetByID(ctx context.Context, id int64) (*Message, error) {
	var m Message
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get message by id: %w", err)
	}
	return &m, nil
}

func (r *repository) GetAttachmentInfo(ctx context.Context, messageID int64) (*storage.AttachmentInfo, error) {
	msg, err := r.GetByID(ctx, messageID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, storage.ErrAttachmentNotFound
		}
		return nil, err
	}
	path := ""
	mime := ""
	if msg.AttachmentPath != nil {
		path = *msg.AttachmentPath
	}
	if msg.AttachmentMIME != nil {
		mime = *msg.AttachmentMIME
	}
	return &storage.AttachmentInfo{
		Path: path,
		MIME: mime,
	}, nil
}

func (r *repository) UpdateDelivery(ctx context.Context, id int64, status DeliveryStatus, telegramMessageID *int64) error {
	updates := map[string]any{
		"delivery_status": status,
	}
	if telegramMessageID != nil {
		updates["telegram_message_id"] = telegramMessageID
	}

	res := r.db.WithContext(ctx).Model(&Message{}).
		Where("id = ?", id).
		Updates(updates)

	if res.Error != nil {
		return fmt.Errorf("failed to update message delivery status: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *repository) ListPendingOutbox(ctx context.Context, limit int) ([]Message, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	var msgs []Message
	// Beri umur minimum 5 detik agar tidak berbenturan dengan pengiriman sinkron
	cutoff := time.Now().Add(-5 * time.Second)
	err := r.db.WithContext(ctx).
		Where("delivery_status = ? AND created_at <= ?", DeliveryStatusPending, cutoff).
		Order("id ASC").
		Limit(limit).
		Find(&msgs).Error

	if err != nil {
		return nil, fmt.Errorf("failed to list pending outbox messages: %w", err)
	}
	return msgs, nil
}
