package ws

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/gofiber/websocket/v2"
)

type Event struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

type entry struct {
	conn   *websocket.Conn
	cancel context.CancelFunc
}

type Hub struct {
	mu      sync.RWMutex
	entries map[string]*entry
}

func NewHub() *Hub {
	return &Hub{entries: make(map[string]*entry)}
}

func (h *Hub) Register(sessionID string, conn *websocket.Conn, cancel context.CancelFunc) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if e, ok := h.entries[sessionID]; ok {
		e.cancel()
	}
	h.entries[sessionID] = &entry{conn: conn, cancel: cancel}
}

func (h *Hub) Unregister(sessionID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if e, ok := h.entries[sessionID]; ok {
		e.cancel()
		delete(h.entries, sessionID)
	}
}

func (h *Hub) StopTimer(sessionID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if e, ok := h.entries[sessionID]; ok {
		e.cancel()
	}
}

func (h *Hub) Send(sessionID string, event Event) {
	h.mu.RLock()
	e, ok := h.entries[sessionID]
	h.mu.RUnlock()
	if !ok {
		return
	}
	b, _ := json.Marshal(event)
	e.conn.WriteMessage(websocket.TextMessage, b)
}
