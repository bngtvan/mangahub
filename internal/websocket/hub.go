package websocket

import (
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ClientConnection represents an incoming client registration request.
type ClientConnection struct {
	Conn     *websocket.Conn
	UserID   string
	Username string
}

// ChatMessage is broadcast to all active chat clients.
type ChatMessage struct {
	UserID    string `json:"user_id"`
	Username  string `json:"username"`
	Message   string `json:"message"`
	Timestamp int64  `json:"timestamp"`
}

// ChatHub keeps track of active websocket clients.
type ChatHub struct {
	Clients    map[*websocket.Conn]string
	Broadcast  chan ChatMessage
	Register   chan ClientConnection
	Unregister chan *websocket.Conn

	mu sync.RWMutex
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// NewChatHub creates an initialized chat hub.
func NewChatHub() *ChatHub {
	return &ChatHub{
		Clients:    make(map[*websocket.Conn]string),
		Broadcast:  make(chan ChatMessage, 128),
		Register:   make(chan ClientConnection, 64),
		Unregister: make(chan *websocket.Conn, 64),
	}
}

// Run starts the central event loop for connection and message handling.
func (h *ChatHub) Run() {
	for {
		select {
		case client := <-h.Register:
			h.mu.Lock()
			h.Clients[client.Conn] = client.Username
			h.mu.Unlock()

			h.Broadcast <- ChatMessage{
				UserID:    client.UserID,
				Username:  "system",
				Message:   client.Username + " joined the chat",
				Timestamp: time.Now().Unix(),
			}

		case conn := <-h.Unregister:
			h.remove(conn)

		case msg := <-h.Broadcast:
			h.broadcast(msg)
		}
	}
}

// ServeWS upgrades an HTTP request to websocket and processes chat messages.
func (h *ChatHub) ServeWS(w http.ResponseWriter, r *http.Request, userID, username string) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	h.Register <- ClientConnection{Conn: conn, UserID: userID, Username: username}

	for {
		var incoming struct {
			Message string `json:"message"`
		}
		if err := conn.ReadJSON(&incoming); err != nil {
			h.Unregister <- conn
			return
		}
		h.Broadcast <- ChatMessage{
			UserID:    userID,
			Username:  username,
			Message:   incoming.Message,
			Timestamp: time.Now().Unix(),
		}
	}
}

func (h *ChatHub) remove(conn *websocket.Conn) {
	h.mu.Lock()
	username, exists := h.Clients[conn]
	if exists {
		delete(h.Clients, conn)
	}
	h.mu.Unlock()

	_ = conn.Close()
	if exists {
		h.Broadcast <- ChatMessage{
			UserID:    "system",
			Username:  "system",
			Message:   username + " left the chat",
			Timestamp: time.Now().Unix(),
		}
	}
}

func (h *ChatHub) broadcast(msg ChatMessage) {
	h.mu.RLock()
	clients := make([]*websocket.Conn, 0, len(h.Clients))
	for conn := range h.Clients {
		clients = append(clients, conn)
	}
	h.mu.RUnlock()

	for _, conn := range clients {
		if err := conn.WriteJSON(msg); err != nil {
			h.Unregister <- conn
		}
	}
}
