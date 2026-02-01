package storage

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"
)

// TimeSeriesOptimizer provides SQLite optimizations for time-series workloads
type TimeSeriesOptimizer struct {
	db     *sql.DB
	config TimeSeriesConfig

	// Partitioning state
	partitions   map[string]*Partition // table_name -> partition info
	partitionsMu sync.RWMutex

	// Batch insert buffer
	insertBuffer   map[string][]BatchRow
	insertBufferMu sync.Mutex
	flushInterval  time.Duration
	maxBatchSize   int

	// Downsampling state
	downsampleRules []DownsampleRule
	downsampleMu    sync.RWMutex

	// Background workers
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// TimeSeriesConfig configures the optimizer
type TimeSeriesConfig struct {
	// PartitionInterval is the time period for each partition (default: 1 hour)
	PartitionInterval time.Duration
	// MaxPartitions is how many partitions to keep (default: 168 = 7 days hourly)
	MaxPartitions int
	// FlushInterval is how often to flush batch inserts (default: 100ms)
	FlushInterval time.Duration
	// MaxBatchSize is max rows to buffer before flush (default: 1000)
	MaxBatchSize int
	// EnableCompression enables zlib compression for older partitions
	EnableCompression bool
	// DownsampleAfter enables automatic downsampling after this duration
	DownsampleAfter time.Duration
}

// DefaultTimeSeriesConfig returns sensible defaults
func DefaultTimeSeriesConfig() TimeSeriesConfig {
	return TimeSeriesConfig{
		PartitionInterval: time.Hour,
		MaxPartitions:     168, // 7 days
		FlushInterval:     100 * time.Millisecond,
		MaxBatchSize:      1000,
		EnableCompression: true,
		DownsampleAfter:   24 * time.Hour,
	}
}

// Partition represents a time-based table partition
type Partition struct {
	TableName  string    `json:"table_name"`
	PartName   string    `json:"partition_name"`
	StartTime  time.Time `json:"start_time"`
	EndTime    time.Time `json:"end_time"`
	RowCount   int64     `json:"row_count"`
	SizeBytes  int64     `json:"size_bytes"`
	Compressed bool      `json:"compressed"`
	CreatedAt  time.Time `json:"created_at"`
}

// BatchRow represents a row to be inserted in a batch
type BatchRow struct {
	Table     string
	Columns   []string
	Values    []interface{}
	Timestamp time.Time
}

// DownsampleRule defines how to downsample data
type DownsampleRule struct {
	SourceTable     string        `json:"source_table"`
	TargetTable     string        `json:"target_table"`
	SourceInterval  time.Duration `json:"source_interval"`
	TargetInterval  time.Duration `json:"target_interval"`
	AggregateColumn string        `json:"aggregate_column"`
	AggregateFunc   string        `json:"aggregate_func"` // avg, sum, min, max, count
	GroupByColumns  []string      `json:"group_by_columns"`
	Enabled         bool          `json:"enabled"`
}

// TimeRangeQuery represents a time-bounded query with hints
type TimeRangeQuery struct {
	Table     string
	StartTime time.Time
	EndTime   time.Time
	Columns   []string
	Where     string
	Args      []interface{}
	OrderBy   string
	Limit     int
}

// QueryPlan represents how a query will be executed
type QueryPlan struct {
	Partitions    []string `json:"partitions"`
	UseIndex      string   `json:"use_index"`
	EstimatedRows int64    `json:"estimated_rows"`
	Optimizations []string `json:"optimizations"`
}

// NewTimeSeriesOptimizer creates a new time-series optimizer
func NewTimeSeriesOptimizer(db *sql.DB, config TimeSeriesConfig) *TimeSeriesOptimizer {
	if config.PartitionInterval == 0 {
		config.PartitionInterval = DefaultTimeSeriesConfig().PartitionInterval
	}
	if config.MaxPartitions == 0 {
		config.MaxPartitions = DefaultTimeSeriesConfig().MaxPartitions
	}
	if config.FlushInterval == 0 {
		config.FlushInterval = DefaultTimeSeriesConfig().FlushInterval
	}
	if config.MaxBatchSize == 0 {
		config.MaxBatchSize = DefaultTimeSeriesConfig().MaxBatchSize
	}

	opt := &TimeSeriesOptimizer{
		db:            db,
		config:        config,
		partitions:    make(map[string]*Partition),
		insertBuffer:  make(map[string][]BatchRow),
		flushInterval: config.FlushInterval,
		maxBatchSize:  config.MaxBatchSize,
		stopCh:        make(chan struct{}),
	}

	return opt
}

// Start begins background workers for batching and maintenance
func (o *TimeSeriesOptimizer) Start() {
	o.wg.Add(2)

	// Batch flusher
	go func() {
		defer o.wg.Done()
		ticker := time.NewTicker(o.flushInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				o.FlushBatches()
			case <-o.stopCh:
				o.FlushBatches() // Final flush
				return
			}
		}
	}()

	// Partition maintenance
	go func() {
		defer o.wg.Done()
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				o.runMaintenance()
			case <-o.stopCh:
				return
			}
		}
	}()
}

