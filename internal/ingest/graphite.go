package ingest

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// GraphiteParser parses Graphite plaintext and pickle protocols
type GraphiteParser struct{}

// ParsePlaintext parses Graphite plaintext format
// Format: metric.path value timestamp\n
// Example: servers.web01.cpu.usage 42.5 1609459200
func (p *GraphiteParser) ParsePlaintext(r io.Reader) (*Batch, error) {
	batch := &Batch{
		Samples: make([]Sample, 0),
		Source:  "graphite",
	}

	scanner := bufio.NewScanner(r)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		sample, err := p.parsePlaintextLine(line)
		if err != nil {
			// Log but continue - don't fail entire batch for one bad line
			continue
		}

		batch.Samples = append(batch.Samples, sample)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanner error: %w", err)
	}

	return batch, nil
}

func (p *GraphiteParser) parsePlaintextLine(line string) (Sample, error) {
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return Sample{}, fmt.Errorf("invalid format: need at least metric and value")
	}

	metric := parts[0]
	value, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return Sample{}, fmt.Errorf("invalid value: %w", err)
	}

	var ts time.Time
	if len(parts) >= 3 {
		tsInt, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			return Sample{}, fmt.Errorf("invalid timestamp: %w", err)
		}
		ts = time.Unix(tsInt, 0)
	} else {
		ts = time.Now()
	}

	// Convert Graphite path to metric name and tags
	name, tags := p.pathToMetricAndTags(metric)

	return Sample{
		Metric:    name,
		Value:     value,
		Timestamp: ts,
		Tags:      tags,
	}, nil
}

// pathToMetricAndTags converts a Graphite path like "servers.web01.cpu.usage"
// into a metric name and tags
func (p *GraphiteParser) pathToMetricAndTags(path string) (string, map[string]string) {
	parts := strings.Split(path, ".")
	tags := make(map[string]string)

	if len(parts) <= 2 {
		return path, tags
	}

	// Common patterns:
	// servers.hostname.metric.submetric -> metric_submetric, host=hostname
	// app.service.metric -> metric, service=service
	// statsd.gauge.metric -> metric, type=gauge

	// Try to detect hostname-like segments
	for i, part := range parts {
		if looksLikeHostname(part) {
			tags["host"] = part
			// Remove from path
			parts = append(parts[:i], parts[i+1:]...)
			break
		}
	}

	// First segment often indicates category
	if len(parts) > 1 {
		category := parts[0]
		switch category {
		case "servers", "hosts", "nodes":
			// Already handled hostname above
			parts = parts[1:]
		case "apps", "services", "app", "service":
			if len(parts) > 2 {
				tags["service"] = parts[1]
				parts = parts[2:]
			}
		case "statsd":
			if len(parts) > 2 {
				tags["type"] = parts[1]
				parts = parts[2:]
			}
		}
	}

	// Remaining parts form the metric name
	metricName := strings.Join(parts, "_")
	if metricName == "" {
		metricName = path
	}

	return metricName, tags
}

func looksLikeHostname(s string) bool {
	// Hostnames often contain numbers or specific patterns
	if strings.Contains(s, "-") || strings.Contains(s, "_") {
		return true
	}
	// Check for patterns like web01, db02, etc.
	for i, c := range s {
		if c >= '0' && c <= '9' && i > 0 {
			return true
		}
	}
	return false
}

// ParsePickle parses Graphite pickle protocol (Python pickle format)
// Format: 4-byte big-endian length + pickled list of (path, (timestamp, value)) tuples
func (p *GraphiteParser) ParsePickle(r io.Reader) (*Batch, error) {
	batch := &Batch{
		Samples: make([]Sample, 0),
		Source:  "graphite-pickle",
	}

	// Read length prefix
	var length uint32
	if err := binary.Read(r, binary.BigEndian, &length); err != nil {
		if err == io.EOF {
			return batch, nil
		}
		return nil, fmt.Errorf("failed to read pickle length: %w", err)
	}

	if length > 10*1024*1024 { // 10MB max
		return nil, fmt.Errorf("pickle payload too large: %d bytes", length)
	}

	// Read pickle data
	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, fmt.Errorf("failed to read pickle data: %w", err)
	}

	// Parse pickle (simplified - handles common Carbon formats)
	samples, err := p.parsePickleData(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse pickle: %w", err)
	}

	batch.Samples = samples
	return batch, nil
}

