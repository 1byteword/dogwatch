package scripts

// MySQLScripts contains MySQL database analysis scripts
var MySQLScripts = []*Script{
	{
		Name:        "mysql/slow_queries",
		Category:    "mysql",
		Title:       "Slow MySQL Queries",
		Description: "Find MySQL queries exceeding the duration threshold",
		Query: `
			SELECT
				message as query,
				avg(duration_ms) as avg_latency,
				count(*) as occurrences,
				max(duration_ms) as max_latency
			FROM logs
			WHERE service LIKE '%mysql%'
			  AND duration_ms > {{.threshold}}
			GROUP BY message
			ORDER BY avg_latency DESC
			LIMIT {{.limit}}
		`,
		Parameters: []Parameter{
			{Name: "threshold", Type: "int", Default: 100, Description: "Minimum query duration in milliseconds"},
			{Name: "limit", Type: "int", Default: 20, Description: "Maximum results to return"},
		},
		Columns: []Column{
			{Name: "query", Type: "string"},
			{Name: "avg_latency", Type: "duration"},
			{Name: "occurrences", Type: "int"},
			{Name: "max_latency", Type: "duration"},
		},
	},
	{
		Name:        "mysql/connections",
		Category:    "mysql",
		Title:       "MySQL Connection Statistics",
		Description: "Analyze MySQL connection patterns and counts",
		Query: `
			SELECT
				service,
				host,
				count(*) as connections,
				avg(duration_ms) as avg_query_time
			FROM traces
			WHERE service LIKE '%mysql%'
			   OR name LIKE '%mysql%'
			GROUP BY service, host
			ORDER BY connections DESC
			LIMIT {{.limit}}
		`,
		Parameters: []Parameter{
			{Name: "limit", Type: "int", Default: 20, Description: "Maximum results to return"},
		},
		Columns: []Column{
			{Name: "service", Type: "string"},
			{Name: "host", Type: "string"},
			{Name: "connections", Type: "int"},
			{Name: "avg_query_time", Type: "duration"},
		},
	},
	{
		Name:        "mysql/errors",
		Category:    "mysql",
		Title:       "MySQL Errors",
		Description: "Find MySQL error patterns and frequencies",
		Query: `
			SELECT
				message,
				level,
				count(*) as occurrences,
				max(timestamp) as last_seen
			FROM logs
			WHERE service LIKE '%mysql%'
			  AND level IN ('ERROR', 'error', 'WARN', 'warn')
			GROUP BY message, level
			ORDER BY occurrences DESC
			LIMIT {{.limit}}
		`,
		Parameters: []Parameter{
			{Name: "limit", Type: "int", Default: 20, Description: "Maximum results to return"},
		},
		Columns: []Column{
			{Name: "message", Type: "string"},
			{Name: "level", Type: "string"},
			{Name: "occurrences", Type: "int"},
			{Name: "last_seen", Type: "string"},
		},
	},
	{
		Name:        "mysql/query_patterns",
		Category:    "mysql",
		Title:       "MySQL Query Patterns",
		Description: "Analyze common MySQL query patterns by operation type",
		Query: `
			SELECT
				CASE
					WHEN message LIKE 'SELECT%' THEN 'SELECT'
					WHEN message LIKE 'INSERT%' THEN 'INSERT'
					WHEN message LIKE 'UPDATE%' THEN 'UPDATE'
					WHEN message LIKE 'DELETE%' THEN 'DELETE'
					ELSE 'OTHER'
				END as operation,
				count(*) as count,
				avg(duration_ms) as avg_latency,
				p95(duration_ms) as p95_latency
			FROM logs
			WHERE service LIKE '%mysql%'
			GROUP BY operation
			ORDER BY count DESC
		`,
		Parameters: []Parameter{},
		Columns: []Column{
			{Name: "operation", Type: "string"},
			{Name: "count", Type: "int"},
			{Name: "avg_latency", Type: "duration"},
			{Name: "p95_latency", Type: "duration"},
		},
	},
}

