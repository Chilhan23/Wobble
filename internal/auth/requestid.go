package auth

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const HeaderXRequestID = "X-Request-ID"

// RequestIDMiddleware menyematkan Request ID pada header respons dan context request
func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		reqID := c.GetHeader(HeaderXRequestID)
		if reqID == "" {
			reqID = uuid.New().String()
		}

		c.Set("request_id", reqID)
		c.Header(HeaderXRequestID, reqID)
		c.Next()
	}
}
