package query

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// MetricsDataSource reads from the metrics database
type MetricsDataSource struct {
	db *sql.DB
}

// NewMetricsDataSource creates a new metrics data source
func NewMetricsDataSource(db *sql.DB) *MetricsDataSource {
	return &MetricsDataSource{db: db}
}

// Scan reads metrics data
func (ds *MetricsDataSource) Scan(ctx context.Context, source string, metric string, timeRange TimeRangeSpec, predicates []Expr) ([]Row, error) {
	if ds.db == nil {
		return nil, fmt.Errorf("metrics database not configured")
	}

	// Build query - matches custommetrics.Store schema
	query := `SELECT timestamp, name, type, value, tags FROM metrics WHERE timestamp >= ? AND timestamp <= ?`
	args := []interface{}{timeRange.Start, timeRange.End}

	if metric != "" {
		query += " AND name = ?"
		args = append(args, metric)
	}

	query += " ORDER BY timestamp DESC LIMIT 10000"

	rows, err := ds.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query error: %w", err)
	}
	defer rows.Close()

	var result []Row
	for rows.Next() {
		var ts time.Time
		var name, metricType string
		var value float64
		var tagsJSON sql.NullString

		if err := rows.Scan(&ts, &name, &metricType, &value, &tagsJSON); err != nil {
			continue
		}

		row := Row{
			"timestamp": ts,
			"name":      name,
			"type":      metricType,
			"value":     value,
		}

		// Parse tags
		if tagsJSON.Valid && tagsJSON.String != "" {
			var tags map[string]string
			if json.Unmarshal([]byte(tagsJSON.String), &tags) == nil {
				for k, v := range tags {
					row[k] = v
				}
			}
		}

		result = append(result, row)
	}

	return result, nil
}

// LogsDataSource reads from the logs database
type LogsDataSource struct {
	db *sql.DB
}

// NewLogsDataSource creates a new logs data source
func NewLogsDataSource(db *sql.DB) *LogsDataSource {
	return &LogsDataSource{db: db}
}

// Scan reads log data
func (ds *LogsDataSource) Scan(ctx context.Context, source string, metric string, timeRange TimeRangeSpec, predicates []Expr) ([]Row, error) {
	if ds.db == nil {
		return nil, fmt.Errorf("logs database not configured")
	}

	// Query matches logs.Store schema
	query := `SELECT timestamp, level, message, service, host, trace_id, span_id, attrs
			  FROM logs WHERE timestamp >= ? AND timestamp <= ? ORDER BY timestamp DESC LIMIT 10000`
	args := []interface{}{timeRange.Start, timeRange.End}

	rows, err := ds.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query error: %w", err)
	}
	defer rows.Close()

	var result []Row
	for rows.Next() {
		var ts time.Time
		var level, message string
		var service, host, traceID, spanID, attrsJSON sql.NullString

		if err := rows.Scan(&ts, &level, &message, &service, &host, &traceID, &spanID, &attrsJSON); err != nil {
			continue
		}

		row := Row{
			"timestamp": ts,
			"level":     level,
			"message":   message,
			"service":   service.String,
			"host":      host.String,
			"trace_id":  traceID.String,
			"span_id":   spanID.String,
		}

		// Parse attributes
		if attrsJSON.Valid && attrsJSON.String != "" {
			var attrs map[string]string
			if json.Unmarshal([]byte(attrsJSON.String), &attrs) == nil {
				for k, v := range attrs {
					row[k] = v
				}
			}
		}

		result = append(result, row)
	}

	return result, nil
}

// TracesDataSource reads from the traces database
type TracesDataSource struct {
	db *sql.DB
}

// NewTracesDataSource creates a new traces data source
func NewTracesDataSource(db *sql.DB) *TracesDataSource {
	return &TracesDataSource{db: db}
}

