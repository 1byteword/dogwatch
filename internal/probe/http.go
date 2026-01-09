package probe

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
)

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -cflags "-O2 -g -Wall" -target amd64 http ../../bpf/http.c -- -I../../bpf

const (
	EventTypeRequest  = 1
	EventTypeResponse = 2
	MaxPayloadSize    = 256
)

// HTTPEvent represents a parsed HTTP request or response
type HTTPEvent struct {
	Timestamp   time.Time
	PID         uint32
	TID         uint32
	UID         uint32
	Comm        string
	EventType   string // "request" or "response"
	Method      string // GET, POST, etc.
	Path        string // /api/users
	StatusCode  int    // 200, 404, etc.
	Protocol    string // HTTP/1.1
	RawPayload  string // First line
	PayloadSize int
}

// rawHTTPEvent matches the C struct layout
type rawHTTPEvent struct {
	TsNs        uint64
	SockCookie  uint64
	PID         uint32
	TID         uint32
	UID         uint32
	SAddr       uint32
	DAddr       uint32
	SPort       uint16
	DPort       uint16
	PayloadSize uint32
	EventType   uint8
	Pad         [3]byte
	Comm        [16]byte
	Payload     [MaxPayloadSize]byte
}

// HTTPProbe manages HTTP eBPF probes
type HTTPProbe struct {
	objs       httpObjects
	links      []link.Link
	reader     *ringbuf.Reader
	eventsChan chan HTTPEvent

	// Request tracking for latency calculation
	requests   map[string]time.Time // key: pid:tid
	requestsMu sync.RWMutex
}

// NewHTTPProbe creates and loads the HTTP eBPF probe
func NewHTTPProbe() (*HTTPProbe, error) {
	objs := httpObjects{}
	if err := loadHttpObjects(&objs, nil); err != nil {
		return nil, fmt.Errorf("loading HTTP BPF objects: %w", err)
	}

	p := &HTTPProbe{
		objs:       objs,
		links:      make([]link.Link, 0, 4),
		eventsChan: make(chan HTTPEvent, 100),
		requests:   make(map[string]time.Time),
	}

	// Attach tracepoint for sys_enter_write
	tpWrite, err := link.Tracepoint("syscalls", "sys_enter_write", objs.TraceWriteEntry, nil)
	if err != nil {
		p.Close()
		return nil, fmt.Errorf("attaching tracepoint sys_enter_write: %w", err)
	}
	p.links = append(p.links, tpWrite)

	// Attach tracepoint for sys_enter_read
	tpReadEnter, err := link.Tracepoint("syscalls", "sys_enter_read", objs.TraceReadEntry, nil)
	if err != nil {
		p.Close()
		return nil, fmt.Errorf("attaching tracepoint sys_enter_read: %w", err)
	}
	p.links = append(p.links, tpReadEnter)

	// Attach tracepoint for sys_exit_read
	tpReadExit, err := link.Tracepoint("syscalls", "sys_exit_read", objs.TraceReadExit, nil)
	if err != nil {
		p.Close()
		return nil, fmt.Errorf("attaching tracepoint sys_exit_read: %w", err)
	}
	p.links = append(p.links, tpReadExit)

	// Attach tracepoint for sys_enter_sendto
	tpSendto, err := link.Tracepoint("syscalls", "sys_enter_sendto", objs.TraceSendtoEntry, nil)
	if err != nil {
		p.Close()
		return nil, fmt.Errorf("attaching tracepoint sys_enter_sendto: %w", err)
	}
	p.links = append(p.links, tpSendto)

	// Attach tracepoint for sys_enter_recvfrom
	tpRecvfromEnter, err := link.Tracepoint("syscalls", "sys_enter_recvfrom", objs.TraceRecvfromEntry, nil)
	if err != nil {
		p.Close()
		return nil, fmt.Errorf("attaching tracepoint sys_enter_recvfrom: %w", err)
	}
	p.links = append(p.links, tpRecvfromEnter)

	// Attach tracepoint for sys_exit_recvfrom
	tpRecvfromExit, err := link.Tracepoint("syscalls", "sys_exit_recvfrom", objs.TraceRecvfromExit, nil)
	if err != nil {
		p.Close()
		return nil, fmt.Errorf("attaching tracepoint sys_exit_recvfrom: %w", err)
	}
	p.links = append(p.links, tpRecvfromExit)

	// Open ring buffer reader
	rd, err := ringbuf.NewReader(objs.HttpEvents)
	if err != nil {
		p.Close()
		return nil, fmt.Errorf("opening HTTP ringbuf reader: %w", err)
	}
	p.reader = rd

	return p, nil
}

// Events returns a channel that receives HTTP events
func (p *HTTPProbe) Events() <-chan HTTPEvent {
	return p.eventsChan
}

// Run starts reading events from the ring buffer
func (p *HTTPProbe) Run() error {
	for {
		record, err := p.reader.Read()
		if err != nil {
			if errors.Is(err, ringbuf.ErrClosed) {
				return nil
			}
			return fmt.Errorf("reading from HTTP ringbuf: %w", err)
		}

		var raw rawHTTPEvent
		if err := binary.Read(bytes.NewReader(record.RawSample), binary.LittleEndian, &raw); err != nil {
			continue
		}

		event := p.parseEvent(raw)
		if event != nil {
			select {
			case p.eventsChan <- *event:
			default:
				// Drop if channel full
			}
		}
	}
}

func (p *HTTPProbe) parseEvent(raw rawHTTPEvent) *HTTPEvent {
	payload := nullTerminatedString(raw.Payload[:])
	if len(payload) == 0 {
		return nil
	}

	// Get first line
	firstLine := strings.Split(payload, "\r\n")[0]
	if len(firstLine) == 0 {
		firstLine = strings.Split(payload, "\n")[0]
	}

	event := &HTTPEvent{
		Timestamp:   time.Now(), // Using wall clock since ktime is monotonic
		PID:         raw.PID,
		TID:         raw.TID,
		UID:         raw.UID,
		Comm:        nullTerminatedString(raw.Comm[:]),
		RawPayload:  firstLine,
		PayloadSize: int(raw.PayloadSize),
	}

	if raw.EventType == EventTypeRequest {
		event.EventType = "request"
		p.parseRequest(event, firstLine)
	} else {
		event.EventType = "response"
		p.parseResponse(event, firstLine)
	}

	return event
}

var requestRegex = regexp.MustCompile(`^(GET|POST|PUT|DELETE|PATCH|HEAD|OPTIONS)\s+(\S+)\s+(HTTP/\d\.\d)`)
var responseRegex = regexp.MustCompile(`^(HTTP/\d\.\d)\s+(\d{3})`)

func (p *HTTPProbe) parseRequest(event *HTTPEvent, line string) {
	matches := requestRegex.FindStringSubmatch(line)
	if len(matches) >= 4 {
		event.Method = matches[1]
		event.Path = matches[2]
		event.Protocol = matches[3]
	}
}

func (p *HTTPProbe) parseResponse(event *HTTPEvent, line string) {
	matches := responseRegex.FindStringSubmatch(line)
	if len(matches) >= 3 {
		event.Protocol = matches[1]
		event.StatusCode, _ = strconv.Atoi(matches[2])
	}
}

// Close cleans up resources
func (p *HTTPProbe) Close() error {
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
