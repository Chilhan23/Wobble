package storage

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/gin-gonic/gin"
)

var ErrAttachmentNotFound = errors.New("attachment record not found")

type AttachmentInfo struct {
	Path string
	MIME string
}

type AttachmentProvider interface {
	GetAttachmentInfo(ctx context.Context, messageID int64) (*AttachmentInfo, error)
}

type Handler struct {
	storageService Service
	provider       AttachmentProvider
}

func NewHandler(storageService Service, provider AttachmentProvider) *Handler {
	return &Handler{
		storageService: storageService,
		provider:       provider,
	}
}

// RegisterRoutes mendaftarkan endpoint pengunduhan file bertanda tangan
func (h *Handler) RegisterRoutes(r *gin.Engine) {
	r.GET("/files/:message_id", h.ServeFile)
}

// ServeFile melayani file lampiran setelah memvalidasi tanda tangan HMAC dan kedaluwarsa
func (h *Handler) ServeFile(c *gin.Context) {
	rawMessageID := c.Param("message_id")
	messageID, err := strconv.ParseInt(rawMessageID, 10, 64)
	if err != nil || messageID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "invalid message id",
		})
		return
	}

	exp := c.Query("exp")
	sig := c.Query("sig")

	// 1. Verifikasi HMAC signature dan expired time
	if !h.storageService.VerifySignature(messageID, exp, sig) {
		c.JSON(http.StatusForbidden, gin.H{
			"status":  false,
			"message": "forbidden: invalid or expired signature",
		})
		return
	}

	// 2. Ambil metadata file dari provider
	info, err := h.provider.GetAttachmentInfo(c.Request.Context(), messageID)
	if err != nil {
		if errors.Is(err, ErrAttachmentNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"status":  false,
				"message": "attachment record not found",
			})
			return
		}
		slog.Error("failed to retrieve attachment metadata", "err", err, "message_id", messageID)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "failed to retrieve attachment metadata",
		})
		return
	}

	if info.Path == "" {
		c.JSON(http.StatusNotFound, gin.H{
			"status":  false,
			"message": "no attachment for this message",
		})
		return
	}

	absPath := h.storageService.GetAbsolutePath(info.Path)
	if absPath == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "invalid attachment path",
		})
		return
	}

	if _, statErr := os.Stat(absPath); os.IsNotExist(statErr) {
		c.JSON(http.StatusNotFound, gin.H{
			"status":  false,
			"message": "attachment file not found on disk",
		})
		return
	}

	// 3. Set security headers
	c.Header("X-Content-Type-Options", "nosniff")
	if info.MIME != "" {
		c.Header("Content-Type", info.MIME)
	}

	filename := filepath.Base(absPath)
	c.Header("Content-Disposition", "inline; filename=\""+filename+"\"")

	// 4. Stream file
	c.File(absPath)
}
