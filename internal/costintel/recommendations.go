package costintel

import (
	"fmt"
	"sort"
	"time"
)

// RecommendationType categorizes the type of recommendation
type RecommendationType string

const (
	RecommendDropUnused       RecommendationType = "drop_unused"
	RecommendSampleHighVolume RecommendationType = "sample_high_volume"
	RecommendDropDebugLogs    RecommendationType = "drop_debug_logs"
	RecommendAggregateMetrics RecommendationType = "aggregate_metrics"
	RecommendDropTags         RecommendationType = "drop_high_cardinality_tags"
	RecommendReduceRetention  RecommendationType = "reduce_retention"
	RecommendSetQuota         RecommendationType = "set_quota"
	RecommendOptimizeQuery    RecommendationType = "optimize_query"
)

// RecommendationPriority indicates urgency
type RecommendationPriority string

const (
	PriorityCritical RecommendationPriority = "critical" // >30% savings
	PriorityHigh     RecommendationPriority = "high"     // 15-30% savings
	PriorityMedium   RecommendationPriority = "medium"   // 5-15% savings
	PriorityLow      RecommendationPriority = "low"      // <5% savings
)

// Recommendation represents a cost optimization suggestion
type Recommendation struct {
	ID              string                 `json:"id"`
	Type            RecommendationType     `json:"type"`
	Priority        RecommendationPriority `json:"priority"`
	Title           string                 `json:"title"`
	Description     string                 `json:"description"`
	Impact          RecommendationImpact   `json:"impact"`
	DataType        string                 `json:"data_type"` // metrics, logs, traces
	Target          string                 `json:"target"`    // metric name, service, etc.
	Action          RecommendedAction      `json:"action"`
	Evidence        []Evidence             `json:"evidence"`
	CreatedAt       time.Time              `json:"created_at"`
	Status          string                 `json:"status"` // pending, applied, dismissed
	AppliedAt       *time.Time             `json:"applied_at,omitempty"`
	DismissedAt     *time.Time             `json:"dismissed_at,omitempty"`
	DismissedReason string                 `json:"dismissed_reason,omitempty"`
}

// RecommendationImpact shows the potential savings
type RecommendationImpact struct {
	MonthlySavings     float64 `json:"monthly_savings"`
	AnnualSavings      float64 `json:"annual_savings"`
	DataReductionPct   float64 `json:"data_reduction_pct"`
	DataReductionBytes int64   `json:"data_reduction_bytes"`
	SeriesReduction    int64   `json:"series_reduction,omitempty"`
	EventsReduction    int64   `json:"events_reduction,omitempty"`
}

// RecommendedAction describes what to do
type RecommendedAction struct {
	ActionType    string            `json:"action_type"` // create_rule, update_config, etc.
	RuleConfig    *RuleConfig       `json:"rule_config,omitempty"`
	ConfigChanges map[string]string `json:"config_changes,omitempty"`
	APIEndpoint   string            `json:"api_endpoint,omitempty"`
	Payload       string            `json:"payload,omitempty"`
}

// RuleConfig for data shaping rules
type RuleConfig struct {
	Name        string            `json:"name"`
	DataType    string            `json:"data_type"`
	Action      string            `json:"action"` // drop, sample, aggregate, transform
	NamePattern string            `json:"name_pattern,omitempty"`
	TagMatches  map[string]string `json:"tag_matches,omitempty"`
	SampleRate  float64           `json:"sample_rate,omitempty"`
	DropTags    []string          `json:"drop_tags,omitempty"`
	LevelMatch  string            `json:"level_match,omitempty"`
}

// Evidence supporting the recommendation
type Evidence struct {
	Type        string  `json:"type"` // usage_stats, cardinality, query_patterns
	Description string  `json:"description"`
	Value       float64 `json:"value,omitempty"`
	Unit        string  `json:"unit,omitempty"`
	Details     string  `json:"details,omitempty"`
}

