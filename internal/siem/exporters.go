package siem

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Exporter defines the interface for SIEM exporters
type Exporter interface {
	Export(events []FormattedEvent) error
	Close() error
	Name() string
}

// FormattedEvent wraps a formatted event with metadata
type FormattedEvent struct {
	Original  SecurityEvent
	Formatted string
	Format    ExportFormat
	Timestamp time.Time
}

// SyslogExporter exports events via syslog (UDP/TCP/TLS)
type SyslogExporter struct {
	Protocol string // udp, tcp, tls
	Address  string
	Facility int
	Hostname string

	conn     net.Conn
	mu       sync.Mutex
	closed   bool
	tlsConf  *tls.Config
}

// NewSyslogExporter creates a new syslog exporter
func NewSyslogExporter(config DestinationConfig) (*SyslogExporter, error) {
	exp := &SyslogExporter{
		Protocol: config.SyslogProtocol,
		Address:  config.SyslogAddress,
		Facility: config.SyslogFacility,
		Hostname: config.SyslogHostname,
	}

	if exp.Hostname == "" {
		hostname, _ := os.Hostname()
		exp.Hostname = hostname
	}

	// Configure TLS if needed
	if config.SyslogProtocol == "tls" {
		exp.tlsConf = &tls.Config{
			InsecureSkipVerify: config.TLSSkipVerify,
		}

		if config.TLSCertFile != "" && config.TLSKeyFile != "" {
			cert, err := tls.LoadX509KeyPair(config.TLSCertFile, config.TLSKeyFile)
			if err != nil {
				return nil, fmt.Errorf("load TLS cert: %w", err)
			}
			exp.tlsConf.Certificates = []tls.Certificate{cert}
		}
	}

	return exp, nil
}

// Export sends events to syslog
func (e *SyslogExporter) Export(events []FormattedEvent) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return fmt.Errorf("exporter is closed")
	}

	// Establish connection if needed
	if e.conn == nil {
		if err := e.connect(); err != nil {
			return fmt.Errorf("connect: %w", err)
		}
	}

	for _, event := range events {
		msg := e.formatSyslogMessage(event)
		if _, err := e.conn.Write([]byte(msg)); err != nil {
			// Try to reconnect once
			e.conn.Close()
			e.conn = nil
			if err := e.connect(); err != nil {
				return fmt.Errorf("reconnect: %w", err)
			}
			if _, err := e.conn.Write([]byte(msg)); err != nil {
				return fmt.Errorf("write after reconnect: %w", err)
			}
		}
	}

	return nil
}

// connect establishes the connection
func (e *SyslogExporter) connect() error {
	var conn net.Conn
	var err error

	switch e.Protocol {
	case "udp":
		conn, err = net.DialTimeout("udp", e.Address, 10*time.Second)
	case "tcp":
		conn, err = net.DialTimeout("tcp", e.Address, 10*time.Second)
	case "tls":
		dialer := &net.Dialer{Timeout: 10 * time.Second}
		conn, err = tls.DialWithDialer(dialer, "tcp", e.Address, e.tlsConf)
	default:
		return fmt.Errorf("unsupported protocol: %s", e.Protocol)
	}

	if err != nil {
		return err
	}

	e.conn = conn
	return nil
}

// formatSyslogMessage formats an event as RFC 5424 syslog
func (e *SyslogExporter) formatSyslogMessage(event FormattedEvent) string {
	// RFC 5424: <PRI>VERSION TIMESTAMP HOSTNAME APP-NAME PROCID MSGID STRUCTURED-DATA MSG
	// PRI = facility * 8 + severity
	severity := mapSeverityToSyslog(event.Original.Severity)
	pri := e.Facility*8 + severity

	timestamp := event.Timestamp.Format(time.RFC3339)
	appName := "dogwatch"
	procID := "-"
	msgID := event.Original.ID
	if msgID == "" {
		msgID = "-"
	}

	return fmt.Sprintf("<%d>1 %s %s %s %s %s - %s\n",
		pri, timestamp, e.Hostname, appName, procID, msgID, event.Formatted)
}

