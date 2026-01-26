package otlp

import (
	"time"

	"dogwatch/internal/logs"

	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
)

// processLogs converts OTLP resource logs and stores them
func processLogs(logStore *logs.Store, resourceLogs []*logspb.ResourceLogs) error {
	if logStore == nil {
		return nil
	}

	var entries []logs.LogEntry

	for _, rl := range resourceLogs {
		serviceName, host := extractLogResourceInfo(rl.Resource)

		for _, scopeLogs := range rl.ScopeLogs {
			for _, logRecord := range scopeLogs.LogRecords {
				entry := convertLogRecord(logRecord, serviceName, host)
				entries = append(entries, entry)
			}
		}
	}

	if len(entries) > 0 {
		return logStore.InsertBatch(entries)
	}
	return nil
}

// extractLogResourceInfo extracts service name and host from resource
func extractLogResourceInfo(resource *resourcepb.Resource) (serviceName, host string) {
	if resource == nil {
		return "unknown", ""
	}
	for _, attr := range resource.Attributes {
		switch attr.Key {
		case "service.name":
			serviceName = anyValueToString(attr.Value)
		case "host.name":
			host = anyValueToString(attr.Value)
		}
	}
	if serviceName == "" {
		serviceName = "unknown"
	}
	return
}

// convertLogRecord converts an OTLP LogRecord to internal LogEntry
func convertLogRecord(record *logspb.LogRecord, serviceName, host string) logs.LogEntry {
	ts := time.Unix(0, int64(record.TimeUnixNano))
	if record.TimeUnixNano == 0 && record.ObservedTimeUnixNano != 0 {
		ts = time.Unix(0, int64(record.ObservedTimeUnixNano))
	}

	entry := logs.LogEntry{
		Timestamp: ts,
		Level:     convertSeverity(record.SeverityNumber),
		Message:   getLogBody(record),
		Service:   serviceName,
		Host:      host,
		TraceID:   traceIDToHex(record.TraceId),
		SpanID:    spanIDToHex(record.SpanId),
		Attrs:     convertAttributes(record.Attributes),
	}

	// Add severity text if different from computed level
	if record.SeverityText != "" && entry.Attrs == nil {
		entry.Attrs = make(map[string]string)
	}
	if record.SeverityText != "" {
		entry.Attrs["severity_text"] = record.SeverityText
	}

	return entry
}

// convertSeverity maps OTLP severity number (1-24) to dogwatch LogLevel
// OTLP SeverityNumber ranges:
//   1-4:   TRACE, TRACE2, TRACE3, TRACE4
//   5-8:   DEBUG, DEBUG2, DEBUG3, DEBUG4
//   9-12:  INFO, INFO2, INFO3, INFO4
//   13-16: WARN, WARN2, WARN3, WARN4
//   17-20: ERROR, ERROR2, ERROR3, ERROR4
//   21-24: FATAL, FATAL2, FATAL3, FATAL4
func convertSeverity(severityNumber logspb.SeverityNumber) logs.LogLevel {
	num := int32(severityNumber)
	switch {
	case num <= 4:
		// TRACE level maps to debug
		return logs.LevelDebug
	case num <= 8:
		// DEBUG level
		return logs.LevelDebug
	case num <= 12:
		// INFO level
		return logs.LevelInfo
	case num <= 16:
		// WARN level
		return logs.LevelWarn
	case num <= 20:
		// ERROR level
		return logs.LevelError
	default:
		// FATAL level (21+)
		return logs.LevelFatal
	}
}

// getLogBody extracts the log message from Body field
func getLogBody(record *logspb.LogRecord) string {
	if record.Body == nil {
		return ""
	}
	return anyValueToString(record.Body)
}