// Stop shuts down the optimizer
func (o *TimeSeriesOptimizer) Stop() {
	close(o.stopCh)
	o.wg.Wait()
}

// CreatePartitionedTable creates a table structure for partitioning
func (o *TimeSeriesOptimizer) CreatePartitionedTable(tableName, schema string, timestampColumn string) error {
	// Create base table for metadata
	metaSchema := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s_partitions (
			partition_name TEXT PRIMARY KEY,
			start_time TEXT NOT NULL,
			end_time TEXT NOT NULL,
			row_count INTEGER DEFAULT 0,
			size_bytes INTEGER DEFAULT 0,
			compressed INTEGER DEFAULT 0,
			created_at TEXT DEFAULT CURRENT_TIMESTAMP
		)
	`, tableName)

	if _, err := o.db.Exec(metaSchema); err != nil {
		return fmt.Errorf("creating partition metadata table: %w", err)
	}

	// Create covering index for time-range queries
	indexSchema := fmt.Sprintf(`
		CREATE INDEX IF NOT EXISTS idx_%s_time_covering
		ON %s(%s, id)
	`, tableName, tableName, timestampColumn)

	if _, err := o.db.Exec(indexSchema); err != nil {
		// Not fatal - table might not exist yet
		log.Printf("[timeseries] Index creation skipped for %s: %v", tableName, err)
	}

	return nil
}

// GetPartitionForTime returns or creates the partition for a given timestamp
func (o *TimeSeriesOptimizer) GetPartitionForTime(baseTable string, ts time.Time) string {
	// Round timestamp to partition boundary
	partStart := ts.Truncate(o.config.PartitionInterval)
	partEnd := partStart.Add(o.config.PartitionInterval)

	// Generate partition name
	partName := fmt.Sprintf("%s_p%s", baseTable, partStart.Format("20060102_150405"))

	o.partitionsMu.Lock()
	defer o.partitionsMu.Unlock()

	// Check if partition exists
	if _, ok := o.partitions[partName]; !ok {
		// Create new partition
		o.partitions[partName] = &Partition{
			TableName: baseTable,
			PartName:  partName,
			StartTime: partStart,
			EndTime:   partEnd,
			CreatedAt: time.Now(),
		}
	}

	return partName
}

// BatchInsert buffers a row for batch insertion
func (o *TimeSeriesOptimizer) BatchInsert(row BatchRow) error {
	o.insertBufferMu.Lock()
	defer o.insertBufferMu.Unlock()

	o.insertBuffer[row.Table] = append(o.insertBuffer[row.Table], row)

	// Flush if buffer is full
	if len(o.insertBuffer[row.Table]) >= o.maxBatchSize {
		return o.flushTableBuffer(row.Table)
	}

	return nil
}

// FlushBatches flushes all pending batch inserts
func (o *TimeSeriesOptimizer) FlushBatches() {
	o.insertBufferMu.Lock()
	defer o.insertBufferMu.Unlock()

	for table := range o.insertBuffer {
		if len(o.insertBuffer[table]) > 0 {
			if err := o.flushTableBuffer(table); err != nil {
				log.Printf("[timeseries] Batch flush error for %s: %v", table, err)
			}
		}
	}
}

// flushTableBuffer flushes a single table's buffer (must hold insertBufferMu)
func (o *TimeSeriesOptimizer) flushTableBuffer(table string) error {
	rows := o.insertBuffer[table]
	if len(rows) == 0 {
		return nil
	}

	// Clear buffer first
	o.insertBuffer[table] = nil

	// Build batch insert
	tx, err := o.db.Begin()
	if err != nil {
		return err
	}

	for _, row := range rows {
		placeholders := make([]string, len(row.Values))
		for i := range placeholders {
			placeholders[i] = "?"
		}

		query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
			row.Table,
			strings.Join(row.Columns, ", "),
			strings.Join(placeholders, ", "))

		if _, err := tx.Exec(query, row.Values...); err != nil {
			tx.Rollback()
			return fmt.Errorf("batch insert: %w", err)
		}
	}

	return tx.Commit()
}

// QueryWithTimeRange executes a time-range optimized query
func (o *TimeSeriesOptimizer) QueryWithTimeRange(ctx context.Context, q TimeRangeQuery) (*sql.Rows, *QueryPlan, error) {
	plan := &QueryPlan{
		Optimizations: []string{},
	}

	// Build optimized query
	var queryBuilder strings.Builder
	queryBuilder.WriteString("SELECT ")

	if len(q.Columns) > 0 {
		queryBuilder.WriteString(strings.Join(q.Columns, ", "))
	} else {
		queryBuilder.WriteString("*")
	}

	queryBuilder.WriteString(" FROM ")
	queryBuilder.WriteString(q.Table)

	// Add time range filter using index hints
	var args []interface{}
	whereClause := ""

	if !q.StartTime.IsZero() && !q.EndTime.IsZero() {
		// Use timestamp column for range query
		whereClause = "timestamp >= ? AND timestamp < ?"
		args = append(args, q.StartTime.Format(time.RFC3339), q.EndTime.Format(time.RFC3339))
		plan.UseIndex = fmt.Sprintf("idx_%s_time_covering", q.Table)
		plan.Optimizations = append(plan.Optimizations, "time_range_pushdown")
	} else if !q.StartTime.IsZero() {
		whereClause = "timestamp >= ?"
		args = append(args, q.StartTime.Format(time.RFC3339))
	} else if !q.EndTime.IsZero() {
		whereClause = "timestamp < ?"
		args = append(args, q.EndTime.Format(time.RFC3339))
	}

	// Combine with additional WHERE clause
	if whereClause != "" && q.Where != "" {
		queryBuilder.WriteString(" WHERE (")
		queryBuilder.WriteString(whereClause)
		queryBuilder.WriteString(") AND (")
		queryBuilder.WriteString(q.Where)
		queryBuilder.WriteString(")")
	} else if whereClause != "" {
		queryBuilder.WriteString(" WHERE ")
		queryBuilder.WriteString(whereClause)
	} else if q.Where != "" {
		queryBuilder.WriteString(" WHERE ")
		queryBuilder.WriteString(q.Where)
	}

	args = append(args, q.Args...)

	if q.OrderBy != "" {
		queryBuilder.WriteString(" ORDER BY ")
		queryBuilder.WriteString(q.OrderBy)
	} else {
		// Default to timestamp descending for time-series
		queryBuilder.WriteString(" ORDER BY timestamp DESC")
		plan.Optimizations = append(plan.Optimizations, "default_time_order")
	}

	if q.Limit > 0 {
		queryBuilder.WriteString(fmt.Sprintf(" LIMIT %d", q.Limit))
	}

	query := queryBuilder.String()

	// Estimate rows (simple heuristic)
	if !q.StartTime.IsZero() && !q.EndTime.IsZero() {
		duration := q.EndTime.Sub(q.StartTime)
		// Assume ~1000 rows per hour for estimation
		plan.EstimatedRows = int64(duration.Hours()) * 1000
	}

	rows, err := o.db.QueryContext(ctx, query, args...)
	return rows, plan, err
}

// AddDownsampleRule adds a downsampling rule
func (o *TimeSeriesOptimizer) AddDownsampleRule(rule DownsampleRule) {
	o.downsampleMu.Lock()
	defer o.downsampleMu.Unlock()
	o.downsampleRules = append(o.downsampleRules, rule)
}

// RunDownsampling executes all enabled downsampling rules
func (o *TimeSeriesOptimizer) RunDownsampling(ctx context.Context) error {
	o.downsampleMu.RLock()
	rules := make([]DownsampleRule, len(o.downsampleRules))
	copy(rules, o.downsampleRules)
	o.downsampleMu.RUnlock()

	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}

		if err := o.executeDownsample(ctx, rule); err != nil {
			log.Printf("[timeseries] Downsample error for %s: %v", rule.SourceTable, err)
		}
	}

	return nil
}

// executeDownsample runs a single downsampling rule
func (o *TimeSeriesOptimizer) executeDownsample(ctx context.Context, rule DownsampleRule) error {
	// Ensure target table exists
	createSQL := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			timestamp TEXT NOT NULL,
			%s REAL NOT NULL,
			%s
			PRIMARY KEY (timestamp %s)
		)
	`, rule.TargetTable,
		rule.AggregateColumn,
		buildGroupBySchema(rule.GroupByColumns),
		buildGroupByPrimaryKey(rule.GroupByColumns))

	if _, err := o.db.ExecContext(ctx, createSQL); err != nil {
		return fmt.Errorf("creating target table: %w", err)
	}

	// Find data to downsample (older than DownsampleAfter)
	cutoff := time.Now().Add(-o.config.DownsampleAfter)

	// Build aggregation query
	groupBy := "strftime('%Y-%m-%d %H:00:00', timestamp)"
	if rule.TargetInterval == 24*time.Hour {
		groupBy = "strftime('%Y-%m-%d', timestamp)"
	} else if rule.TargetInterval == 5*time.Minute {
		groupBy = "strftime('%Y-%m-%d %H:', timestamp) || printf('%02d:00', (strftime('%M', timestamp) / 5) * 5)"
	}

	if len(rule.GroupByColumns) > 0 {
		groupBy += ", " + strings.Join(rule.GroupByColumns, ", ")
	}

	selectCols := groupBy
	if len(rule.GroupByColumns) > 0 {
		selectCols = fmt.Sprintf("strftime('%%Y-%%m-%%dT%%H:00:00Z', timestamp), %s(%s), %s",
			rule.AggregateFunc,
			rule.AggregateColumn,
			strings.Join(rule.GroupByColumns, ", "))
	} else {
		selectCols = fmt.Sprintf("strftime('%%Y-%%m-%%dT%%H:00:00Z', timestamp), %s(%s)",
			rule.AggregateFunc,
			rule.AggregateColumn)
	}

	query := fmt.Sprintf(`
		INSERT OR REPLACE INTO %s
		SELECT %s
		FROM %s
		WHERE timestamp < ?
		GROUP BY %s
	`, rule.TargetTable, selectCols, rule.SourceTable, groupBy)

	result, err := o.db.ExecContext(ctx, query, cutoff.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("executing downsample: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected > 0 {
		log.Printf("[timeseries] Downsampled %d rows from %s to %s", rowsAffected, rule.SourceTable, rule.TargetTable)
	}

	return nil
}

