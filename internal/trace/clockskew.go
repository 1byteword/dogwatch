package trace

import (
	"encoding/json"
	"log"
	"math"
	"sort"
	"sync"
	"time"
)

// ClockSkewConfig configures the clock skew tolerance system
type ClockSkewConfig struct {
	// MaxSkewTolerance is the maximum clock skew to automatically correct
	MaxSkewTolerance time.Duration

	// DetectionWindow is the time window for detecting skew patterns
	DetectionWindow time.Duration

	// MinSamplesForDetection is the minimum span pairs needed to detect skew
	MinSamplesForDetection int

	// ConfidenceThreshold is the minimum confidence for applying corrections
	ConfidenceThreshold float64

	// NTPDriftCheckInterval is how often to check for NTP drift
	NTPDriftCheckInterval time.Duration

	// EnableAutoCorrection enables automatic timestamp correction
	EnableAutoCorrection bool
}

// DefaultClockSkewConfig returns sensible defaults
func DefaultClockSkewConfig() ClockSkewConfig {
	return ClockSkewConfig{
		MaxSkewTolerance:       5 * time.Second,
		DetectionWindow:        5 * time.Minute,
		MinSamplesForDetection: 10,
		ConfidenceThreshold:    0.7,
		NTPDriftCheckInterval:  time.Minute,
		EnableAutoCorrection:   true,
	}
}

// ServicePair represents a pair of services for skew tracking
type ServicePair struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

// SkewMeasurement represents a single skew measurement
type SkewMeasurement struct {
	Timestamp time.Time     `json:"timestamp"`
	Skew      time.Duration `json:"skew"`
	SpanPair  string        `json:"span_pair"` // "parent_span_id:child_span_id"
}

// SkewStats represents aggregated skew statistics for a service pair
type SkewStats struct {
	ServicePair        ServicePair   `json:"service_pair"`
	SampleCount        int           `json:"sample_count"`
	MeanSkew           time.Duration `json:"mean_skew"`
	MedianSkew         time.Duration `json:"median_skew"`
	StdDevSkew         time.Duration `json:"std_dev_skew"`
	MinSkew            time.Duration `json:"min_skew"`
	MaxSkew            time.Duration `json:"max_skew"`
	Confidence         float64       `json:"confidence"` // 0.0 to 1.0
	LastUpdated        time.Time     `json:"last_updated"`
	CorrectionApplied  time.Duration `json:"correction_applied"`
	ViolationCount     int           `json:"violation_count"` // Child starts before parent
	DriftRatePerSecond float64       `json:"drift_rate_per_second"`
}

// TimestampConfidence represents confidence in a span's timestamps
type TimestampConfidence struct {
	SpanID              string    `json:"span_id"`
	StartTimeConfidence float64   `json:"start_time_confidence"` // 0.0 to 1.0
	EndTimeConfidence   float64   `json:"end_time_confidence"`
	CorrectionApplied   bool      `json:"correction_applied"`
	OriginalStartTime   time.Time `json:"original_start_time,omitempty"`
	CorrectedStartTime  time.Time `json:"corrected_start_time,omitempty"`
}

// ClockSkewManager handles clock skew detection and correction
type ClockSkewManager struct {
	config ClockSkewConfig

	mu sync.RWMutex

	// Skew measurements per service pair
	measurements map[ServicePair][]SkewMeasurement

	// Computed skew stats per service pair
	stats map[ServicePair]*SkewStats

	// NTP drift tracking per service
	ntpDrift map[string][]NTPDriftSample

	// Correction offsets to apply
	corrections map[string]time.Duration // service -> correction offset

	// Statistics
	totalSpansProcessed   int64
	totalCorrections      int64
	totalViolationsFixed  int64
	totalViolationsFound  int64

	// Lifecycle
	stopCh  chan struct{}
	started bool
}

// NTPDriftSample tracks drift over time for a service
type NTPDriftSample struct {
	Timestamp   time.Time     `json:"timestamp"`
	Drift       time.Duration `json:"drift"`
	ServiceName string        `json:"service_name"`
}

// NewClockSkewManager creates a new clock skew manager
func NewClockSkewManager(config ClockSkewConfig) *ClockSkewManager {
	return &ClockSkewManager{
		config:       config,
		measurements: make(map[ServicePair][]SkewMeasurement),
		stats:        make(map[ServicePair]*SkewStats),
		ntpDrift:     make(map[string][]NTPDriftSample),
		corrections:  make(map[string]time.Duration),
		stopCh:       make(chan struct{}),
	}
}

// Start begins background processing
func (m *ClockSkewManager) Start() {
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return
	}
	m.started = true
	m.mu.Unlock()

	go m.maintenanceLoop()
	log.Printf("[clockskew] Manager started with max tolerance %v", m.config.MaxSkewTolerance)
}

