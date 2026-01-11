// Package federation provides gossip-based P2P cluster federation for dogwatch.
// Each node is fully functional standalone - federation adds cluster-wide visibility.
package federation

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"github.com/hashicorp/memberlist"
)

// NodeMeta contains metadata about a node in the cluster
type NodeMeta struct {
	ID          string    `json:"id"`
	Hostname    string    `json:"hostname"`
	HTTPPort    int       `json:"http_port"`
	Version     string    `json:"version"`
	StartedAt   time.Time `json:"started_at"`
	Labels      map[string]string `json:"labels,omitempty"`
}

// NodeStatus represents the current status of a node
type NodeStatus struct {
	NodeMeta
	Address       string    `json:"address"`
	State         string    `json:"state"` // alive, suspect, dead
	LastSeen      time.Time `json:"last_seen"`

	// Metrics summary
	CPUPercent    float64 `json:"cpu_percent"`
	MemPercent    float64 `json:"mem_percent"`
	ActiveIncidents int   `json:"active_incidents"`
	TotalRequests   int64 `json:"total_requests"`
	ErrorRate       float64 `json:"error_rate"`
}

// Cluster manages the gossip-based federation
type Cluster struct {
	config     *Config
	memberlist *memberlist.Memberlist

	// Local node info
	localMeta  NodeMeta

	// Shared cluster state
	state      *SharedState

	// Event handlers
	eventCh    chan memberlist.NodeEvent

	// Callbacks for state changes
	onJoin     func(node *NodeStatus)
	onLeave    func(nodeID string)
	onUpdate   func(node *NodeStatus)

	mu         sync.RWMutex
	started    bool
}

// Config holds cluster configuration
type Config struct {
	// NodeName is the unique identifier for this node (default: hostname)
	NodeName    string

	// BindAddr is the address to bind for gossip (default: 0.0.0.0)
	BindAddr    string

	// BindPort is the port for gossip protocol (default: 7946)
	BindPort    int

	// AdvertiseAddr is the address to advertise to other nodes
	AdvertiseAddr string

	// AdvertisePort is the port to advertise
	AdvertisePort int

	// HTTPPort is the local HTTP port for the web UI
	HTTPPort    int

	// Seeds are initial nodes to join (can be empty for first node)
	Seeds       []string

	// Labels are key-value pairs for node identification
	Labels      map[string]string

	// Version is the dogwatch version
	Version     string

	// EncryptionKey for securing gossip (optional, 16/24/32 bytes)
	EncryptionKey []byte
}

// DefaultConfig returns sensible defaults
func DefaultConfig() *Config {
	hostname, _ := os.Hostname()
	return &Config{
		NodeName:    hostname,
		BindAddr:    "0.0.0.0",
		BindPort:    7946,
		HTTPPort:    9999,
		Labels:      make(map[string]string),
		Version:     "1.0.0",
	}
}

// NewCluster creates a new federation cluster
func NewCluster(cfg *Config) (*Cluster, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	hostname, _ := os.Hostname()
	if cfg.NodeName == "" {
		cfg.NodeName = hostname
	}

	c := &Cluster{
		config:  cfg,
		eventCh: make(chan memberlist.NodeEvent, 256),
		localMeta: NodeMeta{
			ID:        cfg.NodeName,
			Hostname:  hostname,
			HTTPPort:  cfg.HTTPPort,
			Version:   cfg.Version,
			StartedAt: time.Now(),
			Labels:    cfg.Labels,
		},
	}

	// Initialize shared state
	c.state = NewSharedState(cfg.NodeName)

	return c, nil
}

