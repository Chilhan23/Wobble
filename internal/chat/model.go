package chat

import (
	"time"

	"github.com/google/uuid"
)

type SenderType string

const (
	SenderTypeUser       SenderType = "user"
	SenderTypeAI         SenderType = "ai"
	SenderTypeProgrammer SenderType = "programmer"
	SenderTypeSystem     SenderType = "system"
)

type DeliveryStatus string

const (
	DeliveryStatusPending DeliveryStatus = "pending"
	DeliveryStatusSent    DeliveryStatus = "sent"
	DeliveryStatusFailed  DeliveryStatus = "failed"
)

type AttachmentType string

const (
	AttachmentTypeImage    AttachmentType = "image"
	AttachmentTypeVideo    AttachmentType = "video"
	AttachmentTypeDocument AttachmentType = "document"
)

type Message struct {
	ID                int64           `gorm:"primaryKey;autoIncrement" json:"id"`
	TicketID          int64           `gorm:"column:ticket_id;not null" json:"ticket_id"`
	SenderType        SenderType      `gorm:"column:sender_type;size:20;not null" json:"sender_type"`
	SenderName        *string         `gorm:"column:sender_name;size:150" json:"sender_name,omitempty"`
	Message           string          `gorm:"column:message;not null;default:''" json:"message"`
	AttachmentType    *AttachmentType `gorm:"column:attachment_type;size:20" json:"attachment_type,omitempty"`
	AttachmentPath    *string         `gorm:"column:attachment_path" json:"attachment_path,omitempty"`
	AttachmentMIME    *string         `gorm:"column:attachment_mime;size:100" json:"attachment_mime,omitempty"`
	AttachmentSize    *int64          `gorm:"column:attachment_size" json:"attachment_size,omitempty"`
	TelegramMessageID *int64          `gorm:"column:telegram_message_id" json:"telegram_message_id,omitempty"`
	DeliveryStatus    DeliveryStatus  `gorm:"column:delivery_status;size:10;not null;default:'sent'" json:"delivery_status"`
	ClientMsgID       *uuid.UUID      `gorm:"column:client_msg_id;type:uuid" json:"client_msg_id,omitempty"`
	CreatedAt         time.Time       `gorm:"column:created_at;not null;default:now()" json:"created_at"`
}

func (Message) TableName() string {
	return "messages"
}