// Stop halts background processing
func (m *ClockSkewManager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.started {
		return
	}

	close(m.stopCh)
	m.started = false
}

// maintenanceLoop periodically updates stats and cleans old data
func (m *ClockSkewManager) maintenanceLoop() {
	ticker := time.NewTicker(m.config.NTPDriftCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.updateAllStats()
			m.cleanOldMeasurements()
		case <-m.stopCh:
			return
		}
	}
}

// ProcessSpanPair processes a parent-child span pair to detect clock skew
func (m *ClockSkewManager) ProcessSpanPair(parent, child Span) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.totalSpansProcessed++

	// Skip if same service
	if parent.ServiceName == child.ServiceName {
		return
	}

	pair := ServicePair{
		Source: parent.ServiceName,
		Target: child.ServiceName,
	}

	// Calculate skew: positive means child started before parent ended
	// In normal case, child should start after or at parent's start
	skew := parent.StartTime.Sub(child.StartTime)

	// Detect violation: child starts before parent
	isViolation := child.StartTime.Before(parent.StartTime)
	if isViolation {
		m.totalViolationsFound++
	}

	// Record measurement
	measurement := SkewMeasurement{
		Timestamp: time.Now(),
		Skew:      skew,
		SpanPair:  parent.SpanID + ":" + child.SpanID,
	}

	m.measurements[pair] = append(m.measurements[pair], measurement)

	// Update stats if enough samples
	if len(m.measurements[pair]) >= m.config.MinSamplesForDetection {
		m.updateStats(pair)
	}
}

// CorrectSpan applies clock skew correction to a span if needed
func (m *ClockSkewManager) CorrectSpan(span *Span, parentStartTime time.Time) (*TimestampConfidence, bool) {
	if !m.config.EnableAutoCorrection {
		return nil, false
	}

	m.mu.RLock()
	correction, hasCorrection := m.corrections[span.ServiceName]
	m.mu.RUnlock()

	confidence := &TimestampConfidence{
		SpanID:              span.SpanID,
		StartTimeConfidence: 1.0,
		EndTimeConfidence:   1.0,
		CorrectionApplied:   false,
	}

	// Check for parent-child ordering violation
	if span.StartTime.Before(parentStartTime) {
		// Violation detected - child starts before parent
		m.mu.Lock()
		m.totalViolationsFixed++
		m.mu.Unlock()

		confidence.OriginalStartTime = span.StartTime
		confidence.CorrectionApplied = true

		// Adjust child to start at parent start time (minimum valid time)
		adjustment := parentStartTime.Sub(span.StartTime)
		span.StartTime = parentStartTime
		span.EndTime = span.EndTime.Add(adjustment)
		confidence.CorrectedStartTime = span.StartTime

		// Lower confidence since we had to correct
		confidence.StartTimeConfidence = 0.6
		confidence.EndTimeConfidence = 0.6

		return confidence, true
	}

	// Apply service-level correction if available
	if hasCorrection && correction != 0 {
		// Only apply if within tolerance
		if absInt64(int64(correction)) <= int64(m.config.MaxSkewTolerance) {
			confidence.OriginalStartTime = span.StartTime
			confidence.CorrectionApplied = true

			span.StartTime = span.StartTime.Add(correction)
			span.EndTime = span.EndTime.Add(correction)
			confidence.CorrectedStartTime = span.StartTime

			m.mu.Lock()
			m.totalCorrections++
			m.mu.Unlock()

			// Confidence based on how much correction was needed
			confidence.StartTimeConfidence = 1.0 - (float64(absInt64(int64(correction))) / float64(m.config.MaxSkewTolerance))
			confidence.EndTimeConfidence = confidence.StartTimeConfidence

			return confidence, true
		}
	}

	return confidence, false
}