// RecommendationReport contains all recommendations
type RecommendationReport struct {
	GeneratedAt       time.Time        `json:"generated_at"`
	TotalSavings      float64          `json:"total_monthly_savings"`
	AnnualSavings     float64          `json:"total_annual_savings"`
	RecommendationCount int            `json:"recommendation_count"`
	ByPriority        map[string]int   `json:"by_priority"`
	ByType            map[string]int   `json:"by_type"`
	Recommendations   []Recommendation `json:"recommendations"`
}

// UsageDataProvider provides usage data for analysis
type UsageDataProvider interface {
	// GetUnusedMetrics returns metrics not queried in the given duration
	GetUnusedMetrics(since time.Duration) []UnusedMetric
	// GetHighVolumeServices returns services with high data volume
	GetHighVolumeServices(threshold int64) []HighVolumeService
	// GetHighCardinalityMetrics returns metrics with high cardinality
	GetHighCardinalityMetrics(threshold int) []HighCardinalityMetric
	// GetLogLevelDistribution returns log counts by level
	GetLogLevelDistribution() map[string]int64
	// GetQueryPatterns returns what data is actually queried
	GetQueryPatterns() []QueryPattern
}

// UnusedMetric represents a metric that hasn't been queried
type UnusedMetric struct {
	Name         string    `json:"name"`
	LastQueried  time.Time `json:"last_queried"`
	DataPoints   int64     `json:"data_points"`
	BytesStored  int64     `json:"bytes_stored"`
	MonthlyCost  float64   `json:"monthly_cost"`
}

// HighVolumeService represents a service generating lots of data
type HighVolumeService struct {
	Service      string  `json:"service"`
	EventsPerDay int64   `json:"events_per_day"`
	BytesPerDay  int64   `json:"bytes_per_day"`
	MonthlyCost  float64 `json:"monthly_cost"`
	DataType     string  `json:"data_type"`
}

// HighCardinalityMetric represents a metric with cardinality issues
type HighCardinalityMetric struct {
	Name           string   `json:"name"`
	UniqueSeriesCount int   `json:"unique_series"`
	ProblematicTags []string `json:"problematic_tags"`
	MonthlyCost    float64  `json:"monthly_cost"`
}

// QueryPattern represents how data is being queried
type QueryPattern struct {
	DataType     string    `json:"data_type"`
	Pattern      string    `json:"pattern"`
	QueryCount   int64     `json:"query_count"`
	LastQueried  time.Time `json:"last_queried"`
}

// RecommendationEngine generates cost optimization recommendations
type RecommendationEngine struct {
}

// NewRecommendationEngine creates a new recommendation engine
func NewRecommendationEngine() *RecommendationEngine {
	return &RecommendationEngine{}
}

// Analyze generates recommendations based on usage data
func (e *RecommendationEngine) Analyze(provider UsageDataProvider) *RecommendationReport {
	report := &RecommendationReport{
		GeneratedAt: time.Now(),
		ByPriority:  make(map[string]int),
		ByType:      make(map[string]int),
	}

	var recommendations []Recommendation

	// Analyze unused metrics
	unusedRecs := e.analyzeUnusedMetrics(provider)
	recommendations = append(recommendations, unusedRecs...)

	// Analyze high-volume services
	volumeRecs := e.analyzeHighVolumeServices(provider)
	recommendations = append(recommendations, volumeRecs...)

	// Analyze high-cardinality metrics
	cardinalityRecs := e.analyzeHighCardinality(provider)
	recommendations = append(recommendations, cardinalityRecs...)

	// Analyze log levels
	logRecs := e.analyzeLogLevels(provider)
	recommendations = append(recommendations, logRecs...)

	// Analyze query patterns for optimization
	queryRecs := e.analyzeQueryPatterns(provider)
	recommendations = append(recommendations, queryRecs...)

	// Sort by savings (highest first)
	sort.Slice(recommendations, func(i, j int) bool {
		return recommendations[i].Impact.MonthlySavings > recommendations[j].Impact.MonthlySavings
	})

	// Calculate totals and counts
	var totalSavings float64
	for _, rec := range recommendations {
		totalSavings += rec.Impact.MonthlySavings
		report.ByPriority[string(rec.Priority)]++
		report.ByType[string(rec.Type)]++
	}

	report.TotalSavings = totalSavings
	report.AnnualSavings = totalSavings * 12
	report.RecommendationCount = len(recommendations)
	report.Recommendations = recommendations

	return report
}

