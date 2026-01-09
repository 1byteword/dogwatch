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

// Event represents a TCP connection event from the kernel
type Event struct {
	PID   uint32
	UID   uint32
	SAddr net.IP
	DAddr net.IP
	SPort uint16
	DPort uint16
	TsNs  uint64
	Comm  string
}

// bpfEvent matches the C struct layout
// Note: Field order must match C struct for correct alignment
type bpfEvent struct {
	TsNs  uint64
	Pid   uint32
	Uid   uint32
	Saddr uint32
	Daddr uint32
	Sport uint16
	Dport uint16
	Comm  [16]byte
}

// Probe manages the eBPF probe lifecycle
type Probe struct {
	objs       bpfObjects
	links      []link.Link
	reader     *ringbuf.Reader
	eventsChan chan Event
}

// New creates and loads the eBPF probe
func New() (*Probe, error) {
	// Load pre-compiled eBPF programs
	objs := bpfObjects{}
	if err := loadBpfObjects(&objs, nil); err != nil {
		return nil, fmt.Errorf("loading BPF objects: %w", err)
	}

	p := &Probe{
		objs:       objs,
		links:      make([]link.Link, 0, 3),
		eventsChan: make(chan Event, 100),
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
			PID:   raw.Pid,
			UID:   raw.Uid,
			SAddr: intToIP(raw.Saddr),
			DAddr: intToIP(raw.Daddr),
			SPort: raw.Sport,
			DPort: raw.Dport,
			TsNs:  raw.TsNs,
			Comm:  nullTerminatedString(raw.Comm[:]),
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
