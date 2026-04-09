package events

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/google/uuid"
)

// Event represents a server-side event to be broadcast to connected WebSocket clients.
type Event struct {
	Payload any    `json:"payload"`
	Type    string `json:"type"`
}

// Hub is a thread-safe pub/sub hub that broadcasts events to registered WebSocket clients.
type Hub struct {
	clients map[string]chan []byte // clientID -> JSON-encoded event channel
	closed  chan struct{}
	mu      sync.RWMutex
	once    sync.Once
}

// NewHub creates a new event hub.
func NewHub() *Hub {
	return &Hub{
		clients: make(map[string]chan []byte),
		closed:  make(chan struct{}),
	}
}

// Register adds a new client and returns its ID and event channel.
func (h *Hub) Register() (string, <-chan []byte) {
	id := uuid.New().String()
	ch := make(chan []byte, 64)

	h.mu.Lock()
	h.clients[id] = ch
	h.mu.Unlock()

	log.Printf("WebSocket client %s registered (%d total)", id, h.ClientCount())
	return id, ch
}

// Unregister removes a client and closes its channel.
func (h *Hub) Unregister(clientID string) {
	h.mu.Lock()
	if ch, ok := h.clients[clientID]; ok {
		close(ch)
		delete(h.clients, clientID)
	}
	h.mu.Unlock()

	log.Printf("WebSocket client %s unregistered", clientID)
}

// Broadcast sends an event to all connected clients.
// Events are dropped for clients whose buffers are full (non-blocking).
// No-op after Close() has been called.
func (h *Hub) Broadcast(event Event) {
	select {
	case <-h.closed:
		return
	default:
	}

	data, err := json.Marshal(event)
	if err != nil {
		log.Printf("Failed to marshal event %s: %v", event.Type, err)
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for id, ch := range h.clients {
		select {
		case ch <- data:
		default:
			log.Printf("WebSocket client %s buffer full, dropping event %s", id, event.Type)
		}
	}
}

// Close shuts down the hub, closing all client channels.
func (h *Hub) Close() {
	h.once.Do(func() {
		close(h.closed)

		h.mu.Lock()
		defer h.mu.Unlock()
		for id, ch := range h.clients {
			close(ch)
			delete(h.clients, id)
		}
	})
}

// ClientCount returns the number of connected clients.
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