func (e *RecommendationEngine) analyzeUnusedMetrics(provider UsageDataProvider) []Recommendation {
	var recs []Recommendation

	unused := provider.GetUnusedMetrics(7 * 24 * time.Hour) // Not queried in 7 days

	for _, metric := range unused {
		if metric.MonthlyCost < 1.0 { // Skip tiny savings
			continue
		}

		priority := e.calculatePriority(metric.MonthlyCost)

		rec := Recommendation{
			ID:          fmt.Sprintf("unused-%s-%d", metric.Name, time.Now().UnixNano()),
			Type:        RecommendDropUnused,
			Priority:    priority,
			Title:       fmt.Sprintf("Drop unused metric: %s", metric.Name),
			Description: fmt.Sprintf("Metric '%s' hasn't been queried in over 7 days but continues to consume storage and incur costs.", metric.Name),
			DataType:    "metrics",
			Target:      metric.Name,
			Impact: RecommendationImpact{
				MonthlySavings:   metric.MonthlyCost,
				AnnualSavings:    metric.MonthlyCost * 12,
				DataReductionPct: 100,
				SeriesReduction:  metric.DataPoints,
			},
			Action: RecommendedAction{
				ActionType: "create_rule",
				RuleConfig: &RuleConfig{
					Name:        fmt.Sprintf("Drop unused metric %s", metric.Name),
					DataType:    "metric",
					Action:      "drop",
					NamePattern: fmt.Sprintf("^%s$", metric.Name),
				},
				APIEndpoint: "/api/shaping/rules",
			},
			Evidence: []Evidence{
				{
					Type:        "usage_stats",
					Description: "Last queried",
					Details:     metric.LastQueried.Format(time.RFC3339),
				},
				{
					Type:        "usage_stats",
					Description: "Data points stored",
					Value:       float64(metric.DataPoints),
					Unit:        "points",
				},
			},
			CreatedAt: time.Now(),
			Status:    "pending",
		}
		recs = append(recs, rec)
	}

	return recs
}

func (e *RecommendationEngine) analyzeHighVolumeServices(provider UsageDataProvider) []Recommendation {
	var recs []Recommendation

	// Services generating more than 1GB/day
	highVolume := provider.GetHighVolumeServices(1024 * 1024 * 1024)

	for _, svc := range highVolume {
		if svc.MonthlyCost < 5.0 { // Skip small savings
			continue
		}

		// Recommend 50% sampling for high-volume services
		savingsAt50Pct := svc.MonthlyCost * 0.5
		priority := e.calculatePriority(savingsAt50Pct)

		rec := Recommendation{
			ID:          fmt.Sprintf("volume-%s-%d", svc.Service, time.Now().UnixNano()),
			Type:        RecommendSampleHighVolume,
			Priority:    priority,
			Title:       fmt.Sprintf("Sample high-volume service: %s", svc.Service),
			Description: fmt.Sprintf("Service '%s' generates %.1f GB/day of %s data. Sampling at 50%% would maintain statistical validity while reducing costs.", svc.Service, float64(svc.BytesPerDay)/(1024*1024*1024), svc.DataType),
			DataType:    svc.DataType,
			Target:      svc.Service,
			Impact: RecommendationImpact{
				MonthlySavings:     savingsAt50Pct,
				AnnualSavings:      savingsAt50Pct * 12,
				DataReductionPct:   50,
				DataReductionBytes: svc.BytesPerDay * 15, // Half month
				EventsReduction:    svc.EventsPerDay * 15,
			},
			Action: RecommendedAction{
				ActionType: "create_rule",
				RuleConfig: &RuleConfig{
					Name:        fmt.Sprintf("Sample %s at 50%%", svc.Service),
					DataType:    svc.DataType,
					Action:      "sample",
					TagMatches:  map[string]string{"service": svc.Service},
					SampleRate:  0.5,
				},
				APIEndpoint: "/api/shaping/rules",
			},
			Evidence: []Evidence{
				{
					Type:        "usage_stats",
					Description: "Daily volume",
					Value:       float64(svc.BytesPerDay) / (1024 * 1024 * 1024),
					Unit:        "GB/day",
				},
				{
					Type:        "usage_stats",
					Description: "Daily events",
					Value:       float64(svc.EventsPerDay),
					Unit:        "events/day",
				},
			},
			CreatedAt: time.Now(),
			Status:    "pending",
		}
		recs = append(recs, rec)
	}

	return recs
}

