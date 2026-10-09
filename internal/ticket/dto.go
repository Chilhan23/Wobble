package ticket

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type InitTicketRequest struct {
	UserID         string          `json:"user_id" binding:"required"`
	UserName       string          `json:"user_name" binding:"required"`
	ModuleName     string          `json:"module_name"`
	DiagnosticInfo json.RawMessage `json:"diagnostic_info"`
}

type TicketDTO struct {
	PublicID           uuid.UUID       `json:"public_id"`
	TicketCode         string          `json:"ticket_code"`
	Status             string          `json:"status"`
	ModuleName         string          `json:"module_name"`
	AssignedProgrammer *string         `json:"assigned_programmer"`
	UserID             string          `json:"user_id,omitempty"`
	UserName           string          `json:"user_name,omitempty"`
	CSATRating         *int16          `json:"csat_rating,omitempty"`
	CSATReview         *string         `json:"csat_review,omitempty"`
	ClaimedAt          *time.Time      `json:"claimed_at,omitempty"`
	ResolvedAt         *time.Time      `json:"resolved_at,omitempty"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at,omitempty"`
	DiagnosticInfo     json.RawMessage `json:"diagnostic_info,omitempty"`
}

type InitTicketResponse struct {
	Status    bool      `json:"status"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	Ticket    TicketDTO `json:"ticket"`
}

type RateTicketRequest struct {
	Rating int16  `json:"rating" binding:"required,min=1,max=5"`
	Review string `json:"review"`
}

// ToDTO mengonversi model internal Ticket menjadi TicketDTO untuk publik/API
func (t *Ticket) ToDTO() TicketDTO {
	return TicketDTO{
		PublicID:           t.PublicID,
		TicketCode:         t.TicketCode,
		Status:             string(t.Status),
		ModuleName:         t.ModuleName,
		AssignedProgrammer: t.AssignedProgrammer,
		UserID:             t.UserID,
		UserName:           t.UserName,
		CSATRating:         t.CSATRating,
		CSATReview:         t.CSATReview,
		ClaimedAt:          t.ClaimedAt,
		ResolvedAt:         t.ResolvedAt,
		CreatedAt:          t.CreatedAt,
		UpdatedAt:          t.UpdatedAt,
		DiagnosticInfo:     t.DiagnosticInfo,
	}
}