// Scan reads trace data
func (ds *TracesDataSource) Scan(ctx context.Context, source string, metric string, timeRange TimeRangeSpec, predicates []Expr) ([]Row, error) {
	if ds.db == nil {
		return nil, fmt.Errorf("traces database not configured")
	}

	// Query matches trace.Store schema - start_time is RFC3339 TEXT
	query := `SELECT trace_id, span_id, parent_span_id, name, service_name, kind, start_time, end_time, duration_ms, status, status_message, attributes
			  FROM spans WHERE start_time >= ? AND start_time <= ? ORDER BY start_time DESC LIMIT 10000`
	args := []interface{}{
		timeRange.Start.UTC().Format(time.RFC3339),
		timeRange.End.UTC().Format(time.RFC3339),
	}

	rows, err := ds.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query error: %w", err)
	}
	defer rows.Close()

	var result []Row
	for rows.Next() {
		var traceID, spanID, startTimeStr, endTimeStr string
		var name, serviceName string
		var durationMs float64
		var parentSpanID, kind, status, statusMsg, attrsJSON sql.NullString

		if err := rows.Scan(&traceID, &spanID, &parentSpanID, &name, &serviceName, &kind, &startTimeStr, &endTimeStr, &durationMs, &status, &statusMsg, &attrsJSON); err != nil {
			continue
		}

		startTime, _ := time.Parse(time.RFC3339Nano, startTimeStr)

		row := Row{
			"trace_id":       traceID,
			"span_id":        spanID,
			"parent_span_id": parentSpanID.String,
			"name":           name,
			"operation":      name, // Alias for query compatibility
			"service":        serviceName,
			"service_name":   serviceName,
			"kind":           kind.String,
			"timestamp":      startTime,
			"duration":       durationMs,
			"duration_ms":    durationMs,
			"status":         status.String,
			"status_message": statusMsg.String,
		}

		// Parse attributes
		if attrsJSON.Valid && attrsJSON.String != "" {
			var attrs map[string]interface{}
			if json.Unmarshal([]byte(attrsJSON.String), &attrs) == nil {
				for k, v := range attrs {
					row[k] = v
				}
			}
		}

		result = append(result, row)
	}

	return result, nil
}

// InMemoryDataSource provides in-memory data for testing
type InMemoryDataSource struct {
	data []Row
}

// NewInMemoryDataSource creates a new in-memory data source
func NewInMemoryDataSource(data []Row) *InMemoryDataSource {
	return &InMemoryDataSource{data: data}
}

// Scan returns the in-memory data filtered by time range
func (ds *InMemoryDataSource) Scan(ctx context.Context, source string, metric string, timeRange TimeRangeSpec, predicates []Expr) ([]Row, error) {
	var result []Row

	for _, row := range ds.data {
		// Filter by metric name if specified
		if metric != "" {
			if name, ok := row["name"].(string); ok && name != metric {
				continue
			}
		}

		// Filter by time range
		if ts, ok := row["timestamp"].(time.Time); ok {
			if ts.Before(timeRange.Start) || ts.After(timeRange.End) {
				continue
			}
		}

		result = append(result, row)
	}

	return result, nil
}

// AddRow adds a row to the in-memory data source
func (ds *InMemoryDataSource) AddRow(row Row) {
	ds.data = append(ds.data, row)
}

// Clear removes all data
func (ds *InMemoryDataSource) Clear() {
	ds.data = nil
}

// ScanHistograms reads native histogram data from the database
func (ds *MetricsDataSource) ScanHistograms(ctx context.Context, metric string, timeRange TimeRangeSpec, tags map[string]string) ([]HistogramRow, error) {
	if ds.db == nil {
		return nil, fmt.Errorf("metrics database not configured")
	}

	query := `SELECT timestamp, name, tags, count, sum, min_val, max_val, bounds, bucket_counts, exemplars
		FROM histograms WHERE timestamp >= ? AND timestamp <= ?`
	args := []interface{}{timeRange.Start, timeRange.End}

	if metric != "" {
		query += " AND name = ?"
		args = append(args, metric)
	}

	query += " ORDER BY timestamp DESC LIMIT 10000"

	rows, err := ds.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query error: %w", err)
	}
	defer rows.Close()

	var result []HistogramRow
	for rows.Next() {
		var hr HistogramRow
		var ts time.Time
		var name string
		var tagsJSON, boundsJSON, countsJSON, exemplarsJSON sql.NullString
		var minVal, maxVal sql.NullFloat64
		var count int64
		var sum float64

		if err := rows.Scan(&ts, &name, &tagsJSON, &count, &sum, &minVal, &maxVal, &boundsJSON, &countsJSON, &exemplarsJSON); err != nil {
			continue
		}

		hr.Timestamp = ts
		hr.Name = name
		hr.Count = uint64(count)
		hr.Sum = sum

		if minVal.Valid {
			hr.Min = &minVal.Float64
		}
		if maxVal.Valid {
			hr.Max = &maxVal.Float64
		}

		if tagsJSON.Valid && tagsJSON.String != "" {
			json.Unmarshal([]byte(tagsJSON.String), &hr.Tags)
		}
		if boundsJSON.Valid {
			json.Unmarshal([]byte(boundsJSON.String), &hr.ExplicitBounds)
		}
		if countsJSON.Valid {
			json.Unmarshal([]byte(countsJSON.String), &hr.BucketCounts)
		}

		// Filter by tags if specified
		if len(tags) > 0 {
			match := true
			for k, v := range tags {
				if hr.Tags[k] != v {
					match = false
					break
				}
			}
			if !match {
				continue
			}
		}

		result = append(result, hr)
	}

	return result, nil
}

