/*
Package probe - SSL/HTTPS Probe (DISABLED FOR MVP)

STATUS: NOT WORKING - Code kept for future reference and debugging

This probe attempts to intercept HTTPS traffic by attaching uprobes to OpenSSL
functions. While the code compiles and loads successfully, the uprobes never fire.

DEBUGGING ATTEMPTS:
1. Verified library path detection - found that symlinks (/lib -> /usr/lib) have
   different inodes. Fixed by NOT resolving symlinks.

2. Created test program (cmd/testup) to verify symbol resolution works:
   - SSL_write: offset 0x2b4d0
   - SSL_read: offset 0x2b050
   - All symbols resolve correctly

3. Verified with bpftrace that the functions CAN be traced:
   bpftrace -e 'uprobe:/lib/x86_64-linux-gnu/libssl.so.3:SSL_write { printf("hit\n"); }'
   This WORKS - proves the functions are called and traceable.

4. Added debug output in BPF code to emit events regardless of HTTP detection.
   Still no events received.

5. Checked kernel logs (dmesg) for BPF errors - none found.

CONCLUSION:
The issue appears to be in how cilium/ebpf attaches uprobes vs how bpftrace does.
Possible differences in:
- Perf event vs link-based attachment
- Address calculation with PIE/ASLR
- Cookie/reference tracking

For MVP, this probe is disabled. Plain HTTP tracing via syscall tracepoints works.

FUTURE WORK:
- Try perf_event_open based uprobe attachment
- Investigate cilium/ebpf source for uprobe handling
- Consider using bpftrace as a subprocess for SSL interception
*/
package probe

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
)

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -cflags "-O2 -g -Wall" -target amd64 ssl ../../bpf/ssl.c -- -I../../bpf

const (
	SSLEventTypeRequest  = 1
	SSLEventTypeResponse = 2
	SSLMaxPayloadSize    = 256
)

// SSLEvent represents a parsed SSL/HTTPS event
type SSLEvent struct {
	Timestamp  time.Time
	PID        uint32
	TID        uint32
	UID        uint32
	Comm       string
	EventType  string // "request" or "response"
	Method     string
	Path       string
	StatusCode int
	Protocol   string
	RawPayload string
	Len        int
}

// rawSSLEvent matches the C struct layout
type rawSSLEvent struct {
	TsNs      uint64
	PID       uint32
	TID       uint32
	UID       uint32
	Len       uint32
	EventType uint8
	Pad       [3]byte
	Comm      [16]byte
	Payload   [SSLMaxPayloadSize]byte
}

// SSLProbe manages SSL/HTTPS eBPF probes
type SSLProbe struct {
	objs       sslObjects
	links      []link.Link
	reader     *ringbuf.Reader
	eventsChan chan HTTPEvent // Reuse HTTPEvent type
	sslLibPath string
}

// Common OpenSSL library paths (order matters - check /lib first as that's where apps link)
var sslLibPaths = []string{
	"/lib/x86_64-linux-gnu/libssl.so.3",
	"/lib/x86_64-linux-gnu/libssl.so.1.1",
	"/lib64/libssl.so.3",
	"/lib64/libssl.so.1.1",
	"/usr/lib/x86_64-linux-gnu/libssl.so.3",
	"/usr/lib/x86_64-linux-gnu/libssl.so.1.1",
	"/usr/lib/x86_64-linux-gnu/libssl.so",
	"/usr/lib/libssl.so.3",
	"/usr/lib/libssl.so.1.1",
	"/usr/lib/libssl.so",
}

// findSSLLibrary finds the OpenSSL library on the system
func findSSLLibrary() (string, error) {
	// Try common paths first
	// IMPORTANT: Don't resolve symlinks - use the exact path that processes load
	// Different paths may have different inodes, and uprobes need the exact inode match
	for _, path := range sslLibPaths {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, nil
		}
	}

	// Try to find via ldconfig
	// This is a fallback - the common paths should work for most systems
	return "", fmt.Errorf("OpenSSL library not found. Tried: %v", sslLibPaths)
}

