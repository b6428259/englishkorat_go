package websocket

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	fiberws "github.com/gofiber/websocket/v2"
	"github.com/gorilla/websocket"
)

// Hub maintains the set of active clients and broadcasts messages to the clients.
type Hub struct {
	// Registered clients.
	clients map[*Client]bool

	// Inbound messages from the clients.
	broadcast chan []byte

	// Register requests from the clients.
	register chan *Client

	// Unregister requests from clients.
	unregister chan *Client

	// Mutex for thread safety
	mutex sync.RWMutex
}

// Client is a middleman between the websocket connection and the hub.
type Client struct {
	hub *Hub

	// The websocket connection.
	conn *websocket.Conn

	// Buffered channel of outbound messages.
	send chan []byte

	// User ID for filtering notifications
	userID uint
}

// Message represents a WebSocket message
type Message struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// NotificationMessage represents a notification WebSocket message
type NotificationMessage struct {
	Type         string      `json:"type"`
	Notification interface{} `json:"notification"`
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// Allow connections from any origin for development
		// In production, you should check the origin
		return true
	},
}

// NewHub creates a new Hub
func NewHub() *Hub {
	return &Hub{
		broadcast:  make(chan []byte),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		clients:    make(map[*Client]bool),
	}
}

// Run starts the hub
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mutex.Lock()
			h.clients[client] = true
			h.mutex.Unlock()
			log.Printf("WebSocket client connected. User ID: %d", client.userID)

		case client := <-h.unregister:
			h.mutex.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mutex.Unlock()
			log.Printf("WebSocket client disconnected. User ID: %d", client.userID)

		case message := <-h.broadcast:
			h.mutex.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
			h.mutex.RUnlock()
		}
	}
}

// BroadcastToUser sends a message to all connections for a specific user
func (h *Hub) BroadcastToUser(userID uint, message interface{}) {
	data, err := json.Marshal(message)
	if err != nil {
		log.Printf("Error marshaling WebSocket message: %v", err)
		return
	}

	// Debug: log payload size and target user
	log.Printf("BroadcastToUser: user=%d payload_bytes=%d", userID, len(data))

	h.mutex.RLock()
	sent := 0
	dropped := 0
	for client := range h.clients {
		if client.userID == userID {
			select {
			case client.send <- data:
				sent++
			default:
				// If the send channel is full or blocked, remove client
				dropped++
				close(client.send)
				delete(h.clients, client)
			}
		}
	}
	h.mutex.RUnlock()

	log.Printf("BroadcastToUser: user=%d sent=%d dropped=%d", userID, sent, dropped)
}

// Broadcast sends a message to all connected clients
func (h *Hub) Broadcast(message interface{}) {
	data, err := json.Marshal(message)
	if err != nil {
		log.Printf("Error marshaling WebSocket message: %v", err)
		return
	}

	select {
	case h.broadcast <- data:
	default:
		log.Println("Broadcast channel is full")
	}
}

// GetClientCount returns the number of connected clients
func (h *Hub) GetClientCount() int {
	h.mutex.RLock()
	defer h.mutex.RUnlock()
	return len(h.clients)
}

// ServeWS handles websocket requests from the peer.
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request, userID uint) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket upgrade error:", err)
		return
	}

	h.ServeConn(conn, userID)
}

// ServeConn handles an already-established websocket connection
func (h *Hub) ServeConn(conn *websocket.Conn, userID uint) {
	client := &Client{
		hub:    h,
		conn:   conn,
		send:   make(chan []byte, 256),
		userID: userID,
	}

	client.hub.register <- client

	// Allow collection of memory referenced by the caller by doing all work in
	// new goroutines.
	go client.writePump()
	go client.readPump()
}

// ServeFiberWS handles Fiber websocket connections
func (h *Hub) ServeFiberWS(c *fiberws.Conn, userID uint) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("ServeFiberWS panic for user %d: %v", userID, r)
		}
	}()

	client := &Client{
		hub:    h,
		conn:   nil, // We'll handle Fiber connection differently
		send:   make(chan []byte, 256),
		userID: userID,
	}

	// Register client
	h.register <- client

	log.Printf("Starting Fiber WebSocket pumps for user %d", userID)

	// Start write pump in a goroutine, run read pump in this goroutine.
	go h.fiberWritePump(client, c)
	// Run read pump inline to avoid passing the Fiber connection across goroutines
	h.fiberReadPump(client, c)
}

// fiberWritePump handles writing to Fiber websocket connections
func (h *Hub) fiberWritePump(client *Client, c *fiberws.Conn) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("fiberWritePump panic for user %d: %v", client.userID, r)
		}
		h.unregister <- client
		c.Close()
		log.Printf("fiberWritePump ended for user %d", client.userID)
	}()

	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	for {
		select {
		case message, ok := <-client.send:
			c.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.WriteMessage(fiberws.CloseMessage, []byte{})
				return
			}

			// Debug: log outgoing message size
			log.Printf("fiberWritePump: sending to user=%d bytes=%d", client.userID, len(message))
			if err := c.WriteMessage(fiberws.TextMessage, message); err != nil {
				log.Printf("WebSocket write error for user %d: %v", client.userID, err)
				return
			}

		case <-ticker.C:
			c.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.WriteMessage(fiberws.PingMessage, nil); err != nil {
				log.Printf("WebSocket ping error for user %d: %v", client.userID, err)
				return
			}
		}
	}
}

// fiberReadPump handles reading from Fiber websocket connections
func (h *Hub) fiberReadPump(client *Client, c *fiberws.Conn) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("fiberReadPump panic for user %d: %v", client.userID, r)
		}
		h.unregister <- client
		c.Close()
		log.Printf("fiberReadPump ended for user %d", client.userID)
	}()

	c.SetReadLimit(maxMessageSize)
	c.SetReadDeadline(time.Now().Add(pongWait))
	c.SetPongHandler(func(string) error {
		c.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, _, err := c.ReadMessage()
		if err != nil {
			// Log detailed error type and value to aid debugging (e.g., nil *Conn)
			log.Printf("WebSocket read error for user %d: (type=%T) %v", client.userID, err, err)
			if fiberws.IsUnexpectedCloseError(err, fiberws.CloseGoingAway, fiberws.CloseAbnormalClosure) {
				log.Printf("WebSocket unexpected close for user %d: %v", client.userID, err)
			}
			break
		}
		// Handle incoming messages if needed
		// For notifications, we typically only send from server to client
	}
}