// updateStats computes statistics for a service pair
func (m *ClockSkewManager) updateStats(pair ServicePair) {
	measurements := m.measurements[pair]
	if len(measurements) == 0 {
		return
	}

	// Extract skew values
	skews := make([]time.Duration, len(measurements))
	for i, m := range measurements {
		skews[i] = m.Skew
	}

	// Sort for percentile calculations
	sortedSkews := make([]time.Duration, len(skews))
	copy(sortedSkews, skews)
	sort.Slice(sortedSkews, func(i, j int) bool {
		return sortedSkews[i] < sortedSkews[j]
	})

	// Calculate statistics
	var sum int64
	var violationCount int
	for _, s := range skews {
		sum += int64(s)
		if s > 0 {
			violationCount++
		}
	}
	mean := time.Duration(sum / int64(len(skews)))

	// Median
	median := sortedSkews[len(sortedSkews)/2]

	// Min/Max
	minSkew := sortedSkews[0]
	maxSkew := sortedSkews[len(sortedSkews)-1]

	// Standard deviation
	var sumSquares int64
	for _, s := range skews {
		diff := int64(s - mean)
		sumSquares += diff * diff
	}
	variance := float64(sumSquares) / float64(len(skews))
	stdDev := time.Duration(math.Sqrt(variance))

	// Calculate confidence based on consistency
	// Higher confidence if measurements are consistent (low stddev relative to mean)
	confidence := 1.0
	if absInt64(int64(mean)) > 0 {
		variability := float64(stdDev) / float64(absInt64(int64(mean)))
		confidence = 1.0 - math.Min(variability, 1.0)
	}
	confidence = math.Max(0.1, confidence)

	// Calculate drift rate (skew change over time)
	var driftRate float64
	if len(measurements) >= 2 {
		first := measurements[0]
		last := measurements[len(measurements)-1]
		timeDelta := last.Timestamp.Sub(first.Timestamp).Seconds()
		if timeDelta > 0 {
			skewDelta := float64(last.Skew - first.Skew)
			driftRate = skewDelta / timeDelta / float64(time.Second)
		}
	}

	stats := &SkewStats{
		ServicePair:        pair,
		SampleCount:        len(measurements),
		MeanSkew:           mean,
		MedianSkew:         median,
		StdDevSkew:         stdDev,
		MinSkew:            minSkew,
		MaxSkew:            maxSkew,
		Confidence:         confidence,
		LastUpdated:        time.Now(),
		ViolationCount:     violationCount,
		DriftRatePerSecond: driftRate,
	}

	// Calculate correction if confidence is high enough
	if confidence >= m.config.ConfidenceThreshold {
		// Use median for correction (more robust to outliers)
		stats.CorrectionApplied = median

		// Update service correction
		m.corrections[pair.Target] = -median
	}

	m.stats[pair] = stats
}

// updateAllStats updates stats for all pairs
func (m *ClockSkewManager) updateAllStats() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for pair := range m.measurements {
		m.updateStats(pair)
	}
}

// cleanOldMeasurements removes measurements outside the detection window
func (m *ClockSkewManager) cleanOldMeasurements() {
	m.mu.Lock()
	defer m.mu.Unlock()

	cutoff := time.Now().Add(-m.config.DetectionWindow)

	for pair, measurements := range m.measurements {
		var kept []SkewMeasurement
		for _, meas := range measurements {
			if meas.Timestamp.After(cutoff) {
				kept = append(kept, meas)
			}
		}
		m.measurements[pair] = kept
	}

	// Clean old NTP drift samples
	for service, samples := range m.ntpDrift {
		var kept []NTPDriftSample
		for _, sample := range samples {
			if sample.Timestamp.After(cutoff) {
				kept = append(kept, sample)
			}
		}
		m.ntpDrift[service] = kept
	}
}

// GetSkewStats returns skew statistics for a service pair
func (m *ClockSkewManager) GetSkewStats(source, target string) *SkewStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	pair := ServicePair{Source: source, Target: target}
	return m.stats[pair]
}

// GetAllSkewStats returns all skew statistics
func (m *ClockSkewManager) GetAllSkewStats() []SkewStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]SkewStats, 0, len(m.stats))
	for _, stats := range m.stats {
		result = append(result, *stats)
	}
	return result
}

// GetServiceCorrections returns all active correction offsets
func (m *ClockSkewManager) GetServiceCorrections() map[string]time.Duration {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]time.Duration)
	for k, v := range m.corrections {
		result[k] = v
	}
	return result
}

// RecordNTPDrift records an NTP drift measurement for a service
func (m *ClockSkewManager) RecordNTPDrift(service string, drift time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sample := NTPDriftSample{
		Timestamp:   time.Now(),
		Drift:       drift,
		ServiceName: service,
	}

	m.ntpDrift[service] = append(m.ntpDrift[service], sample)

	// Keep limited history
	if len(m.ntpDrift[service]) > 1000 {
		m.ntpDrift[service] = m.ntpDrift[service][500:]
	}
}

// GetNTPDriftStats returns NTP drift statistics for a service
func (m *ClockSkewManager) GetNTPDriftStats(service string) *NTPDriftStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	samples := m.ntpDrift[service]
	if len(samples) == 0 {
		return nil
	}

	var sum int64
	minDrift := samples[0].Drift
	maxDrift := samples[0].Drift

	for _, s := range samples {
		sum += int64(s.Drift)
		if s.Drift < minDrift {
			minDrift = s.Drift
		}
		if s.Drift > maxDrift {
			maxDrift = s.Drift
		}
	}

	mean := time.Duration(sum / int64(len(samples)))

	return &NTPDriftStats{
		ServiceName: service,
		SampleCount: len(samples),
		MeanDrift:   mean,
		MinDrift:    minDrift,
		MaxDrift:    maxDrift,
		LastSample:  samples[len(samples)-1].Timestamp,
	}
}