// Start initializes the gossip cluster
func (c *Cluster) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.started {
		return fmt.Errorf("cluster already started")
	}

	// Create memberlist configuration
	mlConfig := memberlist.DefaultLANConfig()
	mlConfig.Name = c.config.NodeName
	mlConfig.BindAddr = c.config.BindAddr
	mlConfig.BindPort = c.config.BindPort

	if c.config.AdvertiseAddr != "" {
		mlConfig.AdvertiseAddr = c.config.AdvertiseAddr
	}
	if c.config.AdvertisePort > 0 {
		mlConfig.AdvertisePort = c.config.AdvertisePort
	}

	// Set up encryption if provided
	if len(c.config.EncryptionKey) > 0 {
		mlConfig.SecretKey = c.config.EncryptionKey
	}

	// Create delegate for custom message handling
	mlConfig.Delegate = &clusterDelegate{cluster: c}
	mlConfig.Events = &eventDelegate{cluster: c, ch: c.eventCh}

	// Reduce log noise
	mlConfig.LogOutput = &logWriter{prefix: "[federation]"}

	// Create the memberlist
	list, err := memberlist.Create(mlConfig)
	if err != nil {
		return fmt.Errorf("failed to create memberlist: %w", err)
	}
	c.memberlist = list

	// Join seed nodes if provided
	if len(c.config.Seeds) > 0 {
		n, err := list.Join(c.config.Seeds)
		if err != nil {
			log.Printf("[federation] Warning: failed to join seeds: %v", err)
		} else {
			log.Printf("[federation] Joined %d seed nodes", n)
		}
	}

	// Start event handler
	go c.handleEvents()

	// Start periodic state broadcast
	go c.broadcastLoop()

	c.started = true
	log.Printf("[federation] Cluster started: %s (gossip port %d)", c.config.NodeName, c.config.BindPort)

	return nil
}

// Stop gracefully leaves the cluster
func (c *Cluster) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started || c.memberlist == nil {
		return nil
	}

	// Graceful leave with timeout
	if err := c.memberlist.Leave(5 * time.Second); err != nil {
		log.Printf("[federation] Warning: leave failed: %v", err)
	}

	if err := c.memberlist.Shutdown(); err != nil {
		return fmt.Errorf("shutdown failed: %w", err)
	}

	close(c.eventCh)
	c.started = false
	log.Printf("[federation] Cluster stopped")

	return nil
}

// Join attempts to join an existing cluster
func (c *Cluster) Join(addrs []string) (int, error) {
	if c.memberlist == nil {
		return 0, fmt.Errorf("cluster not started")
	}
	return c.memberlist.Join(addrs)
}

// Members returns all known cluster members
func (c *Cluster) Members() []*NodeStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.memberlist == nil {
		return nil
	}

	members := c.memberlist.Members()
	nodes := make([]*NodeStatus, 0, len(members))

	for _, m := range members {
		node := &NodeStatus{
			Address:  m.Addr.String(),
			LastSeen: time.Now(),
			State:    "alive",
		}

		// Parse metadata
		if len(m.Meta) > 0 {
			json.Unmarshal(m.Meta, &node.NodeMeta)
		} else {
			node.NodeMeta = NodeMeta{
				ID:       m.Name,
				Hostname: m.Name,
			}
		}

		// Get node state from shared state
		if state := c.state.GetNodeState(m.Name); state != nil {
			node.CPUPercent = state.CPUPercent
			node.MemPercent = state.MemPercent
			node.ActiveIncidents = state.ActiveIncidents
			node.TotalRequests = state.TotalRequests
			node.ErrorRate = state.ErrorRate
		}

		nodes = append(nodes, node)
	}

	return nodes
}

// LocalNode returns the local node's status
func (c *Cluster) LocalNode() *NodeStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.memberlist == nil {
		return nil
	}

	local := c.memberlist.LocalNode()
	return &NodeStatus{
		NodeMeta: c.localMeta,
		Address:  local.Addr.String(),
		State:    "alive",
		LastSeen: time.Now(),
	}
}

// NumMembers returns the number of nodes in the cluster
func (c *Cluster) NumMembers() int {
	if c.memberlist == nil {
		return 0
	}
	return c.memberlist.NumMembers()
}

// GetState returns the shared cluster state
func (c *Cluster) GetState() *SharedState {
	return c.state
}

