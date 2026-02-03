package ingest

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// StatsDParser parses StatsD protocol
type StatsDParser struct {
	// Aggregation state for gauges/counters/timers
	counters   map[string]float64
	gauges     map[string]float64
	timers     map[string][]float64
	sets       map[string]map[string]struct{}
	sampleRate float64
}

// NewStatsDParser creates a new StatsD parser
func NewStatsDParser() *StatsDParser {
	return &StatsDParser{
		counters:   make(map[string]float64),
		gauges:     make(map[string]float64),
		timers:     make(map[string][]float64),
		sets:       make(map[string]map[string]struct{}),
		sampleRate: 1.0,
	}
}

// StatsDMetric represents a parsed StatsD metric
type StatsDMetric struct {
	Name       string
	Value      float64
	Type       string // c=counter, g=gauge, ms=timer, s=set, h=histogram
	SampleRate float64
	Tags       map[string]string
}

// Parse parses StatsD protocol input
// Format: metric_name:value|type|@sample_rate|#tag1:value1,tag2:value2
// Examples:
//   gorets:1|c                     - counter increment
//   glork:320|ms                   - timer
//   gaugor:333|g                   - gauge
//   uniques:765|s                  - set
//   gorets:1|c|@0.1                - counter with 10% sample rate
//   users.online:10|g|#host:web01  - gauge with tags (DogStatsD)
func (p *StatsDParser) Parse(r io.Reader) (*Batch, error) {
	batch := &Batch{
		Samples: make([]Sample, 0),
		Source:  "statsd",
	}

	now := time.Now()
	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// StatsD allows multiple metrics per line separated by newlines
		// Some implementations also allow multiple metrics in one UDP packet
		metrics := strings.Split(line, "\n")
		for _, metricLine := range metrics {
			metricLine = strings.TrimSpace(metricLine)
			if metricLine == "" {
				continue
			}

			metric, err := p.parseLine(metricLine)
			if err != nil {
				continue // Skip invalid lines
			}

			samples := p.metricToSamples(metric, now)
			batch.Samples = append(batch.Samples, samples...)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanner error: %w", err)
	}

	return batch, nil
}

func (p *StatsDParser) parseLine(line string) (*StatsDMetric, error) {
	metric := &StatsDMetric{
		SampleRate: 1.0,
		Tags:       make(map[string]string),
	}

	// Split by | to get parts
	parts := strings.Split(line, "|")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid format: missing type")
	}

	// First part: name:value
	nameValue := strings.SplitN(parts[0], ":", 2)
	if len(nameValue) != 2 {
		return nil, fmt.Errorf("invalid format: missing value")
	}

	metric.Name = nameValue[0]

	// Handle gauge delta (+/-)
	valueStr := nameValue[1]
	if metric.Type == "g" && (strings.HasPrefix(valueStr, "+") || strings.HasPrefix(valueStr, "-")) {
		// Delta gauge - will be handled in aggregation
	}

	value, err := strconv.ParseFloat(valueStr, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid value: %w", err)
	}
	metric.Value = value

	// Second part: type
	metric.Type = parts[1]

	// Parse remaining parts (sample rate, tags)
	for _, part := range parts[2:] {
		if strings.HasPrefix(part, "@") {
			// Sample rate
			rate, err := strconv.ParseFloat(part[1:], 64)
			if err == nil && rate > 0 && rate <= 1 {
				metric.SampleRate = rate
			}
		} else if strings.HasPrefix(part, "#") {
			// DogStatsD tags
			tagStr := part[1:]
			tags := strings.Split(tagStr, ",")
			for _, tag := range tags {
				kv := strings.SplitN(tag, ":", 2)
				if len(kv) == 2 {
					metric.Tags[kv[0]] = kv[1]
				} else if len(kv) == 1 {
					metric.Tags[kv[0]] = "true"
				}
			}
		}
	}

	return metric, nil
}

