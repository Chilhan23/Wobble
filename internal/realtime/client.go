package realtime

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"wobble/internal/auth"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 1024
	sendBufferSize = 64
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Auth dilakukan lewat token JWT, bukan cookie
	},
}

type Client struct {
	hub     *Hub
	conn    *websocket.Conn
	roomKey string
	send    chan []byte
}

func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		_ = c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				slog.Debug("websocket connection closed", "room", c.roomKey, "err", err)
			}
			break
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// Channel ditutup oleh hub
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			// 1 event = 1 frame WebSocket
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// ServeWS menangani upgrade handshake WebSocket di GET /ws?token=<jwt>
func ServeWS(hub *Hub, tokenService auth.TokenService) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString := strings.TrimSpace(c.Query("token"))
		if tokenString == "" {
			authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
			if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
				tokenString = strings.TrimSpace(authHeader[7:])
			}
		}

		if tokenString == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"status":  false,
				"message": "unauthorized: missing token query param or authorization header",
			})
			return
		}

		claims, err := tokenService.Verify(tokenString)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"status":  false,
				"message": "unauthorized: invalid or expired token",
			})
			return
		}

		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			slog.Error("websocket upgrade failed", "err", err)
			return
		}

		client := &Client{
			hub:     hub,
			conn:    conn,
			roomKey: claims.Subject, // PublicID UUID string dari tiket
			send:    make(chan []byte, sendBufferSize),
		}

		hub.register <- client

		go client.writePump()
		go client.readPump()
	}
}
