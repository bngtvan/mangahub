package tcp

import (
	"bufio"
	"encoding/json"
	"log"
	"net"
	"sync"
	"time"
)

// ProgressUpdate is the JSON message exchanged by the TCP sync service.
type ProgressUpdate struct {
	UserID    string `json:"user_id"`
	MangaID   string `json:"manga_id"`
	Chapter   int    `json:"chapter"`
	Timestamp int64  `json:"timestamp"`
}

// ProgressSyncServer handles multi-client TCP broadcasting for progress updates.
type ProgressSyncServer struct {
	Port        string
	Connections map[string]net.Conn
	Broadcast   chan ProgressUpdate

	mu sync.RWMutex
}

// NewProgressSyncServer creates a new TCP sync server instance.
func NewProgressSyncServer(port string) *ProgressSyncServer {
	return &ProgressSyncServer{
		Port:        port,
		Connections: make(map[string]net.Conn),
		Broadcast:   make(chan ProgressUpdate, 128),
	}
}

// Start begins listening for TCP clients and broadcasting updates.
func (s *ProgressSyncServer) Start() error {
	ln, err := net.Listen("tcp", ":"+s.Port)
	if err != nil {
		return err
	}

	go s.broadcastLoop()
	log.Printf("TCP progress sync server listening on :%s", s.Port)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("Accept error: %v", err)
			continue
		}
		go s.handleConnection(conn)
	}
}

func (s *ProgressSyncServer) handleConnection(conn net.Conn) {
	clientID := conn.RemoteAddr().String()

	s.mu.Lock()
	s.Connections[clientID] = conn
	s.mu.Unlock()

	log.Printf("TCP client connected: %s", clientID)
	defer s.removeConnection(clientID)

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		var update ProgressUpdate
		if err := json.Unmarshal(scanner.Bytes(), &update); err != nil {
			log.Printf("Invalid progress update from %s: %v", clientID, err)
			continue
		}
		if update.Timestamp == 0 {
			update.Timestamp = time.Now().Unix()
		}
		s.Broadcast <- update
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Connection read error (%s): %v", clientID, err)
	}
}

func (s *ProgressSyncServer) removeConnection(clientID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if conn, ok := s.Connections[clientID]; ok {
		_ = conn.Close()
		delete(s.Connections, clientID)
		log.Printf("TCP client disconnected: %s", clientID)
	}
}

func (s *ProgressSyncServer) broadcastLoop() {
	for update := range s.Broadcast {
		payload, err := json.Marshal(update)
		if err != nil {
			log.Printf("Broadcast marshal error: %v", err)
			continue
		}
		payload = append(payload, '\n')

		s.mu.RLock()
		for clientID, conn := range s.Connections {
			if _, err := conn.Write(payload); err != nil {
				log.Printf("Broadcast write error (%s): %v", clientID, err)
			}
		}
		s.mu.RUnlock()
	}
}
