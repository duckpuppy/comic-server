package websocket

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// EventType represents the type of WebSocket event
type EventType string

const (
	EventDeviceDiscovered   EventType = "device_discovered"
	EventDeviceConnected    EventType = "device_connected"
	EventDeviceDisconnected EventType = "device_disconnected"
	EventSyncStarted        EventType = "sync_started"
	EventSyncProgress       EventType = "sync_progress"
	EventSyncCompleted      EventType = "sync_completed"
	EventSyncFailed         EventType = "sync_failed"
	EventDeviceRegistered   EventType = "device_registered"
	EventDeviceUnregistered EventType = "device_unregistered"
	EventDeviceUpdated      EventType = "device_updated"
	EventScrapeStarted      EventType = "scrape_started"
	EventScrapeProgress     EventType = "scrape_progress"
	EventScrapeCompleted    EventType = "scrape_completed"
	EventScrapeReviewNeeded EventType = "scrape_review_needed"
	EventLibraryReloaded    EventType = "library_reloaded"
)

// Event represents a WebSocket event to be broadcast
type Event struct {
	Type      EventType   `json:"type"`
	Timestamp time.Time   `json:"timestamp"`
	Data      interface{} `json:"data"`
}

// Hub maintains the set of active WebSocket clients and broadcasts events to them
type Hub struct {
	// Registered clients
	clients map[*Client]bool

	// Inbound events from the application
	broadcast chan Event

	// Register requests from clients
	register chan *Client

	// Unregister requests from clients
	unregister chan *Client

	// Mutex for thread-safe access to clients map
	mu sync.RWMutex
}

// NewHub creates a new WebSocket hub
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan Event, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

// Run starts the hub's main loop
func (h *Hub) Run() {
	log.Info().Msg("WebSocket hub started")

	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			log.Debug().
				Str("remote_addr", client.conn.RemoteAddr().String()).
				Int("total_clients", len(h.clients)).
				Msg("WebSocket client registered")

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
				log.Debug().
					Str("remote_addr", client.conn.RemoteAddr().String()).
					Int("total_clients", len(h.clients)).
					Msg("WebSocket client unregistered")
			}
			h.mu.Unlock()

		case event := <-h.broadcast:
			// Marshal event to JSON once
			message, err := json.Marshal(event)
			if err != nil {
				log.Error().Err(err).Msg("Failed to marshal WebSocket event")
				continue
			}

			log.Debug().
				Str("event_type", string(event.Type)).
				Int("clients", len(h.clients)).
				Msg("Broadcasting event to WebSocket clients")

			// Send to all connected clients
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					// Client's send buffer is full, close the connection
					close(client.send)
					delete(h.clients, client)
					log.Warn().
						Str("remote_addr", client.conn.RemoteAddr().String()).
						Msg("WebSocket client send buffer full, disconnecting")
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Broadcast sends an event to all connected WebSocket clients
func (h *Hub) Broadcast(eventType EventType, data interface{}) {
	event := Event{
		Type:      eventType,
		Timestamp: time.Now(),
		Data:      data,
	}

	select {
	case h.broadcast <- event:
	default:
		log.Warn().
			Str("event_type", string(eventType)).
			Msg("WebSocket broadcast channel full, dropping event")
	}
}

// ClientCount returns the number of connected WebSocket clients
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// Register registers a new client with the hub
func (h *Hub) Register(client *Client) {
	h.register <- client
}

// Unregister unregisters a client from the hub
func (h *Hub) Unregister(client *Client) {
	h.unregister <- client
}
