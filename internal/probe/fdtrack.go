package probe

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// SocketProtocol identifies the network protocol of a tracked socket
type SocketProtocol uint8

const (
	ProtoUnknown SocketProtocol = iota
	ProtoTCP
	ProtoUDP
	ProtoTCP6
	ProtoUDP6
)

func (p SocketProtocol) String() string {
	switch p {
	case ProtoTCP:
		return "tcp"
	case ProtoUDP:
		return "udp"
	case ProtoTCP6:
		return "tcp6"
	case ProtoUDP6:
		return "udp6"
	default:
		return "unknown"
	}
}

// SocketMeta holds metadata about a network socket associated with an FD
type SocketMeta struct {
	LocalIP    net.IP         `json:"local_ip"`
	LocalPort  uint16         `json:"local_port"`
	RemoteIP   net.IP         `json:"remote_ip"`
	RemotePort uint16         `json:"remote_port"`
	Protocol   SocketProtocol `json:"protocol"`
	Family     uint16         `json:"family"` // AF_INET or AF_INET6
	State      uint8          `json:"state"`  // TCP state (1=ESTABLISHED, 10=LISTEN, etc.)
	Inode      uint64         `json:"inode"`
	FirstSeen  time.Time      `json:"first_seen"`
	LastSeen   time.Time      `json:"last_seen"`
	Direction  string         `json:"direction"` // "inbound" or "outbound"
}

// IsNetworkSocket returns true (this is always the case for SocketMeta entries)
func (s *SocketMeta) IsNetworkSocket() bool {
	return true
}

// fdKey uniquely identifies an FD within a process
type fdKey struct {
	PID uint32
	FD  int32
}

// FDTracker maps file descriptors to socket metadata for network-aware tracing.
// It watches for new TCP connections from the existing TCP probe and resolves
// which FDs are network sockets by reading /proc/<pid>/fd and /proc/net/tcp.
// The HTTP and DB probes can then query this tracker to skip non-network FDs,
// saving CPU and enabling protocol-aware connection association.
type FDTracker struct {
	mu sync.RWMutex

	// Primary FD -> socket metadata map
	sockets map[fdKey]*SocketMeta

	// Inode -> socket metadata for fast inode-based lookups during /proc scanning
	inodeMeta map[uint64]*SocketMeta

	// Stats
	totalTracked   int64
	totalEvicted   int64
	totalLookups   int64
	totalHits      int64
	totalMisses    int64
	totalScans     int64
	skippedNonNet  int64

	// Configuration
	maxEntries     int
	cleanupAge     time.Duration
	scanInterval   time.Duration

	// Lifecycle
	stopChan chan struct{}
	stopped  int32 // atomic
}

// FDTrackerConfig holds configuration for the FD tracker
type FDTrackerConfig struct {
	// MaxEntries is the maximum number of FD entries to track (default 65536)
	MaxEntries int
	// CleanupAge is how long to keep FD entries after last seen (default 5m)
	CleanupAge time.Duration
	// ScanInterval is how often to scan /proc for socket state updates (default 30s)
	ScanInterval time.Duration
}

// DefaultFDTrackerConfig returns sensible default configuration
func DefaultFDTrackerConfig() FDTrackerConfig {
	return FDTrackerConfig{
		MaxEntries:   65536,
		CleanupAge:   5 * time.Minute,
		ScanInterval: 30 * time.Second,
	}
}

// NewFDTracker creates a new FD tracker with the given configuration
func NewFDTracker(cfg FDTrackerConfig) *FDTracker {
	if cfg.MaxEntries <= 0 {
		cfg.MaxEntries = 65536
	}
	if cfg.CleanupAge <= 0 {
		cfg.CleanupAge = 5 * time.Minute
	}
	if cfg.ScanInterval <= 0 {
		cfg.ScanInterval = 30 * time.Second
	}

	t := &FDTracker{
		sockets:      make(map[fdKey]*SocketMeta, cfg.MaxEntries/4),
		inodeMeta:    make(map[uint64]*SocketMeta, cfg.MaxEntries/4),
		maxEntries:   cfg.MaxEntries,
		cleanupAge:   cfg.CleanupAge,
		scanInterval: cfg.ScanInterval,
		stopChan:     make(chan struct{}),
	}

	return t
}

