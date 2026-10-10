package telegram

import (
	"context"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"

	"wobble/internal/chat"
	"wobble/internal/tenant"
	"wobble/internal/ticket"
)

type Notifier struct {
	client     Client
	chatID     int64
	tenantRepo tenant.Repository
	uploadDir  string
}

func NewNotifier(client Client, chatID int64, tenantRepo tenant.Repository, uploadDir ...string) *Notifier {
	dir := "./uploads"
	if len(uploadDir) > 0 && uploadDir[0] != "" {
		dir = uploadDir[0]
	}
	return &Notifier{
		client:     client,
		chatID:     chatID,
		tenantRepo: tenantRepo,
		uploadDir:  dir,
	}
}

func (n *Notifier) getTenantDisplayName(ctx context.Context, tenantID int64) string {
	if n.tenantRepo != nil {
		if ten, err := n.tenantRepo.GetByID(ctx, tenantID); err == nil && ten != nil {
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

func (n *Notifier) NotifyNewTicket(ctx context.Context, t *ticket.Ticket, summary string) (int64, error) {
	appName := n.getTenantDisplayName(ctx, t.TenantID)

	text := fmt.Sprintf(
		"🎫 <b>TIKET BANTUAN BARU</b>\n"+
			"<b>System:</b> %s\n"+
			"<b>Kode:</b> <code>%s</code>\n"+
			"<b>User:</b> %s\n"+
			"<b>Modul:</b> %s\n\n"+
			"<b>Kendala Awal:</b>\n%s",
		html.EscapeString(appName),
		html.EscapeString(t.TicketCode),
		html.EscapeString(t.UserName),
		html.EscapeString(t.ModuleName),
		html.EscapeString(summary),
	)

	keyboard := &InlineKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{
			{
				{
					Text:         "🧑‍💻 Klaim Tiket Ini",
					CallbackData: "claim:" + t.PublicID.String(),
				},
			},
		},
	}

	return n.client.SendMessage(ctx, n.chatID, nil, text, keyboard)
}

func (n *Notifier) SendToTopic(ctx context.Context, threadID int64, m *chat.Message) (int64, error) {
	sender := "User"
	if m.SenderName != nil && *m.SenderName != "" {
		sender = *m.SenderName
	}

	if m.AttachmentPath != nil && *m.AttachmentPath != "" {
		absPath := filepath.Join(n.uploadDir, *m.AttachmentPath)
		if f, err := os.Open(absPath); err == nil {
			defer f.Close()
			fileName := filepath.Base(absPath)
			caption := fmt.Sprintf("👤 <b>%s</b>", html.EscapeString(sender))
			if m.Message != "" {
				caption = fmt.Sprintf("👤 <b>%s:</b>\n%s", html.EscapeString(sender), html.EscapeString(m.Message))
			}

			if m.AttachmentType != nil && *m.AttachmentType == chat.AttachmentTypeImage {
				msgID, sendErr := n.client.SendPhoto(ctx, n.chatID, &threadID, f, fileName, caption)
				if sendErr == nil {
					return msgID, nil
				}
			} else if m.AttachmentType != nil && *m.AttachmentType == chat.AttachmentTypeVideo {
				msgID, sendErr := n.client.SendVideo(ctx, n.chatID, &threadID, f, fileName, caption)
				if sendErr == nil {
					return msgID, nil
				}
			} else {
				msgID, sendErr := n.client.SendDocument(ctx, n.chatID, &threadID, f, fileName, caption)
				if sendErr == nil {
					return msgID, nil
				}
			}
		}
	}

	var text string
	if m.AttachmentType != nil && *m.AttachmentType != "" {
		attLabel := string(*m.AttachmentType)
		if m.Message != "" {
			text = fmt.Sprintf("👤 <b>%s:</b>\n%s\n📎 <i>[Lampiran %s]</i>", html.EscapeString(sender), html.EscapeString(m.Message), html.EscapeString(attLabel))
		} else {
			text = fmt.Sprintf("👤 <b>%s:</b> <i>[Lampiran %s]</i>", html.EscapeString(sender), html.EscapeString(attLabel))
		}
	} else {
		text = fmt.Sprintf("👤 <b>%s:</b>\n%s", html.EscapeString(sender), html.EscapeString(m.Message))
	}

	return n.client.SendMessage(ctx, n.chatID, &threadID, text, nil)
}

func (n *Notifier) NotifyTicketResolved(ctx context.Context, t *ticket.Ticket) error {
	appName := n.getTenantDisplayName(ctx, t.TenantID)

	// Hapus topik Forum di Telegram agar tidak menumpuk di Supergroup
	if t.TelegramThreadID != nil && *t.TelegramThreadID > 0 {
		_ = n.client.DeleteForumTopic(ctx, n.chatID, *t.TelegramThreadID)
	}

	// Edit kartu di General untuk menghapus tombol Klaim dan menandai tiket selesai
	if t.TGCardMessageID != nil && *t.TGCardMessageID > 0 {
		statusLabel := "🔒 DITUTUP / SELESAI"
		if t.AssignedProgrammer != nil && *t.AssignedProgrammer != "" {
			statusLabel = fmt.Sprintf("✅ SELESAI (Ditangani: %s)", *t.AssignedProgrammer)
		}

		cardText := fmt.Sprintf(
			"🎫 <b>TIKET BANTUAN</b>\n"+
				"<b>System:</b> %s\n"+
				"<b>Kode:</b> <code>%s</code>\n"+
				"<b>User:</b> %s\n"+
				"<b>Modul:</b> %s\n\n"+
				"%s",
			html.EscapeString(appName),
			html.EscapeString(t.TicketCode),
			html.EscapeString(t.UserName),
			html.EscapeString(t.ModuleName),
			html.EscapeString(statusLabel),
		)
		_ = n.client.EditMessageText(ctx, n.chatID, *t.TGCardMessageID, cardText, nil)
	}

	return nil
}

func (n *Notifier) NotifyTicketRating(ctx context.Context, t *ticket.Ticket, rating int16, review string) error {
	stars := strings.Repeat("⭐", int(rating))

	// Update kartu di General untuk menampilkan rating dan ulasan pengguna
	if t.TGCardMessageID != nil && *t.TGCardMessageID > 0 {
		appName := n.getTenantDisplayName(ctx, t.TenantID)
		progName := "Tim IT Support"
		if t.AssignedProgrammer != nil && *t.AssignedProgrammer != "" {
			progName = *t.AssignedProgrammer
		}

		reviewText := ""
		if review != "" {
			reviewText = fmt.Sprintf("\n<b>Ulasan:</b> <i>%s</i>", html.EscapeString(review))
		}

		cardText := fmt.Sprintf(
			"🎫 <b>TIKET BANTUAN</b>\n"+
				"<b>System:</b> %s\n"+
				"<b>Kode:</b> <code>%s</code>\n"+
				"<b>User:</b> %s\n"+
				"<b>Modul:</b> %s\n\n"+
				"✅ <b>SELESAI (Ditangani: %s)</b>\n"+
				"🌟 <b>Rating:</b> %d/5 %s%s",
			html.EscapeString(appName),
			html.EscapeString(t.TicketCode),
			html.EscapeString(t.UserName),
			html.EscapeString(t.ModuleName),
			html.EscapeString(progName),
			rating, stars, reviewText,
		)
		_ = n.client.EditMessageText(ctx, n.chatID, *t.TGCardMessageID, cardText, nil)
	}

	return nil
}
