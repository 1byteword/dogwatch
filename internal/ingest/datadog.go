package ingest

import (
	"compress/gzip"
	"compress/zlib"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

// DataDogParser parses DataDog agent protocol
type DataDogParser struct{}

// DataDogSeries represents the DataDog /api/v1/series payload
type DataDogSeries struct {
	Series []DataDogMetric `json:"series"`
}

// DataDogMetric represents a single DataDog metric series
type DataDogMetric struct {
	Metric   string          `json:"metric"`
	Type     string          `json:"type"` // gauge, rate, count
	Interval int64           `json:"interval,omitempty"`
	Points   [][]interface{} `json:"points"` // [[timestamp, value], ...]
	Host     string          `json:"host,omitempty"`
	Tags     []string        `json:"tags,omitempty"`
	Unit     string          `json:"unit,omitempty"`
	Metadata *DDMetadata     `json:"metadata,omitempty"`
}

// DDMetadata contains optional metadata
type DDMetadata struct {
	Origin *DDOrigin `json:"origin,omitempty"`
}

// DDOrigin contains origin information
type DDOrigin struct {
	OriginType    string `json:"origin_type,omitempty"`
	OriginProduct string `json:"origin_product,omitempty"`
}

// DataDogV2Payload represents the V2 API payload
type DataDogV2Payload struct {
	Series []DataDogV2Series `json:"series"`
}

// DataDogV2Series represents a V2 series
type DataDogV2Series struct {
	Metric    string            `json:"metric"`
	Type      int               `json:"type"` // 0=unspecified, 1=count, 2=rate, 3=gauge
	Points    []DataDogV2Point  `json:"points"`
	Tags      []string          `json:"tags,omitempty"`
	Unit      string            `json:"unit,omitempty"`
	Resources []DataDogResource `json:"resources,omitempty"`
}

// DataDogV2Point represents a V2 point
type DataDogV2Point struct {
	Timestamp int64   `json:"timestamp"`
	Value     float64 `json:"value"`
}

// DataDogResource represents a resource
type DataDogResource struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

// ParseV1Series parses DataDog V1 /api/v1/series format
func (p *DataDogParser) ParseV1Series(r io.Reader, contentEncoding string) (*Batch, error) {
	// Handle compression
	reader, err := p.decompressReader(r, contentEncoding)
	if err != nil {
		return nil, err
	}
	if closer, ok := reader.(io.Closer); ok && reader != r {
		defer closer.Close()
	}

	batch := &Batch{
		Samples: make([]Sample, 0),
		Source:  "datadog",
	}

	var payload DataDogSeries
	if err := json.NewDecoder(reader).Decode(&payload); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	for _, series := range payload.Series {
		samples := p.parseV1Series(series)
		batch.Samples = append(batch.Samples, samples...)
	}

	return batch, nil
}

func (p *DataDogParser) parseV1Series(series DataDogMetric) []Sample {
	var samples []Sample

	// Parse tags
	tags := p.parseTags(series.Tags)
	if series.Host != "" {
		tags["host"] = series.Host
	}
	if series.Type != "" {
		tags["__dd_type"] = series.Type
	}

	// Convert metric name (DataDog uses dots, we keep them)
	metricName := p.normalizeMetricName(series.Metric)

	for _, point := range series.Points {
		if len(point) < 2 {
			continue
		}

		ts, value := p.parsePoint(point)
		if ts.IsZero() {
			continue
		}

		samples = append(samples, Sample{
			Metric:    metricName,
			Value:     value,
			Timestamp: ts,
			Tags:      copyTags(tags),
		})
	}

	return samples
}

// ParseV2Series parses DataDog V2 /api/v2/series format
func (p *DataDogParser) ParseV2Series(r io.Reader, contentEncoding string) (*Batch, error) {
	reader, err := p.decompressReader(r, contentEncoding)
	if err != nil {
		return nil, err
	}
	if closer, ok := reader.(io.Closer); ok && reader != r {
		defer closer.Close()
	}

	batch := &Batch{
		Samples: make([]Sample, 0),
		Source:  "datadog-v2",
	}

	var payload DataDogV2Payload
	if err := json.NewDecoder(reader).Decode(&payload); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	for _, series := range payload.Series {
		samples := p.parseV2Series(series)
		batch.Samples = append(batch.Samples, samples...)
	}

	return batch, nil
}

func (p *DataDogParser) parseV2Series(series DataDogV2Series) []Sample {
	var samples []Sample

	tags := p.parseTags(series.Tags)

	// Add type tag
	typeNames := []string{"unspecified", "count", "rate", "gauge"}
	if series.Type >= 0 && series.Type < len(typeNames) {
		tags["__dd_type"] = typeNames[series.Type]
	}

	// Add resource tags
	for _, res := range series.Resources {
		if res.Type != "" && res.Name != "" {
			tags[res.Type] = res.Name
		}
	}

	metricName := p.normalizeMetricName(series.Metric)

	for _, point := range series.Points {
		ts := time.Unix(point.Timestamp, 0)
		samples = append(samples, Sample{
			Metric:    metricName,
			Value:     point.Value,
			Timestamp: ts,
			Tags:      copyTags(tags),
		})
	}

	return samples
}

// ParseCheckRun parses DataDog /api/v1/check_run format (service checks)
func (p *DataDogParser) ParseCheckRun(r io.Reader, contentEncoding string) (*Batch, error) {
	reader, err := p.decompressReader(r, contentEncoding)
	if err != nil {
		return nil, err
	}
	if closer, ok := reader.(io.Closer); ok && reader != r {
		defer closer.Close()
	}

	batch := &Batch{
		Samples: make([]Sample, 0),
		Source:  "datadog-check",
	}

	// Check run can be single object or array
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	var checks []DataDogCheckRun
	if len(data) > 0 && data[0] == '[' {
		if err := json.Unmarshal(data, &checks); err != nil {
			return nil, err
		}
	} else {
		var check DataDogCheckRun
		if err := json.Unmarshal(data, &check); err != nil {
			return nil, err
		}
		checks = []DataDogCheckRun{check}
	}

	for _, check := range checks {
		tags := p.parseTags(check.Tags)
		if check.HostName != "" {
			tags["host"] = check.HostName
		}
		tags["check"] = check.Check

		// Convert status to numeric value (0=OK, 1=WARNING, 2=CRITICAL, 3=UNKNOWN)
		value := float64(check.Status)

		ts := time.Now()
		if check.Timestamp > 0 {
			ts = time.Unix(check.Timestamp, 0)
		}

		batch.Samples = append(batch.Samples, Sample{
			Metric:    "datadog_check_status",
			Value:     value,
			Timestamp: ts,
			Tags:      tags,
		})
	}

	return batch, nil
}

// DataDogCheckRun represents a service check
type DataDogCheckRun struct {
	Check     string   `json:"check"`
	HostName  string   `json:"host_name"`
	Status    int      `json:"status"` // 0=OK, 1=WARNING, 2=CRITICAL, 3=UNKNOWN
	Timestamp int64    `json:"timestamp,omitempty"`
	Message   string   `json:"message,omitempty"`
	Tags      []string `json:"tags,omitempty"`
}

// ParseDistributionPoints parses DataDog distribution metrics
func (p *DataDogParser) ParseDistributionPoints(r io.Reader, contentEncoding string) (*Batch, error) {
	reader, err := p.decompressReader(r, contentEncoding)
	if err != nil {
		return nil, err
	}
	if closer, ok := reader.(io.Closer); ok && reader != r {
		defer closer.Close()
	}

	batch := &Batch{
		Samples: make([]Sample, 0),
		Source:  "datadog-dist",
	}

	var payload struct {
		Series []struct {
			Metric string          `json:"metric"`
			Points [][]interface{} `json:"points"`
			Tags   []string        `json:"tags"`
			Host   string          `json:"host"`
		} `json:"series"`
	}

	if err := json.NewDecoder(reader).Decode(&payload); err != nil {
		return nil, err
	}

	for _, series := range payload.Series {
		tags := p.parseTags(series.Tags)
		if series.Host != "" {
			tags["host"] = series.Host
		}
		tags["__dd_type"] = "distribution"

		metricName := p.normalizeMetricName(series.Metric)

		for _, point := range series.Points {
			if len(point) < 2 {
				continue
			}

			// Distribution points: [timestamp, [values...]]
			tsFloat, _ := point[0].(float64)
			ts := time.Unix(int64(tsFloat), 0)

			// Values array - compute summary statistics
			if values, ok := point[1].([]interface{}); ok {
				var sum, min, max float64
				count := len(values)
				if count > 0 {
					min = toFloat64(values[0])
					max = min
					for _, v := range values {
						val := toFloat64(v)
						sum += val
						if val < min {
							min = val
						}
						if val > max {
							max = val
						}
					}
				}

				// Emit summary metrics
				batch.Samples = append(batch.Samples,
					Sample{Metric: metricName + "_count", Value: float64(count), Timestamp: ts, Tags: copyTags(tags)},
					Sample{Metric: metricName + "_sum", Value: sum, Timestamp: ts, Tags: copyTags(tags)},
					Sample{Metric: metricName + "_min", Value: min, Timestamp: ts, Tags: copyTags(tags)},
					Sample{Metric: metricName + "_max", Value: max, Timestamp: ts, Tags: copyTags(tags)},
				)
				if count > 0 {
					batch.Samples = append(batch.Samples,
						Sample{Metric: metricName + "_avg", Value: sum / float64(count), Timestamp: ts, Tags: copyTags(tags)},
					)
				}
			}
		}
	}

	return batch, nil
}

func (p *DataDogParser) parseTags(tags []string) map[string]string {
	result := make(map[string]string)
	for _, tag := range tags {
		idx := strings.Index(tag, ":")
		if idx > 0 {
			result[tag[:idx]] = tag[idx+1:]
		} else {
			// Tag without value
			result[tag] = "true"
		}
	}
	return result
}

func (p *DataDogParser) parsePoint(point []interface{}) (time.Time, float64) {
	if len(point) < 2 {
		return time.Time{}, 0
	}

	// Timestamp
	var ts time.Time
	switch t := point[0].(type) {
	case float64:
		ts = time.Unix(int64(t), 0)
	case int64:
		ts = time.Unix(t, 0)
	}

	// Value
	var value float64
	switch v := point[1].(type) {
	case float64:
		value = v
	case int64:
		value = float64(v)
	case int:
		value = float64(v)
	}

	return ts, value
}

func (p *DataDogParser) normalizeMetricName(name string) string {
	// DataDog uses dots, keep them but sanitize
	return strings.ReplaceAll(name, " ", "_")
}

func (p *DataDogParser) decompressReader(r io.Reader, contentEncoding string) (io.Reader, error) {
	switch strings.ToLower(contentEncoding) {
	case "gzip":
		return gzip.NewReader(r)
	case "deflate":
		return zlib.NewReader(r)
	default:
		return r, nil
	}
}