// Start begins the background cleanup and scanning goroutine
func (t *FDTracker) Start() {
	go t.backgroundLoop()
	log.Printf("[fdtrack] FD tracker started (max=%d, cleanup=%v, scan=%v)",
		t.maxEntries, t.cleanupAge, t.scanInterval)
}

// Stop shuts down the background goroutine
func (t *FDTracker) Stop() {
	if atomic.CompareAndSwapInt32(&t.stopped, 0, 1) {
		close(t.stopChan)
	}
}

// backgroundLoop runs periodic cleanup and /proc scanning
func (t *FDTracker) backgroundLoop() {
	cleanupTicker := time.NewTicker(t.cleanupAge / 2)
	defer cleanupTicker.Stop()

	scanTicker := time.NewTicker(t.scanInterval)
	defer scanTicker.Stop()

	for {
		select {
		case <-cleanupTicker.C:
			t.cleanup()
		case <-scanTicker.C:
			t.scanActiveProcesses()
		case <-t.stopChan:
			return
		}
	}
}

// TrackAccept records a new socket FD from an accept/accept4 syscall return.
// Called when the TCP probe sees an inet_csk_accept kretprobe event.
// The pid and connection metadata come from the TCP probe event.
func (t *FDTracker) TrackAccept(pid uint32, localIP net.IP, localPort uint16,
	remoteIP net.IP, remotePort uint16, family uint16) {

	meta := &SocketMeta{
		LocalIP:    localIP,
		LocalPort:  localPort,
		RemoteIP:   remoteIP,
		RemotePort: remotePort,
		Protocol:   protoFromFamily(family),
		Family:     family,
		State:      1, // ESTABLISHED
		FirstSeen:  time.Now(),
		LastSeen:   time.Now(),
		Direction:  "inbound",
	}

	// Resolve FD by scanning /proc/<pid>/fd for the matching socket inode
	fd, inode := t.resolveFDForConnection(pid, localIP, localPort, remoteIP, remotePort)
	if fd >= 0 {
		meta.Inode = inode
		t.storeFD(pid, int32(fd), meta)
	}
}

// TrackConnect records a new socket FD from a connect syscall.
// Called when the TCP probe sees a tcp_connect kretprobe event.
func (t *FDTracker) TrackConnect(pid uint32, localIP net.IP, localPort uint16,
	remoteIP net.IP, remotePort uint16, family uint16) {

	meta := &SocketMeta{
		LocalIP:    localIP,
		LocalPort:  localPort,
		RemoteIP:   remoteIP,
		RemotePort: remotePort,
		Protocol:   protoFromFamily(family),
		Family:     family,
		State:      1, // ESTABLISHED
		FirstSeen:  time.Now(),
		LastSeen:   time.Now(),
		Direction:  "outbound",
	}

	fd, inode := t.resolveFDForConnection(pid, localIP, localPort, remoteIP, remotePort)
	if fd >= 0 {
		meta.Inode = inode
		t.storeFD(pid, int32(fd), meta)
	}
}

// TrackClose removes tracking for an FD when it is closed.
// Can be called when we detect a closed FD during scanning.
func (t *FDTracker) TrackClose(pid uint32, fd int32) {
	key := fdKey{PID: pid, FD: fd}
	t.mu.Lock()
	if meta, ok := t.sockets[key]; ok {
		delete(t.sockets, key)
		if meta.Inode > 0 {
			delete(t.inodeMeta, meta.Inode)
		}
		atomic.AddInt64(&t.totalEvicted, 1)
	}
	t.mu.Unlock()
}

