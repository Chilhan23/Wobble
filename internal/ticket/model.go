package ticket

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusOpen      Status = "open"
	StatusEscalated Status = "escalated"
	StatusResolved  Status = "resolved"
)

type Ticket struct {
	ID                 int64           `gorm:"primaryKey;autoIncrement" json:"id"`
	PublicID           uuid.UUID       `gorm:"column:public_id;type:uuid;default:gen_random_uuid();unique;not null" json:"public_id"`
	TenantID           int64           `gorm:"column:tenant_id;not null" json:"tenant_id"`
	TicketCode         string          `gorm:"column:ticket_code;size:50;not null;unique" json:"ticket_code"`
	UserID             string          `gorm:"column:user_id;size:100;not null" json:"user_id"`
	UserName           string          `gorm:"column:user_name;size:150;not null" json:"user_name"`
	ModuleName         string          `gorm:"column:module_name;size:100;not null;default:'Umum'" json:"module_name"`
	DiagnosticInfo     json.RawMessage `gorm:"column:diagnostic_info;type:jsonb" json:"diagnostic_info,omitempty"`
	Status             Status          `gorm:"column:status;size:20;not null;default:'open'" json:"status"`
	TelegramThreadID   *int64          `gorm:"column:telegram_thread_id" json:"telegram_thread_id,omitempty"`
	TGCardMessageID    *int64          `gorm:"column:tg_card_message_id" json:"tg_card_message_id,omitempty"`
	AssignedProgrammer *string         `gorm:"column:assigned_programmer;size:150" json:"assigned_programmer,omitempty"`
	AssignedTGUserID   *int64          `gorm:"column:assigned_tg_user_id" json:"assigned_tg_user_id,omitempty"`
	CSATRating         *int16          `gorm:"column:csat_rating" json:"csat_rating,omitempty"`
	CSATReview         *string         `gorm:"column:csat_review" json:"csat_review,omitempty"`
	ClaimedAt          *time.Time      `gorm:"column:claimed_at" json:"claimed_at,omitempty"`
	ResolvedAt         *time.Time      `gorm:"column:resolved_at" json:"resolved_at,omitempty"`
	LastUserMessageAt  *time.Time      `gorm:"column:last_user_message_at" json:"last_user_message_at,omitempty"`
	CreatedAt          time.Time       `gorm:"column:created_at;not null;default:now()" json:"created_at"`
	UpdatedAt          time.Time       `gorm:"column:updated_at;not null;default:now()" json:"updated_at"`
}

func (Ticket) TableName() string {
	return "tickets"
}