func (e *RecommendationEngine) analyzeHighCardinality(provider UsageDataProvider) []Recommendation {
	var recs []Recommendation

	// Metrics with more than 10,000 unique series
	highCardinality := provider.GetHighCardinalityMetrics(10000)

	for _, metric := range highCardinality {
		if metric.MonthlyCost < 5.0 {
			continue
		}

		// Dropping problematic tags could reduce series by 90%+
		estimatedReduction := 0.9
		savings := metric.MonthlyCost * estimatedReduction
		priority := e.calculatePriority(savings)

		rec := Recommendation{
			ID:          fmt.Sprintf("cardinality-%s-%d", metric.Name, time.Now().UnixNano()),
			Type:        RecommendDropTags,
			Priority:    priority,
			Title:       fmt.Sprintf("Drop high-cardinality tags from: %s", metric.Name),
			Description: fmt.Sprintf("Metric '%s' has %d unique series due to high-cardinality tags (%v). Dropping these tags would significantly reduce cardinality.", metric.Name, metric.UniqueSeriesCount, metric.ProblematicTags),
			DataType:    "metrics",
			Target:      metric.Name,
			Impact: RecommendationImpact{
				MonthlySavings:   savings,
				AnnualSavings:    savings * 12,
				DataReductionPct: estimatedReduction * 100,
				SeriesReduction:  int64(float64(metric.UniqueSeriesCount) * estimatedReduction),
			},
			Action: RecommendedAction{
				ActionType: "create_rule",
				RuleConfig: &RuleConfig{
					Name:        fmt.Sprintf("Drop cardinality tags from %s", metric.Name),
					DataType:    "metric",
					Action:      "transform",
					NamePattern: fmt.Sprintf("^%s$", metric.Name),
					DropTags:    metric.ProblematicTags,
				},
				APIEndpoint: "/api/shaping/rules",
			},
			Evidence: []Evidence{
				{
					Type:        "cardinality",
					Description: "Unique series count",
					Value:       float64(metric.UniqueSeriesCount),
					Unit:        "series",
				},
				{
					Type:        "cardinality",
					Description: "Problematic tags",
					Details:     fmt.Sprintf("%v", metric.ProblematicTags),
				},
			},
			CreatedAt: time.Now(),
			Status:    "pending",
		}
		recs = append(recs, rec)
	}

	return recs
}

func (e *RecommendationEngine) analyzeLogLevels(provider UsageDataProvider) []Recommendation {
	var recs []Recommendation

	distribution := provider.GetLogLevelDistribution()

	debugCount := distribution["debug"] + distribution["DEBUG"] + distribution["trace"] + distribution["TRACE"]
	totalCount := int64(0)
	for _, count := range distribution {
		totalCount += count
	}

	if totalCount == 0 {
		return recs
	}

	debugPct := float64(debugCount) / float64(totalCount) * 100

	// If debug logs are more than 20% of total, recommend dropping
	if debugPct > 20 {
		// Estimate cost based on typical log size (500 bytes avg)
		debugBytes := debugCount * 500
		monthlyCost := float64(debugBytes) / (1024 * 1024 * 1024) * 0.10 * 30 // $0.10/GB

		if monthlyCost < 1.0 {
			return recs
		}

		priority := e.calculatePriority(monthlyCost)

		rec := Recommendation{
			ID:          fmt.Sprintf("debug-logs-%d", time.Now().UnixNano()),
			Type:        RecommendDropDebugLogs,
			Priority:    priority,
			Title:       "Drop debug/trace level logs",
			Description: fmt.Sprintf("Debug and trace logs account for %.1f%% of all logs. Dropping these in production would significantly reduce log volume without losing important information.", debugPct),
			DataType:    "logs",
			Target:      "debug,trace",
			Impact: RecommendationImpact{
				MonthlySavings:     monthlyCost,
				AnnualSavings:      monthlyCost * 12,
				DataReductionPct:   debugPct,
				DataReductionBytes: debugBytes * 30,
				EventsReduction:    debugCount * 30,
			},
			Action: RecommendedAction{
				ActionType: "create_rule",
				RuleConfig: &RuleConfig{
					Name:       "Drop debug logs",
					DataType:   "log",
					Action:     "drop",
					LevelMatch: "debug|trace",
				},
				APIEndpoint: "/api/shaping/rules",
			},
			Evidence: []Evidence{
				{
					Type:        "usage_stats",
					Description: "Debug log percentage",
					Value:       debugPct,
					Unit:        "%",
				},
				{
					Type:        "usage_stats",
					Description: "Daily debug events",
					Value:       float64(debugCount),
					Unit:        "events/day",
				},
			},
			CreatedAt: time.Now(),
			Status:    "pending",
		}
		recs = append(recs, rec)
	}

	return recs
}