// parsePickleData parses Python pickle format used by Carbon
// This is a simplified parser that handles the common pickle opcodes
func (p *GraphiteParser) parsePickleData(data []byte) ([]Sample, error) {
	var samples []Sample
	r := bytes.NewReader(data)

	// Pickle parsing state
	stack := make([]interface{}, 0)
	memo := make(map[int]interface{})
	mark := -1

	for {
		op, err := r.ReadByte()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		switch op {
		case 0x80: // PROTO
			r.ReadByte() // version
		case '(': // MARK
			mark = len(stack)
		case 'l': // LIST
			if mark >= 0 {
				list := make([]interface{}, len(stack)-mark)
				copy(list, stack[mark:])
				stack = stack[:mark]
				stack = append(stack, list)
				mark = -1
			}
		case 't': // TUPLE
			if mark >= 0 {
				tuple := make([]interface{}, len(stack)-mark)
				copy(tuple, stack[mark:])
				stack = stack[:mark]
				stack = append(stack, tuple)
				mark = -1
			}
		case 'S', 'V': // STRING, UNICODE
			str, _ := readPickleString(r)
			stack = append(stack, str)
		case 'U': // SHORT_BINSTRING
			length, _ := r.ReadByte()
			buf := make([]byte, length)
			r.Read(buf)
			stack = append(stack, string(buf))
		case 'X': // BINUNICODE
			var length uint32
			binary.Read(r, binary.LittleEndian, &length)
			buf := make([]byte, length)
			r.Read(buf)
			stack = append(stack, string(buf))
		case 'I': // INT
			str, _ := readPickleLine(r)
			val, _ := strconv.ParseInt(str, 10, 64)
			stack = append(stack, val)
		case 'F': // FLOAT
			str, _ := readPickleLine(r)
			val, _ := strconv.ParseFloat(str, 64)
			stack = append(stack, val)
		case 'G': // BINFLOAT
			var val float64
			binary.Read(r, binary.BigEndian, &val)
			stack = append(stack, val)
		case 'J': // BININT
			var val int32
			binary.Read(r, binary.LittleEndian, &val)
			stack = append(stack, int64(val))
		case 'K': // BININT1
			val, _ := r.ReadByte()
			stack = append(stack, int64(val))
		case 'M': // BININT2
			var val uint16
			binary.Read(r, binary.LittleEndian, &val)
			stack = append(stack, int64(val))
		case 'N': // NONE
			stack = append(stack, nil)
		case 'p': // PUT
			idx, _ := readPickleLine(r)
			i, _ := strconv.Atoi(idx)
			if len(stack) > 0 {
				memo[i] = stack[len(stack)-1]
			}
		case 'q': // BINPUT
			i, _ := r.ReadByte()
			if len(stack) > 0 {
				memo[int(i)] = stack[len(stack)-1]
			}
		case 'g': // GET
			idx, _ := readPickleLine(r)
			i, _ := strconv.Atoi(idx)
			stack = append(stack, memo[i])
		case 'h': // BINGET
			i, _ := r.ReadByte()
			stack = append(stack, memo[int(i)])
		case 'a': // APPEND
			if len(stack) >= 2 {
				val := stack[len(stack)-1]
				list := stack[len(stack)-2].([]interface{})
				stack = stack[:len(stack)-2]
				stack = append(stack, append(list, val))
			}
		case 'e': // APPENDS
			if mark >= 0 && mark < len(stack) {
				list := stack[mark-1].([]interface{})
				items := stack[mark:]
				stack = stack[:mark-1]
				stack = append(stack, append(list, items...))
				mark = -1
			}
		case ']': // EMPTY_LIST
			stack = append(stack, make([]interface{}, 0))
		case ')': // EMPTY_TUPLE
			stack = append(stack, make([]interface{}, 0))
		case '.': // STOP
			break
		}
	}

	// Convert stack to samples
	// Expected format: list of (metric_path, (timestamp, value)) tuples
	if len(stack) > 0 {
		if list, ok := stack[0].([]interface{}); ok {
			for _, item := range list {
				if tuple, ok := item.([]interface{}); ok && len(tuple) >= 2 {
					path, _ := tuple[0].(string)
					if datapoints, ok := tuple[1].([]interface{}); ok {
						for _, dp := range datapoints {
							if point, ok := dp.([]interface{}); ok && len(point) >= 2 {
								ts := toInt64(point[0])
								val := toFloat64(point[1])
								name, tags := p.pathToMetricAndTags(path)
								samples = append(samples, Sample{
									Metric:    name,
									Value:     val,
									Timestamp: time.Unix(ts, 0),
									Tags:      tags,
								})
							}
						}
					} else if point, ok := tuple[1].([]interface{}); ok && len(point) >= 2 {
						// Single datapoint: (path, (timestamp, value))
						ts := toInt64(point[0])
						val := toFloat64(point[1])
						name, tags := p.pathToMetricAndTags(path)
						samples = append(samples, Sample{
							Metric:    name,
							Value:     val,
							Timestamp: time.Unix(ts, 0),
							Tags:      tags,
						})
					}
				}
			}
		}
	}

	return samples, nil
}

func readPickleString(r *bytes.Reader) (string, error) {
	var buf bytes.Buffer
	for {
		b, err := r.ReadByte()
		if err != nil {
			return buf.String(), err
		}
		if b == '\n' {
			break
		}
		buf.WriteByte(b)
	}
	s := buf.String()
	// Remove quotes if present
	if len(s) >= 2 && (s[0] == '\'' || s[0] == '"') {
		s = s[1 : len(s)-1]
	}
	return s, nil
}

func readPickleLine(r *bytes.Reader) (string, error) {
	var buf bytes.Buffer
	for {
		b, err := r.ReadByte()
		if err != nil {
			return buf.String(), err
		}
		if b == '\n' {
			break
		}
		buf.WriteByte(b)
	}
	return buf.String(), nil
}

func toInt64(v interface{}) int64 {
	switch val := v.(type) {
	case int64:
		return val
	case int:
		return int64(val)
	case float64:
		return int64(val)
	}
	return 0
}

func toFloat64(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case int64:
		return float64(val)
	case int:
		return float64(val)
	}
	return 0
}
