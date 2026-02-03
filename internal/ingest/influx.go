package ingest

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// InfluxParser parses InfluxDB line protocol
type InfluxParser struct {
	DefaultPrecision string // ns, us, ms, s (default: ns)
}

// ParseLineProtocol parses InfluxDB line protocol format
// Format: measurement,tag1=val1,tag2=val2 field1=val1,field2=val2 timestamp
// Example: cpu,host=server01,region=us-west usage=42.5,system=12.3 1609459200000000000
func (p *InfluxParser) ParseLineProtocol(r io.Reader, precision string) (*Batch, error) {
	if precision == "" {
		precision = p.DefaultPrecision
		if precision == "" {
			precision = "ns"
		}
	}

	batch := &Batch{
		Samples: make([]Sample, 0),
		Source:  "influxdb",
	}

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		samples, err := p.parseLine(line, precision)
		if err != nil {
			// Log but continue
			continue
		}

		batch.Samples = append(batch.Samples, samples...)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanner error: %w", err)
	}

	return batch, nil
}

func (p *InfluxParser) parseLine(line, precision string) ([]Sample, error) {
	var samples []Sample

	// Split into: measurement+tags, fields, timestamp
	// This is tricky because values can contain spaces if quoted

	// Find the measurement and tags (before first unquoted space)
	measurementEnd := findUnquotedSpace(line)
	if measurementEnd == -1 {
		return nil, fmt.Errorf("invalid format: no fields found")
	}

	measurementPart := line[:measurementEnd]
	rest := strings.TrimLeft(line[measurementEnd:], " ")

	// Find fields (before optional timestamp)
	fieldsEnd := findUnquotedSpace(rest)
	var fieldsPart, timestampPart string
	if fieldsEnd == -1 {
		fieldsPart = rest
	} else {
		fieldsPart = rest[:fieldsEnd]
		timestampPart = strings.TrimSpace(rest[fieldsEnd:])
	}

	// Parse measurement and tags
	measurement, tags := p.parseMeasurementAndTags(measurementPart)

	// Parse timestamp
	var ts time.Time
	if timestampPart != "" {
		tsInt, err := strconv.ParseInt(timestampPart, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid timestamp: %w", err)
		}
		ts = p.convertTimestamp(tsInt, precision)
	} else {
		ts = time.Now()
	}

	// Parse fields
	fields, err := p.parseFields(fieldsPart)
	if err != nil {
		return nil, fmt.Errorf("invalid fields: %w", err)
	}

	// Create a sample for each numeric field
	for fieldName, fieldValue := range fields {
		// Only process numeric values
		val, ok := fieldValue.(float64)
		if !ok {
			continue
		}

		// Metric name: measurement_fieldname (unless single field named "value")
		metricName := measurement
		if fieldName != "value" || len(fields) > 1 {
			metricName = measurement + "_" + fieldName
		}

		samples = append(samples, Sample{
			Metric:    metricName,
			Value:     val,
			Timestamp: ts,
			Tags:      copyTags(tags),
		})
	}

	return samples, nil
}

func (p *InfluxParser) parseMeasurementAndTags(s string) (string, map[string]string) {
	tags := make(map[string]string)

	// Split by unescaped commas
	parts := splitUnescaped(s, ',')
	if len(parts) == 0 {
		return s, tags
	}

	measurement := unescapeInflux(parts[0])

	for _, part := range parts[1:] {
		idx := strings.Index(part, "=")
		if idx > 0 {
			key := unescapeInflux(part[:idx])
			value := unescapeInflux(part[idx+1:])
			tags[key] = value
		}
	}

	return measurement, tags
}

func (p *InfluxParser) parseFields(s string) (map[string]interface{}, error) {
	fields := make(map[string]interface{})

	// Split by unescaped commas
	parts := splitUnescaped(s, ',')

	for _, part := range parts {
		idx := strings.Index(part, "=")
		if idx <= 0 {
			continue
		}

		key := unescapeInflux(part[:idx])
		valueStr := part[idx+1:]

		value := p.parseFieldValue(valueStr)
		fields[key] = value
	}

	return fields, nil
}

func (p *InfluxParser) parseFieldValue(s string) interface{} {
	s = strings.TrimSpace(s)

	// String (quoted)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return unescapeInfluxString(s[1 : len(s)-1])
	}

	// Boolean
	if s == "true" || s == "t" || s == "T" || s == "TRUE" {
		return true
	}
	if s == "false" || s == "f" || s == "F" || s == "FALSE" {
		return false
	}

	// Integer (ends with 'i')
	if len(s) > 0 && s[len(s)-1] == 'i' {
		if val, err := strconv.ParseInt(s[:len(s)-1], 10, 64); err == nil {
			return float64(val) // Convert to float for consistency
		}
	}

	// Unsigned integer (ends with 'u')
	if len(s) > 0 && s[len(s)-1] == 'u' {
		if val, err := strconv.ParseUint(s[:len(s)-1], 10, 64); err == nil {
			return float64(val)
		}
	}

	// Float
	if val, err := strconv.ParseFloat(s, 64); err == nil {
		return val
	}

	// Default to string
	return s
}

func (p *InfluxParser) convertTimestamp(ts int64, precision string) time.Time {
	switch precision {
	case "s":
		return time.Unix(ts, 0)
	case "ms":
		return time.Unix(ts/1000, (ts%1000)*1e6)
	case "us", "µs":
		return time.Unix(ts/1e6, (ts%1e6)*1000)
	case "ns":
		return time.Unix(0, ts)
	default:
		return time.Unix(0, ts)
	}
}

func findUnquotedSpace(s string) int {
	inQuote := false
	escape := false

	for i, c := range s {
		if escape {
			escape = false
			continue
		}
		if c == '\\' {
			escape = true
			continue
		}
		if c == '"' {
			inQuote = !inQuote
			continue
		}
		if c == ' ' && !inQuote {
			return i
		}
	}
	return -1
}

func splitUnescaped(s string, sep rune) []string {
	var parts []string
	var current strings.Builder
	escape := false

	for _, c := range s {
		if escape {
			current.WriteRune(c)
			escape = false
			continue
		}
		if c == '\\' {
			current.WriteRune(c)
			escape = true
			continue
		}
		if c == sep {
			parts = append(parts, current.String())
			current.Reset()
			continue
		}
		current.WriteRune(c)
	}

	if current.Len() > 0 {
		parts = append(parts, current.String())
	}

	return parts
}

func unescapeInflux(s string) string {
	// Unescape commas, spaces, and equals signs
	s = strings.ReplaceAll(s, "\\,", ",")
	s = strings.ReplaceAll(s, "\\ ", " ")
	s = strings.ReplaceAll(s, "\\=", "=")
	return s
}

func unescapeInfluxString(s string) string {
	// Unescape quotes and backslashes in strings
	s = strings.ReplaceAll(s, "\\\"", "\"")
	s = strings.ReplaceAll(s, "\\\\", "\\")
	return s
}

func copyTags(tags map[string]string) map[string]string {
	cp := make(map[string]string, len(tags))
	for k, v := range tags {
		cp[k] = v
	}
	return cp
}