// mapSeverityToSyslog maps dogwatch severity to syslog severity (0-7)
func mapSeverityToSyslog(severity string) int {
	switch severity {
	case "critical":
		return 2 // Critical
	case "high":
		return 3 // Error
	case "medium":
		return 4 // Warning
	case "low":
		return 5 // Notice
	case "info":
		return 6 // Informational
	default:
		return 6
	}
}

func (e *SyslogExporter) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.closed = true
	if e.conn != nil {
		return e.conn.Close()
	}
	return nil
}

// Name returns the exporter name
func (e *SyslogExporter) Name() string {
	return fmt.Sprintf("syslog://%s", e.Address)
}

// FileExporter exports events to rotating files
type FileExporter struct {
	Path       string
	MaxSizeMB  int
	MaxAgeDays int
	MaxBackups int
	Compress   bool

	mu       sync.Mutex
	file     *os.File
	size     int64
	closed   bool
}

// NewFileExporter creates a new file exporter
func NewFileExporter(config DestinationConfig) (*FileExporter, error) {
	exp := &FileExporter{
		Path:       config.FilePath,
		MaxSizeMB:  config.FileMaxSizeMB,
		MaxAgeDays: config.FileMaxAgeDays,
		MaxBackups: config.FileMaxBackups,
		Compress:   config.FileCompress,
	}

	if exp.MaxSizeMB <= 0 {
		exp.MaxSizeMB = 100
	}
	if exp.MaxBackups <= 0 {
		exp.MaxBackups = 10
	}

	// Ensure directory exists
	dir := filepath.Dir(exp.Path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create directory: %w", err)
	}

	return exp, nil
}

// Export writes events to file
func (e *FileExporter) Export(events []FormattedEvent) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return fmt.Errorf("exporter is closed")
	}

	// Open file if needed
	if e.file == nil {
		if err := e.openFile(); err != nil {
			return fmt.Errorf("open file: %w", err)
		}
	}

	for _, event := range events {
		line := event.Formatted + "\n"
		n, err := e.file.WriteString(line)
		if err != nil {
			return fmt.Errorf("write: %w", err)
		}
		e.size += int64(n)

		// Check if rotation is needed
		if e.size >= int64(e.MaxSizeMB)*1024*1024 {
			if err := e.rotate(); err != nil {
				return fmt.Errorf("rotate: %w", err)
			}
		}
	}

	return e.file.Sync()
}

// openFile opens or creates the log file
func (e *FileExporter) openFile() error {
	file, err := os.OpenFile(e.Path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	info, err := file.Stat()
	if err != nil {
		file.Close()
		return err
	}

	e.file = file
	e.size = info.Size()
	return nil
}

// rotate rotates the log file
func (e *FileExporter) rotate() error {
	if e.file != nil {
		e.file.Close()
		e.file = nil
	}

	// Rotate existing backups
	for i := e.MaxBackups - 1; i >= 1; i-- {
		oldPath := fmt.Sprintf("%s.%d", e.Path, i)
		newPath := fmt.Sprintf("%s.%d", e.Path, i+1)
		os.Rename(oldPath, newPath)
	}

	// Rename current file
	os.Rename(e.Path, e.Path+".1")

	// Open new file
	return e.openFile()
}

func (e *FileExporter) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.closed = true
	if e.file != nil {
		return e.file.Close()
	}
	return nil
}

// Name returns the exporter name
func (e *FileExporter) Name() string {
	return fmt.Sprintf("file://%s", e.Path)
}

// HTTPExporter exports events via HTTP webhooks
type HTTPExporter struct {
	Endpoint string
	Method   string
	Headers  map[string]string
	AuthType string
	AuthUser string
	AuthPass string
	Token    string
	APIKey   string
	APIKeyHeader string
	Timeout  time.Duration

	client *http.Client
	closed bool
}