// UpdateLocalMetrics updates this node's metrics in the shared state
func (c *Cluster) UpdateLocalMetrics(cpu, mem float64, incidents int, requests int64, errorRate float64) {
	c.state.UpdateLocalNode(cpu, mem, incidents, requests, errorRate)
}

// BroadcastIncident broadcasts an incident event to the cluster
func (c *Cluster) BroadcastIncident(inc *IncidentEvent) {
	c.state.AddIncident(inc)
	c.broadcast(MessageTypeIncident, inc)
}

// BroadcastIncidentUpdate broadcasts an incident status change
func (c *Cluster) BroadcastIncidentUpdate(incidentID string, status string, user string) {
	update := &IncidentUpdate{
		IncidentID: incidentID,
		Status:     status,
		User:       user,
		Timestamp:  time.Now(),
		NodeID:     c.config.NodeName,
	}
	c.state.UpdateIncident(update)
	c.broadcast(MessageTypeIncidentUpdate, update)
}

// OnJoin sets a callback for when a node joins
func (c *Cluster) OnJoin(fn func(node *NodeStatus)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onJoin = fn
}

// OnLeave sets a callback for when a node leaves
func (c *Cluster) OnLeave(fn func(nodeID string)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onLeave = fn
}

// handleEvents processes cluster membership events
func (c *Cluster) handleEvents() {
	for event := range c.eventCh {
		c.mu.RLock()
		onJoin := c.onJoin
		onLeave := c.onLeave
		c.mu.RUnlock()

		switch event.Event {
		case memberlist.NodeJoin:
			log.Printf("[federation] Node joined: %s (%s)", event.Node.Name, event.Node.Addr)
			if onJoin != nil {
				node := &NodeStatus{
					Address: event.Node.Addr.String(),
					State:   "alive",
				}
				if len(event.Node.Meta) > 0 {
					json.Unmarshal(event.Node.Meta, &node.NodeMeta)
				}
				onJoin(node)
			}
			// Request state sync from new node
			c.requestStateSync(event.Node.Name)

		case memberlist.NodeLeave:
			log.Printf("[federation] Node left: %s", event.Node.Name)
			c.state.RemoveNode(event.Node.Name)
			if onLeave != nil {
				onLeave(event.Node.Name)
			}

		case memberlist.NodeUpdate:
			log.Printf("[federation] Node updated: %s", event.Node.Name)
		}
	}
}

// broadcastLoop periodically broadcasts local state
func (c *Cluster) broadcastLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if c.memberlist != nil && c.started {
				c.broadcastLocalState()
			}
		}
	}
}

// broadcastLocalState sends local node state to the cluster
func (c *Cluster) broadcastLocalState() {
	state := c.state.GetLocalState()
	if state == nil {
		return
	}
	c.broadcast(MessageTypeNodeState, state)
}

// broadcast sends a message to all cluster members
func (c *Cluster) broadcast(msgType MessageType, payload interface{}) {
	if c.memberlist == nil {
		return
	}

	msg := &Message{
		Type:      msgType,
		NodeID:    c.config.NodeName,
		Timestamp: time.Now(),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[federation] Failed to marshal payload: %v", err)
		return
	}
	msg.Payload = data

	msgBytes, err := json.Marshal(msg)
	if err != nil {
		log.Printf("[federation] Failed to marshal message: %v", err)
		return
	}

	// Use unreliable broadcast for state updates (UDP, best effort)
	for _, member := range c.memberlist.Members() {
		if member.Name == c.config.NodeName {
			continue
		}
		c.memberlist.SendBestEffort(member, msgBytes)
	}
}

// requestStateSync requests full state from a node
func (c *Cluster) requestStateSync(nodeID string) {
	msg := &Message{
		Type:      MessageTypeSyncRequest,
		NodeID:    c.config.NodeName,
		Timestamp: time.Now(),
	}

	msgBytes, _ := json.Marshal(msg)

	for _, member := range c.memberlist.Members() {
		if member.Name == nodeID {
			c.memberlist.SendReliable(member, msgBytes)
			break
		}
	}
}

