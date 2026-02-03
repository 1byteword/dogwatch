package scripts

// HTTPScripts contains HTTP traffic analysis scripts
var HTTPScripts = []*Script{
	{
		Name:        "http/slow_requests",
		Category:    "http",
		Title:       "Slow HTTP Requests",
		Description: "Find HTTP requests exceeding the latency threshold",
		Query: `
			SELECT service, name as endpoint, duration_ms as latency, status, trace_id
			FROM traces
			WHERE duration_ms > {{.threshold}}
			  AND kind = 'SERVER'
			ORDER BY duration_ms DESC
			LIMIT {{.limit}}
		`,
		Parameters: []Parameter{
			{Name: "threshold", Type: "int", Default: 500, Description: "Minimum latency in milliseconds"},
			{Name: "limit", Type: "int", Default: 50, Description: "Maximum results to return"},
		},
		Columns: []Column{
			{Name: "service", Type: "string"},
			{Name: "endpoint", Type: "string"},
			{Name: "latency", Type: "duration"},
			{Name: "status", Type: "string"},
			{Name: "trace_id", Type: "string"},
		},
	},
	{
		Name:        "http/error_rates",
		Category:    "http",
		Title:       "HTTP Error Rates",
		Description: "Calculate error rates by service and endpoint",
		Query: `
			SELECT
				service,
				name as endpoint,
				count(*) as total,
				sum(CASE WHEN status = 'ERROR' THEN 1 ELSE 0 END) as errors,
				(sum(CASE WHEN status = 'ERROR' THEN 1 ELSE 0 END) * 100.0 / count(*)) as error_rate
			FROM traces
			WHERE kind = 'SERVER'
			GROUP BY service, name
			HAVING errors > 0
			ORDER BY error_rate DESC
			LIMIT {{.limit}}
		`,
		Parameters: []Parameter{
			{Name: "limit", Type: "int", Default: 50, Description: "Maximum results to return"},
		},
		Columns: []Column{
			{Name: "service", Type: "string"},
			{Name: "endpoint", Type: "string"},
			{Name: "total", Type: "int"},
			{Name: "errors", Type: "int"},
			{Name: "error_rate", Type: "float"},
		},
	},
	{
		Name:        "http/status_codes",
		Category:    "http",
		Title:       "HTTP Status Code Distribution",
		Description: "Analyze HTTP status code distribution by service",
		Query: `
			SELECT
				service,
				status,
				count(*) as count
			FROM traces
			WHERE kind = 'SERVER'
			GROUP BY service, status
			ORDER BY service, count DESC
			LIMIT {{.limit}}
		`,
		Parameters: []Parameter{
			{Name: "limit", Type: "int", Default: 100, Description: "Maximum results to return"},
		},
		Columns: []Column{
			{Name: "service", Type: "string"},
			{Name: "status", Type: "string"},
			{Name: "count", Type: "int"},
		},
	},
	{
		Name:        "http/latency_by_endpoint",
		Category:    "http",
		Title:       "Endpoint Latency Statistics",
		Description: "Calculate latency percentiles by endpoint",
		Query: `
			SELECT
				service,
				name as endpoint,
				count(*) as requests,
				avg(duration_ms) as avg_latency,
				p50(duration_ms) as p50,
				p95(duration_ms) as p95,
				p99(duration_ms) as p99
			FROM traces
			WHERE kind = 'SERVER'
			GROUP BY service, name
			ORDER BY avg_latency DESC
			LIMIT {{.limit}}
		`,
		Parameters: []Parameter{
			{Name: "limit", Type: "int", Default: 50, Description: "Maximum results to return"},
		},
		Columns: []Column{
			{Name: "service", Type: "string"},
			{Name: "endpoint", Type: "string"},
			{Name: "requests", Type: "int"},
			{Name: "avg_latency", Type: "duration"},
			{Name: "p50", Type: "duration"},
			{Name: "p95", Type: "duration"},
			{Name: "p99", Type: "duration"},
		},
	},
	{
		Name:        "http/throughput",
		Category:    "http",
		Title:       "Request Throughput",
		Description: "Analyze request throughput by service over time",
		Query: `
			SELECT
				service,
				time_bucket('1m', timestamp) as bucket,
				count(*) as requests,
				avg(duration_ms) as avg_latency
			FROM traces
			WHERE kind = 'SERVER'
			GROUP BY service, bucket
			ORDER BY bucket DESC, requests DESC
			LIMIT {{.limit}}
		`,
		Parameters: []Parameter{
			{Name: "limit", Type: "int", Default: 100, Description: "Maximum results to return"},
		},
		Columns: []Column{
			{Name: "service", Type: "string"},
			{Name: "bucket", Type: "string"},
			{Name: "requests", Type: "int"},
			{Name: "avg_latency", Type: "duration"},
		},
	},
}
