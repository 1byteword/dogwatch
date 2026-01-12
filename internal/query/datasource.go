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

	// Build query
	query := `SELECT timestamp, name, value, labels FROM metrics WHERE timestamp >= ? AND timestamp <= ?`
	args := []interface{}{timeRange.Start.UnixNano(), timeRange.End.UnixNano()}

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
		var ts int64
		var name string
		var value float64
		var labelsJSON string

		if err := rows.Scan(&ts, &name, &value, &labelsJSON); err != nil {
			continue
		}

		row := Row{
			"timestamp": time.Unix(0, ts),
			"name":      name,
			"value":     value,
		}

		// Parse labels
		var labels map[string]string
		if json.Unmarshal([]byte(labelsJSON), &labels) == nil {
			for k, v := range labels {
				row[k] = v
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

	query := `SELECT timestamp, level, service, message, attributes FROM logs WHERE timestamp >= ? AND timestamp <= ? ORDER BY timestamp DESC LIMIT 10000`
	args := []interface{}{timeRange.Start.UnixNano(), timeRange.End.UnixNano()}

	rows, err := ds.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query error: %w", err)
	}
	defer rows.Close()

	var result []Row
	for rows.Next() {
		var ts int64
		var level, service, message, attrsJSON string

		if err := rows.Scan(&ts, &level, &service, &message, &attrsJSON); err != nil {
			continue
		}

		row := Row{
			"timestamp": time.Unix(0, ts),
			"level":     level,
			"service":   service,
			"message":   message,
		}

		// Parse attributes
		var attrs map[string]interface{}
		if json.Unmarshal([]byte(attrsJSON), &attrs) == nil {
			for k, v := range attrs {
				row[k] = v
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

	query := `SELECT trace_id, span_id, parent_span_id, service, operation, start_time, duration_ns, status, attributes
			  FROM spans WHERE start_time >= ? AND start_time <= ? ORDER BY start_time DESC LIMIT 10000`
	args := []interface{}{timeRange.Start.UnixNano(), timeRange.End.UnixNano()}

	rows, err := ds.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query error: %w", err)
	}
	defer rows.Close()

	var result []Row
	for rows.Next() {
		var traceID, spanID, parentSpanID, service, operation, status, attrsJSON string
		var startTime, durationNs int64

		if err := rows.Scan(&traceID, &spanID, &parentSpanID, &service, &operation, &startTime, &durationNs, &status, &attrsJSON); err != nil {
			continue
		}

		row := Row{
			"trace_id":       traceID,
			"span_id":        spanID,
			"parent_span_id": parentSpanID,
			"service":        service,
			"operation":      operation,
			"timestamp":      time.Unix(0, startTime),
			"duration":       float64(durationNs) / 1e6, // Convert to ms
			"status":         status,
		}

		// Parse attributes
		var attrs map[string]interface{}
		if json.Unmarshal([]byte(attrsJSON), &attrs) == nil {
			for k, v := range attrs {
				row[k] = v
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
