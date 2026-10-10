package auth

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"wobble/internal/tenant"
)

const (
	CtxKeyTenant         = "auth_tenant"
	CtxKeyTicketClaims   = "auth_ticket_claims"
	CtxKeyTicketPublicID = "auth_ticket_public_id"
	CtxKeyTenantID       = "auth_tenant_id"
	CtxKeyUserID         = "auth_user_id"
)

// RequireAPIKey memvalidasi header X-API-Key terhadap repository tenant.
// Jika API key tidak ada, tidak valid, atau tenant nonaktif, tolak dengan 401.
func RequireAPIKey(tenantRepo tenant.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey := strings.TrimSpace(c.GetHeader("X-API-Key"))
		if apiKey == "" {
			apiKey = strings.TrimSpace(c.GetHeader("X-Api-Key"))
		}

		if apiKey == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"status":  false,
				"message": "unauthorized: missing X-API-Key header",
			})
			return
		}

		hash := HashAPIKey(apiKey)
		t, err := tenantRepo.GetByAPIKeyHash(c.Request.Context(), hash)
		if err != nil || t == nil {
			t, err = tenantRepo.GetByKeyIdentifier(c.Request.Context(), apiKey)
		}
		if err != nil || t == nil || !t.IsActive {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"status":  false,
				"message": "unauthorized: invalid or inactive API key",
			})
			return
		}

		c.Set(CtxKeyTenant, t)
		c.Set(CtxKeyTenantID, t.ID)
		c.Next()
	}
}

// RequireTicketToken memvalidasi Bearer JWT token dari header Authorization.
func RequireTicketToken(tokenService TokenService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var tokenString string

		authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
		if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
			tokenString = strings.TrimSpace(authHeader[7:])
		}

		if tokenString == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"status":  false,
				"message": "unauthorized: missing bearer token",
			})
			return
		}

		claims, err := tokenService.Verify(tokenString)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"status":  false,
				"message": "unauthorized: invalid or expired token",
			})
			return
		}

		publicID, err := claims.PublicID()
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"status":  false,
				"message": "unauthorized: invalid ticket public ID in token",
			})
			return
		}

		c.Set(CtxKeyTicketClaims, claims)
		c.Set(CtxKeyTicketPublicID, publicID)
		c.Set(CtxKeyTenantID, claims.TenantID)
		c.Set(CtxKeyUserID, claims.UserID)
		c.Next()
	}
}

// VerifyTelegramSecret memverifikasi header X-Telegram-Bot-Api-Secret-Token
// menggunakan perbandingan waktu konstan (ConstantTimeCompare).
func VerifyTelegramSecret(expectedSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		incomingSecret := c.GetHeader("X-Telegram-Bot-Api-Secret-Token")
		if incomingSecret == "" || expectedSecret == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"status":  false,
				"message": "unauthorized: missing telegram webhook secret",
			})
			return
		}

		if subtle.ConstantTimeCompare([]byte(incomingSecret), []byte(expectedSecret)) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"status":  false,
				"message": "unauthorized: invalid telegram webhook secret",
			})
			return
		}

		c.Next()
	}
}

// CORSMiddleware mengatur header CORS standar tanpa cookie/kredensial,
// karena autentikasi menggunakan API key dan Bearer token.
func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")

		reqHeaders := c.GetHeader("Access-Control-Request-Headers")
		if reqHeaders != "" {
			c.Header("Access-Control-Allow-Headers", reqHeaders)
		} else {
			c.Header("Access-Control-Allow-Headers", "*")
		}

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// Helper context getter
func GetTenant(c *gin.Context) (*tenant.Tenant, bool) {
	val, exists := c.Get(CtxKeyTenant)
	if !exists {
		return nil, false
	}
	t, ok := val.(*tenant.Tenant)
	return t, ok
}

func GetTicketClaims(c *gin.Context) (*TicketClaims, bool) {
	val, exists := c.Get(CtxKeyTicketClaims)
	if !exists {
		return nil, false
	}
	claims, ok := val.(*TicketClaims)
	return claims, ok
}

func GetTicketPublicID(c *gin.Context) (uuid.UUID, bool) {
	val, exists := c.Get(CtxKeyTicketPublicID)
	if !exists {
		return uuid.Nil, false
	}
	id, ok := val.(uuid.UUID)
	return id, ok
}

func GetTenantID(c *gin.Context) (int64, bool) {
	val, exists := c.Get(CtxKeyTenantID)
	if !exists {
		return 0, false
	}
	id, ok := val.(int64)
	return id, ok
}

func GetUserID(c *gin.Context) (string, bool) {
	val, exists := c.Get(CtxKeyUserID)
	if !exists {
		return "", false
	}
	id, ok := val.(string)
	return id, ok
}