// NTPDriftStats represents aggregated NTP drift statistics
type NTPDriftStats struct {
	ServiceName string        `json:"service_name"`
	SampleCount int           `json:"sample_count"`
	MeanDrift   time.Duration `json:"mean_drift"`
	MinDrift    time.Duration `json:"min_drift"`
	MaxDrift    time.Duration `json:"max_drift"`
	LastSample  time.Time     `json:"last_sample"`
}

// GetStats returns overall clock skew manager statistics
func (m *ClockSkewManager) GetStats() ClockSkewStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return ClockSkewStats{
		TotalSpansProcessed:  m.totalSpansProcessed,
		TotalCorrections:     m.totalCorrections,
		TotalViolationsFound: m.totalViolationsFound,
		TotalViolationsFixed: m.totalViolationsFixed,
		ServicePairCount:     len(m.stats),
		CorrectionCount:      len(m.corrections),
	}
}

// ClockSkewStats holds overall statistics
type ClockSkewStats struct {
	TotalSpansProcessed  int64 `json:"total_spans_processed"`
	TotalCorrections     int64 `json:"total_corrections"`
	TotalViolationsFound int64 `json:"total_violations_found"`
	TotalViolationsFixed int64 `json:"total_violations_fixed"`
	ServicePairCount     int   `json:"service_pair_count"`
	CorrectionCount      int   `json:"correction_count"`
}

// SetCorrection manually sets a correction offset for a service
func (m *ClockSkewManager) SetCorrection(service string, correction time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.corrections[service] = correction
	log.Printf("[clockskew] Set manual correction for %s: %v", service, correction)
}

// ClearCorrection removes a correction for a service
func (m *ClockSkewManager) ClearCorrection(service string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.corrections, service)
	log.Printf("[clockskew] Cleared correction for %s", service)
}

// UpdateConfig updates the configuration
func (m *ClockSkewManager) UpdateConfig(config ClockSkewConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.config = config
	log.Printf("[clockskew] Config updated: max tolerance=%v, auto correction=%v",
		config.MaxSkewTolerance, config.EnableAutoCorrection)
}

// GetConfig returns the current configuration
func (m *ClockSkewManager) GetConfig() ClockSkewConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config
}

// Helper function
func absInt64(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

// SkewReport provides a comprehensive skew report
type SkewReport struct {
	GeneratedAt      time.Time       `json:"generated_at"`
	Config           ClockSkewConfig `json:"config"`
	Stats            ClockSkewStats  `json:"stats"`
	ServicePairStats []SkewStats     `json:"service_pair_stats"`
	ActiveCorrections map[string]time.Duration `json:"active_corrections"`
	NTPDriftByService map[string]*NTPDriftStats `json:"ntp_drift_by_service"`
}

// GenerateReport creates a comprehensive skew report
func (m *ClockSkewManager) GenerateReport() *SkewReport {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Get all stats
	pairStats := make([]SkewStats, 0, len(m.stats))
	for _, stats := range m.stats {
		pairStats = append(pairStats, *stats)
	}

	// Sort by violation count descending
	sort.Slice(pairStats, func(i, j int) bool {
		return pairStats[i].ViolationCount > pairStats[j].ViolationCount
	})

	// Get NTP drift stats
	ntpStats := make(map[string]*NTPDriftStats)
	for service := range m.ntpDrift {
		ntpStats[service] = m.GetNTPDriftStats(service)
	}

	// Copy corrections
	corrections := make(map[string]time.Duration)
	for k, v := range m.corrections {
		corrections[k] = v
	}

	return &SkewReport{
		GeneratedAt:       time.Now(),
		Config:            m.config,
		Stats:             m.GetStats(),
		ServicePairStats:  pairStats,
		ActiveCorrections: corrections,
		NTPDriftByService: ntpStats,
	}
}

// MarshalJSON for SkewStats
func (s SkewStats) MarshalJSON() ([]byte, error) {
	type Alias SkewStats
	return json.Marshal(&struct {
		MeanSkew          string `json:"mean_skew_str"`
		MedianSkew        string `json:"median_skew_str"`
		StdDevSkew        string `json:"std_dev_skew_str"`
		MinSkew           string `json:"min_skew_str"`
		MaxSkew           string `json:"max_skew_str"`
		CorrectionApplied string `json:"correction_applied_str"`
		*Alias
	}{
		MeanSkew:          s.MeanSkew.String(),
		MedianSkew:        s.MedianSkew.String(),
		StdDevSkew:        s.StdDevSkew.String(),
		MinSkew:           s.MinSkew.String(),
		MaxSkew:           s.MaxSkew.String(),
		CorrectionApplied: s.CorrectionApplied.String(),
		Alias:             (*Alias)(&s),
	})
}
