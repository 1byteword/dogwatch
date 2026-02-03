package scripts

// SecurityScripts contains security-focused analysis scripts
var SecurityScripts = []*Script{
	{
		Name:        "security/outbound_connections",
		Category:    "security",
		Title:       "Outbound Connections",
		Description: "Analyze outbound network connections by destination",
		Query: `
			SELECT
				service,
				name as destination,
				count(*) as connections,
				avg(duration_ms) as avg_latency
			FROM traces
			WHERE kind = 'CLIENT'
			GROUP BY service, name
			ORDER BY connections DESC
			LIMIT {{.limit}}
		`,
		Parameters: []Parameter{
			{Name: "limit", Type: "int", Default: 50, Description: "Maximum results to return"},
		},
		Columns: []Column{
			{Name: "service", Type: "string"},
			{Name: "destination", Type: "string"},
			{Name: "connections", Type: "int"},
			{Name: "avg_latency", Type: "duration"},
		},
	},
	{
		Name:        "security/unusual_ports",
		Category:    "security",
		Title:       "Unusual Port Connections",
		Description: "Find connections to non-standard ports",
		Query: `
			SELECT
				service,
				name as endpoint,
				count(*) as connections,
				timestamp
			FROM traces
			WHERE kind = 'CLIENT'
			  AND name NOT LIKE '%:80%'
			  AND name NOT LIKE '%:443%'
			  AND name NOT LIKE '%:8080%'
			  AND name NOT LIKE '%:8443%'
			GROUP BY service, name, timestamp
			ORDER BY connections DESC
			LIMIT {{.limit}}
		`,
		Parameters: []Parameter{
			{Name: "limit", Type: "int", Default: 50, Description: "Maximum results to return"},
		},
		Columns: []Column{
			{Name: "service", Type: "string"},
			{Name: "endpoint", Type: "string"},
			{Name: "connections", Type: "int"},
			{Name: "timestamp", Type: "string"},
		},
	},
	{
		Name:        "security/error_spikes",
		Category:    "security",
		Title:       "Error Rate Spikes",
		Description: "Detect unusual error rate increases",
		Query: `
			SELECT
				service,
				time_bucket('5m', timestamp) as window,
				count(*) as total,
				sum(CASE WHEN status = 'ERROR' THEN 1 ELSE 0 END) as errors,
				(sum(CASE WHEN status = 'ERROR' THEN 1 ELSE 0 END) * 100.0 / count(*)) as error_rate
			FROM traces
			GROUP BY service, window
			HAVING error_rate > {{.threshold}}
			ORDER BY window DESC, error_rate DESC
			LIMIT {{.limit}}
		`,
		Parameters: []Parameter{
			{Name: "threshold", Type: "float", Default: 10.0, Description: "Error rate threshold percentage"},
			{Name: "limit", Type: "int", Default: 50, Description: "Maximum results to return"},
		},
		Columns: []Column{
			{Name: "service", Type: "string"},
			{Name: "window", Type: "string"},
			{Name: "total", Type: "int"},
			{Name: "errors", Type: "int"},
			{Name: "error_rate", Type: "float"},
		},
	},
	{
		Name:        "security/auth_failures",
		Category:    "security",
		Title:       "Authentication Failures",
		Description: "Find authentication and authorization failures",
		Query: `
			SELECT
				service,
				message,
				level,
				count(*) as occurrences,
				max(timestamp) as last_seen
			FROM logs
			WHERE (message LIKE '%auth%' OR message LIKE '%login%' OR message LIKE '%401%' OR message LIKE '%403%')
			  AND (level = 'ERROR' OR level = 'WARN' OR level = 'error' OR level = 'warn')
			GROUP BY service, message, level
			ORDER BY occurrences DESC
			LIMIT {{.limit}}
		`,
		Parameters: []Parameter{
			{Name: "limit", Type: "int", Default: 50, Description: "Maximum results to return"},
		},
		Columns: []Column{
			{Name: "service", Type: "string"},
			{Name: "message", Type: "string"},
			{Name: "level", Type: "string"},
			{Name: "occurrences", Type: "int"},
			{Name: "last_seen", Type: "string"},
		},
	},
	{
		Name:        "security/sensitive_data",
		Category:    "security",
		Title:       "Potential Sensitive Data Exposure",
		Description: "Find logs that may contain sensitive data patterns",
		Query: `
			SELECT
				service,
				message,
				timestamp
			FROM logs
			WHERE message MATCHES '(password|secret|token|api_key|apikey|auth|credential)'
			LIMIT {{.limit}}
		`,
		Parameters: []Parameter{
			{Name: "limit", Type: "int", Default: 100, Description: "Maximum results to return"},
		},
		Columns: []Column{
			{Name: "service", Type: "string"},
			{Name: "message", Type: "string"},
			{Name: "timestamp", Type: "string"},
		},
	},
}

