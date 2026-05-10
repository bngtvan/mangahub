package udp

import (
	"encoding/json"
	"log"
	"net"
	"strconv"
	"sync"
	"time"
)

// Notification is the JSON payload broadcast by the UDP notifier.
type Notification struct {
	Type      string `json:"type"`
	MangaID   string `json:"manga_id"`
	Message   string `json:"message"`
	Timestamp int64  `json:"timestamp"`
}

// NotificationServer stores registered UDP clients and broadcasts chapter updates.
type NotificationServer struct {
	Port    string
	Clients []net.UDPAddr

	mu   sync.Mutex
	conn *net.UDPConn
}

// NewNotificationServer creates a UDP notification server.
func NewNotificationServer(port string) *NotificationServer {
	return &NotificationServer{Port: port}
}

// Start begins listening for registration messages and notifications.
func (s *NotificationServer) Start() error {
	addr := &net.UDPAddr{Port: parsePort(s.Port)}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return err
	}
	s.conn = conn
	log.Printf("UDP notification server listening on :%s", s.Port)

	buf := make([]byte, 4096)
	for {
		n, clientAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			log.Printf("UDP read error: %v", err)
			continue
		}

		var incoming Notification
		if err := json.Unmarshal(buf[:n], &incoming); err != nil {
			log.Printf("Invalid UDP payload from %s: %v", clientAddr.String(), err)
			continue
		}

		if incoming.Type == "register" {
			s.registerClient(*clientAddr)
			continue
		}

		if incoming.Timestamp == 0 {
			incoming.Timestamp = time.Now().Unix()
		}
		s.BroadcastNotification(incoming)
	}
}

// BroadcastNotification sends a notification to all registered clients.
func (s *NotificationServer) BroadcastNotification(notification Notification) {
	if s.conn == nil {
		log.Printf("UDP connection is not initialized")
		return
	}
	payload, err := json.Marshal(notification)
	if err != nil {
		log.Printf("UDP marshal error: %v", err)
		return
	}

	s.mu.Lock()
	clients := make([]net.UDPAddr, len(s.Clients))
	copy(clients, s.Clients)
	s.mu.Unlock()

	for _, client := range clients {
		if _, err := s.conn.WriteToUDP(payload, &client); err != nil {
			log.Printf("UDP broadcast error to %s: %v", client.String(), err)
		}
	}
}

func (s *NotificationServer) registerClient(addr net.UDPAddr) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, c := range s.Clients {
		if c.IP.Equal(addr.IP) && c.Port == addr.Port {
			return
		}
	}
	s.Clients = append(s.Clients, addr)
	log.Printf("UDP client registered: %s", addr.String())
}

func parsePort(port string) int {
	p, err := strconv.Atoi(port)
	if err != nil || p <= 0 {
		return 9091
	}
	if p <= 0 {
		return 9091
	}
	return p
}