// PostgresScripts contains PostgreSQL database analysis scripts
var PostgresScripts = []*Script{
	{
		Name:        "postgres/slow_queries",
		Category:    "postgres",
		Title:       "Slow PostgreSQL Queries",
		Description: "Find PostgreSQL queries exceeding the duration threshold",
		Query: `
			SELECT
				message as query,
				avg(duration_ms) as avg_latency,
				count(*) as occurrences,
				max(duration_ms) as max_latency
			FROM logs
			WHERE service LIKE '%postgres%' OR service LIKE '%pg%'
			  AND duration_ms > {{.threshold}}
			GROUP BY message
			ORDER BY avg_latency DESC
			LIMIT {{.limit}}
		`,
		Parameters: []Parameter{
			{Name: "threshold", Type: "int", Default: 100, Description: "Minimum query duration in milliseconds"},
			{Name: "limit", Type: "int", Default: 20, Description: "Maximum results to return"},
		},
		Columns: []Column{
			{Name: "query", Type: "string"},
			{Name: "avg_latency", Type: "duration"},
			{Name: "occurrences", Type: "int"},
			{Name: "max_latency", Type: "duration"},
		},
	},
	{
		Name:        "postgres/connections",
		Category:    "postgres",
		Title:       "PostgreSQL Connection Statistics",
		Description: "Analyze PostgreSQL connection patterns",
		Query: `
			SELECT
				service,
				host,
				count(*) as connections,
				avg(duration_ms) as avg_query_time
			FROM traces
			WHERE service LIKE '%postgres%' OR service LIKE '%pg%'
			GROUP BY service, host
			ORDER BY connections DESC
			LIMIT {{.limit}}
		`,
		Parameters: []Parameter{
			{Name: "limit", Type: "int", Default: 20, Description: "Maximum results to return"},
		},
		Columns: []Column{
			{Name: "service", Type: "string"},
			{Name: "host", Type: "string"},
			{Name: "connections", Type: "int"},
			{Name: "avg_query_time", Type: "duration"},
		},
	},
}

// RedisScripts contains Redis analysis scripts
var RedisScripts = []*Script{
	{
		Name:        "redis/slow_commands",
		Category:    "redis",
		Title:       "Slow Redis Commands",
		Description: "Find Redis commands exceeding the latency threshold",
		Query: `
			SELECT
				message as command,
				avg(duration_ms) as avg_latency,
				count(*) as occurrences,
				max(duration_ms) as max_latency
			FROM logs
			WHERE service LIKE '%redis%'
			  AND duration_ms > {{.threshold}}
			GROUP BY message
			ORDER BY avg_latency DESC
			LIMIT {{.limit}}
		`,
		Parameters: []Parameter{
			{Name: "threshold", Type: "int", Default: 10, Description: "Minimum command duration in milliseconds"},
			{Name: "limit", Type: "int", Default: 20, Description: "Maximum results to return"},
		},
		Columns: []Column{
			{Name: "command", Type: "string"},
			{Name: "avg_latency", Type: "duration"},
			{Name: "occurrences", Type: "int"},
			{Name: "max_latency", Type: "duration"},
		},
	},
	{
		Name:        "redis/command_stats",
		Category:    "redis",
		Title:       "Redis Command Statistics",
		Description: "Analyze Redis command distribution and performance",
		Query: `
			SELECT
				name as command,
				count(*) as calls,
				avg(duration_ms) as avg_latency,
				p99(duration_ms) as p99_latency
			FROM traces
			WHERE service LIKE '%redis%'
			GROUP BY name
			ORDER BY calls DESC
			LIMIT {{.limit}}
		`,
		Parameters: []Parameter{
			{Name: "limit", Type: "int", Default: 20, Description: "Maximum results to return"},
		},
		Columns: []Column{
			{Name: "command", Type: "string"},
			{Name: "calls", Type: "int"},
			{Name: "avg_latency", Type: "duration"},
			{Name: "p99_latency", Type: "duration"},
		},
	},
}
