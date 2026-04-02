package ws

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
	"github.com/rakibulbh/safe-london/internal/domain"
)

const (
	pingInterval = 30 * time.Second
	pongWait     = 35 * time.Second
	writeWait    = 10 * time.Second
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type Hub struct {
	mu        sync.RWMutex
	clients   map[*websocket.Conn]bool
	broadcast chan domain.EnrichedAlert
}

func NewHub() *Hub {
	return &Hub{
		clients:   make(map[*websocket.Conn]bool),
		broadcast: make(chan domain.EnrichedAlert, 64),
	}
}

// Run starts the broadcast loop — call in a goroutine.
func (h *Hub) Run() {
	for alert := range h.broadcast {
		data, err := json.Marshal(map[string]interface{}{
			"success": true,
			"data":    alert,
		})
		if err != nil {
			slog.Error("marshal broadcast", "err", err)
			continue
		}

		h.mu.RLock()
		for conn := range h.clients {
			conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				slog.Warn("write to client failed, removing", "err", err)
				conn.Close()
				go h.removeClient(conn)
			}
		}
		h.mu.RUnlock()
	}
}

// Broadcast implements domain.AlertBroadcaster.
func (h *Hub) Broadcast(alert domain.EnrichedAlert) {
	h.broadcast <- alert
}

// HandleWS is the Echo handler for WebSocket upgrade.
func (h *Hub) HandleWS(c echo.Context) error {
	conn, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}

	h.mu.Lock()
	h.clients[conn] = true
	h.mu.Unlock()

	slog.Info("ws client connected", "remote", conn.RemoteAddr())

	go h.keepAlive(conn)
	go h.readPump(conn)
	return nil
}

func (h *Hub) keepAlive(conn *websocket.Conn) {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	for range ticker.C {
		conn.SetWriteDeadline(time.Now().Add(writeWait))
		if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
			h.removeClient(conn)
			return
		}
	}
}

// readPump drains reads so pong handlers fire, and detects disconnects.
func (h *Hub) readPump(conn *websocket.Conn) {
	defer h.removeClient(conn)
	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
}

func (h *Hub) removeClient(conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.clients[conn]; ok {
		delete(h.clients, conn)
		conn.Close()
		slog.Info("ws client disconnected")
	}
}

func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