// Lookup checks if a given (pid, fd) maps to a network socket.
// Returns the SocketMeta if found, nil if the FD is not a tracked network socket.
// This is the primary function called by HTTP/DB probes to filter non-network FDs.
func (t *FDTracker) Lookup(pid uint32, fd int32) *SocketMeta {
	atomic.AddInt64(&t.totalLookups, 1)

	key := fdKey{PID: pid, FD: fd}
	t.mu.RLock()
	meta, ok := t.sockets[key]
	t.mu.RUnlock()

	if ok {
		atomic.AddInt64(&t.totalHits, 1)
		// Update last seen (racy but acceptable for stats)
		meta.LastSeen = time.Now()
		return meta
	}

	atomic.AddInt64(&t.totalMisses, 1)
	return nil
}

// IsNetworkFD returns true if the given (pid, fd) is a tracked network socket.
// This is a convenience wrapper around Lookup for simple boolean checks.
func (t *FDTracker) IsNetworkFD(pid uint32, fd int32) bool {
	return t.Lookup(pid, fd) != nil
}

// GetSocketsForPID returns all tracked sockets for a given process.
func (t *FDTracker) GetSocketsForPID(pid uint32) map[int32]*SocketMeta {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make(map[int32]*SocketMeta)
	for key, meta := range t.sockets {
		if key.PID == pid {
			result[key.FD] = meta
		}
	}
	return result
}

// Stats returns current FD tracker statistics
func (t *FDTracker) Stats() FDTrackerStats {
	t.mu.RLock()
	activeEntries := len(t.sockets)
	t.mu.RUnlock()

	return FDTrackerStats{
		ActiveEntries: activeEntries,
		TotalTracked:  atomic.LoadInt64(&t.totalTracked),
		TotalEvicted:  atomic.LoadInt64(&t.totalEvicted),
		TotalLookups:  atomic.LoadInt64(&t.totalLookups),
		TotalHits:     atomic.LoadInt64(&t.totalHits),
		TotalMisses:   atomic.LoadInt64(&t.totalMisses),
		TotalScans:    atomic.LoadInt64(&t.totalScans),
		SkippedNonNet: atomic.LoadInt64(&t.skippedNonNet),
		HitRate:       t.hitRate(),
	}
}

// FDTrackerStats holds statistics about the FD tracker
type FDTrackerStats struct {
	ActiveEntries int     `json:"active_entries"`
	TotalTracked  int64   `json:"total_tracked"`
	TotalEvicted  int64   `json:"total_evicted"`
	TotalLookups  int64   `json:"total_lookups"`
	TotalHits     int64   `json:"total_hits"`
	TotalMisses   int64   `json:"total_misses"`
	TotalScans    int64   `json:"total_scans"`
	SkippedNonNet int64   `json:"skipped_non_net"`
	HitRate       float64 `json:"hit_rate"`
}

func (t *FDTracker) hitRate() float64 {
	lookups := atomic.LoadInt64(&t.totalLookups)
	if lookups == 0 {
		return 0
	}
	return float64(atomic.LoadInt64(&t.totalHits)) / float64(lookups)
}

// storeFD stores an FD -> socket metadata mapping
func (t *FDTracker) storeFD(pid uint32, fd int32, meta *SocketMeta) {
	key := fdKey{PID: pid, FD: fd}
	t.mu.Lock()
	defer t.mu.Unlock()

	// Evict if at capacity
	if len(t.sockets) >= t.maxEntries {
		t.evictOldest()
	}

	t.sockets[key] = meta
	if meta.Inode > 0 {
		t.inodeMeta[meta.Inode] = meta
	}
	atomic.AddInt64(&t.totalTracked, 1)
}

// evictOldest removes the oldest entries to make room. Must be called with mu held.
func (t *FDTracker) evictOldest() {
	// Remove entries older than cleanup age first
	now := time.Now()
	cutoff := now.Add(-t.cleanupAge)
	evicted := 0
	target := t.maxEntries / 10 // evict at least 10%

	for key, meta := range t.sockets {
		if meta.LastSeen.Before(cutoff) || evicted < target {
			if meta.Inode > 0 {
				delete(t.inodeMeta, meta.Inode)
			}
			delete(t.sockets, key)
			evicted++
			atomic.AddInt64(&t.totalEvicted, 1)
		}
		if evicted >= target {
			break
		}
	}
}