// QueryHistogramSnapshot implements HistogramSource interface for the executor
func (ds *MetricsDataSource) QueryHistogramSnapshot(name string, tags map[string]string, start, end time.Time) (*HistogramSnapshot, error) {
	ctx := context.Background()
	timeRange := TimeRangeSpec{Start: start, End: end}

	histRows, err := ds.ScanHistograms(ctx, name, timeRange, tags)
	if err != nil {
		return nil, err
	}
	if len(histRows) == 0 {
		return nil, nil
	}

	// Aggregate all histogram data points into a single snapshot
	snapshot := &HistogramSnapshot{}
	for i, hr := range histRows {
		if i == 0 {
			snapshot.Bounds = append([]float64{}, hr.ExplicitBounds...)
			snapshot.Counts = make([]uint64, len(hr.BucketCounts))
		}

		// Merge counts
		for j, c := range hr.BucketCounts {
			if j < len(snapshot.Counts) {
				snapshot.Counts[j] += c
			}
		}

		snapshot.TotalCount += hr.Count
		snapshot.Sum += hr.Sum

		if hr.Min != nil && (snapshot.Min == 0 || *hr.Min < snapshot.Min) {
			snapshot.Min = *hr.Min
		}
		if hr.Max != nil && *hr.Max > snapshot.Max {
			snapshot.Max = *hr.Max
		}
	}

	return snapshot, nil
}

// HistogramRow represents a native histogram data row
type HistogramRow struct {
	Timestamp      time.Time
	Name           string
	Tags           map[string]string
	Count          uint64
	Sum            float64
	Min            *float64
	Max            *float64
	ExplicitBounds []float64
	BucketCounts   []uint64
}

// HistogramSnapshot matches the executor's HistogramSnapshot type
type HistogramSnapshot struct {
	Bounds     []float64
	Counts     []uint64
	TotalCount uint64
	Sum        float64
	Min        float64
	Max        float64
}

// Quantile computes the estimated quantile value using linear interpolation
func (s *HistogramSnapshot) Quantile(q float64) float64 {
	if q < 0 {
		q = 0
	}
	if q > 1 {
		q = 1
	}
	if s.TotalCount == 0 {
		return 0
	}

	rank := q * float64(s.TotalCount)
	var prevCount uint64
	var prevBound float64

	for i, count := range s.Counts {
		if count == 0 {
			continue
		}
		if float64(count) >= rank {
			bucketCount := float64(count - prevCount)
			if bucketCount == 0 {
				prevCount = count
				if i < len(s.Bounds) {
					prevBound = s.Bounds[i]
				}
				continue
			}
			posInBucket := rank - float64(prevCount)
			fraction := posInBucket / bucketCount
			var upperBound float64
			if i < len(s.Bounds) {
				upperBound = s.Bounds[i]
			} else if s.Max > 0 {
				upperBound = s.Max
			} else if len(s.Bounds) > 0 {
				upperBound = s.Bounds[len(s.Bounds)-1] * 2
			} else {
				upperBound = 1
			}
			return prevBound + fraction*(upperBound-prevBound)
		}
		prevCount = count
		if i < len(s.Bounds) {
			prevBound = s.Bounds[i]
		}
	}

	if len(s.Bounds) > 0 {
		return s.Bounds[len(s.Bounds)-1]
	}
	return s.Max
}

// LiveMetricsDataSource provides real-time system metrics
type LiveMetricsDataSource struct {
	getMetrics func() map[string]float64
}

// NewLiveMetricsDataSource creates a data source from a metrics function
func NewLiveMetricsDataSource(fn func() map[string]float64) *LiveMetricsDataSource {
	return &LiveMetricsDataSource{getMetrics: fn}
}

// Scan returns current metrics as rows
func (ds *LiveMetricsDataSource) Scan(ctx context.Context, source string, metric string, timeRange TimeRangeSpec, predicates []Expr) ([]Row, error) {
	if ds.getMetrics == nil {
		return nil, nil
	}

	metrics := ds.getMetrics()
	now := time.Now()

	var result []Row
	for name, value := range metrics {
		if metric != "" && name != metric {
			continue
		}

		result = append(result, Row{
			"timestamp": now,
			"name":      name,
			"value":     value,
		})
	}

	return result, nil
}
