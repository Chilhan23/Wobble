package ticket

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"wobble/internal/auth"
)

type Handler struct {
	service Service
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

// RegisterRoutes mendaftarkan endpoint rute tiket
func (h *Handler) RegisterRoutes(apiV1 *gin.RouterGroup, requireAPIKey gin.HandlerFunc, requireTicketToken gin.HandlerFunc, rateLimitInit ...gin.HandlerFunc) {
	// Server-to-Server (PHP SIMRS)
	initHandlers := []gin.HandlerFunc{requireAPIKey}
	for _, rl := range rateLimitInit {
		if rl != nil {
			initHandlers = append(initHandlers, rl)
		}
	}
	initHandlers = append(initHandlers, h.InitTicket)
	apiV1.POST("/tickets/init", initHandlers...)

	// Browser / Client Widget (Bearer Token)
	ticketGroup := apiV1.Group("/ticket", requireTicketToken)
	{
		ticketGroup.GET("", h.GetTicket)
		ticketGroup.POST("/resolve", h.ResolveTicket)
		ticketGroup.POST("/rate", h.RateTicket)
	}
}

// InitTicket menangani POST /api/v1/tickets/init
func (h *Handler) InitTicket(c *gin.Context) {
	tenantID, ok := auth.GetTenantID(c)
	if !ok || tenantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status":  false,
			"message": "unauthorized: tenant context missing",
		})
		return
	}

	var req InitTicketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "invalid request payload",
		})
		return
	}

	resp, err := h.service.Init(c.Request.Context(), tenantID, req)
	if err != nil {
		slog.Error("failed to initialize ticket", "err", err, "tenant_id", tenantID)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "failed to initialize ticket",
		})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// GetTicket menangani GET /api/v1/ticket
func (h *Handler) GetTicket(c *gin.Context) {
	publicID, ok := auth.GetTicketPublicID(c)
	if !ok || publicID == uuid.Nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status":  false,
			"message": "unauthorized: ticket public id missing from token",
		})
		return
	}

	dto, err := h.service.GetByPublicID(c.Request.Context(), publicID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"status":  false,
				"message": "ticket not found",
			})
			return
		}
		slog.Error("failed to get ticket", "err", err, "ticket_id", publicID)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "failed to get ticket",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": true,
		"ticket": dto,
	})
}

// ResolveTicket menangani POST /api/v1/ticket/resolve
func (h *Handler) ResolveTicket(c *gin.Context) {
	publicID, ok := auth.GetTicketPublicID(c)
	if !ok || publicID == uuid.Nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status":  false,
			"message": "unauthorized: ticket public id missing from token",
		})
		return
	}

	if err := h.service.Resolve(c.Request.Context(), publicID); err != nil {
		if errors.Is(err, ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"status":  false,
				"message": "ticket not found",
			})
			return
		}
		slog.Error("failed to resolve ticket", "err", err, "ticket_id", publicID)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "failed to resolve ticket",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  true,
		"message": "ticket resolved successfully",
	})
}

// RateTicket menangani POST /api/v1/ticket/rate
func (h *Handler) RateTicket(c *gin.Context) {
	publicID, ok := auth.GetTicketPublicID(c)
	if !ok || publicID == uuid.Nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status":  false,
			"message": "unauthorized: ticket public id missing from token",
		})
		return
	}

	var req RateTicketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "invalid rating payload",
		})
		return
	}

	if err := h.service.Rate(c.Request.Context(), publicID, req); err != nil {
		if errors.Is(err, ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"status":  false,
				"message": "ticket not found",
			})
			return
		}
		if errors.Is(err, ErrAlreadyRated) {
			c.JSON(http.StatusConflict, gin.H{
				"status":  false,
				"message": "ticket has already been rated",
			})
			return
		}
		if errors.Is(err, ErrInvalidStatus) {
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  false,
				"message": "ticket must be resolved before submitting CSAT rating",
			})
			return
		}
		slog.Error("failed to submit rating", "err", err, "ticket_id", publicID)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "failed to submit rating",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  true,
		"message": "csat rating submitted successfully",
	})
}
