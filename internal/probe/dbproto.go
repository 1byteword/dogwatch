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

	// Prepared statement tracking
	preparedStmts *PreparedStatementCache
}

type queryInfo struct {
	StartTime  time.Time
	Query      string
	DBType     string
	IsPrepare  bool // True if this was a PREPARE statement
}

// NewDBProbe creates and loads the database protocol eBPF probe
func NewDBProbe() (*DBProbe, error) {
	objs := dbprotoObjects{}
	if err := loadDbprotoObjects(&objs, nil); err != nil {
		return nil, fmt.Errorf("loading DB BPF objects: %w", err)
	}

	p := &DBProbe{
		objs:          objs,
		links:         make([]link.Link, 0, 6),
		eventsChan:    make(chan DBEvent, 200),
		queries:       make(map[string]queryInfo),
		preparedStmts: NewPreparedStatementCache(),
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
			StartTime:  event.Timestamp,
			Query:      event.Query,
			DBType:     event.DBType,
			IsPrepare:  event.Operation == "PREPARE",
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

			// If this was a PREPARE statement and response is OK, cache the prepared statement
			if qi.IsPrepare && event.Operation == "OK" && p.preparedStmts != nil {
				payload := raw.Payload[:raw.PayloadSize]
				if stmtID, ok := ExtractMySQLPrepareStmtID(payload); ok {
					p.preparedStmts.StorePrepared(raw.PID, raw.TID, stmtID, qi.Query)
				}
			}
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

// looksLikeText checks if data appears to be printable text (not encrypted/binary)
func looksLikeText(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	// Check first 64 bytes (or all if shorter)
	checkLen := len(data)
	if checkLen > 64 {
		checkLen = 64
	}
	printable := 0
	for i := 0; i < checkLen; i++ {
		c := data[i]
		// Printable ASCII, tab, newline, carriage return
		if (c >= 32 && c <= 126) || c == '\t' || c == '\n' || c == '\r' {
			printable++
		}
	}
	// At least 80% should be printable for valid SQL/commands
	return float64(printable)/float64(checkLen) >= 0.8
}

// MySQL wire protocol parsing
func (p *DBProbe) parseMySQLQuery(event *DBEvent, payload []byte) {
	if len(payload) < 6 {
		return
	}

	// Parse 3-byte length (little-endian)
	pktLen := int(payload[0]) | int(payload[1])<<8 | int(payload[2])<<16
	if pktLen < 1 || pktLen > 16777215 {
		return
	}

	// Verify packet length is consistent
	expectedTotal := pktLen + 4
	if expectedTotal > len(payload)+100 || len(payload) > expectedTotal+1000 {
		return
	}

	// Note: sequence number resets to 0 for each new command in MySQL protocol
	// but we don't filter on it since persistent connections can have any state
	_ = payload[3] // seqNum, unused

	cmd := payload[4]
	data := payload[5:]

	// Skip encrypted/binary data
	if len(data) > 0 && !looksLikeText(data) {
		return
	}

	// Trim leading null bytes from data (sometimes protocol noise)
	data = trimLeadingNulls(data)

	switch cmd {
	case 0x03: // COM_QUERY
		// Verify query starts with SQL keyword
		if !looksLikeSQLQuery(data) {
			return
		}
		event.Operation = "QUERY"
		event.Query = sanitizeQuery(string(data))
		event.Table = extractTableName(event.Query)
	case 0x16: // COM_STMT_PREPARE
		if !looksLikeSQLQuery(data) {
			return
		}
		event.Operation = "PREPARE"
		event.Query = sanitizeQuery(string(data))
		// Note: Statement ID comes in the response, not the request
		// We track the PREPARE SQL temporarily and link it when we see the response
	case 0x17: // COM_STMT_EXECUTE
		event.Operation = "EXECUTE"
		// Try to extract statement ID and look up the prepared SQL
		if p.preparedStmts != nil && len(payload) >= 9 {
			stmtID := uint32(payload[5]) | uint32(payload[6])<<8 | uint32(payload[7])<<16 | uint32(payload[8])<<24
			stmtIDStr := fmt.Sprintf("%d", stmtID)
			if stmt := p.preparedStmts.GetPrepared(event.PID, event.TID, stmtIDStr); stmt != nil {
				event.Query = stmt.SQL
				event.Table = extractTableName(event.Query)
			}
		}
	case 0x02: // COM_INIT_DB
		// Database name should be printable
		if len(data) > 0 && !isAlphaOrUnderscore(data[0]) {
			return
		}
		event.Operation = "USE"
		event.Query = sanitizeQuery(string(data))
	default:
		return // Unknown command
	}

	// Extract operation from query
	if event.Operation == "QUERY" && len(event.Query) > 0 {
		event.Operation = extractSQLOperation(event.Query)
	}
}

// looksLikeSQLQuery checks if data starts with a common SQL keyword
func looksLikeSQLQuery(data []byte) bool {
	if len(data) < 3 {
		return false
	}
	// Trim leading whitespace
	start := 0
	for start < len(data) && (data[start] == ' ' || data[start] == '\t' || data[start] == '\n') {
		start++
	}
	if start >= len(data) {
		return false
	}

	// Get first character (uppercase)
	c := data[start]
	if c >= 'a' && c <= 'z' {
		c -= 32
	}

	// Common SQL keywords
	switch c {
	case 'S': // SELECT, SET, SHOW
		return true
	case 'I': // INSERT
		return true
	case 'U': // UPDATE, USE
		return true
	case 'D': // DELETE, DROP, DESC
		return true
	case 'C': // CREATE, COMMIT, CALL
		return true
	case 'A': // ALTER
		return true
	case 'B': // BEGIN
		return true
	case 'R': // ROLLBACK, REPLACE
		return true
	case 'E': // EXPLAIN, EXECUTE
		return true
	case 'T': // TRUNCATE
		return true
	case 'G': // GRANT
		return true
	case 'L': // LOCK, LOAD
		return true
	case 'W': // WITH
		return true
	case '/': // /* comment */
		return true
	case '(': // Subquery
		return true
	}
	return false
}

// isAlphaOrUnderscore checks if byte is a-z, A-Z, or _
func isAlphaOrUnderscore(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || b == '_'
}

// trimLeadingNulls removes leading null/control bytes from data
func trimLeadingNulls(data []byte) []byte {
	start := 0
	// Skip null bytes and other non-printable control characters
	for start < len(data) && data[start] < 32 && data[start] != '\t' && data[start] != '\n' {
		start++
	}
	return data[start:]
}

func (p *DBProbe) parseMySQLResponse(event *DBEvent, payload []byte) {
	if len(payload) < 5 {
		return
	}

	// Check if payload looks like valid MySQL response
	if !looksLikeMySQLResponse(payload) {
		return
	}

	// Parse header
	pktLen := int(payload[0]) | int(payload[1])<<8 | int(payload[2])<<16
	seqNum := payload[3]
	status := payload[4]

	// Sequence number validation (responses have seq >= 1 typically)
	if seqNum > 100 {
		return
	}

	switch status {
	case 0x00: // OK packet
		// OK packet should have reasonable length (min 3 bytes for affected_rows, last_insert_id)
		if pktLen < 3 {
			return
		}
		event.Operation = "OK"
		if len(payload) > 5 {
			// Affected rows is length-encoded int
			event.RowsAffected = int(payload[5])
		}
	case 0xff: // ERR packet
		// ERR: 0xff + 2-byte error code + optional '#' + 5-byte state + message
		if pktLen < 3 || len(payload) < 7 {
			return
		}
		// Validate error code range (MySQL errors are 1000-4999 typically)
		errCode := int(payload[5]) | int(payload[6])<<8
		if errCode < 1000 || errCode > 10000 {
			// Check for '#' marker which indicates SQLSTATE follows
			if len(payload) < 8 || payload[7] != '#' {
				return
			}
		}
		event.Operation = "ERROR"
		// Error message starts after error code and optional SQLSTATE
		msgStart := 7
		if len(payload) > 7 && payload[7] == '#' {
			msgStart = 13 // Skip '#' + 5-char SQLSTATE
		}
		if len(payload) > msgStart && looksLikeText(payload[msgStart:]) {
			event.Error = sanitizeQuery(string(payload[msgStart:]))
		}
	case 0xfe: // EOF packet
		// EOF packet is exactly 5 bytes payload in MySQL 4.1+
		if pktLen != 5 && pktLen != 1 {
			return
		}
		event.Operation = "EOF"
	default:
		// Result set - first byte is field count
		// Be conservative: only accept small field counts (1-50 typical)
		if status >= 1 && status <= 50 {
			// Field count packet should be small
			if pktLen <= 9 {
				event.Operation = "RESULT"
			}
		}
	}
}

// looksLikeMySQLResponse checks if payload appears to be valid MySQL protocol
func looksLikeMySQLResponse(payload []byte) bool {
	if len(payload) < 5 {
		return false
	}
	// Check packet length field (first 3 bytes, little-endian)
	packetLen := int(payload[0]) | int(payload[1])<<8 | int(payload[2])<<16
	// Packet length should be reasonable and match payload
	if packetLen <= 0 || packetLen > 16*1024*1024 {
		return false
	}
	// Sequence number should be small
	seqNum := payload[3]
	if seqNum > 100 {
		return false
	}
	return true
}

// PostgreSQL wire protocol parsing
func (p *DBProbe) parsePostgresQuery(event *DBEvent, payload []byte) {
	if len(payload) < 5 {
		return
	}

	// Check if it looks like valid PostgreSQL protocol
	if !looksLikePostgresMessage(payload) {
		return
	}

	msgType := payload[0]

	switch msgType {
	case 'Q': // Simple Query
		event.Operation = "QUERY"
		// Length is bytes 1-4 (big-endian), then query string
		if len(payload) > 5 && looksLikeText(payload[5:]) {
			event.Query = sanitizeQuery(strings.TrimRight(string(payload[5:]), "\x00"))
			event.Table = extractTableName(event.Query)
			event.Operation = extractSQLOperation(event.Query)
		}
	case 'P': // Parse (prepared statement)
		event.Operation = "PREPARE"
		// Statement name (null-terminated), then query
		if idx := bytes.IndexByte(payload[5:], 0); idx >= 0 {
			stmtName := string(payload[5 : 5+idx])
			queryStart := 5 + idx + 1
			if queryStart < len(payload) && looksLikeText(payload[queryStart:]) {
				event.Query = sanitizeQuery(strings.TrimRight(string(payload[queryStart:]), "\x00"))
				// Store the prepared statement if we have a name
				if p.preparedStmts != nil && stmtName != "" {
					p.preparedStmts.StorePrepared(event.PID, event.TID, stmtName, event.Query)
				}
			}
		}
	case 'E': // Execute
		event.Operation = "EXECUTE"
		// Extract portal name and look up the prepared statement
		if p.preparedStmts != nil {
			if idx := bytes.IndexByte(payload[5:], 0); idx >= 0 {
				portalName := string(payload[5 : 5+idx])
				if stmt := p.preparedStmts.GetPrepared(event.PID, event.TID, portalName); stmt != nil {
					event.Query = stmt.SQL
					event.Table = extractTableName(event.Query)
				}
			}
		}
	case 'B': // Bind
		event.Operation = "BIND"
	}
}

// looksLikePostgresMessage checks if payload looks like valid PostgreSQL protocol
func looksLikePostgresMessage(payload []byte) bool {
	if len(payload) < 5 {
		return false
	}
	// First byte should be a valid message type (printable ASCII letter)
	msgType := payload[0]
	validTypes := "QPEBCDTRZCS12n"
	isValidType := false
	for i := 0; i < len(validTypes); i++ {
		if msgType == validTypes[i] {
			isValidType = true
			break
		}
	}
	if !isValidType {
		return false
	}
	// Length field (bytes 1-4, big-endian) should be reasonable
	msgLen := int(payload[1])<<24 | int(payload[2])<<16 | int(payload[3])<<8 | int(payload[4])
	return msgLen > 0 && msgLen < 16*1024*1024
}

func (p *DBProbe) parsePostgresResponse(event *DBEvent, payload []byte) {
	if len(payload) < 5 {
		return
	}

	// Check if it looks like valid PostgreSQL protocol
	if !looksLikePostgresMessage(payload) {
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
		if len(payload) > 5 && looksLikeText(payload[5:]) {
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
	if len(payload) < 3 {
		return
	}

	switch payload[0] {
	case '+': // Simple string: +OK\r\n
		// Must have \r\n
		if !bytes.Contains(payload[:min(len(payload), 100)], []byte("\r\n")) {
			return
		}
		event.Operation = "OK"
		line := strings.TrimRight(string(payload[1:]), "\r\n")
		if line != "OK" {
			event.Query = line
		}
	case '-': // Error: -ERR message\r\n or -WRONGTYPE message\r\n
		// Must have \r\n and start with known error prefix
		if !bytes.Contains(payload[:min(len(payload), 200)], []byte("\r\n")) {
			return
		}
		// Redis errors start with error type like ERR, WRONGTYPE, NOSCRIPT, etc.
		// NOT with ---- (like certificates)
		if len(payload) > 4 && payload[1] == '-' && payload[2] == '-' && payload[3] == '-' {
			return // Looks like certificate ----BEGIN
		}
		line := strings.TrimRight(string(payload[1:]), "\r\n")
		// Validate it looks like a Redis error (uppercase word at start)
		if len(line) > 0 && (line[0] < 'A' || line[0] > 'Z') {
			return
		}
		event.Operation = "ERROR"
		event.Error = line
	case ':': // Integer: :123\r\n
		if !bytes.Contains(payload[:min(len(payload), 50)], []byte("\r\n")) {
			return
		}
		event.Operation = "INT"
		line := strings.TrimRight(string(payload[1:]), "\r\n")
		fmt.Sscanf(line, "%d", &event.RowsAffected)
	case '$': // Bulk string: $6\r\nfoobar\r\n
		// Must have length followed by \r\n
		if !bytes.Contains(payload[:min(len(payload), 20)], []byte("\r\n")) {
			return
		}
		event.Operation = "BULK"
	case '*': // Array: *2\r\n...
		// Must have count followed by \r\n
		if !bytes.Contains(payload[:min(len(payload), 20)], []byte("\r\n")) {
			return
		}
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

// PreparedStatementStats returns statistics about cached prepared statements
func (p *DBProbe) PreparedStatementStats() map[string]interface{} {
	if p.preparedStmts == nil {
		return map[string]interface{}{
			"enabled": false,
		}
	}
	stats := p.preparedStmts.Stats()
	stats["enabled"] = true
	return stats
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