func (e *RecommendationEngine) analyzeQueryPatterns(provider UsageDataProvider) []Recommendation {
	var recs []Recommendation

	patterns := provider.GetQueryPatterns()

	// Find data types that are ingested but rarely queried
	queryCountByType := make(map[string]int64)
	for _, p := range patterns {
		queryCountByType[p.DataType] += p.QueryCount
	}

	// If traces are queried less than 10 times per day, suggest sampling
	if traceQueries, ok := queryCountByType["traces"]; ok && traceQueries < 10 {
		rec := Recommendation{
			ID:          fmt.Sprintf("low-query-traces-%d", time.Now().UnixNano()),
			Type:        RecommendSampleHighVolume,
			Priority:    PriorityMedium,
			Title:       "Consider sampling traces - low query volume",
			Description: fmt.Sprintf("Traces are queried only %d times per day on average. Consider sampling at 10%% to reduce costs while maintaining enough data for debugging.", traceQueries),
			DataType:    "traces",
			Target:      "all",
			Impact: RecommendationImpact{
				MonthlySavings:   50.0, // Estimate
				AnnualSavings:    600.0,
				DataReductionPct: 90,
			},
			Action: RecommendedAction{
				ActionType: "create_rule",
				RuleConfig: &RuleConfig{
					Name:       "Sample all traces at 10%",
					DataType:   "trace",
					Action:     "sample",
					SampleRate: 0.1,
				},
				APIEndpoint: "/api/shaping/rules",
			},
			Evidence: []Evidence{
				{
					Type:        "query_patterns",
					Description: "Daily trace queries",
					Value:       float64(traceQueries),
					Unit:        "queries/day",
				},
			},
			CreatedAt: time.Now(),
			Status:    "pending",
		}
		recs = append(recs, rec)
	}

	return recs
}

func (e *RecommendationEngine) calculatePriority(monthlySavings float64) RecommendationPriority {
	switch {
	case monthlySavings >= 100:
		return PriorityCritical
	case monthlySavings >= 50:
		return PriorityHigh
	case monthlySavings >= 10:
		return PriorityMedium
	default:
		return PriorityLow
	}
}

// QuickWins returns the top N recommendations by savings
func (report *RecommendationReport) QuickWins(n int) []Recommendation {
	if n > len(report.Recommendations) {
		n = len(report.Recommendations)
	}
	return report.Recommendations[:n]
}

// FilterByType returns recommendations of a specific type
func (report *RecommendationReport) FilterByType(t RecommendationType) []Recommendation {
	var filtered []Recommendation
	for _, rec := range report.Recommendations {
		if rec.Type == t {
			filtered = append(filtered, rec)
		}
	}
	return filtered
}

// FilterByPriority returns recommendations of a specific priority
func (report *RecommendationReport) FilterByPriority(p RecommendationPriority) []Recommendation {
	var filtered []Recommendation
	for _, rec := range report.Recommendations {
		if rec.Priority == p {
			filtered = append(filtered, rec)
		}
	}
	return filtered
}