// runMaintenance runs periodic maintenance tasks
func (o *TimeSeriesOptimizer) runMaintenance() {
	ctx := context.Background()

	// Run downsampling
	if err := o.RunDownsampling(ctx); err != nil {
		log.Printf("[timeseries] Maintenance error: %v", err)
	}

	// Clean old partitions
	o.cleanOldPartitions()

	// Analyze tables for query optimization
	o.analyzeOptimize()
}

// cleanOldPartitions removes partitions beyond MaxPartitions
func (o *TimeSeriesOptimizer) cleanOldPartitions() {
	o.partitionsMu.Lock()
	defer o.partitionsMu.Unlock()

	if len(o.partitions) <= o.config.MaxPartitions {
		return
	}

	// Sort partitions by time
	var parts []*Partition
	for _, p := range o.partitions {
		parts = append(parts, p)
	}
	sort.Slice(parts, func(i, j int) bool {
		return parts[i].StartTime.Before(parts[j].StartTime)
	})

	// Remove oldest partitions
	toRemove := len(parts) - o.config.MaxPartitions
	for i := 0; i < toRemove; i++ {
		delete(o.partitions, parts[i].PartName)
	}
}

// analyzeOptimize runs ANALYZE on tables for better query planning
func (o *TimeSeriesOptimizer) analyzeOptimize() {
	_, err := o.db.Exec("ANALYZE")
	if err != nil {
		log.Printf("[timeseries] ANALYZE error: %v", err)
	}
}

