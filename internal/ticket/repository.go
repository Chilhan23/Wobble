package ticket

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrNotFound       = errors.New("ticket not found")
	ErrConflict       = errors.New("ticket conflict")
	ErrAlreadyClaimed = errors.New("ticket already claimed by another programmer")
	ErrAlreadyRated   = errors.New("ticket already rated")
	ErrInvalidStatus  = errors.New("invalid ticket status for this operation")
)

type Repository interface {
	Create(ctx context.Context, t *Ticket) (*Ticket, error)
	GetByID(ctx context.Context, id int64) (*Ticket, error)
	GetByPublicID(ctx context.Context, publicID uuid.UUID) (*Ticket, error)
	GetActiveByUser(ctx context.Context, tenantID int64, userID string) (*Ticket, error)
	GetByThreadID(ctx context.Context, threadID int64) (*Ticket, error)
	Claim(ctx context.Context, id int64, programmerName string, tgUserID int64) (*Ticket, error)
	RevertClaim(ctx context.Context, id int64) error
	SetThread(ctx context.Context, id int64, threadID int64) error
	SetCardMessageID(ctx context.Context, id int64, cardMessageID int64) error
	Resolve(ctx context.Context, id int64) error
	SaveRating(ctx context.Context, id int64, rating int16, review string) error
	TouchLastUserMessage(ctx context.Context, id int64) error
	TouchActivity(ctx context.Context, id int64) error
	ListIdle(ctx context.Context, openThreshold time.Duration, escalatedThreshold time.Duration) ([]Ticket, error)
}

type repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) Repository {
	return &repository{db: db}
}

func (r *repository) Create(ctx context.Context, t *Ticket) (*Ticket, error) {
	if t.PublicID == uuid.Nil {
		t.PublicID = uuid.New()
	}
	if t.Status == "" {
		t.Status = StatusOpen
	}

	err := r.db.WithContext(ctx).Create(t).Error
	if err != nil {
		// Jika terjadi race condition pada index unik uq_tickets_active_per_user (status <> 'resolved')
		existing, getErr := r.GetActiveByUser(ctx, t.TenantID, t.UserID)
		if getErr == nil && existing != nil {
			return existing, nil
		}
		return nil, fmt.Errorf("failed to create ticket: %w", err)
	}

	return t, nil
}

func (r *repository) GetByID(ctx context.Context, id int64) (*Ticket, error) {
	var t Ticket
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&t).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get ticket by id: %w", err)
	}
	return &t, nil
}

func (r *repository) GetByPublicID(ctx context.Context, publicID uuid.UUID) (*Ticket, error) {
	var t Ticket
	err := r.db.WithContext(ctx).Where("public_id = ?", publicID).First(&t).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get ticket by public id: %w", err)
	}
	return &t, nil
}

func (r *repository) GetActiveByUser(ctx context.Context, tenantID int64, userID string) (*Ticket, error) {
	var t Ticket
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND user_id = ? AND status <> ?", tenantID, userID, StatusResolved).
		Order("id DESC").
		First(&t).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get active ticket: %w", err)
	}
	return &t, nil
}

func (r *repository) GetByThreadID(ctx context.Context, threadID int64) (*Ticket, error) {
	var t Ticket
	err := r.db.WithContext(ctx).
		Where("telegram_thread_id = ?", threadID).
		First(&t).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get ticket by thread id: %w", err)
	}
	return &t, nil
}