// cleanup removes stale entries and validates that tracked FDs still exist
func (t *FDTracker) cleanup() {
	now := time.Now()
	cutoff := now.Add(-t.cleanupAge)

	t.mu.Lock()
	defer t.mu.Unlock()

	for key, meta := range t.sockets {
		// Remove aged-out entries
		if meta.LastSeen.Before(cutoff) {
			if meta.Inode > 0 {
				delete(t.inodeMeta, meta.Inode)
			}
			delete(t.sockets, key)
			atomic.AddInt64(&t.totalEvicted, 1)
			continue
		}

		// Validate FD still exists in /proc (detect close without explicit TrackClose)
		fdPath := fmt.Sprintf("/proc/%d/fd/%d", key.PID, key.FD)
		if _, err := os.Readlink(fdPath); err != nil {
			if meta.Inode > 0 {
				delete(t.inodeMeta, meta.Inode)
			}
			delete(t.sockets, key)
			atomic.AddInt64(&t.totalEvicted, 1)
		}
	}
}

// scanActiveProcesses scans /proc to discover new socket FDs for tracked PIDs.
// This catches sockets that were created outside of observed accept/connect events.
func (t *FDTracker) scanActiveProcesses() {
	atomic.AddInt64(&t.totalScans, 1)

	// Build set of PIDs we already track
	t.mu.RLock()
	pids := make(map[uint32]bool)
	for key := range t.sockets {
		pids[key.PID] = true
	}
	t.mu.RUnlock()

	// Scan each tracked PID for new socket FDs
	for pid := range pids {
		t.scanProcessFDs(pid)
	}
}

// scanProcessFDs scans /proc/<pid>/fd for socket FDs not yet tracked
func (t *FDTracker) scanProcessFDs(pid uint32) {
	fdDir := fmt.Sprintf("/proc/%d/fd", pid)
	entries, err := os.ReadDir(fdDir)
	if err != nil {
		return // Process may have exited
	}

	for _, entry := range entries {
		fdNum, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		key := fdKey{PID: pid, FD: int32(fdNum)}

		// Skip if already tracked
		t.mu.RLock()
		_, exists := t.sockets[key]
		t.mu.RUnlock()
		if exists {
			continue
		}

		// Read the symlink to check if it's a socket
		link, err := os.Readlink(filepath.Join(fdDir, entry.Name()))
		if err != nil {
			continue
		}

		// Socket FDs look like "socket:[12345]" where 12345 is the inode
		if !strings.HasPrefix(link, "socket:[") {
			atomic.AddInt64(&t.skippedNonNet, 1)
			continue
		}

		inodeStr := link[8 : len(link)-1] // Extract inode number
		inode, err := strconv.ParseUint(inodeStr, 10, 64)
		if err != nil {
			continue
		}

		// Look up the inode in /proc/net/tcp and /proc/net/tcp6
		meta := t.lookupSocketByInode(pid, inode)
		if meta != nil {
			t.storeFD(pid, int32(fdNum), meta)
		}
	}
}

// resolveFDForConnection finds the FD number for a specific connection
// by scanning /proc/<pid>/fd and matching against /proc/net/tcp entries
func (t *FDTracker) resolveFDForConnection(pid uint32, localIP net.IP, localPort uint16,
	remoteIP net.IP, remotePort uint16) (int, uint64) {

	// First, find the inode for this connection in /proc/net/tcp
	inode := findInodeForConnection(pid, localIP, localPort, remoteIP, remotePort)
	if inode == 0 {
		return -1, 0
	}

	// Then find the FD that points to this inode
	fdDir := fmt.Sprintf("/proc/%d/fd", pid)
	entries, err := os.ReadDir(fdDir)
	if err != nil {
		return -1, 0
	}

	socketPrefix := fmt.Sprintf("socket:[%d]", inode)
	for _, entry := range entries {
		link, err := os.Readlink(filepath.Join(fdDir, entry.Name()))
		if err != nil {
			continue
		}
		if link == socketPrefix {
			fd, err := strconv.Atoi(entry.Name())
			if err != nil {
				continue
			}
			return fd, inode
		}
	}

	return -1, inode
}