// NewSSLProbe creates and loads the SSL eBPF probe
func NewSSLProbe() (*SSLProbe, error) {
	// Find OpenSSL library
	sslPath, err := findSSLLibrary()
	if err != nil {
		return nil, fmt.Errorf("finding OpenSSL: %w", err)
	}

	// Load BPF objects
	objs := sslObjects{}
	if err := loadSslObjects(&objs, nil); err != nil {
		return nil, fmt.Errorf("loading SSL BPF objects: %w", err)
	}

	p := &SSLProbe{
		objs:       objs,
		links:      make([]link.Link, 0, 4),
		eventsChan: make(chan HTTPEvent, 100),
		sslLibPath: sslPath,
	}

	// Open the executable for uprobes
	ex, err := link.OpenExecutable(sslPath)
	if err != nil {
		p.Close()
		return nil, fmt.Errorf("opening %s: %w", sslPath, err)
	}

	// Try using TraceFSUprobe for attachment - this method works with bpftrace
	// and avoids the BPF link attachment issues on some kernel/library combinations
	fmt.Printf("  Using library: %s\n", sslPath)

	// Attach uprobe to SSL_write using explicit options
	upWrite, err := ex.Uprobe("SSL_write", objs.UprobeSslWrite, &link.UprobeOptions{
		// PID 0 means all processes
		PID: 0,
	})
	if err != nil {
		fmt.Printf("  Warning: SSL_write uprobe failed: %v\n", err)
	} else {
		p.links = append(p.links, upWrite)
		fmt.Println("  Attached: SSL_write")
	}

	// Attach uprobe to SSL_write_ex (OpenSSL 3.x)
	upWriteEx, err := ex.Uprobe("SSL_write_ex", objs.UprobeSslWriteEx, &link.UprobeOptions{
		PID: 0,
	})
	if err != nil {
		fmt.Printf("  Warning: SSL_write_ex uprobe failed: %v\n", err)
	} else {
		p.links = append(p.links, upWriteEx)
		fmt.Println("  Attached: SSL_write_ex")
	}

	// Attach uprobe to SSL_read (entry)
	upReadEntry, err := ex.Uprobe("SSL_read", objs.UprobeSslReadEntry, &link.UprobeOptions{
		PID: 0,
	})
	if err != nil {
		fmt.Printf("  Warning: SSL_read entry uprobe failed: %v\n", err)
	} else {
		p.links = append(p.links, upReadEntry)
		fmt.Println("  Attached: SSL_read (entry)")
	}

	// Attach uretprobe to SSL_read (return)
	upReadRet, err := ex.Uretprobe("SSL_read", objs.UretprobeSslRead, &link.UprobeOptions{
		PID: 0,
	})
	if err != nil {
		fmt.Printf("  Warning: SSL_read return uretprobe failed: %v\n", err)
	} else {
		p.links = append(p.links, upReadRet)
		fmt.Println("  Attached: SSL_read (return)")
	}

	// Attach uprobe to SSL_read_ex (entry) - OpenSSL 3.x
	upReadExEntry, err := ex.Uprobe("SSL_read_ex", objs.UprobeSslReadExEntry, &link.UprobeOptions{
		PID: 0,
	})
	if err != nil {
		fmt.Printf("  Warning: SSL_read_ex entry uprobe failed: %v\n", err)
	} else {
		p.links = append(p.links, upReadExEntry)
		fmt.Println("  Attached: SSL_read_ex (entry)")
	}

	// Attach uretprobe to SSL_read_ex (return) - OpenSSL 3.x
	upReadExRet, err := ex.Uretprobe("SSL_read_ex", objs.UretprobeSslReadEx, &link.UprobeOptions{
		PID: 0,
	})
	if err != nil {
		fmt.Printf("  Warning: SSL_read_ex return uretprobe failed: %v\n", err)
	} else {
		p.links = append(p.links, upReadExRet)
		fmt.Println("  Attached: SSL_read_ex (return)")
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
func (p *SSLProbe) SSLLibPath() string {
	return p.sslLibPath
}

// Events returns a channel that receives HTTPS events
func (p *SSLProbe) Events() <-chan HTTPEvent {
	return p.eventsChan
}

// Run starts reading events from the ring buffer
func (p *SSLProbe) Run() error {
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

var sslRequestRegex = regexp.MustCompile(`^(GET|POST|PUT|DELETE|PATCH|HEAD|OPTIONS)\s+(\S+)\s+(HTTP/\d\.\d)`)
var sslResponseRegex = regexp.MustCompile(`^(HTTP/\d\.\d)\s+(\d{3})`)

func (p *SSLProbe) parseEvent(raw rawSSLEvent) *HTTPEvent {
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
		matches := sslRequestRegex.FindStringSubmatch(firstLine)
		if len(matches) >= 4 {
			event.Method = matches[1]
			event.Path = matches[2]
			event.Protocol = matches[3]
		}
	} else {
		event.EventType = "response"
		matches := sslResponseRegex.FindStringSubmatch(firstLine)
		if len(matches) >= 3 {
			event.Protocol = matches[1]
			event.StatusCode, _ = strconv.Atoi(matches[2])
		}
	}

	return event
}

// Close cleans up resources
func (p *SSLProbe) Close() error {
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
