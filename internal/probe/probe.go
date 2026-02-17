package probe

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"net"

	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
)

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -cflags "-O2 -g -Wall -Werror" -target amd64 bpf ../../bpf/tcpconnect.c -- -I../../bpf
//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -cflags "-O2 -g -Wall -Werror" -target arm64 bpf ../../bpf/tcpconnect.c -- -I../../bpf

// Address family constants matching the kernel definitions
const (
	afINET  = 2
	afINET6 = 10
)

// Event represents a TCP connection event from the kernel
type Event struct {
	PID    uint32
	UID    uint32
	SAddr  net.IP
	DAddr  net.IP
	SPort  uint16
	DPort  uint16
	TsNs   uint64
	Family uint16 // afINET (2) or afINET6 (10)
	Comm   string
}

// bpfEvent matches the C struct layout
// Note: Field order must match C struct for correct alignment
type bpfEvent struct {
	TsNs    uint64
	Pid     uint32
	Uid     uint32
	SaddrV6 [16]byte
	DaddrV6 [16]byte
	Sport   uint16
	Dport   uint16
	Family  uint16
	Pad     [2]byte
	Comm    [16]byte
}

// Probe manages the eBPF probe lifecycle
type Probe struct {
	objs       bpfObjects
	links      []link.Link
	reader     *ringbuf.Reader
	eventsChan chan Event
	fdTracker  *FDTracker
}

// New creates and loads the eBPF probe
func New() (*Probe, error) {
	// Load pre-compiled eBPF programs
	objs := bpfObjects{}
	if err := loadBpfObjects(&objs, nil); err != nil {
		return nil, fmt.Errorf("loading BPF objects: %w", err)
	}

	// Create FD tracker for mapping FDs to socket metadata
	fdTracker := NewFDTracker(DefaultFDTrackerConfig())
	fdTracker.Start()

	p := &Probe{
		objs:       objs,
		links:      make([]link.Link, 0, 3),
		eventsChan: make(chan Event, 100),
		fdTracker:  fdTracker,
	}

	// Attach kprobe to tcp_connect
	kp, err := link.Kprobe("tcp_connect", objs.KprobeTcpConnect, nil)
	if err != nil {
		p.Close()
		return nil, fmt.Errorf("attaching kprobe tcp_connect: %w", err)
	}
	p.links = append(p.links, kp)

	// Attach kretprobe to tcp_connect
	krp, err := link.Kretprobe("tcp_connect", objs.KretprobeTcpConnect, nil)
	if err != nil {
		p.Close()
		return nil, fmt.Errorf("attaching kretprobe tcp_connect: %w", err)
	}
	p.links = append(p.links, krp)

	// Attach kretprobe to inet_csk_accept
	krpAccept, err := link.Kretprobe("inet_csk_accept", objs.KretprobeInetCskAccept, nil)
	if err != nil {
		p.Close()
		return nil, fmt.Errorf("attaching kretprobe inet_csk_accept: %w", err)
	}
	p.links = append(p.links, krpAccept)

	// Open ring buffer reader
	rd, err := ringbuf.NewReader(objs.Events)
	if err != nil {
		p.Close()
		return nil, fmt.Errorf("opening ringbuf reader: %w", err)
	}
	p.reader = rd

	return p, nil
}

// Events returns a channel that receives TCP connection events
func (p *Probe) Events() <-chan Event {
	return p.eventsChan
}

// FDTracker returns the FD-to-socket tracker.
// Other probes (HTTP, DB) can use this to determine if an FD is a network
// socket before parsing payload data, avoiding wasted CPU on non-network FDs.
func (p *Probe) FDTracker() *FDTracker {
	return p.fdTracker
}

// Run starts reading events from the ring buffer
// Call this in a goroutine
func (p *Probe) Run() error {
	for {
		record, err := p.reader.Read()
		if err != nil {
			if errors.Is(err, ringbuf.ErrClosed) {
				return nil
			}
			return fmt.Errorf("reading from ringbuf: %w", err)
		}

		var raw bpfEvent
		if err := binary.Read(bytes.NewReader(record.RawSample), binary.LittleEndian, &raw); err != nil {
			continue
		}

		event := Event{
			PID:    raw.Pid,
			UID:    raw.Uid,
			SAddr:  rawAddrToIP(raw.Family, raw.SaddrV6),
			DAddr:  rawAddrToIP(raw.Family, raw.DaddrV6),
			SPort:  raw.Sport,
			DPort:  raw.Dport,
			TsNs:   raw.TsNs,
			Family: raw.Family,
			Comm:   nullTerminatedString(raw.Comm[:]),
		}

		// Feed event to FD tracker for FD-to-socket mapping.
		// The BPF code uses different addr layouts for connect vs accept:
		//   connect: SAddr=local, DAddr=remote, Sport=local, Dport=remote
		//   accept:  SAddr=remote, DAddr=local, Sport=remote, Dport=local
		// We track both directions so the tracker can resolve FDs either way.
		if p.fdTracker != nil {
			// Try outbound (connect) interpretation
			go p.fdTracker.TrackConnect(event.PID,
				event.SAddr, event.SPort,
				event.DAddr, event.DPort,
				event.Family)
			// Try inbound (accept) interpretation
			go p.fdTracker.TrackAccept(event.PID,
				event.DAddr, event.DPort,
				event.SAddr, event.SPort,
				event.Family)
		}

		select {
		case p.eventsChan <- event:
		default:
			// Drop event if channel is full
		}
	}
}

// Close cleans up all resources
func (p *Probe) Close() error {
	if p.fdTracker != nil {
		p.fdTracker.Stop()
	}
	if p.reader != nil {
		p.reader.Close()
	}
	for _, l := range p.links {
		l.Close()
	}
	p.objs.Close()
	close(p.eventsChan)
	return nil
}

// rawAddrToIP converts the raw 16-byte address from BPF into a net.IP.
// For AF_INET, only the first 4 bytes are used (little-endian u32).
// For AF_INET6, all 16 bytes are used (network byte order).
func rawAddrToIP(family uint16, addr [16]byte) net.IP {
	if family == afINET6 {
		ip := make(net.IP, 16)
		copy(ip, addr[:])
		return ip
	}
	// AF_INET: the 4-byte IPv4 address is stored little-endian in addr[0..3]
	ip := make(net.IP, 4)
	copy(ip, addr[:4])
	return ip
}

// intToIP converts a uint32 IPv4 address (little-endian) to net.IP.
// Kept for backward compatibility with other probe code.
func intToIP(ip uint32) net.IP {
	result := make(net.IP, 4)
	binary.LittleEndian.PutUint32(result, ip)
	return result
}

func nullTerminatedString(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}