// CreateCoveringIndex creates a covering index for common query patterns
func (o *TimeSeriesOptimizer) CreateCoveringIndex(table string, columns []string, includes []string) error {
	indexName := fmt.Sprintf("idx_%s_%s_covering", table, strings.Join(columns, "_"))

	allCols := append(columns, includes...)
	query := fmt.Sprintf("CREATE INDEX IF NOT EXISTS %s ON %s(%s)",
		indexName, table, strings.Join(allCols, ", "))

	_, err := o.db.Exec(query)
	return err
}

// GetStats returns optimizer statistics
func (o *TimeSeriesOptimizer) GetStats() *TimeSeriesStats {
	o.insertBufferMu.Lock()
	bufferedRows := 0
	for _, rows := range o.insertBuffer {
		bufferedRows += len(rows)
	}
	o.insertBufferMu.Unlock()

	o.partitionsMu.RLock()
	partitionCount := len(o.partitions)
	o.partitionsMu.RUnlock()

	o.downsampleMu.RLock()
	downsampleRules := len(o.downsampleRules)
	o.downsampleMu.RUnlock()

	return &TimeSeriesStats{
		PartitionCount:    partitionCount,
		BufferedRows:      bufferedRows,
		DownsampleRules:   downsampleRules,
		FlushIntervalMs:   o.flushInterval.Milliseconds(),
		MaxBatchSize:      o.maxBatchSize,
		PartitionInterval: o.config.PartitionInterval.String(),
	}
}