// NewHTTPExporter creates a new HTTP exporter
func NewHTTPExporter(config DestinationConfig) (*HTTPExporter, error) {
	exp := &HTTPExporter{
		Endpoint:     config.HTTPEndpoint,
		Method:       config.HTTPMethod,
		Headers:      config.HTTPHeaders,
		AuthType:     config.HTTPAuthType,
		AuthUser:     config.HTTPAuthUser,
		AuthPass:     config.HTTPAuthPassword,
		Token:        config.HTTPAuthToken,
		APIKey:       config.HTTPAPIKey,
		APIKeyHeader: config.HTTPAPIKeyHeader,
		Timeout:      config.HTTPTimeout,
	}

	if exp.Method == "" {
		exp.Method = "POST"
	}
	if exp.Timeout <= 0 {
		exp.Timeout = 30 * time.Second
	}
	if exp.APIKeyHeader == "" {
		exp.APIKeyHeader = "X-API-Key"
	}

	// Configure TLS client
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: config.TLSSkipVerify,
		},
	}

	exp.client = &http.Client{
		Transport: transport,
		Timeout:   exp.Timeout,
	}

	return exp, nil
}

// Export sends events via HTTP
func (e *HTTPExporter) Export(events []FormattedEvent) error {
	if e.closed {
		return fmt.Errorf("exporter is closed")
	}

	// Build request body - newline-delimited events
	var buf bytes.Buffer
	for _, event := range events {
		buf.WriteString(event.Formatted)
		buf.WriteByte('\n')
	}

	req, err := http.NewRequest(e.Method, e.Endpoint, &buf)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	// Set content type based on format
	if len(events) > 0 {
		switch events[0].Format {
		case FormatJSON:
			req.Header.Set("Content-Type", "application/x-ndjson")
		default:
			req.Header.Set("Content-Type", "text/plain")
		}
	}

	// Add custom headers
	for k, v := range e.Headers {
		req.Header.Set(k, v)
	}

	// Add authentication
	switch e.AuthType {
	case "basic":
		req.SetBasicAuth(e.AuthUser, e.AuthPass)
	case "bearer":
		req.Header.Set("Authorization", "Bearer "+e.Token)
	case "api_key":
		req.Header.Set(e.APIKeyHeader, e.APIKey)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func (e *HTTPExporter) Close() error {
	e.closed = true
	return nil
}

// Name returns the exporter name
func (e *HTTPExporter) Name() string {
	return fmt.Sprintf("http://%s", e.Endpoint)
}

// CreateExporter creates an exporter based on configuration
func CreateExporter(config Config) (Exporter, error) {
	switch config.ExporterType {
	case ExporterTypeSyslog:
		return NewSyslogExporter(config.Destination)
	case ExporterTypeFile:
		return NewFileExporter(config.Destination)
	case ExporterTypeHTTP:
		return NewHTTPExporter(config.Destination)
	default:
		return nil, fmt.Errorf("unsupported exporter type: %s", config.ExporterType)
	}
}

// TestExporter tests connectivity to an exporter
func TestExporter(config Config) error {
	exp, err := CreateExporter(config)
	if err != nil {
		return fmt.Errorf("create exporter: %w", err)
	}
	defer exp.Close()

	// Send a test event
	testEvent := FormattedEvent{
		Original: SecurityEvent{
			ID:        "test-event",
			Timestamp: time.Now(),
			EventType: "test",
			Severity:  "info",
			Title:     "SIEM Export Test",
		},
		Formatted: "CEF:0|Dogwatch|Dogwatch|1.0|test|SIEM Export Test|1|msg=Test event",
		Format:    config.Format,
		Timestamp: time.Now(),
	}

	return exp.Export([]FormattedEvent{testEvent})
}
