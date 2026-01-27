package probe

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
)

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -cflags "-O2 -g -Wall" -target amd64 dbproto ../../bpf/dbproto.c -- -I../../bpf

const (
	DBTypeUnknown  = 0
	DBTypeMySQL    = 1
	DBTypePostgres = 2
	DBTypeRedis    = 3

	DBEventTypeQuery    = 1
	DBEventTypeResponse = 2

	MaxDBPayloadSize = 512
)

// DBEvent represents a parsed database operation
type DBEvent struct {
	Timestamp   time.Time
	PID         uint32
	TID         uint32
	UID         uint32
	Comm        string
	DBType      string // "mysql", "postgres", "redis"
	EventType   string // "query" or "response"
	Operation   string // SELECT, INSERT, GET, SET, etc.
	Query       string // The actual query/command
	Table       string // Table name if detected
	Key         string // Redis key if detected
	RowsAffected int   // For responses
	Error       string // Error message if any
	PayloadSize int
	Latency     time.Duration // Calculated latency
}

// rawDBEvent matches the C struct layout exactly
type rawDBEvent struct {
	TsNs        uint64
	PID         uint32
	TID         uint32
	UID         uint32
	PayloadSize uint32
	DBType      uint8
	EventType   uint8
	Pad         [2]byte
	Comm        [16]byte
	Payload     [MaxDBPayloadSize]byte
}

// DBProbe manages database protocol eBPF probes
type DBProbe struct {
	objs       dbprotoObjects
	links      []link.Link
	reader     *ringbuf.Reader
	eventsChan chan DBEvent

	// Query tracking for latency calculation
	queries   map[string]queryInfo // key: pid:tid
	queriesMu sync.RWMutex
}

type queryInfo struct {
	StartTime time.Time
	Query     string
	DBType    string
}

// NewDBProbe creates and loads the database protocol eBPF probe
func NewDBProbe() (*DBProbe, error) {
	objs := dbprotoObjects{}
	if err := loadDbprotoObjects(&objs, nil); err != nil {
		return nil, fmt.Errorf("loading DB BPF objects: %w", err)
	}

	p := &DBProbe{
		objs:       objs,
		links:      make([]link.Link, 0, 6),
		eventsChan: make(chan DBEvent, 200),
		queries:    make(map[string]queryInfo),
	}

	// Attach tracepoint for sys_enter_write
	tpWrite, err := link.Tracepoint("syscalls", "sys_enter_write", objs.TraceDbWriteEntry, nil)
	if err != nil {
		p.Close()
		return nil, fmt.Errorf("attaching tracepoint sys_enter_write: %w", err)
	}
	p.links = append(p.links, tpWrite)

	// Attach tracepoint for sys_enter_read
	tpReadEnter, err := link.Tracepoint("syscalls", "sys_enter_read", objs.TraceDbReadEntry, nil)
	if err != nil {
		p.Close()
		return nil, fmt.Errorf("attaching tracepoint sys_enter_read: %w", err)
	}
	p.links = append(p.links, tpReadEnter)

	// Attach tracepoint for sys_exit_read
	tpReadExit, err := link.Tracepoint("syscalls", "sys_exit_read", objs.TraceDbReadExit, nil)
	if err != nil {
		p.Close()
		return nil, fmt.Errorf("attaching tracepoint sys_exit_read: %w", err)
	}
	p.links = append(p.links, tpReadExit)

	// Attach tracepoint for sys_enter_sendto
	tpSendto, err := link.Tracepoint("syscalls", "sys_enter_sendto", objs.TraceDbSendtoEntry, nil)
	if err != nil {
		p.Close()
		return nil, fmt.Errorf("attaching tracepoint sys_enter_sendto: %w", err)
	}
	p.links = append(p.links, tpSendto)

	// Attach tracepoint for sys_enter_recvfrom
	tpRecvfromEnter, err := link.Tracepoint("syscalls", "sys_enter_recvfrom", objs.TraceDbRecvfromEntry, nil)
	if err != nil {
		p.Close()
		return nil, fmt.Errorf("attaching tracepoint sys_enter_recvfrom: %w", err)
	}
	p.links = append(p.links, tpRecvfromEnter)

	// Attach tracepoint for sys_exit_recvfrom
	tpRecvfromExit, err := link.Tracepoint("syscalls", "sys_exit_recvfrom", objs.TraceDbRecvfromExit, nil)
	if err != nil {
		p.Close()
		return nil, fmt.Errorf("attaching tracepoint sys_exit_recvfrom: %w", err)
	}
	p.links = append(p.links, tpRecvfromExit)

	// Open ring buffer reader
	rd, err := ringbuf.NewReader(objs.DbEvents)
	if err != nil {
		p.Close()
		return nil, fmt.Errorf("opening DB ringbuf reader: %w", err)
	}
	p.reader = rd

	return p, nil
}