func (p *StatsDParser) metricToSamples(m *StatsDMetric, ts time.Time) []Sample {
	var samples []Sample

	// Normalize metric name
	name := strings.ReplaceAll(m.Name, " ", "_")

	// Adjust value for sample rate
	value := m.Value
	if m.SampleRate > 0 && m.SampleRate < 1 {
		value = m.Value / m.SampleRate
	}

	switch m.Type {
	case "c": // Counter
		samples = append(samples, Sample{
			Metric:    name + "_total",
			Value:     value,
			Timestamp: ts,
			Tags:      copyTags(m.Tags),
		})

	case "g": // Gauge
		samples = append(samples, Sample{
			Metric:    name,
			Value:     value,
			Timestamp: ts,
			Tags:      copyTags(m.Tags),
		})

	case "ms", "h": // Timer / Histogram
		// Emit as histogram value
		samples = append(samples, Sample{
			Metric:    name,
			Value:     value,
			Timestamp: ts,
			Tags:      copyTags(m.Tags),
		})
		// Also track count
		countTags := copyTags(m.Tags)
		samples = append(samples, Sample{
			Metric:    name + "_count",
			Value:     1 / m.SampleRate, // Account for sampling
			Timestamp: ts,
			Tags:      countTags,
		})

	case "s": // Set - count unique values
		// For sets, we emit the count of unique values
		// In a stateless parser, we just emit 1 for each unique value seen
		samples = append(samples, Sample{
			Metric:    name + "_unique",
			Value:     1,
			Timestamp: ts,
			Tags:      copyTags(m.Tags),
		})

	default:
		// Unknown type - treat as gauge
		samples = append(samples, Sample{
			Metric:    name,
			Value:     value,
			Timestamp: ts,
			Tags:      copyTags(m.Tags),
		})
	}

	return samples
}

// FlushTimers computes summary statistics for accumulated timer values
// and returns samples for percentiles, mean, etc.
func (p *StatsDParser) FlushTimers(ts time.Time) []Sample {
	var samples []Sample

	for name, values := range p.timers {
		if len(values) == 0 {
			continue
		}

		// Sort for percentiles
		sortFloat64s(values)

		count := float64(len(values))
		sum := 0.0
		for _, v := range values {
			sum += v
		}

		tags := make(map[string]string)

		samples = append(samples,
			Sample{Metric: name + "_count", Value: count, Timestamp: ts, Tags: copyTags(tags)},
			Sample{Metric: name + "_sum", Value: sum, Timestamp: ts, Tags: copyTags(tags)},
			Sample{Metric: name + "_mean", Value: sum / count, Timestamp: ts, Tags: copyTags(tags)},
			Sample{Metric: name + "_min", Value: values[0], Timestamp: ts, Tags: copyTags(tags)},
			Sample{Metric: name + "_max", Value: values[len(values)-1], Timestamp: ts, Tags: copyTags(tags)},
			Sample{Metric: name + "_p50", Value: percentile(values, 0.50), Timestamp: ts, Tags: copyTags(tags)},
			Sample{Metric: name + "_p90", Value: percentile(values, 0.90), Timestamp: ts, Tags: copyTags(tags)},
			Sample{Metric: name + "_p95", Value: percentile(values, 0.95), Timestamp: ts, Tags: copyTags(tags)},
			Sample{Metric: name + "_p99", Value: percentile(values, 0.99), Timestamp: ts, Tags: copyTags(tags)},
		)
	}

	// Clear timers after flush
	p.timers = make(map[string][]float64)

	return samples
}

func sortFloat64s(a []float64) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j] < a[j-1]; j-- {
			a[j], a[j-1] = a[j-1], a[j]
		}
	}
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}

	rank := p * float64(len(sorted)-1)
	lower := int(rank)
	upper := lower + 1
	if upper >= len(sorted) {
		return sorted[len(sorted)-1]
	}

	frac := rank - float64(lower)
	return sorted[lower]*(1-frac) + sorted[upper]*frac
}
