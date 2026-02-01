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