// ServiceScripts contains service-level analysis scripts
var ServiceScripts = []*Script{
	{
		Name:        "service/dependencies",
		Category:    "service",
		Title:       "Service Dependencies",
		Description: "Map service dependencies from trace data",
		Query: `
			SELECT
				service as source,
				name as destination,
				count(*) as calls,
				avg(duration_ms) as avg_latency,
				sum(CASE WHEN status = 'ERROR' THEN 1 ELSE 0 END) as errors
			FROM traces
			WHERE kind = 'CLIENT'
			GROUP BY service, name
			ORDER BY calls DESC
			LIMIT {{.limit}}
		`,
		Parameters: []Parameter{
			{Name: "limit", Type: "int", Default: 100, Description: "Maximum results to return"},
		},
		Columns: []Column{
			{Name: "source", Type: "string"},
			{Name: "destination", Type: "string"},
			{Name: "calls", Type: "int"},
			{Name: "avg_latency", Type: "duration"},
			{Name: "errors", Type: "int"},
		},
	},
	{
		Name:        "service/error_rates",
		Category:    "service",
		Title:       "Service Error Rates",
		Description: "Calculate error rates across all services",
		Query: `
			SELECT
				service,
				count(*) as total,
				sum(CASE WHEN status = 'ERROR' THEN 1 ELSE 0 END) as errors,
				(sum(CASE WHEN status = 'ERROR' THEN 1 ELSE 0 END) * 100.0 / count(*)) as error_rate
			FROM traces
			GROUP BY service
			ORDER BY error_rate DESC
			LIMIT {{.limit}}
		`,
		Parameters: []Parameter{
			{Name: "limit", Type: "int", Default: 50, Description: "Maximum results to return"},
		},
		Columns: []Column{
			{Name: "service", Type: "string"},
			{Name: "total", Type: "int"},
			{Name: "errors", Type: "int"},
			{Name: "error_rate", Type: "float"},
		},
	},
	{
		Name:        "service/latency",
		Category:    "service",
		Title:       "Service Latency Statistics",
		Description: "Analyze latency distribution across services",
		Query: `
			SELECT
				service,
				count(*) as requests,
				avg(duration_ms) as avg_latency,
				p50(duration_ms) as p50,
				p95(duration_ms) as p95,
				p99(duration_ms) as p99,
				max(duration_ms) as max_latency
			FROM traces
			WHERE kind = 'SERVER'
			GROUP BY service
			ORDER BY p99 DESC
			LIMIT {{.limit}}
		`,
		Parameters: []Parameter{
			{Name: "limit", Type: "int", Default: 50, Description: "Maximum results to return"},
		},
		Columns: []Column{
			{Name: "service", Type: "string"},
			{Name: "requests", Type: "int"},
			{Name: "avg_latency", Type: "duration"},
			{Name: "p50", Type: "duration"},
			{Name: "p95", Type: "duration"},
			{Name: "p99", Type: "duration"},
			{Name: "max_latency", Type: "duration"},
		},
	},
	{
		Name:        "service/throughput",
		Category:    "service",
		Title:       "Service Throughput",
		Description: "Measure request throughput per service",
		Query: `
			SELECT
				service,
				time_bucket('1m', timestamp) as window,
				count(*) as requests,
				avg(duration_ms) as avg_latency
			FROM traces
			WHERE kind = 'SERVER'
			GROUP BY service, window
			ORDER BY window DESC, requests DESC
			LIMIT {{.limit}}
		`,
		Parameters: []Parameter{
			{Name: "limit", Type: "int", Default: 100, Description: "Maximum results to return"},
		},
		Columns: []Column{
			{Name: "service", Type: "string"},
			{Name: "window", Type: "string"},
			{Name: "requests", Type: "int"},
			{Name: "avg_latency", Type: "duration"},
		},
	},
}

