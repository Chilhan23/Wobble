package chat

import (
	"time"

	"github.com/google/uuid"
)

type SendMessageRequest struct {
	ClientMsgID *uuid.UUID `json:"client_msg_id"`
	Message     string     `json:"message" binding:"required,max=4000"`
}

type AttachmentInput struct {
	RelativePath string
	MIMEType     string
	Size         int64
	Type         AttachmentType
}

type AttachmentDTO struct {
	Type string `json:"type"`
	URL  string `json:"url"`
	MIME string `json:"mime"`
	Size int64  `json:"size"`
}

type MessageDTO struct {
	ID             int64          `json:"id"`
	SenderType     string         `json:"sender_type"`
	SenderName     *string        `json:"sender_name"`
	Message        string         `json:"message"`
	Attachment     *AttachmentDTO `json:"attachment"`
	DeliveryStatus string         `json:"delivery_status"`
	ClientMsgID    *uuid.UUID     `json:"client_msg_id,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
}

type SendMessageResponse struct {
	Status  bool       `json:"status"`
	Message MessageDTO `json:"message"`
}

type ListMessagesResponse struct {
	Status   bool         `json:"status"`
	Messages []MessageDTO `json:"messages"`
}

// ToDTO mengonversi model internal Message ke MessageDTO
func (m *Message) ToDTO(signedURL string) MessageDTO {
	dto := MessageDTO{
		ID:             m.ID,
		SenderType:     string(m.SenderType),
		SenderName:     m.SenderName,
		Message:        m.Message,
		DeliveryStatus: string(m.DeliveryStatus),
		ClientMsgID:    m.ClientMsgID,
		CreatedAt:      m.CreatedAt,
	}

	if m.AttachmentPath != nil && *m.AttachmentPath != "" {
		var attType, attMime string
		var attSize int64
		if m.AttachmentType != nil {
			attType = string(*m.AttachmentType)
		}
		if m.AttachmentMIME != nil {
			attMime = *m.AttachmentMIME
		}
		if m.AttachmentSize != nil {
			attSize = *m.AttachmentSize
		}

		dto.Attachment = &AttachmentDTO{
			Type: attType,
			URL:  signedURL,
			MIME: attMime,
			Size: attSize,
		}
	}

	return dto
}