// findInodeForConnection searches /proc/net/tcp{,6} for a matching connection
func findInodeForConnection(pid uint32, localIP net.IP, localPort uint16,
	remoteIP net.IP, remotePort uint16) uint64 {

	// Try /proc/<pid>/net/tcp first (process-specific view)
	paths := []string{
		fmt.Sprintf("/proc/%d/net/tcp", pid),
		fmt.Sprintf("/proc/%d/net/tcp6", pid),
	}

	for _, path := range paths {
		inode := searchProcNetTCP(path, localIP, localPort, remoteIP, remotePort)
		if inode > 0 {
			return inode
		}
	}

	return 0
}

// searchProcNetTCP searches a /proc/net/tcp file for a matching connection
func searchProcNetTCP(path string, localIP net.IP, localPort uint16,
	remoteIP net.IP, remotePort uint16) uint64 {

	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	// Skip header line
	if !scanner.Scan() {
		return 0
	}

	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}

		// Parse local address (field 1): hex_ip:hex_port
		lAddr, lPort, ok := parseProcNetAddr(fields[1])
		if !ok {
			continue
		}

		// Parse remote address (field 2): hex_ip:hex_port
		rAddr, rPort, ok := parseProcNetAddr(fields[2])
		if !ok {
			continue
		}

		// Match the connection
		if lPort == localPort && rPort == remotePort &&
			lAddr.Equal(localIP) && rAddr.Equal(remoteIP) {
			// Field 9 is the inode
			inode, err := strconv.ParseUint(fields[9], 10, 64)
			if err != nil {
				continue
			}
			return inode
		}
	}

	return 0
}

// parseProcNetAddr parses a hex address:port from /proc/net/tcp format.
// In /proc/net/tcp, IPv4 addresses are printed as "%08X" of the __be32 value
// read as a native-endian integer. On little-endian (x86), this means
// "0100007F" represents 127.0.0.1 (bytes 7F,00,00,01 stored in memory,
// read as LE int = 0x0100007F).
// The hex string bytes are therefore in reverse order of the actual IP octets.
func parseProcNetAddr(s string) (net.IP, uint16, bool) {
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return nil, 0, false
	}

	port, err := strconv.ParseUint(parts[1], 16, 16)
	if err != nil {
		return nil, 0, false
	}

	hexIP := parts[0]
	switch len(hexIP) {
	case 8: // IPv4
		// Parse as a 32-bit integer and extract bytes in reverse order.
		// "0100007F" -> uint32 0x0100007F -> bytes in LE: 7F,00,00,01 -> 127.0.0.1
		val, err := strconv.ParseUint(hexIP, 16, 32)
		if err != nil {
			return nil, 0, false
		}
		ip := make(net.IP, 4)
		ip[0] = byte(val & 0xFF)
		ip[1] = byte((val >> 8) & 0xFF)
		ip[2] = byte((val >> 16) & 0xFF)
		ip[3] = byte((val >> 24) & 0xFF)
		return ip, uint16(port), true

	case 32: // IPv6
		// IPv6 in /proc/net/tcp6 is stored as 4 little-endian 32-bit words.
		// Each 8-char hex group is a 32-bit word in the same LE format as IPv4.
		ip := make(net.IP, 16)
		for word := 0; word < 4; word++ {
			hexWord := hexIP[word*8 : word*8+8]
			val, err := strconv.ParseUint(hexWord, 16, 32)
			if err != nil {
				return nil, 0, false
			}
			ip[word*4+0] = byte(val & 0xFF)
			ip[word*4+1] = byte((val >> 8) & 0xFF)
			ip[word*4+2] = byte((val >> 16) & 0xFF)
			ip[word*4+3] = byte((val >> 24) & 0xFF)
		}
		return ip, uint16(port), true

	default:
		return nil, 0, false
	}
}