// TimeSeriesStats contains optimizer statistics
type TimeSeriesStats struct {
	PartitionCount    int    `json:"partition_count"`
	BufferedRows      int    `json:"buffered_rows"`
	DownsampleRules   int    `json:"downsample_rules"`
	FlushIntervalMs   int64  `json:"flush_interval_ms"`
	MaxBatchSize      int    `json:"max_batch_size"`
	PartitionInterval string `json:"partition_interval"`
}

// Helper functions

func buildGroupBySchema(cols []string) string {
	if len(cols) == 0 {
		return ""
	}
	var parts []string
	for _, col := range cols {
		parts = append(parts, col+" TEXT")
	}
	return strings.Join(parts, ", ") + ","
}

func buildGroupByPrimaryKey(cols []string) string {
	if len(cols) == 0 {
		return ""
	}
	return ", " + strings.Join(cols, ", ")
}

// OptimizeForTimeSeries applies time-series optimizations to existing tables
func (o *TimeSeriesOptimizer) OptimizeForTimeSeries(table, timestampCol string) error {
	// Create time-based index
	indexSQL := fmt.Sprintf(`
		CREATE INDEX IF NOT EXISTS idx_%s_%s ON %s(%s DESC)
	`, table, timestampCol, table, timestampCol)

	if _, err := o.db.Exec(indexSQL); err != nil {
		return fmt.Errorf("creating time index: %w", err)
	}

	// Enable WAL mode for better concurrent writes
	if _, err := o.db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		log.Printf("[timeseries] WAL mode warning: %v", err)
	}

	// Optimize page size for time-series data
	if _, err := o.db.Exec("PRAGMA page_size=4096"); err != nil {
		log.Printf("[timeseries] Page size warning: %v", err)
	}

	// Enable memory-mapped I/O for read performance
	if _, err := o.db.Exec("PRAGMA mmap_size=268435456"); err != nil { // 256MB
		log.Printf("[timeseries] mmap warning: %v", err)
	}

	return nil
}

// ListPartitions returns all known partitions for a table
func (o *TimeSeriesOptimizer) ListPartitions(baseTable string) []*Partition {
	o.partitionsMu.RLock()
	defer o.partitionsMu.RUnlock()

	var result []*Partition
	for _, p := range o.partitions {
		if p.TableName == baseTable {
			result = append(result, p)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].StartTime.After(result[j].StartTime)
	})

	return result
}

// ListDownsampleRules returns all downsampling rules
func (o *TimeSeriesOptimizer) ListDownsampleRules() []DownsampleRule {
	o.downsampleMu.RLock()
	defer o.downsampleMu.RUnlock()

	result := make([]DownsampleRule, len(o.downsampleRules))
	copy(result, o.downsampleRules)
	return result
}

// SetDownsampleRuleEnabled enables or disables a rule
func (o *TimeSeriesOptimizer) SetDownsampleRuleEnabled(sourceTable string, enabled bool) {
	o.downsampleMu.Lock()
	defer o.downsampleMu.Unlock()

	for i := range o.downsampleRules {
		if o.downsampleRules[i].SourceTable == sourceTable {
			o.downsampleRules[i].Enabled = enabled
		}
	}
}

// RemoveDownsampleRule removes a downsampling rule
func (o *TimeSeriesOptimizer) RemoveDownsampleRule(sourceTable string) {
	o.downsampleMu.Lock()
	defer o.downsampleMu.Unlock()

	var remaining []DownsampleRule
	for _, rule := range o.downsampleRules {
		if rule.SourceTable != sourceTable {
			remaining = append(remaining, rule)
		}
	}
	o.downsampleRules = remaining
}
