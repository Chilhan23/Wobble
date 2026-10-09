package auth

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/time/rate"
)

type clientLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type RateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*clientLimiter
	rate     rate.Limit
	burst    int
}

func NewRateLimiter(r rate.Limit, burst int) *RateLimiter {
	rl := &RateLimiter{
		limiters: make(map[string]*clientLimiter),
		rate:     r,
		burst:    burst,
	}

	// Rutin pembersihan pemegang token yang tidak aktif (> 10 menit)
	go rl.cleanupRoutine(10 * time.Minute)

	return rl
}

func (rl *RateLimiter) getLimiter(key string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	v, exists := rl.limiters[key]
	if !exists {
		limiter := rate.NewLimiter(rl.rate, rl.burst)
		rl.limiters[key] = &clientLimiter{
			limiter:  limiter,
			lastSeen: time.Now(),
		}
		return limiter
	}

	v.lastSeen = time.Now()
	return v.limiter
}

func (rl *RateLimiter) cleanupRoutine(staleThreshold time.Duration) {
	ticker := time.NewTicker(staleThreshold)
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for k, v := range rl.limiters {
			if now.Sub(v.lastSeen) > staleThreshold {
				delete(rl.limiters, k)
			}
		}
		rl.mu.Unlock()
	}
}

// RateLimitByIP membatasi frekuensi request berdasarkan Client IP
func RateLimitByIP(r rate.Limit, burst int) gin.HandlerFunc {
	rl := NewRateLimiter(r, burst)
	return func(c *gin.Context) {
		ip := c.ClientIP()
		if !rl.getLimiter(ip).Allow() {
			c.Header("Retry-After", "60")
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"status":  false,
				"message": "too many requests, please slow down",
			})
			return
		}
		c.Next()
	}
}

// RateLimitByTenant membatasi frekuensi request berdasarkan Tenant (KeyIdentifier) dari konteks API key
func RateLimitByTenant(r rate.Limit, burst int) gin.HandlerFunc {
	rl := NewRateLimiter(r, burst)
	return func(c *gin.Context) {
		key := c.ClientIP()
		if ten, ok := GetTenant(c); ok && ten != nil && ten.KeyIdentifier != "" {
			key = "tenant:" + ten.KeyIdentifier
		}

		if !rl.getLimiter(key).Allow() {
			c.Header("Retry-After", "60")
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"status":  false,
				"message": "too many requests, please slow down",
			})
			return
		}
		c.Next()
	}
}

// RateLimitByTicket membatasi frekuensi request berdasarkan Ticket Public ID
func RateLimitByTicket(r rate.Limit, burst int) gin.HandlerFunc {
	rl := NewRateLimiter(r, burst)
	return func(c *gin.Context) {
		publicID, ok := GetTicketPublicID(c)
		key := c.ClientIP()
		if ok && publicID != uuid.Nil {
			key = publicID.String()
		}

		if !rl.getLimiter(key).Allow() {
			c.Header("Retry-After", "60")
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"status":  false,
				"message": "ticket request limit exceeded, please wait a moment",
			})
			return
		}
		c.Next()
	}
}