func (r *repository) Claim(ctx context.Context, id int64, programmerName string, tgUserID int64) (*Ticket, error) {
	var updated Ticket
	now := time.Now()

	res := r.db.WithContext(ctx).Raw(`
		UPDATE tickets
		SET status = ?,
		    assigned_programmer = ?,
		    assigned_tg_user_id = ?,
		    claimed_at = ?
		WHERE id = ? AND status = ?
		RETURNING *
	`, StatusEscalated, programmerName, tgUserID, now, id, StatusOpen).Scan(&updated)

	if res.Error != nil {
		return nil, fmt.Errorf("failed to claim ticket: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return nil, ErrAlreadyClaimed
	}

	return &updated, nil
}

func (r *repository) RevertClaim(ctx context.Context, id int64) error {
	res := r.db.WithContext(ctx).Exec(`
		UPDATE tickets
		SET status = ?,
		    assigned_programmer = NULL,
		    assigned_tg_user_id = NULL,
		    claimed_at = NULL
		WHERE id = ? AND status = ? AND telegram_thread_id IS NULL
	`, StatusOpen, id, StatusEscalated)

	if res.Error != nil {
		return fmt.Errorf("failed to revert claim: %w", res.Error)
	}
	return nil
}

func (r *repository) SetThread(ctx context.Context, id int64, threadID int64) error {
	res := r.db.WithContext(ctx).Model(&Ticket{}).
		Where("id = ?", id).
		Update("telegram_thread_id", threadID)
	if res.Error != nil {
		return fmt.Errorf("failed to set thread id: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *repository) SetCardMessageID(ctx context.Context, id int64, cardMessageID int64) error {
	res := r.db.WithContext(ctx).Model(&Ticket{}).
		Where("id = ? AND tg_card_message_id IS NULL", id).
		Update("tg_card_message_id", cardMessageID)
	if res.Error != nil {
		return fmt.Errorf("failed to set card message id: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *repository) Resolve(ctx context.Context, id int64) error {
	now := time.Now()
	res := r.db.WithContext(ctx).Exec(`
		UPDATE tickets
		SET status = ?,
		    resolved_at = ?
		WHERE id = ? AND status <> ?
	`, StatusResolved, now, id, StatusResolved)

	if res.Error != nil {
		return fmt.Errorf("failed to resolve ticket: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *repository) SaveRating(ctx context.Context, id int64, rating int16, review string) error {
	var rev *string
	if review != "" {
		rev = &review
	}

	res := r.db.WithContext(ctx).Exec(`
		UPDATE tickets
		SET csat_rating = ?,
		    csat_review = ?
		WHERE id = ? AND status = ? AND csat_rating IS NULL
	`, rating, rev, id, StatusResolved)

	if res.Error != nil {
		return fmt.Errorf("failed to save rating: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		// Cek apakah tiket tidak ditemukan atau sudah dirating
		var t Ticket
		err := r.db.WithContext(ctx).Select("id, status, csat_rating").Where("id = ?", id).First(&t).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return fmt.Errorf("failed to check rating state: %w", err)
		}
		if t.Status != StatusResolved {
			return ErrInvalidStatus
		}
		if t.CSATRating != nil {
			return ErrAlreadyRated
		}
		return ErrNotFound
	}
	return nil
}

func (r *repository) TouchLastUserMessage(ctx context.Context, id int64) error {
	res := r.db.WithContext(ctx).Model(&Ticket{}).
		Where("id = ?", id).
		Update("last_user_message_at", time.Now())
	if res.Error != nil {
		return fmt.Errorf("failed to touch last user message: %w", res.Error)
	}
	return nil
}

func (r *repository) TouchActivity(ctx context.Context, id int64) error {
	res := r.db.WithContext(ctx).Model(&Ticket{}).
		Where("id = ?", id).
		Update("updated_at", time.Now())
	if res.Error != nil {
		return fmt.Errorf("failed to touch ticket activity: %w", res.Error)
	}
	return nil
}

func (r *repository) ListIdle(ctx context.Context, openThreshold time.Duration, escalatedThreshold time.Duration) ([]Ticket, error) {
	var tickets []Ticket
	now := time.Now()
	openCutoff := now.Add(-openThreshold)
	escalatedCutoff := now.Add(-escalatedThreshold)

	err := r.db.WithContext(ctx).Where(`
		(status = ? AND (
			(last_user_message_at IS NOT NULL AND last_user_message_at < ?) OR
			(last_user_message_at IS NULL AND created_at < ?)
		))
		OR
		(status = ? AND updated_at < ?)
	`, StatusOpen, openCutoff, openCutoff, StatusEscalated, escalatedCutoff).Find(&tickets).Error

	if err != nil {
		return nil, fmt.Errorf("failed to list idle tickets: %w", err)
	}
	return tickets, nil
}
