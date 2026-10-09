package ticket

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"wobble/internal/auth"
)

type TokenIssuer interface {
	Issue(payload auth.TicketTokenPayload) (tokenString string, expiresAt time.Time, err error)
}

type Notifier interface {
	NotifyTicketResolved(ctx context.Context, t *Ticket) error
	NotifyTicketRating(ctx context.Context, t *Ticket, rating int16, review string) error
}

type Service interface {
	Init(ctx context.Context, tenantID int64, req InitTicketRequest) (*InitTicketResponse, error)
	GetByPublicID(ctx context.Context, publicID uuid.UUID) (*TicketDTO, error)
	Resolve(ctx context.Context, publicID uuid.UUID) error
	Rate(ctx context.Context, publicID uuid.UUID, req RateTicketRequest) error
}

type service struct {
	repo     Repository
	tokenSvc TokenIssuer
	notifier Notifier
}

func NewService(repo Repository, tokenSvc TokenIssuer, notifier Notifier) Service {
	return &service{
		repo:     repo,
		tokenSvc: tokenSvc,
		notifier: notifier,
	}
}

func GenerateTicketCode() string {
	b := make([]byte, 3)
	if _, err := rand.Read(b); err != nil {
		// Fallback ke nano timestamp jika rand.Read gagal
		return fmt.Sprintf("TCK-%s-%06x", time.Now().Format("20060102"), time.Now().UnixNano()%0xFFFFFF)
	}
	return fmt.Sprintf("TCK-%s-%s", time.Now().Format("20060102"), hex.EncodeToString(b))
}

func (s *service) Init(ctx context.Context, tenantID int64, req InitTicketRequest) (*InitTicketResponse, error) {
	req.UserID = strings.TrimSpace(req.UserID)
	req.UserName = strings.TrimSpace(req.UserName)
	if req.ModuleName == "" {
		req.ModuleName = "Umum"
	}

	// 1. Cek apakah user sudah punya tiket aktif di tenant ini
	activeTicket, err := s.repo.GetActiveByUser(ctx, tenantID, req.UserID)
	if err == nil && activeTicket != nil {
		token, exp, issueErr := s.tokenSvc.Issue(auth.TicketTokenPayload{
			PublicID: activeTicket.PublicID,
			TenantID: activeTicket.TenantID,
			UserID:   activeTicket.UserID,
			TicketID: activeTicket.ID,
		})
		if issueErr != nil {
			return nil, fmt.Errorf("failed to issue ticket token: %w", issueErr)
		}
		return &InitTicketResponse{
			Status:    true,
			Token:     token,
			ExpiresAt: exp,
			Ticket:    activeTicket.ToDTO(),
		}, nil
	}

	// 2. Buat tiket baru jika belum ada
	newTicket := &Ticket{
		PublicID:       uuid.New(),
		TenantID:       tenantID,
		TicketCode:     GenerateTicketCode(),
		UserID:         req.UserID,
		UserName:       req.UserName,
		ModuleName:     req.ModuleName,
		DiagnosticInfo: req.DiagnosticInfo,
		Status:         StatusOpen,
	}

	createdTicket, err := s.repo.Create(ctx, newTicket)
	if err != nil {
		return nil, fmt.Errorf("failed to create new ticket: %w", err)
	}

	// 3. Terbitkan JWT Token
	token, exp, err := s.tokenSvc.Issue(auth.TicketTokenPayload{
		PublicID: createdTicket.PublicID,
		TenantID: createdTicket.TenantID,
		UserID:   createdTicket.UserID,
		TicketID: createdTicket.ID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to issue ticket token: %w", err)
	}

	return &InitTicketResponse{
		Status:    true,
		Token:     token,
		ExpiresAt: exp,
		Ticket:    createdTicket.ToDTO(),
	}, nil
}

func (s *service) GetByPublicID(ctx context.Context, publicID uuid.UUID) (*TicketDTO, error) {
	t, err := s.repo.GetByPublicID(ctx, publicID)
	if err != nil {
		return nil, err
	}
	dto := t.ToDTO()
	return &dto, nil
}

func (s *service) Resolve(ctx context.Context, publicID uuid.UUID) error {
	t, err := s.repo.GetByPublicID(ctx, publicID)
	if err != nil {
		return err
	}

	if t.Status == StatusResolved {
		return nil // Idempoten: sudah resolved
	}

	if err := s.repo.Resolve(ctx, t.ID); err != nil {
		return err
	}

	t.Status = StatusResolved
	now := time.Now()
	t.ResolvedAt = &now

	if s.notifier != nil {
		_ = s.notifier.NotifyTicketResolved(ctx, t)
	}

	return nil
}

func (s *service) Rate(ctx context.Context, publicID uuid.UUID, req RateTicketRequest) error {
	t, err := s.repo.GetByPublicID(ctx, publicID)
	if err != nil {
		return err
	}

	if t.Status != StatusResolved {
		return ErrInvalidStatus
	}

	if t.CSATRating != nil {
		return ErrAlreadyRated
	}

	if err := s.repo.SaveRating(ctx, t.ID, req.Rating, req.Review); err != nil {
		return err
	}

	if s.notifier != nil {
		_ = s.notifier.NotifyTicketRating(ctx, t, req.Rating, req.Review)
	}

	return nil
}
