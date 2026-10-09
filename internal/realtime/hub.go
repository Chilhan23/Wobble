package realtime

import (
	"encoding/json"
	"log/slog"
	"sync"
)

type EventFrame struct {
	Event string `json:"event"`
	Data  any    `json:"data"`
}

type Publisher interface {
	Publish(roomKey string, event string, data any)
}

type Hub struct {
	rooms      map[string]map[*Client]struct{}
	broadcast  chan broadcastMessage
	register   chan *Client
	unregister chan *Client
	mu         sync.Mutex // menjaga start satu kali
	running    bool
}

type broadcastMessage struct {
	roomKey string
	payload []byte
}

func NewHub() *Hub {
	return &Hub{
		rooms:      make(map[string]map[*Client]struct{}),
		broadcast:  make(chan broadcastMessage, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

func (h *Hub) Run() {
	h.mu.Lock()
	if h.running {
		h.mu.Unlock()
		return
	}
	h.running = true
	h.mu.Unlock()

	for {
		select {
		case client := <-h.register:
			clients := h.rooms[client.roomKey]
			if clients == nil {
				clients = make(map[*Client]struct{})
				h.rooms[client.roomKey] = clients
			}
			clients[client] = struct{}{}

		case client := <-h.unregister:
			if clients, ok := h.rooms[client.roomKey]; ok {
				if _, exists := clients[client]; exists {
					delete(clients, client)
					close(client.send)
					if len(clients) == 0 {
						delete(h.rooms, client.roomKey)
					}
				}
			}

		case msg := <-h.broadcast:
			if clients, ok := h.rooms[msg.roomKey]; ok {
				for client := range clients {
					select {
					case client.send <- msg.payload:
					default:
						// Client lambat / buffer penuh -> putus agar tidak memblokir yang lain
						delete(clients, client)
						close(client.send)
					}
				}
				if len(clients) == 0 {
					delete(h.rooms, msg.roomKey)
				}
			}
		}
	}
}

func (h *Hub) Publish(roomKey string, event string, data any) {
	frame := EventFrame{
		Event: event,
		Data:  data,
	}
	payload, err := json.Marshal(frame)
	if err != nil {
		slog.Error("failed to marshal websocket event frame", "event", event, "room", roomKey, "err", err)
		return
	}

	h.broadcast <- broadcastMessage{
		roomKey: roomKey,
		payload: payload,
	}
}
