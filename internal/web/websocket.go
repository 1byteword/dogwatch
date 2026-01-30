package web

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Message represents a WebSocket message
type Message struct {
	Topic   string      `json:"topic"`
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

// Client represents a WebSocket client connection
type Client struct {
	hub       *Hub
	conn      *wsConn
	send      chan []byte
	topics    map[string]bool
	topicsMu  sync.RWMutex
	createdAt time.Time
}

// Hub maintains active WebSocket clients and broadcasts messages
type Hub struct {
	// Registered clients
	clients map[*Client]bool

	// Topic subscriptions: topic -> set of clients
	topics map[string]map[*Client]bool

	// Inbound messages to broadcast
	broadcast chan *Message

	// Register requests from clients
	register chan *Client

	// Unregister requests from clients
	unregister chan *Client

	mu sync.RWMutex
}

// NewHub creates a new WebSocket hub
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		topics:     make(map[string]map[*Client]bool),
		broadcast:  make(chan *Message, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

// Run starts the hub's main loop
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			log.Printf("[websocket] Client connected, total: %d", len(h.clients))

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
				// Remove from all topics
				for topic := range client.topics {
					if subscribers, ok := h.topics[topic]; ok {
						delete(subscribers, client)
						if len(subscribers) == 0 {
							delete(h.topics, topic)
						}
					}
				}
			}
			h.mu.Unlock()
			log.Printf("[websocket] Client disconnected, total: %d", len(h.clients))

		case msg := <-h.broadcast:
			data, err := json.Marshal(msg)
			if err != nil {
				log.Printf("[websocket] Error marshaling message: %v", err)
				continue
			}

			h.mu.RLock()
			subscribers := h.topics[msg.Topic]
			for client := range subscribers {
				select {
				case client.send <- data:
				default:
					// Client buffer full, skip
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Subscribe subscribes a client to a topic
func (h *Hub) Subscribe(client *Client, topic string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.topics[topic] == nil {
		h.topics[topic] = make(map[*Client]bool)
	}
	h.topics[topic][client] = true

	client.topicsMu.Lock()
	client.topics[topic] = true
	client.topicsMu.Unlock()

	log.Printf("[websocket] Client subscribed to %s, subscribers: %d", topic, len(h.topics[topic]))
}

// Unsubscribe removes a client from a topic
func (h *Hub) Unsubscribe(client *Client, topic string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if subscribers, ok := h.topics[topic]; ok {
		delete(subscribers, client)
		if len(subscribers) == 0 {
			delete(h.topics, topic)
		}
	}

	client.topicsMu.Lock()
	delete(client.topics, topic)
	client.topicsMu.Unlock()
}

// Broadcast sends a message to all subscribers of a topic
func (h *Hub) Broadcast(topic, msgType string, payload interface{}) {
	msg := &Message{
		Topic:   topic,
		Type:    msgType,
		Payload: payload,
	}
	select {
	case h.broadcast <- msg:
	default:
		log.Printf("[websocket] Broadcast buffer full, dropping message for topic: %s", topic)
	}
}

// ClientCount returns the number of connected clients
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// TopicSubscriberCount returns the number of subscribers for a topic
func (h *Hub) TopicSubscriberCount(topic string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.topics[topic])
}

// wsConn is a minimal WebSocket connection wrapper
type wsConn struct {
	rw      http.ResponseWriter
	flusher http.Flusher
	conn    interface {
		Read([]byte) (int, error)
		Write([]byte) (int, error)
		Close() error
	}
	closed bool
	mu     sync.Mutex
}

// WebSocket frame opcodes
const (
	opText   = 1
	opBinary = 2
	opClose  = 8
	opPing   = 9
	opPong   = 10
)

// readFrame reads a WebSocket frame
func (c *wsConn) readFrame() (opcode byte, payload []byte, err error) {
	header := make([]byte, 2)
	if _, err = c.conn.Read(header); err != nil {
		return 0, nil, err
	}

	opcode = header[0] & 0x0f
	masked := header[1]&0x80 != 0
	length := int(header[1] & 0x7f)

	if length == 126 {
		ext := make([]byte, 2)
		if _, err = c.conn.Read(ext); err != nil {
			return 0, nil, err
		}
		length = int(ext[0])<<8 | int(ext[1])
	} else if length == 127 {
		ext := make([]byte, 8)
		if _, err = c.conn.Read(ext); err != nil {
			return 0, nil, err
		}
		length = int(ext[4])<<24 | int(ext[5])<<16 | int(ext[6])<<8 | int(ext[7])
	}

	var mask []byte
	if masked {
		mask = make([]byte, 4)
		if _, err = c.conn.Read(mask); err != nil {
			return 0, nil, err
		}
	}

	payload = make([]byte, length)
	if length > 0 {
		if _, err = c.conn.Read(payload); err != nil {
			return 0, nil, err
		}
		if masked {
			for i := range payload {
				payload[i] ^= mask[i%4]
			}
		}
	}

	return opcode, payload, nil
}

// writeFrame writes a WebSocket frame
func (c *wsConn) writeFrame(opcode byte, payload []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return fmt.Errorf("connection closed")
	}

	var header []byte
	length := len(payload)

	if length < 126 {
		header = []byte{0x80 | opcode, byte(length)}
	} else if length < 65536 {
		header = []byte{0x80 | opcode, 126, byte(length >> 8), byte(length)}
	} else {
		header = []byte{0x80 | opcode, 127, 0, 0, 0, 0,
			byte(length >> 24), byte(length >> 16), byte(length >> 8), byte(length)}
	}

	if _, err := c.conn.Write(header); err != nil {
		return err
	}
	if len(payload) > 0 {
		if _, err := c.conn.Write(payload); err != nil {
			return err
		}
	}
	return nil
}

func (c *wsConn) close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closed {
		c.closed = true
		c.conn.Close()
	}
}

