package ingest

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"
)

// Listener handles incoming metric data over TCP/UDP
type Listener struct {
	store    MetricStore
	addr     string
	protocol string // tcp, udp

	graphite *GraphiteParser
	statsd   *StatsDParser
	opentsdb *OpenTSDBParser

	tcpListener net.Listener
	udpConn     *net.UDPConn
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

// NewListener creates a new TCP/UDP listener
func NewListener(store MetricStore, addr, protocol string) *Listener {
	ctx, cancel := context.WithCancel(context.Background())
	return &Listener{
		store:    store,
		addr:     addr,
		protocol: protocol,
		graphite: &GraphiteParser{},
		statsd:   NewStatsDParser(),
		opentsdb: &OpenTSDBParser{},
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Start begins listening for connections
func (l *Listener) Start() error {
	switch l.protocol {
	case "tcp":
		return l.startTCP()
	case "udp":
		return l.startUDP()
	default:
		return fmt.Errorf("unsupported protocol: %s", l.protocol)
	}
}

// Stop gracefully shuts down the listener
func (l *Listener) Stop() error {
	l.cancel()

	if l.tcpListener != nil {
		l.tcpListener.Close()
	}
	if l.udpConn != nil {
		l.udpConn.Close()
	}

	l.wg.Wait()
	return nil
}

func (l *Listener) startTCP() error {
	listener, err := net.Listen("tcp", l.addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", l.addr, err)
	}
	l.tcpListener = listener

	l.wg.Add(1)
	go func() {
		defer l.wg.Done()
		l.acceptLoop()
	}()

	return nil
}

func (l *Listener) acceptLoop() {
	for {
		conn, err := l.tcpListener.Accept()
		if err != nil {
			select {
			case <-l.ctx.Done():
				return
			default:
				log.Printf("Accept error: %v", err)
				continue
			}
		}

		l.wg.Add(1)
		go func(c net.Conn) {
			defer l.wg.Done()
			defer c.Close()
			l.handleTCPConnection(c)
		}(conn)
	}
}

func (l *Listener) handleTCPConnection(conn net.Conn) {
	conn.SetReadDeadline(time.Now().Add(30 * time.Second))

	// Read first bytes to detect protocol
	reader := bufio.NewReader(conn)

	for {
		select {
		case <-l.ctx.Done():
			return
		default:
		}

		conn.SetReadDeadline(time.Now().Add(30 * time.Second))

		// Peek to detect protocol type
		peek, err := reader.Peek(4)
		if err != nil {
			if err != io.EOF {
				log.Printf("Peek error: %v", err)
			}
			return
		}

		var batch *Batch

		// Detect protocol by content
		if isPickleHeader(peek) {
			// Graphite pickle format
			batch, err = l.handleGraphitePickle(reader)
		} else if isPutCommand(peek) {
			// OpenTSDB telnet format
			batch, err = l.handleOpenTSDBTelnet(reader)
		} else {
			// Try Graphite plaintext (most common)
			batch, err = l.handleGraphitePlaintext(reader)
		}

		if err != nil {
			log.Printf("Parse error: %v", err)
			return
		}

		if batch != nil && len(batch.Samples) > 0 {
			if err := l.store.WriteSamples(batch.Samples); err != nil {
				log.Printf("Store error: %v", err)
			}
		}
	}
}

func isPickleHeader(b []byte) bool {
	// Pickle frames start with 4-byte length, usually large
	if len(b) < 4 {
		return false
	}
	length := binary.BigEndian.Uint32(b)
	return length > 0 && length < 10*1024*1024 // Reasonable pickle size
}

func isPutCommand(b []byte) bool {
	return len(b) >= 4 && string(b[:4]) == "put "
}

func (l *Listener) handleGraphitePlaintext(reader *bufio.Reader) (*Batch, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	return l.graphite.ParsePlaintext(bytes.NewBufferString(line))
}

func (l *Listener) handleGraphitePickle(reader *bufio.Reader) (*Batch, error) {
	return l.graphite.ParsePickle(reader)
}

func (l *Listener) handleOpenTSDBTelnet(reader *bufio.Reader) (*Batch, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	return l.opentsdb.ParseTelnet(bytes.NewBufferString(line))
}

func (l *Listener) startUDP() error {
	addr, err := net.ResolveUDPAddr("udp", l.addr)
	if err != nil {
		return fmt.Errorf("failed to resolve address: %w", err)
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}
	l.udpConn = conn

	// Set read buffer size for high throughput
	conn.SetReadBuffer(8 * 1024 * 1024) // 8MB buffer

	l.wg.Add(1)
	go func() {
		defer l.wg.Done()
		l.udpLoop()
	}()

	return nil
}

func (l *Listener) udpLoop() {
	buf := make([]byte, 65535) // Max UDP packet size

	for {
		select {
		case <-l.ctx.Done():
			return
		default:
		}

		l.udpConn.SetReadDeadline(time.Now().Add(1 * time.Second))
		n, _, err := l.udpConn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			select {
			case <-l.ctx.Done():
				return
			default:
				log.Printf("UDP read error: %v", err)
				continue
			}
		}

		if n == 0 {
			continue
		}

		// Parse StatsD (most common for UDP)
		batch, err := l.statsd.Parse(bytes.NewReader(buf[:n]))
		if err != nil {
			log.Printf("StatsD parse error: %v", err)
			continue
		}

		if len(batch.Samples) > 0 {
			if err := l.store.WriteSamples(batch.Samples); err != nil {
				log.Printf("Store error: %v", err)
			}
		}
	}
}

// ListenerManager manages multiple protocol listeners
type ListenerManager struct {
	store     MetricStore
	listeners map[string]*Listener
	mu        sync.Mutex
}

// NewListenerManager creates a new listener manager
func NewListenerManager(store MetricStore) *ListenerManager {
	return &ListenerManager{
		store:     store,
		listeners: make(map[string]*Listener),
	}
}

// StartGraphite starts Graphite listeners
func (m *ListenerManager) StartGraphite(tcpAddr, pickleAddr string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if tcpAddr != "" {
		l := NewListener(m.store, tcpAddr, "tcp")
		if err := l.Start(); err != nil {
			return fmt.Errorf("graphite tcp: %w", err)
		}
		m.listeners["graphite-tcp"] = l
		log.Printf("Graphite plaintext listening on %s", tcpAddr)
	}

	if pickleAddr != "" {
		l := NewListener(m.store, pickleAddr, "tcp")
		if err := l.Start(); err != nil {
			return fmt.Errorf("graphite pickle: %w", err)
		}
		m.listeners["graphite-pickle"] = l
		log.Printf("Graphite pickle listening on %s", pickleAddr)
	}

	return nil
}

// StartStatsD starts StatsD UDP listener
func (m *ListenerManager) StartStatsD(udpAddr string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	l := NewListener(m.store, udpAddr, "udp")
	if err := l.Start(); err != nil {
		return fmt.Errorf("statsd: %w", err)
	}
	m.listeners["statsd"] = l
	log.Printf("StatsD listening on %s (UDP)", udpAddr)

	return nil
}

// StartOpenTSDB starts OpenTSDB telnet listener
func (m *ListenerManager) StartOpenTSDB(tcpAddr string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	l := NewListener(m.store, tcpAddr, "tcp")
	if err := l.Start(); err != nil {
		return fmt.Errorf("opentsdb: %w", err)
	}
	m.listeners["opentsdb"] = l
	log.Printf("OpenTSDB telnet listening on %s", tcpAddr)

	return nil
}

// StopAll stops all listeners
func (m *ListenerManager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, l := range m.listeners {
		if err := l.Stop(); err != nil {
			log.Printf("Error stopping %s listener: %v", name, err)
		}
	}
	m.listeners = make(map[string]*Listener)
}

// Status returns the status of all listeners
func (m *ListenerManager) Status() map[string]string {
	m.mu.Lock()
	defer m.mu.Unlock()

	status := make(map[string]string)
	for name, l := range m.listeners {
		status[name] = fmt.Sprintf("listening on %s (%s)", l.addr, l.protocol)
	}
	return status
}