// TraceScripts contains distributed tracing analysis scripts
var TraceScripts = []*Script{
	{
		Name:        "trace/slow_traces",
		Category:    "trace",
		Title:       "Slow Traces",
		Description: "Find traces exceeding the duration threshold",
		Query: `
			SELECT
				trace_id,
				service,
				name as operation,
				duration_ms as latency,
				status,
				timestamp
			FROM traces
			WHERE duration_ms > {{.threshold}}
			ORDER BY duration_ms DESC
			LIMIT {{.limit}}
		`,
		Parameters: []Parameter{
			{Name: "threshold", Type: "int", Default: 1000, Description: "Minimum trace duration in milliseconds"},
			{Name: "limit", Type: "int", Default: 50, Description: "Maximum results to return"},
		},
		Columns: []Column{
			{Name: "trace_id", Type: "string"},
			{Name: "service", Type: "string"},
			{Name: "operation", Type: "string"},
			{Name: "latency", Type: "duration"},
			{Name: "status", Type: "string"},
			{Name: "timestamp", Type: "string"},
		},
	},
	{
		Name:        "trace/error_traces",
		Category:    "trace",
		Title:       "Error Traces",
		Description: "Find traces with errors",
		Query: `
			SELECT
				trace_id,
				service,
				name as operation,
				duration_ms as latency,
				status_message as error,
				timestamp
			FROM traces
			WHERE status = 'ERROR'
			ORDER BY timestamp DESC
			LIMIT {{.limit}}
		`,
		Parameters: []Parameter{
			{Name: "limit", Type: "int", Default: 50, Description: "Maximum results to return"},
		},
		Columns: []Column{
			{Name: "trace_id", Type: "string"},
			{Name: "service", Type: "string"},
			{Name: "operation", Type: "string"},
			{Name: "latency", Type: "duration"},
			{Name: "error", Type: "string"},
			{Name: "timestamp", Type: "string"},
		},
	},
	{
		Name:        "trace/span_counts",
		Category:    "trace",
		Title:       "Span Count by Service",
		Description: "Count spans per service to identify chattiness",
		Query: `
			SELECT
				service,
				count(*) as span_count,
				avg(duration_ms) as avg_span_duration
			FROM traces
			GROUP BY service
			ORDER BY span_count DESC
			LIMIT {{.limit}}
		`,
		Parameters: []Parameter{
			{Name: "limit", Type: "int", Default: 50, Description: "Maximum results to return"},
		},
		Columns: []Column{
			{Name: "service", Type: "string"},
			{Name: "span_count", Type: "int"},
			{Name: "avg_span_duration", Type: "duration"},
		},
	},
}

// LogScripts contains log analysis scripts
var LogScripts = []*Script{
	{
		Name:        "log/errors",
		Category:    "log",
		Title:       "Recent Errors",
		Description: "Find recent error log entries",
		Query: `
			SELECT
				service,
				message,
				level,
				timestamp
			FROM logs
			WHERE level IN ('ERROR', 'error', 'FATAL', 'fatal')
			ORDER BY timestamp DESC
			LIMIT {{.limit}}
		`,
		Parameters: []Parameter{
			{Name: "limit", Type: "int", Default: 100, Description: "Maximum results to return"},
		},
		Columns: []Column{
			{Name: "service", Type: "string"},
			{Name: "message", Type: "string"},
			{Name: "level", Type: "string"},
			{Name: "timestamp", Type: "string"},
		},
	},
	{
		Name:        "log/error_patterns",
		Category:    "log",
		Title:       "Error Patterns",
		Description: "Group and count error patterns",
		Query: `
			SELECT
				service,
				message,
				count(*) as occurrences,
				min(timestamp) as first_seen,
				max(timestamp) as last_seen
			FROM logs
			WHERE level IN ('ERROR', 'error')
			GROUP BY service, message
			ORDER BY occurrences DESC
			LIMIT {{.limit}}
		`,
		Parameters: []Parameter{
			{Name: "limit", Type: "int", Default: 50, Description: "Maximum results to return"},
		},
		Columns: []Column{
			{Name: "service", Type: "string"},
			{Name: "message", Type: "string"},
			{Name: "occurrences", Type: "int"},
			{Name: "first_seen", Type: "string"},
			{Name: "last_seen", Type: "string"},
		},
	},
	{
		Name:        "log/volume",
		Category:    "log",
		Title:       "Log Volume by Service",
		Description: "Analyze log volume across services",
		Query: `
			SELECT
				service,
				level,
				count(*) as log_count
			FROM logs
			GROUP BY service, level
			ORDER BY log_count DESC
			LIMIT {{.limit}}
		`,
		Parameters: []Parameter{
			{Name: "limit", Type: "int", Default: 100, Description: "Maximum results to return"},
		},
		Columns: []Column{
			{Name: "service", Type: "string"},
			{Name: "level", Type: "string"},
			{Name: "log_count", Type: "int"},
		},
	},
	{
		Name:        "log/search",
		Category:    "log",
		Title:       "Log Search",
		Description: "Search logs for a specific pattern",
		Query: `
			SELECT
				service,
				message,
				level,
				timestamp
			FROM logs
			WHERE message LIKE '%{{.pattern}}%'
			ORDER BY timestamp DESC
			LIMIT {{.limit}}
		`,
		Parameters: []Parameter{
			{Name: "pattern", Type: "string", Default: "", Description: "Search pattern", Required: true},
			{Name: "limit", Type: "int", Default: 100, Description: "Maximum results to return"},
		},
		Columns: []Column{
			{Name: "service", Type: "string"},
			{Name: "message", Type: "string"},
			{Name: "level", Type: "string"},
			{Name: "timestamp", Type: "string"},
		},
	},
}