// ServeWS handles WebSocket requests
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	// Perform WebSocket handshake
	if r.Header.Get("Upgrade") != "websocket" {
		http.Error(w, "Expected WebSocket upgrade", http.StatusBadRequest)
		return
	}

	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		http.Error(w, "Missing Sec-WebSocket-Key", http.StatusBadRequest)
		return
	}

	// Generate accept key
	acceptKey := computeAcceptKey(key)

	// Hijack the connection
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "WebSocket not supported", http.StatusInternalServerError)
		return
	}

	conn, buf, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Send handshake response
	response := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + acceptKey + "\r\n\r\n"
	buf.WriteString(response)
	buf.Flush()

	wsConn := &wsConn{conn: conn}
	client := &Client{
		hub:       h,
		conn:      wsConn,
		send:      make(chan []byte, 256),
		topics:    make(map[string]bool),
		createdAt: time.Now(),
	}

	h.register <- client

	// Start read and write pumps
	go client.writePump()
	go client.readPump()
}

func computeAcceptKey(key string) string {
	const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	h := sha1.New()
	h.Write([]byte(key + wsGUID))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.close()
	}()

	for {
		opcode, payload, err := c.conn.readFrame()
		if err != nil {
			break
		}

		switch opcode {
		case opClose:
			return
		case opPing:
			c.conn.writeFrame(opPong, payload)
		case opText:
			c.handleMessage(payload)
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			if !ok {
				c.conn.writeFrame(opClose, nil)
				return
			}
			if err := c.conn.writeFrame(opText, message); err != nil {
				return
			}
		case <-ticker.C:
			if err := c.conn.writeFrame(opPing, nil); err != nil {
				return
			}
		}
	}
}

