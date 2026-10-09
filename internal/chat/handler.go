package chat

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"wobble/internal/auth"
	"wobble/internal/storage"
	"wobble/internal/ticket"
)

type StorageSaver interface {
	Save(publicID uuid.UUID, r io.Reader, maxImageBytes, maxVideoBytes int64) (*storage.SavedFile, error)
	Delete(relativePath string) error
}

type Handler struct {
	service       Service
	ticketRepo    ticket.Repository
	storage       StorageSaver
	maxImageBytes int64
	maxVideoBytes int64
}

func NewHandler(service Service, ticketRepo ticket.Repository, storage StorageSaver, maxImageBytes int64, maxVideoBytes int64) *Handler {
	if maxImageBytes <= 0 {
		maxImageBytes = 5 * 1024 * 1024
	}
	if maxVideoBytes <= 0 {
		maxVideoBytes = 15 * 1024 * 1024
	}
	return &Handler{
		service:       service,
		ticketRepo:    ticketRepo,
		storage:       storage,
		maxImageBytes: maxImageBytes,
		maxVideoBytes: maxVideoBytes,
	}
}

// RegisterRoutes mendaftarkan endpoint pesan chat dan lampiran
func (h *Handler) RegisterRoutes(apiV1 *gin.RouterGroup, requireTicketToken gin.HandlerFunc, rateLimitChat gin.HandlerFunc, rateLimitUpload gin.HandlerFunc) {
	ticketGroup := apiV1.Group("/ticket", requireTicketToken)
	{
		if rateLimitChat != nil {
			ticketGroup.POST("/messages", rateLimitChat, h.SendMessage)
		} else {
			ticketGroup.POST("/messages", h.SendMessage)
		}
		ticketGroup.GET("/messages", h.ListMessages)
		if rateLimitUpload != nil {
			ticketGroup.POST("/attachments", rateLimitUpload, h.UploadAttachment)
		} else {
			ticketGroup.POST("/attachments", h.UploadAttachment)
		}
	}

	chatGroup := apiV1.Group("/chat", requireTicketToken)
	{
		if rateLimitChat != nil {
			chatGroup.POST("/send", rateLimitChat, h.SendMessage)
		} else {
			chatGroup.POST("/send", h.SendMessage)
		}
		if rateLimitUpload != nil {
			chatGroup.POST("/upload", rateLimitUpload, h.UploadAttachment)
		} else {
			chatGroup.POST("/upload", h.UploadAttachment)
		}
	}
}

// SendMessage menangani POST /api/v1/ticket/messages (HTTP 202 Accepted)
func (h *Handler) SendMessage(c *gin.Context) {
	publicID, ok := auth.GetTicketPublicID(c)
	if !ok || publicID == uuid.Nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status":  false,
			"message": "unauthorized: ticket public id missing from token",
		})
		return
	}

	var req SendMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "invalid message payload",
		})
		return
	}

	tck, err := h.ticketRepo.GetByPublicID(c.Request.Context(), publicID)
	if err != nil {
		if errors.Is(err, ticket.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"status":  false,
				"message": "ticket not found",
			})
			return
		}
		slog.Error("failed to get ticket", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "failed to retrieve ticket",
		})
		return
	}

	msgDTO, err := h.service.SendUserMessage(c.Request.Context(), tck, req)
	if err != nil {
		if errors.Is(err, ErrTicketResolved) {
			c.JSON(http.StatusConflict, gin.H{
				"status":  false,
				"message": "cannot send message to a resolved ticket",
			})
			return
		}
		if errors.Is(err, ErrEmptyMessage) {
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  false,
				"message": "message cannot be empty",
			})
			return
		}
		slog.Error("failed to send message", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "failed to send message",
		})
		return
	}

	c.JSON(http.StatusAccepted, SendMessageResponse{
		Status:  true,
		Message: *msgDTO,
	})
}