// sendStateSyncResponse sends full state to a requesting node
func (c *Cluster) sendStateSyncResponse(toNode string) {
	response := c.state.GetFullState()

	msg := &Message{
		Type:      MessageTypeSyncResponse,
		NodeID:    c.config.NodeName,
		Timestamp: time.Now(),
	}

	data, _ := json.Marshal(response)
	msg.Payload = data

	msgBytes, _ := json.Marshal(msg)

	for _, member := range c.memberlist.Members() {
		if member.Name == toNode {
			c.memberlist.SendReliable(member, msgBytes)
			break
		}
	}
}

// GetLocalAddr returns the local gossip address
func (c *Cluster) GetLocalAddr() string {
	if c.memberlist == nil {
		return ""
	}
	local := c.memberlist.LocalNode()
	return net.JoinHostPort(local.Addr.String(), fmt.Sprintf("%d", local.Port))
}

// clusterDelegate implements memberlist.Delegate
type clusterDelegate struct {
	cluster *Cluster
}

func (d *clusterDelegate) NodeMeta(limit int) []byte {
	meta, _ := json.Marshal(d.cluster.localMeta)
	if len(meta) > limit {
		return nil
	}
	return meta
}

func (d *clusterDelegate) NotifyMsg(msg []byte) {
	var m Message
	if err := json.Unmarshal(msg, &m); err != nil {
		return
	}
	d.cluster.handleMessage(&m)
}

func (d *clusterDelegate) GetBroadcasts(overhead, limit int) [][]byte {
	return nil
}

func (d *clusterDelegate) LocalState(join bool) []byte {
	state := d.cluster.state.GetFullState()
	data, _ := json.Marshal(state)
	return data
}

func (d *clusterDelegate) MergeRemoteState(buf []byte, join bool) {
	var state FullState
	if err := json.Unmarshal(buf, &state); err != nil {
		return
	}
	d.cluster.state.MergeFullState(&state)
}

// handleMessage processes incoming cluster messages
func (c *Cluster) handleMessage(msg *Message) {
	switch msg.Type {
	case MessageTypeNodeState:
		var state NodeState
		if err := json.Unmarshal(msg.Payload, &state); err == nil {
			c.state.MergeNodeState(msg.NodeID, &state)
		}

	case MessageTypeIncident:
		var inc IncidentEvent
		if err := json.Unmarshal(msg.Payload, &inc); err == nil {
			c.state.AddIncident(&inc)
		}

	case MessageTypeIncidentUpdate:
		var update IncidentUpdate
		if err := json.Unmarshal(msg.Payload, &update); err == nil {
			c.state.UpdateIncident(&update)
		}

	case MessageTypeSyncRequest:
		c.sendStateSyncResponse(msg.NodeID)

	case MessageTypeSyncResponse:
		var state FullState
		if err := json.Unmarshal(msg.Payload, &state); err == nil {
			c.state.MergeFullState(&state)
		}
	}
}

// eventDelegate implements memberlist.EventDelegate
type eventDelegate struct {
	cluster *Cluster
	ch      chan memberlist.NodeEvent
}

func (d *eventDelegate) NotifyJoin(node *memberlist.Node) {
	d.ch <- memberlist.NodeEvent{Event: memberlist.NodeJoin, Node: node}
}

func (d *eventDelegate) NotifyLeave(node *memberlist.Node) {
	d.ch <- memberlist.NodeEvent{Event: memberlist.NodeLeave, Node: node}
}

func (d *eventDelegate) NotifyUpdate(node *memberlist.Node) {
	d.ch <- memberlist.NodeEvent{Event: memberlist.NodeUpdate, Node: node}
}

// logWriter filters memberlist logs
type logWriter struct {
	prefix string
}

func (w *logWriter) Write(p []byte) (n int, err error) {
	// Only log warnings and errors
	msg := string(p)
	if len(msg) > 0 && (msg[0] == '[' || msg[0] == 'W' || msg[0] == 'E') {
		log.Printf("%s %s", w.prefix, msg)
	}
	return len(p), nil
}