// lookupSocketByInode looks up socket metadata from /proc/net/tcp{,6} by inode
func (t *FDTracker) lookupSocketByInode(pid uint32, inode uint64) *SocketMeta {
	// Check inode cache first
	t.mu.RLock()
	if meta, ok := t.inodeMeta[inode]; ok {
		t.mu.RUnlock()
		return meta
	}
	t.mu.RUnlock()

	paths := []struct {
		path  string
		proto SocketProtocol
	}{
		{fmt.Sprintf("/proc/%d/net/tcp", pid), ProtoTCP},
		{fmt.Sprintf("/proc/%d/net/tcp6", pid), ProtoTCP6},
		{fmt.Sprintf("/proc/%d/net/udp", pid), ProtoUDP},
		{fmt.Sprintf("/proc/%d/net/udp6", pid), ProtoUDP6},
	}

	for _, p := range paths {
		meta := searchProcNetByInode(p.path, inode, p.proto)
		if meta != nil {
			return meta
		}
	}

	return nil
}

// searchProcNetByInode searches a /proc/net/* file for an entry matching the given inode
func searchProcNetByInode(path string, targetInode uint64, proto SocketProtocol) *SocketMeta {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	// Skip header
	if !scanner.Scan() {
		return nil
	}

	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}

		// Field 9 is the inode
		inode, err := strconv.ParseUint(fields[9], 10, 64)
		if err != nil || inode != targetInode {
			continue
		}

		// Parse addresses
		localIP, localPort, ok := parseProcNetAddr(fields[1])
		if !ok {
			continue
		}
		remoteIP, remotePort, ok := parseProcNetAddr(fields[2])
		if !ok {
			continue
		}

		// Parse state
		state, _ := strconv.ParseUint(fields[3], 16, 8)

		family := uint16(afINET)
		if proto == ProtoTCP6 || proto == ProtoUDP6 {
			family = uint16(afINET6)
		}

		return &SocketMeta{
			LocalIP:    localIP,
			LocalPort:  localPort,
			RemoteIP:   remoteIP,
			RemotePort: remotePort,
			Protocol:   proto,
			Family:     family,
			State:      uint8(state),
			Inode:      targetInode,
			FirstSeen:  time.Now(),
			LastSeen:   time.Now(),
		}
	}

	return nil
}

// protoFromFamily returns the TCP protocol variant based on address family
func protoFromFamily(family uint16) SocketProtocol {
	if family == afINET6 {
		return ProtoTCP6
	}
	return ProtoTCP
}

// DumpSockets returns all tracked socket entries for debugging
func (t *FDTracker) DumpSockets() []FDSocketEntry {
	t.mu.RLock()
	defer t.mu.RUnlock()

	entries := make([]FDSocketEntry, 0, len(t.sockets))
	for key, meta := range t.sockets {
		entries = append(entries, FDSocketEntry{
			PID:        key.PID,
			FD:         key.FD,
			LocalAddr:  net.JoinHostPort(meta.LocalIP.String(), strconv.Itoa(int(meta.LocalPort))),
			RemoteAddr: net.JoinHostPort(meta.RemoteIP.String(), strconv.Itoa(int(meta.RemotePort))),
			Protocol:   meta.Protocol.String(),
			Direction:  meta.Direction,
			State:      meta.State,
			Inode:      meta.Inode,
			FirstSeen:  meta.FirstSeen,
			LastSeen:   meta.LastSeen,
		})
	}

	return entries
}

// FDSocketEntry is a JSON-serializable representation of a tracked FD
type FDSocketEntry struct {
	PID        uint32    `json:"pid"`
	FD         int32     `json:"fd"`
	LocalAddr  string    `json:"local_addr"`
	RemoteAddr string    `json:"remote_addr"`
	Protocol   string    `json:"protocol"`
	Direction  string    `json:"direction"`
	State      uint8     `json:"state"`
	Inode      uint64    `json:"inode"`
	FirstSeen  time.Time `json:"first_seen"`
	LastSeen   time.Time `json:"last_seen"`
}
