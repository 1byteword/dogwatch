package siem

import (
	"encoding/json"
	"fmt"
	"time"
)

// ExportFormat defines the output format for SIEM export
type ExportFormat string

const (
	FormatCEF  ExportFormat = "cef"
	FormatLEEF ExportFormat = "leef"
	FormatJSON ExportFormat = "json"
)

// ExporterType defines the type of exporter
type ExporterType string

const (
	ExporterTypeSyslog ExporterType = "syslog"
	ExporterTypeFile   ExporterType = "file"
	ExporterTypeHTTP   ExporterType = "http"
	ExporterTypeKafka  ExporterType = "kafka"
)

// Config holds the SIEM export configuration
type Config struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	Enabled      bool         `json:"enabled"`
	Format       ExportFormat `json:"format"`
	ExporterType ExporterType `json:"exporter_type"`

	// Destination configuration
	Destination DestinationConfig `json:"destination"`

	// Filtering
	Filter FilterConfig `json:"filter"`

	// Batching
	Batching BatchingConfig `json:"batching"`

	// Retry policy
	Retry RetryConfig `json:"retry"`

	// Metadata
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// DestinationConfig holds destination-specific configuration
type DestinationConfig struct {
	// Syslog configuration
	SyslogProtocol string `json:"syslog_protocol,omitempty"` // udp, tcp, tls
	SyslogAddress  string `json:"syslog_address,omitempty"`  // host:port
	SyslogFacility int    `json:"syslog_facility,omitempty"` // 0-23
	SyslogHostname string `json:"syslog_hostname,omitempty"` // hostname to use in syslog header

	// TLS configuration (for syslog TLS)
	TLSEnabled    bool   `json:"tls_enabled,omitempty"`
	TLSSkipVerify bool   `json:"tls_skip_verify,omitempty"`
	TLSCertFile   string `json:"tls_cert_file,omitempty"`
	TLSKeyFile    string `json:"tls_key_file,omitempty"`
	TLSCAFile     string `json:"tls_ca_file,omitempty"`

	// File configuration
	FilePath       string `json:"file_path,omitempty"`
	FileMaxSizeMB  int    `json:"file_max_size_mb,omitempty"`
	FileMaxAgeDays int    `json:"file_max_age_days,omitempty"`
	FileMaxBackups int    `json:"file_max_backups,omitempty"`
	FileCompress   bool   `json:"file_compress,omitempty"`

	// HTTP webhook configuration
	HTTPEndpoint     string            `json:"http_endpoint,omitempty"`
	HTTPMethod       string            `json:"http_method,omitempty"` // POST, PUT
	HTTPHeaders      map[string]string `json:"http_headers,omitempty"`
	HTTPAuthType     string            `json:"http_auth_type,omitempty"` // none, basic, bearer, api_key
	HTTPAuthUser     string            `json:"http_auth_user,omitempty"`
	HTTPAuthPassword string            `json:"http_auth_password,omitempty"`
	HTTPAuthToken    string            `json:"http_auth_token,omitempty"`
	HTTPAPIKeyHeader string            `json:"http_api_key_header,omitempty"`
	HTTPAPIKey       string            `json:"http_api_key,omitempty"`
	HTTPTimeout      time.Duration     `json:"http_timeout,omitempty"`

	// Kafka configuration
	KafkaBrokers  []string `json:"kafka_brokers,omitempty"`
	KafkaTopic    string   `json:"kafka_topic,omitempty"`
	KafkaClientID string   `json:"kafka_client_id,omitempty"`
	KafkaSASLUser string   `json:"kafka_sasl_user,omitempty"`
	KafkaSASLPass string   `json:"kafka_sasl_pass,omitempty"`
}

// FilterConfig defines event filtering rules
type FilterConfig struct {
	// Severity filter (only export events >= this severity)
	MinSeverity string `json:"min_severity,omitempty"` // info, low, medium, high, critical

	// Event type filter
	EventTypes []string `json:"event_types,omitempty"` // event, alert

	// Include/exclude rules
	IncludeRules []string `json:"include_rules,omitempty"` // rule IDs to include
	ExcludeRules []string `json:"exclude_rules,omitempty"` // rule IDs to exclude

	// Container filter
	IncludeNamespaces []string `json:"include_namespaces,omitempty"`
	ExcludeNamespaces []string `json:"exclude_namespaces,omitempty"`

	// Host filter
	IncludeHosts []string `json:"include_hosts,omitempty"`
	ExcludeHosts []string `json:"exclude_hosts,omitempty"`
}

// BatchingConfig defines batching settings
type BatchingConfig struct {
	Enabled       bool          `json:"enabled"`
	MaxEvents     int           `json:"max_events"`     // Max events per batch
	MaxWait       time.Duration `json:"max_wait"`       // Max time to wait before flushing
	FlushInterval time.Duration `json:"flush_interval"` // Periodic flush interval
}

// RetryConfig defines retry policy
type RetryConfig struct {
	MaxRetries     int           `json:"max_retries"`
	InitialBackoff time.Duration `json:"initial_backoff"`
	MaxBackoff     time.Duration `json:"max_backoff"`
	Multiplier     float64       `json:"multiplier"`
}