// Events returns a channel that receives database events
func (p *DBProbe) Events() <-chan DBEvent {
	return p.eventsChan
}

// Run starts reading events from the ring buffer
func (p *DBProbe) Run() error {
	for {
		record, err := p.reader.Read()
		if err != nil {
			if errors.Is(err, ringbuf.ErrClosed) {
				return nil
			}
			return fmt.Errorf("reading from DB ringbuf: %w", err)
		}

		var raw rawDBEvent
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

func (p *DBProbe) parseEvent(raw rawDBEvent) *DBEvent {
	event := &DBEvent{
		Timestamp:   time.Now(),
		PID:         raw.PID,
		TID:         raw.TID,
		UID:         raw.UID,
		Comm:        nullTerminatedString(raw.Comm[:]),
		PayloadSize: int(raw.PayloadSize),
	}

	// Set DB type
	switch raw.DBType {
	case DBTypeMySQL:
		event.DBType = "mysql"
	case DBTypePostgres:
		event.DBType = "postgres"
	case DBTypeRedis:
		event.DBType = "redis"
	default:
		return nil
	}

	// Set event type and parse accordingly
	if raw.EventType == DBEventTypeQuery {
		event.EventType = "query"
		p.parseQuery(event, raw)

		// Track query for latency calculation
		key := fmt.Sprintf("%d:%d", raw.PID, raw.TID)
		p.queriesMu.Lock()
		p.queries[key] = queryInfo{
			StartTime: event.Timestamp,
			Query:     event.Query,
			DBType:    event.DBType,
		}
		p.queriesMu.Unlock()
	} else {
		event.EventType = "response"
		p.parseResponse(event, raw)

		// Calculate latency if we have matching query
		key := fmt.Sprintf("%d:%d", raw.PID, raw.TID)
		p.queriesMu.Lock()
		if qi, ok := p.queries[key]; ok {
			event.Latency = event.Timestamp.Sub(qi.StartTime)
			event.Query = qi.Query // Carry query info to response
			delete(p.queries, key)
		}
		p.queriesMu.Unlock()
	}

	return event
}

func (p *DBProbe) parseQuery(event *DBEvent, raw rawDBEvent) {
	payload := raw.Payload[:raw.PayloadSize]

	switch raw.DBType {
	case DBTypeMySQL:
		p.parseMySQLQuery(event, payload)
	case DBTypePostgres:
		p.parsePostgresQuery(event, payload)
	case DBTypeRedis:
		p.parseRedisCommand(event, payload)
	}
}

func (p *DBProbe) parseResponse(event *DBEvent, raw rawDBEvent) {
	payload := raw.Payload[:raw.PayloadSize]

	switch raw.DBType {
	case DBTypeMySQL:
		p.parseMySQLResponse(event, payload)
	case DBTypePostgres:
		p.parsePostgresResponse(event, payload)
	case DBTypeRedis:
		p.parseRedisResponse(event, payload)
	}
}

// MySQL wire protocol parsing
func (p *DBProbe) parseMySQLQuery(event *DBEvent, payload []byte) {
	if len(payload) < 5 {
		return
	}

	// Skip 4-byte header (3 length + 1 seq)
	cmd := payload[4]
	data := payload[5:]

	switch cmd {
	case 0x03: // COM_QUERY
		event.Operation = "QUERY"
		event.Query = sanitizeQuery(string(data))
		event.Table = extractTableName(event.Query)
	case 0x16: // COM_STMT_PREPARE
		event.Operation = "PREPARE"
		event.Query = sanitizeQuery(string(data))
	case 0x17: // COM_STMT_EXECUTE
		event.Operation = "EXECUTE"
	case 0x02: // COM_INIT_DB
		event.Operation = "USE"
		event.Query = string(data)
	}

	// Extract operation from query
	if event.Operation == "QUERY" && len(event.Query) > 0 {
		event.Operation = extractSQLOperation(event.Query)
	}
}

func (p *DBProbe) parseMySQLResponse(event *DBEvent, payload []byte) {
	if len(payload) < 5 {
		return
	}

	// Skip 4-byte header
	status := payload[4]

	switch status {
	case 0x00: // OK packet
		event.Operation = "OK"
		if len(payload) > 5 {
			// Affected rows is length-encoded int
			event.RowsAffected = int(payload[5])
		}
	case 0xff: // ERR packet
		event.Operation = "ERROR"
		if len(payload) > 7 {
			// Error code is 2 bytes, then message
			event.Error = sanitizeQuery(string(payload[9:]))
		}
	case 0xfe: // EOF packet
		event.Operation = "EOF"
	default:
		// Result set (first byte is field count)
		event.Operation = "RESULT"
	}
}

// PostgreSQL wire protocol parsing
func (p *DBProbe) parsePostgresQuery(event *DBEvent, payload []byte) {
	if len(payload) < 5 {
		return
	}

	msgType := payload[0]

	switch msgType {
	case 'Q': // Simple Query
		event.Operation = "QUERY"
		// Length is bytes 1-4 (big-endian), then query string
		if len(payload) > 5 {
			event.Query = sanitizeQuery(strings.TrimRight(string(payload[5:]), "\x00"))
			event.Table = extractTableName(event.Query)
			event.Operation = extractSQLOperation(event.Query)
		}
	case 'P': // Parse (prepared statement)
		event.Operation = "PREPARE"
		// Statement name (null-terminated), then query
		if idx := bytes.IndexByte(payload[5:], 0); idx >= 0 {
			queryStart := 5 + idx + 1
			if queryStart < len(payload) {
				event.Query = sanitizeQuery(strings.TrimRight(string(payload[queryStart:]), "\x00"))
			}
		}
	case 'E': // Execute
		event.Operation = "EXECUTE"
	case 'B': // Bind
		event.Operation = "BIND"
	}
}

func (p *DBProbe) parsePostgresResponse(event *DBEvent, payload []byte) {
	if len(payload) < 5 {
		return
	}

	msgType := payload[0]

	switch msgType {
	case 'T': // RowDescription
		event.Operation = "RESULT"
	case 'D': // DataRow
		event.Operation = "DATA"
	case 'C': // CommandComplete
		event.Operation = "COMPLETE"
		// The tag follows: "SELECT 1", "INSERT 0 1", etc.
		if len(payload) > 5 {
			tag := strings.TrimRight(string(payload[5:]), "\x00")
			parts := strings.Fields(tag)
			if len(parts) > 0 {
				event.Operation = parts[0]
			}
			// Try to extract rows affected
			if len(parts) >= 2 {
				if parts[0] == "INSERT" && len(parts) >= 3 {
					fmt.Sscanf(parts[2], "%d", &event.RowsAffected)
				} else if parts[0] == "UPDATE" || parts[0] == "DELETE" || parts[0] == "SELECT" {
					fmt.Sscanf(parts[1], "%d", &event.RowsAffected)
				}
			}
		}
	case 'E': // ErrorResponse
		event.Operation = "ERROR"
		// Parse error fields
		if len(payload) > 5 {
			event.Error = parsePostgresError(payload[5:])
		}
	case 'Z': // ReadyForQuery
		event.Operation = "READY"
	case '1': // ParseComplete
		event.Operation = "PARSE_OK"
	case '2': // BindComplete
		event.Operation = "BIND_OK"
	}
}

func parsePostgresError(data []byte) string {
	// Error fields are type-byte + null-terminated string
	var msg string
	for i := 0; i < len(data)-1; {
		fieldType := data[i]
		i++
		end := bytes.IndexByte(data[i:], 0)
		if end < 0 {
			break
		}
		value := string(data[i : i+end])
		i += end + 1

		if fieldType == 'M' { // Message field
			msg = value
			break
		}
	}
	return msg
}

// Redis RESP protocol parsing
func (p *DBProbe) parseRedisCommand(event *DBEvent, payload []byte) {
	if len(payload) < 3 {
		return
	}

	// Parse RESP format
	if payload[0] == '*' {
		// Array format: *<count>\r\n$<len>\r\n<arg>\r\n...
		parts := parseRESPArray(payload)
		if len(parts) > 0 {
			event.Operation = strings.ToUpper(parts[0])
			if len(parts) > 1 {
				event.Key = parts[1]
			}
			event.Query = strings.Join(parts, " ")
		}
	} else {
		// Inline command
		line := strings.TrimRight(string(payload), "\r\n")
		parts := strings.Fields(line)
		if len(parts) > 0 {
			event.Operation = strings.ToUpper(parts[0])
			if len(parts) > 1 {
				event.Key = parts[1]
			}
			event.Query = line
		}
	}
}

func (p *DBProbe) parseRedisResponse(event *DBEvent, payload []byte) {
	if len(payload) < 1 {
		return
	}

	switch payload[0] {
	case '+': // Simple string
		event.Operation = "OK"
		line := strings.TrimRight(string(payload[1:]), "\r\n")
		if line != "OK" {
			event.Query = line
		}
	case '-': // Error
		event.Operation = "ERROR"
		event.Error = strings.TrimRight(string(payload[1:]), "\r\n")
	case ':': // Integer
		event.Operation = "INT"
		line := strings.TrimRight(string(payload[1:]), "\r\n")
		fmt.Sscanf(line, "%d", &event.RowsAffected)
	case '$': // Bulk string
		event.Operation = "BULK"
	case '*': // Array
		event.Operation = "ARRAY"
	}
}

// parseRESPArray parses Redis RESP array format
func parseRESPArray(data []byte) []string {
	var parts []string
	lines := bytes.Split(data, []byte("\r\n"))

	i := 0
	if len(lines) > 0 && lines[0][0] == '*' {
		i = 1 // Skip array header
	}

	for i < len(lines) {
		if len(lines[i]) == 0 {
			i++
			continue
		}
		if lines[i][0] == '$' {
			// Bulk string: $<len>\r\n<data>\r\n
			i++
			if i < len(lines) {
				parts = append(parts, string(lines[i]))
			}
		}
		i++
	}

	return parts
}

// Helper functions

func extractSQLOperation(query string) string {
	query = strings.TrimSpace(strings.ToUpper(query))
	ops := []string{"SELECT", "INSERT", "UPDATE", "DELETE", "CREATE", "DROP", "ALTER", "TRUNCATE", "BEGIN", "COMMIT", "ROLLBACK", "SET", "SHOW", "EXPLAIN", "USE"}
	for _, op := range ops {
		if strings.HasPrefix(query, op) {
			return op
		}
	}
	return "QUERY"
}

func extractTableName(query string) string {
	query = strings.ToUpper(query)

	// FROM table_name
	if idx := strings.Index(query, " FROM "); idx >= 0 {
		rest := strings.TrimSpace(query[idx+6:])
		parts := strings.Fields(rest)
		if len(parts) > 0 {
			return strings.Trim(parts[0], "`\"")
		}
	}

	// INTO table_name
	if idx := strings.Index(query, " INTO "); idx >= 0 {
		rest := strings.TrimSpace(query[idx+6:])
		parts := strings.Fields(rest)
		if len(parts) > 0 {
			return strings.Trim(parts[0], "`\"(")
		}
	}

	// UPDATE table_name
	if strings.HasPrefix(query, "UPDATE ") {
		rest := strings.TrimSpace(query[7:])
		parts := strings.Fields(rest)
		if len(parts) > 0 {
			return strings.Trim(parts[0], "`\"")
		}
	}

	return ""
}

func sanitizeQuery(query string) string {
	// Trim null bytes and excessive whitespace
	query = strings.TrimRight(query, "\x00")
	query = strings.TrimSpace(query)
	// Collapse whitespace
	for strings.Contains(query, "  ") {
		query = strings.ReplaceAll(query, "  ", " ")
	}
	// Limit length
	if len(query) > 500 {
		query = query[:500] + "..."
	}
	return query
}

// Close cleans up resources
func (p *DBProbe) Close() error {
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