func (c *Client) handleMessage(data []byte) {
	var msg struct {
		Action string `json:"action"`
		Topic  string `json:"topic"`
	}
	if err := json.Unmarshal(data, &msg); err != nil {
		log.Printf("[websocket] Invalid message: %v", err)
		return
	}

	switch msg.Action {
	case "subscribe":
		if msg.Topic != "" {
			c.hub.Subscribe(c, msg.Topic)
			// Send acknowledgment
			ack := Message{
				Topic:   msg.Topic,
				Type:    "subscribed",
				Payload: map[string]interface{}{"success": true},
			}
			if data, err := json.Marshal(ack); err == nil {
				select {
				case c.send <- data:
				default:
				}
			}
		}
	case "unsubscribe":
		if msg.Topic != "" {
			c.hub.Unsubscribe(c, msg.Topic)
		}
	}
}

// Topics for real-time updates
const (
	TopicSystem     = "system"     // System stats (CPU, memory, etc.)
	TopicServiceMap = "servicemap" // Service map updates
	TopicTraces     = "traces"     // New traces
	TopicLogs       = "logs"       // Log entries
	TopicWatches    = "watches"    // Watch state changes
	TopicAlerts     = "alerts"     // Alert state changes
	TopicIncidents  = "incidents"  // Incident updates
	TopicAnomalies  = "anomalies"  // Anomaly detections
)

// BroadcastSystemStats broadcasts system stats to subscribers
func (h *Hub) BroadcastSystemStats(stats interface{}) {
	h.Broadcast(TopicSystem, "stats", stats)
}

// BroadcastServiceMap broadcasts service map updates
func (h *Hub) BroadcastServiceMap(data interface{}) {
	h.Broadcast(TopicServiceMap, "update", data)
}

// BroadcastTrace broadcasts a new trace
func (h *Hub) BroadcastTrace(trace interface{}) {
	h.Broadcast(TopicTraces, "new", trace)
}

// BroadcastLog broadcasts a log entry
func (h *Hub) BroadcastLog(entry interface{}) {
	h.Broadcast(TopicLogs, "entry", entry)
}

// BroadcastWatchState broadcasts a watch state change
func (h *Hub) BroadcastWatchState(watchID, state string, value float64) {
	h.Broadcast(TopicWatches, "state_change", map[string]interface{}{
		"id":    watchID,
		"state": state,
		"value": value,
	})
}

// BroadcastAlert broadcasts an alert state change
func (h *Hub) BroadcastAlert(alert interface{}) {
	h.Broadcast(TopicAlerts, "update", alert)
}

// BroadcastIncident broadcasts an incident update
func (h *Hub) BroadcastIncident(incident interface{}) {
	h.Broadcast(TopicIncidents, "update", incident)
}

// BroadcastAnomaly broadcasts an anomaly detection
func (h *Hub) BroadcastAnomaly(anomaly interface{}) {
	h.Broadcast(TopicAnomalies, "detected", anomaly)
}

// StartPeriodicBroadcasts starts periodic broadcasts for system stats
func (h *Hub) StartPeriodicBroadcasts(getStats func() interface{}, getServiceMap func() interface{}) {
	// System stats every 2 seconds
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			if h.TopicSubscriberCount(TopicSystem) > 0 && getStats != nil {
				h.BroadcastSystemStats(getStats())
			}
		}
	}()

	// Service map every 10 seconds
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			if h.TopicSubscriberCount(TopicServiceMap) > 0 && getServiceMap != nil {
				h.BroadcastServiceMap(getServiceMap())
			}
		}
	}()
}

// WatchBroadcaster interface for broadcasting watch events
type WatchBroadcaster interface {
	BroadcastWatchState(watchID, state string, value float64)
}

// ValidTopics returns all valid topic names for validation
func ValidTopics() []string {
	return []string{
		TopicSystem,
		TopicServiceMap,
		TopicTraces,
		TopicLogs,
		TopicWatches,
		TopicAlerts,
		TopicIncidents,
		TopicAnomalies,
	}
}

// IsValidTopic checks if a topic name is valid
func IsValidTopic(topic string) bool {
	topic = strings.ToLower(topic)
	for _, t := range ValidTopics() {
		if t == topic {
			return true
		}
	}
	return false
}
