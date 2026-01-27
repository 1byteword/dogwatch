package probe

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/cilium/ebpf/ringbuf"
)

// SSLProbeV2 uses tracefs-based uprobe registration for better compatibility
type SSLProbeV2 struct {
	objs        sslObjects
	reader      *ringbuf.Reader
	eventsChan  chan HTTPEvent
	sslLibPath  string
	attachments []sslAttachment
}

type sslAttachment struct {
	name   string
	perfFD int
}

// NewSSLProbeV2 creates an SSL probe using tracefs uprobes
func NewSSLProbeV2() (*SSLProbeV2, error) {
	// Find OpenSSL library
	sslPath, err := findSSLLibrary()
	if err != nil {
		return nil, fmt.Errorf("finding OpenSSL: %w", err)
	}

	// Get symbol offsets
	offsets, err := GetSSLSymbolOffsets(sslPath)
	if err != nil {
		return nil, fmt.Errorf("getting symbol offsets: %w", err)
	}

	// Load BPF objects
	objs := sslObjects{}
	if err := loadSslObjects(&objs, nil); err != nil {
		return nil, fmt.Errorf("loading SSL BPF objects: %w", err)
	}

	p := &SSLProbeV2{
		objs:        objs,
		eventsChan:  make(chan HTTPEvent, 100),
		sslLibPath:  sslPath,
		attachments: make([]sslAttachment, 0),
	}

	fmt.Printf("  Using library: %s\n", sslPath)
	fmt.Printf("  SSL_write offset: 0x%x\n", offsets.SSLWrite)
	fmt.Printf("  SSL_read offset: 0x%x\n", offsets.SSLRead)

	// Define probes to attach
	probes := []struct {
		name     string
		offset   uint64
		isReturn bool
	}{
		{"dw_ssl_write", offsets.SSLWrite, false},
		{"dw_ssl_read_entry", offsets.SSLRead, false},
		{"dw_ssl_read_ret", offsets.SSLRead, true},
	}

	// Add SSL_write_ex and SSL_read_ex if available
	if offsets.SSLWriteEx != 0 {
		probes = append(probes, struct {
			name     string
			offset   uint64
			isReturn bool
		}{"dw_ssl_write_ex", offsets.SSLWriteEx, false})
	}
	if offsets.SSLReadEx != 0 {
		probes = append(probes,
			struct {
				name     string
				offset   uint64
				isReturn bool
			}{"dw_ssl_read_ex_entry", offsets.SSLReadEx, false},
			struct {
				name     string
				offset   uint64
				isReturn bool
			}{"dw_ssl_read_ex_ret", offsets.SSLReadEx, true},
		)
	}

	// Attach each probe via tracefs
	attached := 0
	for _, probe := range probes {
		// Register the uprobe via tracefs
		eventID, err := RegisterTraceFSProbe(probe.name, sslPath, probe.offset, probe.isReturn)
		if err != nil {
			fmt.Printf("  Warning: failed to register %s: %v\n", probe.name, err)
			continue
		}

		// Get the BPF program FD based on probe name
		var progFD int
		switch probe.name {
		case "dw_ssl_write":
			progFD = objs.UprobeSslWrite.FD()
		case "dw_ssl_write_ex":
			progFD = objs.UprobeSslWriteEx.FD()
		case "dw_ssl_read_entry":
			progFD = objs.UprobeSslReadEntry.FD()
		case "dw_ssl_read_ret":
			progFD = objs.UretprobeSslRead.FD()
		case "dw_ssl_read_ex_entry":
			progFD = objs.UprobeSslReadExEntry.FD()
		case "dw_ssl_read_ex_ret":
			progFD = objs.UretprobeSslReadEx.FD()
		}

		// Attach BPF program to the tracepoint
		perfFD, err := AttachBPFToTracepoint(eventID, progFD)
		if err != nil {
			fmt.Printf("  Warning: failed to attach BPF to %s: %v\n", probe.name, err)
			CleanupTraceFSProbe(probe.name)
			continue
		}

		p.attachments = append(p.attachments, sslAttachment{
			name:   probe.name,
			perfFD: perfFD,
		})

		probeType := "uprobe"
		if probe.isReturn {
			probeType = "uretprobe"
		}
		fmt.Printf("  Attached: %s (%s at 0x%x)\n", probe.name, probeType, probe.offset)
		attached++
	}

	if attached == 0 {
		p.Close()
		return nil, fmt.Errorf("no probes attached successfully")
	}

	// Open ring buffer reader
	rd, err := ringbuf.NewReader(objs.SslEvents)
	if err != nil {
		p.Close()
		return nil, fmt.Errorf("opening SSL ringbuf reader: %w", err)
	}
	p.reader = rd

	return p, nil
}

// SSLLibPath returns the path to the OpenSSL library being traced
func (p *SSLProbeV2) SSLLibPath() string {
	return p.sslLibPath
}

// Events returns a channel that receives HTTPS events
func (p *SSLProbeV2) Events() <-chan HTTPEvent {
	return p.eventsChan
}

// Run starts reading events from the ring buffer
func (p *SSLProbeV2) Run() error {
	for {
		record, err := p.reader.Read()
		if err != nil {
			if errors.Is(err, ringbuf.ErrClosed) {
				return nil
			}
			return fmt.Errorf("reading from SSL ringbuf: %w", err)
		}

		var raw rawSSLEvent
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

var sslRequestRegexV2 = regexp.MustCompile(`^(GET|POST|PUT|DELETE|PATCH|HEAD|OPTIONS)\s+(\S+)\s+(HTTP/\d\.\d)`)
var sslResponseRegexV2 = regexp.MustCompile(`^(HTTP/\d\.\d)\s+(\d{3})`)

func (p *SSLProbeV2) parseEvent(raw rawSSLEvent) *HTTPEvent {
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
		Timestamp:   time.Now(),
		PID:         raw.PID,
		TID:         raw.TID,
		UID:         raw.UID,
		Comm:        nullTerminatedString(raw.Comm[:]),
		RawPayload:  firstLine,
		PayloadSize: int(raw.Len),
	}

	if raw.EventType == SSLEventTypeRequest {
		event.EventType = "request"
		matches := sslRequestRegexV2.FindStringSubmatch(firstLine)
		if len(matches) >= 4 {
			event.Method = matches[1]
			event.Path = matches[2]
			event.Protocol = matches[3]
		}
	} else {
		event.EventType = "response"
		matches := sslResponseRegexV2.FindStringSubmatch(firstLine)
		if len(matches) >= 3 {
			event.Protocol = matches[1]
			event.StatusCode, _ = strconv.Atoi(matches[2])
		}
	}

	return event
}

// Close cleans up resources
func (p *SSLProbeV2) Close() error {
	if p.reader != nil {
		p.reader.Close()
	}

	// Close perf FDs and cleanup tracefs probes
	for _, att := range p.attachments {
		if att.perfFD > 0 {
			syscall.Close(att.perfFD)
		}
		CleanupTraceFSProbe(att.name)
	}

	// Clean up additional perf FDs from multi-CPU attachment
	CleanupAdditionalPerfFDs()

	p.objs.Close()
	close(p.eventsChan)
	return nil
}
