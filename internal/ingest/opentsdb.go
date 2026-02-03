package ingest

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// OpenTSDBParser parses OpenTSDB HTTP and telnet protocols
type OpenTSDBParser struct{}

// OpenTSDBDatapoint represents a single OpenTSDB JSON datapoint
type OpenTSDBDatapoint struct {
	Metric    string            `json:"metric"`
	Timestamp int64             `json:"timestamp"`
	Value     interface{}       `json:"value"` // Can be int or float
	Tags      map[string]string `json:"tags"`
}

// ParseHTTP parses OpenTSDB HTTP JSON format
// Format: Single object or array of objects
// Example: {"metric":"sys.cpu","timestamp":1609459200,"value":42.5,"tags":{"host":"web01"}}
func (p *OpenTSDBParser) ParseHTTP(r io.Reader) (*Batch, error) {
	batch := &Batch{
		Samples: make([]Sample, 0),
		Source:  "opentsdb",
	}

	// Read all data
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read body: %w", err)
	}

	if len(data) == 0 {
		return batch, nil
	}

	// Determine if array or single object
	data = trimBOM(data)
	data = []byte(strings.TrimSpace(string(data)))

	if len(data) == 0 {
		return batch, nil
	}

	var datapoints []OpenTSDBDatapoint

	if data[0] == '[' {
		// Array of datapoints
		if err := json.Unmarshal(data, &datapoints); err != nil {
			return nil, fmt.Errorf("failed to parse JSON array: %w", err)
		}
	} else if data[0] == '{' {
		// Single datapoint
		var dp OpenTSDBDatapoint
		if err := json.Unmarshal(data, &dp); err != nil {
			return nil, fmt.Errorf("failed to parse JSON object: %w", err)
		}
		datapoints = []OpenTSDBDatapoint{dp}
	} else {
		return nil, fmt.Errorf("invalid JSON: expected array or object")
	}

	for _, dp := range datapoints {
		sample, err := p.datapointToSample(dp)
		if err != nil {
			continue // Skip invalid datapoints
		}
		batch.Samples = append(batch.Samples, sample)
	}

	return batch, nil
}

func (p *OpenTSDBParser) datapointToSample(dp OpenTSDBDatapoint) (Sample, error) {
	if dp.Metric == "" {
		return Sample{}, fmt.Errorf("missing metric name")
	}

	// Parse value
	var value float64
	switch v := dp.Value.(type) {
	case float64:
		value = v
	case int:
		value = float64(v)
	case int64:
		value = float64(v)
	case string:
		var err error
		value, err = strconv.ParseFloat(v, 64)
		if err != nil {
			return Sample{}, fmt.Errorf("invalid value: %w", err)
		}
	default:
		return Sample{}, fmt.Errorf("invalid value type")
	}

	// Parse timestamp (can be seconds or milliseconds)
	var ts time.Time
	if dp.Timestamp > 0 {
		if dp.Timestamp > 1e12 { // Milliseconds
			ts = time.Unix(dp.Timestamp/1000, (dp.Timestamp%1000)*1e6)
		} else { // Seconds
			ts = time.Unix(dp.Timestamp, 0)
		}
	} else {
		ts = time.Now()
	}

	// Ensure tags is not nil
	tags := dp.Tags
	if tags == nil {
		tags = make(map[string]string)
	}

	return Sample{
		Metric:    p.normalizeMetricName(dp.Metric),
		Value:     value,
		Timestamp: ts,
		Tags:      tags,
	}, nil
}

// ParseTelnet parses OpenTSDB telnet protocol
// Format: put <metric> <timestamp> <value> <tagk>=<tagv> [<tagk>=<tagv> ...]
// Example: put sys.cpu.user 1609459200 42.5 host=web01 cpu=0
func (p *OpenTSDBParser) ParseTelnet(r io.Reader) (*Batch, error) {
	batch := &Batch{
		Samples: make([]Sample, 0),
		Source:  "opentsdb-telnet",
	}

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		sample, err := p.parseTelnetLine(line)
		if err != nil {
			continue // Skip invalid lines
		}

		batch.Samples = append(batch.Samples, sample)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanner error: %w", err)
	}

	return batch, nil
}

func (p *OpenTSDBParser) parseTelnetLine(line string) (Sample, error) {
	// Handle both "put metric ..." and "metric ..." formats
	if strings.HasPrefix(line, "put ") {
		line = line[4:]
	}

	parts := strings.Fields(line)
	if len(parts) < 3 {
		return Sample{}, fmt.Errorf("invalid format: need metric, timestamp, value")
	}

	metric := parts[0]

	timestamp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return Sample{}, fmt.Errorf("invalid timestamp: %w", err)
	}

	value, err := strconv.ParseFloat(parts[2], 64)
	if err != nil {
		return Sample{}, fmt.Errorf("invalid value: %w", err)
	}

	// Parse tags
	tags := make(map[string]string)
	for _, part := range parts[3:] {
		idx := strings.Index(part, "=")
		if idx > 0 {
			tags[part[:idx]] = part[idx+1:]
		}
	}

	// Determine timestamp precision
	var ts time.Time
	if timestamp > 1e12 { // Milliseconds
		ts = time.Unix(timestamp/1000, (timestamp%1000)*1e6)
	} else { // Seconds
		ts = time.Unix(timestamp, 0)
	}

	return Sample{
		Metric:    p.normalizeMetricName(metric),
		Value:     value,
		Timestamp: ts,
		Tags:      tags,
	}, nil
}

// normalizeMetricName converts OpenTSDB metric names to a standard format
// OpenTSDB uses dots, we keep them but also handle common patterns
func (p *OpenTSDBParser) normalizeMetricName(name string) string {
	// Replace dots with underscores to match Prometheus conventions
	// But keep the original if it's already using underscores
	if strings.Contains(name, "_") {
		return name
	}
	return strings.ReplaceAll(name, ".", "_")
}

// trimBOM removes UTF-8 BOM if present
func trimBOM(data []byte) []byte {
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		return data[3:]
	}
	return data
}