// ListMessages menangani GET /api/v1/ticket/messages?after_id=&limit=
func (h *Handler) ListMessages(c *gin.Context) {
	publicID, ok := auth.GetTicketPublicID(c)
	if !ok || publicID == uuid.Nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status":  false,
			"message": "unauthorized: ticket public id missing from token",
		})
		return
	}

	tck, err := h.ticketRepo.GetByPublicID(c.Request.Context(), publicID)
	if err != nil {
		if errors.Is(err, ticket.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"status":  false,
				"message": "ticket not found",
			})
			return
		}
		slog.Error("failed to get ticket", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "failed to retrieve ticket",
		})
		return
	}

	afterID, _ := strconv.ParseInt(c.Query("after_id"), 10, 64)
	limit, _ := strconv.Atoi(c.Query("limit"))
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	messages, err := h.service.ListMessages(c.Request.Context(), tck.ID, afterID, limit)
	if err != nil {
		slog.Error("failed to retrieve messages", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "failed to retrieve messages",
		})
		return
	}

	c.JSON(http.StatusOK, ListMessagesResponse{
		Status:   true,
		Messages: messages,
	})
}

// UploadAttachment menangani POST /api/v1/ticket/attachments dan POST /api/v1/chat/upload
func (h *Handler) UploadAttachment(c *gin.Context) {
	// 1. Validasi konteks autentikasi
	publicID, ok := auth.GetTicketPublicID(c)
	if !ok || publicID == uuid.Nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status":  false,
			"message": "unauthorized: ticket public id missing from token",
		})
		return
	}

	if h.storage == nil {
		c.JSON(http.StatusNotImplemented, gin.H{
			"status":  false,
			"message": "file storage is not configured",
		})
		return
	}

	// 2. Validasi status tiket sebelum memproses upload
	tck, err := h.ticketRepo.GetByPublicID(c.Request.Context(), publicID)
	if err != nil {
		if errors.Is(err, ticket.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"status":  false,
				"message": "ticket not found",
			})
			return
		}
		slog.Error("failed to get ticket", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "failed to retrieve ticket",
		})
		return
	}
	if tck.Status == ticket.StatusResolved {
		c.JSON(http.StatusConflict, gin.H{
			"status":  false,
			"message": "cannot send attachment to a resolved ticket",
		})
		return
	}

	// 3. Batasi ukuran body sebelum parsing multipart form (video limit + 1MB overhead)
	maxBodyLimit := h.maxVideoBytes + (1 << 20)
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBodyLimit)

	fileHeader, err := c.FormFile("file")
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{
				"status":  false,
				"message": "file exceeds maximum allowed size",
			})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "file field is required in multipart form",
		})
		return
	}

	fileReader, err := fileHeader.Open()
	if err != nil {
		slog.Error("failed to open uploaded file", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "failed to open uploaded file",
		})
		return
	}
	defer fileReader.Close()

	saved, err := h.storage.Save(tck.PublicID, fileReader, h.maxImageBytes, h.maxVideoBytes)
	if err != nil {
		if errors.Is(err, storage.ErrFileTooLarge) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{
				"status":  false,
				"message": "file exceeds maximum allowed size",
			})
			return
		}
		if errors.Is(err, storage.ErrInvalidFileType) {
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  false,
				"message": "unsupported or dangerous file type",
			})
			return
		}
		slog.Error("failed to save uploaded file", "err", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "failed to process uploaded file",
		})
		return
	}

	caption := c.PostForm("message")
	if caption == "" {
		caption = c.PostForm("caption")
	}

	var clientMsgID *uuid.UUID
	if rawClientMsgID := c.PostForm("client_msg_id"); rawClientMsgID != "" {
		if parsed, parseErr := uuid.Parse(rawClientMsgID); parseErr == nil {
			clientMsgID = &parsed
		}
	}

	attInput := &AttachmentInput{
		RelativePath: saved.RelativePath,
		MIMEType:     saved.MIMEType,
		Size:         saved.Size,
		Type:         AttachmentType(saved.Type),
	}

	msgDTO, err := h.service.SendUserAttachment(c.Request.Context(), tck, attInput, caption, clientMsgID)
	if err != nil {
		// Bersihkan file pada disk jika penyimpanan ke database gagal
		_ = h.storage.Delete(saved.RelativePath)
		if errors.Is(err, ErrTicketResolved) {
			c.JSON(http.StatusConflict, gin.H{
				"status":  false,
				"message": "cannot send attachment to a resolved ticket",
			})
			return
		}
		slog.Error("failed to record attachment", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "failed to record attachment",
		})
		return
	}

	c.JSON(http.StatusAccepted, SendMessageResponse{
		Status:  true,
		Message: *msgDTO,
	})
}