// DefaultConfig returns a default SIEM export configuration
func DefaultConfig() Config {
	return Config{
		Enabled:      false,
		Format:       FormatJSON,
		ExporterType: ExporterTypeSyslog,
		Destination: DestinationConfig{
			SyslogProtocol: "udp",
			SyslogAddress:  "localhost:514",
			SyslogFacility: 1, // user-level
		},
		Filter: FilterConfig{
			MinSeverity: "low",
			EventTypes:  []string{"alert"},
		},
		Batching: BatchingConfig{
			Enabled:       true,
			MaxEvents:     100,
			MaxWait:       5 * time.Second,
			FlushInterval: 10 * time.Second,
		},
		Retry: RetryConfig{
			MaxRetries:     3,
			InitialBackoff: 1 * time.Second,
			MaxBackoff:     30 * time.Second,
			Multiplier:     2.0,
		},
	}
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("name is required")
	}

	// Validate format
	switch c.Format {
	case FormatCEF, FormatLEEF, FormatJSON:
		// Valid
	default:
		return fmt.Errorf("invalid format: %s (must be cef, leef, or json)", c.Format)
	}

	// Validate exporter type and destination
	switch c.ExporterType {
	case ExporterTypeSyslog:
		if c.Destination.SyslogAddress == "" {
			return fmt.Errorf("syslog_address is required for syslog exporter")
		}
		if c.Destination.SyslogProtocol != "udp" && c.Destination.SyslogProtocol != "tcp" && c.Destination.SyslogProtocol != "tls" {
			return fmt.Errorf("invalid syslog_protocol: %s (must be udp, tcp, or tls)", c.Destination.SyslogProtocol)
		}
	case ExporterTypeFile:
		if c.Destination.FilePath == "" {
			return fmt.Errorf("file_path is required for file exporter")
		}
	case ExporterTypeHTTP:
		if c.Destination.HTTPEndpoint == "" {
			return fmt.Errorf("http_endpoint is required for http exporter")
		}
	case ExporterTypeKafka:
		if len(c.Destination.KafkaBrokers) == 0 {
			return fmt.Errorf("kafka_brokers is required for kafka exporter")
		}
		if c.Destination.KafkaTopic == "" {
			return fmt.Errorf("kafka_topic is required for kafka exporter")
		}
	default:
		return fmt.Errorf("invalid exporter_type: %s", c.ExporterType)
	}

	// Validate batching
	if c.Batching.Enabled {
		if c.Batching.MaxEvents <= 0 {
			c.Batching.MaxEvents = 100
		}
		if c.Batching.MaxWait <= 0 {
			c.Batching.MaxWait = 5 * time.Second
		}
	}

	// Validate retry
	if c.Retry.MaxRetries < 0 {
		c.Retry.MaxRetries = 0
	}
	if c.Retry.InitialBackoff <= 0 {
		c.Retry.InitialBackoff = 1 * time.Second
	}
	if c.Retry.MaxBackoff <= 0 {
		c.Retry.MaxBackoff = 30 * time.Second
	}
	if c.Retry.Multiplier <= 1 {
		c.Retry.Multiplier = 2.0
	}

	return nil
}

// MatchesFilter checks if an event matches the filter criteria
func (f *FilterConfig) MatchesFilter(event SecurityEvent) bool {
	// Check severity
	if f.MinSeverity != "" && !severityAtLeast(event.Severity, f.MinSeverity) {
		return false
	}

	// Check event type
	if len(f.EventTypes) > 0 && !contains(f.EventTypes, event.EventType) {
		return false
	}

	// Check rule inclusion/exclusion
	if len(f.IncludeRules) > 0 && !contains(f.IncludeRules, event.RuleID) {
		return false
	}
	if len(f.ExcludeRules) > 0 && contains(f.ExcludeRules, event.RuleID) {
		return false
	}

	// Check namespace inclusion/exclusion
	if len(f.IncludeNamespaces) > 0 && !contains(f.IncludeNamespaces, event.Namespace) {
		return false
	}
	if len(f.ExcludeNamespaces) > 0 && contains(f.ExcludeNamespaces, event.Namespace) {
		return false
	}

	// Check host inclusion/exclusion
	if len(f.IncludeHosts) > 0 && !contains(f.IncludeHosts, event.Hostname) {
		return false
	}
	if len(f.ExcludeHosts) > 0 && contains(f.ExcludeHosts, event.Hostname) {
		return false
	}

	return true
}

// severityAtLeast checks if severity is at least the minimum
func severityAtLeast(severity, minSeverity string) bool {
	levels := map[string]int{
		"":         0,
		"info":     1,
		"low":      2,
		"medium":   3,
		"high":     4,
		"critical": 5,
	}

	return levels[severity] >= levels[minSeverity]
}

// contains checks if a slice contains a string
func contains(slice []string, str string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}

// MarshalJSON customizes JSON marshaling for Config
func (c Config) MarshalJSON() ([]byte, error) {
	type Alias Config
	return json.Marshal(&struct {
		Alias
		// Convert durations to strings for JSON
		BatchingMaxWait       string `json:"batching_max_wait_str,omitempty"`
		BatchingFlushInterval string `json:"batching_flush_interval_str,omitempty"`
		HTTPTimeout           string `json:"http_timeout_str,omitempty"`
		RetryInitialBackoff   string `json:"retry_initial_backoff_str,omitempty"`
		RetryMaxBackoff       string `json:"retry_max_backoff_str,omitempty"`
	}{
		Alias:                 Alias(c),
		BatchingMaxWait:       c.Batching.MaxWait.String(),
		BatchingFlushInterval: c.Batching.FlushInterval.String(),
		HTTPTimeout:           c.Destination.HTTPTimeout.String(),
		RetryInitialBackoff:   c.Retry.InitialBackoff.String(),
		RetryMaxBackoff:       c.Retry.MaxBackoff.String(),
	})
}
