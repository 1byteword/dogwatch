// Package anomaly provides fast ML-based anomaly detection using XGBoost and Isolation Forest
package anomaly

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dmitryikh/leaves"
)

// Config for anomaly detection
type Config struct {
	// Window sizes for feature extraction
	ShortWindow  int     `json:"short_window"`  // e.g., 5 minutes
	MediumWindow int     `json:"medium_window"` // e.g., 30 minutes
	LongWindow   int     `json:"long_window"`   // e.g., 2 hours

	// Anomaly thresholds
	AnomalyThreshold   float64 `json:"anomaly_threshold"`   // Score above this = anomaly
	CriticalThreshold  float64 `json:"critical_threshold"`  // Score above this = critical
	MinDataPoints      int     `json:"min_data_points"`     // Minimum points before detection

	// Model paths
	XGBoostModelPath string `json:"xgboost_model_path"`

	// Algorithm selection
	Algorithm string `json:"algorithm"` // "xgboost", "iforest", "ensemble", "statistical"
}

// DefaultConfig returns sensible defaults
func DefaultConfig() Config {
	return Config{
		ShortWindow:       30,   // 30 samples (~5 min at 10s intervals)
		MediumWindow:      180,  // 180 samples (~30 min)
		LongWindow:        720,  // 720 samples (~2 hours)
		AnomalyThreshold:  0.7,
		CriticalThreshold: 0.9,
		MinDataPoints:     60,
		Algorithm:         "ensemble",
	}
}

// AnomalyResult represents a detection result
type AnomalyResult struct {
	MetricName  string    `json:"metric_name"`
	Timestamp   time.Time `json:"timestamp"`
	Value       float64   `json:"value"`
	Score       float64   `json:"score"`       // 0-1, higher = more anomalous
	IsAnomaly   bool      `json:"is_anomaly"`
	IsCritical  bool      `json:"is_critical"`
	Reason      string    `json:"reason"`
	Features    []float64 `json:"features,omitempty"`
	Prediction  float64   `json:"prediction"` // Expected value
	Deviation   float64   `json:"deviation"`  // How far from expected
}

// Detector is the main anomaly detection engine
type Detector struct {
	config     Config
	metrics    sync.Map // map[string]*MetricBuffer
	xgbModel   *leaves.Ensemble
	iforest    *IsolationForest
	modelReady atomic.Bool

	// Callbacks
	onAnomaly func(AnomalyResult)

	mu sync.RWMutex
}

// MetricBuffer is a lock-free circular buffer for metric values
type MetricBuffer struct {
	values    []float64
	times     []int64
	head      int64
	size      int64
	capacity  int64

	// Pre-computed rolling stats (updated atomically)
	shortMean   atomic.Pointer[float64]
	shortStd    atomic.Pointer[float64]
	mediumMean  atomic.Pointer[float64]
	mediumStd   atomic.Pointer[float64]
	longMean    atomic.Pointer[float64]
	longStd     atomic.Pointer[float64]
}

// NewDetector creates a new anomaly detector
func NewDetector(config Config) (*Detector, error) {
	d := &Detector{
		config:  config,
		iforest: NewIsolationForest(100, 256), // 100 trees, 256 sample size
	}

	// Try to load XGBoost model if specified
	if config.XGBoostModelPath != "" {
		if err := d.LoadXGBoostModel(config.XGBoostModelPath); err != nil {
			// Non-fatal, fall back to other algorithms
			fmt.Printf("[anomaly] Warning: Could not load XGBoost model: %v\n", err)
		}
	}

	return d, nil
}

// LoadXGBoostModel loads a trained XGBoost model
func (d *Detector) LoadXGBoostModel(path string) error {
	model, err := leaves.XGEnsembleFromFile(path, false)
	if err != nil {
		return fmt.Errorf("failed to load XGBoost model: %w", err)
	}

	d.mu.Lock()
	d.xgbModel = model
	d.mu.Unlock()
	d.modelReady.Store(true)

	fmt.Printf("[anomaly] Loaded XGBoost model from %s\n", path)
	return nil
}

// SetAnomalyCallback sets the callback for detected anomalies
func (d *Detector) SetAnomalyCallback(fn func(AnomalyResult)) {
	d.mu.Lock()
	d.onAnomaly = fn
	d.mu.Unlock()
}

// Push adds a new data point for a metric
func (d *Detector) Push(metricName string, value float64, ts time.Time) *AnomalyResult {
	// Get or create buffer
	bufI, _ := d.metrics.LoadOrStore(metricName, newMetricBuffer(d.config.LongWindow * 2))
	buf := bufI.(*MetricBuffer)

	// Add to buffer
	buf.Push(value, ts.UnixNano())

	// Check if we have enough data
	size := atomic.LoadInt64(&buf.size)
	if size < int64(d.config.MinDataPoints) {
		return nil
	}

	// Extract features and detect
	features := d.extractFeatures(buf, value)
	result := d.detect(metricName, value, ts, features)

	if result.IsAnomaly {
		d.mu.RLock()
		callback := d.onAnomaly
		d.mu.RUnlock()

		if callback != nil {
			callback(*result)
		}
	}

	return result
}

// extractFeatures computes features for anomaly detection
// This is highly optimized for speed
func (d *Detector) extractFeatures(buf *MetricBuffer, currentValue float64) []float64 {
	features := make([]float64, 0, 20)

	// Get recent values
	shortVals := buf.GetLast(d.config.ShortWindow)
	mediumVals := buf.GetLast(d.config.MediumWindow)
	longVals := buf.GetLast(d.config.LongWindow)

	// Short window stats
	shortMean, shortStd := meanStdFast(shortVals)
	features = append(features, shortMean, shortStd)

	// Medium window stats
	mediumMean, mediumStd := meanStdFast(mediumVals)
	features = append(features, mediumMean, mediumStd)

	// Long window stats
	longMean, longStd := meanStdFast(longVals)
	features = append(features, longMean, longStd)

	// Z-scores at different windows
	if shortStd > 0 {
		features = append(features, (currentValue-shortMean)/shortStd)
	} else {
		features = append(features, 0)
	}
	if mediumStd > 0 {
		features = append(features, (currentValue-mediumMean)/mediumStd)
	} else {
		features = append(features, 0)
	}
	if longStd > 0 {
		features = append(features, (currentValue-longMean)/longStd)
	} else {
		features = append(features, 0)
	}

	// Rate of change
	if len(shortVals) >= 2 {
		roc := (currentValue - shortVals[0]) / float64(len(shortVals))
		features = append(features, roc)
	} else {
		features = append(features, 0)
	}

	// Percentile position
	features = append(features, percentilePosition(longVals, currentValue))

	// Min/Max ratio
	minV, maxV := minMaxFast(longVals)
	if maxV-minV > 0 {
		features = append(features, (currentValue-minV)/(maxV-minV))
	} else {
		features = append(features, 0.5)
	}

	// Trend (linear regression slope)
	features = append(features, linearSlope(shortVals))

	// Volatility (std of differences)
	features = append(features, volatility(shortVals))

	// Current value
	features = append(features, currentValue)

	return features
}

// detect runs the anomaly detection algorithm
func (d *Detector) detect(name string, value float64, ts time.Time, features []float64) *AnomalyResult {
	result := &AnomalyResult{
		MetricName: name,
		Timestamp:  ts,
		Value:      value,
		Features:   features,
	}

	var scores []float64

	switch d.config.Algorithm {
	case "xgboost":
		if d.modelReady.Load() {
			score := d.predictXGBoost(features)
			scores = append(scores, score)
		} else {
			scores = append(scores, d.statisticalScore(features))
		}

	case "iforest":
		score := d.iforest.Score(features)
		scores = append(scores, score)

	case "statistical":
		scores = append(scores, d.statisticalScore(features))

	case "ensemble":
		// Combine multiple methods
		scores = append(scores, d.statisticalScore(features))
		scores = append(scores, d.iforest.Score(features))
		if d.modelReady.Load() {
			scores = append(scores, d.predictXGBoost(features))
		}

	default:
		scores = append(scores, d.statisticalScore(features))
	}

	// Aggregate scores
	if len(scores) > 0 {
		result.Score = maxFloat(scores) // Use max for conservative detection
	}

	// Compute prediction and deviation
	if len(features) >= 2 {
		result.Prediction = features[0] // Short-term mean
		result.Deviation = math.Abs(value - result.Prediction)
	}

	// Determine anomaly status
	result.IsAnomaly = result.Score >= d.config.AnomalyThreshold
	result.IsCritical = result.Score >= d.config.CriticalThreshold

	// Generate reason
	if result.IsAnomaly {
		result.Reason = d.generateReason(features, result)
	}

	return result
}

// predictXGBoost runs XGBoost model prediction
func (d *Detector) predictXGBoost(features []float64) float64 {
	d.mu.RLock()
	model := d.xgbModel
	d.mu.RUnlock()

	if model == nil {
		return 0
	}

	// leaves.PredictSingle returns the prediction value directly
	prediction := model.PredictSingle(features, 0)

	// Sigmoid to convert to probability
	return sigmoid(prediction)
}

// statisticalScore computes anomaly score using statistical methods
func (d *Detector) statisticalScore(features []float64) float64 {
	if len(features) < 9 {
		return 0
	}

	// Z-scores are at indices 6, 7, 8
	zShort := math.Abs(features[6])
	zMedium := math.Abs(features[7])
	zLong := math.Abs(features[8])

	// Maximum z-score
	maxZ := math.Max(math.Max(zShort, zMedium), zLong)

	// Convert to 0-1 score using CDF
	// P(|Z| > z) for standard normal
	score := 2 * (1 - normalCDF(maxZ))

	// Invert so higher = more anomalous
	return 1 - score
}

// generateReason creates a human-readable explanation
func (d *Detector) generateReason(features []float64, result *AnomalyResult) string {
	if len(features) < 12 {
		return "Anomalous behavior detected"
	}

	zShort := features[6]
	zMedium := features[7]
	zLong := features[8]
	roc := features[9]
	percentile := features[10]

	if math.Abs(zShort) > 3 {
		if zShort > 0 {
			return fmt.Sprintf("Sudden spike: %.1f std above short-term average", zShort)
		}
		return fmt.Sprintf("Sudden drop: %.1f std below short-term average", -zShort)
	}

	if math.Abs(zMedium) > 3 {
		if zMedium > 0 {
			return fmt.Sprintf("Elevated: %.1f std above medium-term average", zMedium)
		}
		return fmt.Sprintf("Depressed: %.1f std below medium-term average", -zMedium)
	}

	if math.Abs(zLong) > 3 {
		if zLong > 0 {
			return fmt.Sprintf("Unusually high: %.1f std above long-term average", zLong)
		}
		return fmt.Sprintf("Unusually low: %.1f std below long-term average", -zLong)
	}

	if math.Abs(roc) > 0.5 {
		if roc > 0 {
			return "Rapid increase detected"
		}
		return "Rapid decrease detected"
	}

	if percentile > 0.99 {
		return "Value at historical maximum"
	}
	if percentile < 0.01 {
		return "Value at historical minimum"
	}

	return fmt.Sprintf("Anomaly score: %.2f", result.Score)
}

// GetMetricStats returns current stats for a metric
func (d *Detector) GetMetricStats(metricName string) map[string]float64 {
	bufI, ok := d.metrics.Load(metricName)
	if !ok {
		return nil
	}
	buf := bufI.(*MetricBuffer)

	vals := buf.GetLast(d.config.LongWindow)
	if len(vals) == 0 {
		return nil
	}

	mean, std := meanStdFast(vals)
	minV, maxV := minMaxFast(vals)

	return map[string]float64{
		"mean":   mean,
		"std":    std,
		"min":    minV,
		"max":    maxV,
		"count":  float64(len(vals)),
		"last":   vals[len(vals)-1],
	}
}

// TrainIsolationForest trains the isolation forest on historical data
func (d *Detector) TrainIsolationForest(metricName string) error {
	bufI, ok := d.metrics.Load(metricName)
	if !ok {
		return fmt.Errorf("metric not found: %s", metricName)
	}
	buf := bufI.(*MetricBuffer)

	vals := buf.GetLast(d.config.LongWindow)
	if len(vals) < 100 {
		return fmt.Errorf("not enough data points: %d", len(vals))
	}

	// Create training samples (sliding window features)
	var samples [][]float64
	for i := d.config.ShortWindow; i < len(vals); i++ {
		window := vals[i-d.config.ShortWindow : i]
		mean, std := meanStdFast(window)
		current := vals[i]

		sample := []float64{
			current,
			mean,
			std,
			(current - mean) / (std + 1e-10),
		}
		samples = append(samples, sample)
	}

	d.iforest.Fit(samples)
	return nil
}

// ExportTrainingData exports data for external model training
func (d *Detector) ExportTrainingData(metricName string, path string) error {
	bufI, ok := d.metrics.Load(metricName)
	if !ok {
		return fmt.Errorf("metric not found: %s", metricName)
	}
	buf := bufI.(*MetricBuffer)

	vals := buf.GetLast(d.config.LongWindow)

	// Create feature matrix
	var data []map[string]interface{}
	for i := d.config.ShortWindow; i < len(vals); i++ {
		shortWindow := vals[max(0, i-d.config.ShortWindow):i]
		mediumWindow := vals[max(0, i-d.config.MediumWindow):i]

		shortMean, shortStd := meanStdFast(shortWindow)
		mediumMean, mediumStd := meanStdFast(mediumWindow)

		current := vals[i]

		row := map[string]interface{}{
			"value":        current,
			"short_mean":   shortMean,
			"short_std":    shortStd,
			"medium_mean":  mediumMean,
			"medium_std":   mediumStd,
			"z_short":      (current - shortMean) / (shortStd + 1e-10),
			"z_medium":     (current - mediumMean) / (mediumStd + 1e-10),
		}
		data = append(data, row)
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	return json.NewEncoder(f).Encode(data)
}

// ============ MetricBuffer Implementation ============

func newMetricBuffer(capacity int) *MetricBuffer {
	return &MetricBuffer{
		values:   make([]float64, capacity),
		times:    make([]int64, capacity),
		capacity: int64(capacity),
	}
}

func (b *MetricBuffer) Push(value float64, timestamp int64) {
	head := atomic.AddInt64(&b.head, 1) - 1
	idx := head % b.capacity

	b.values[idx] = value
	b.times[idx] = timestamp

	size := atomic.LoadInt64(&b.size)
	if size < b.capacity {
		atomic.AddInt64(&b.size, 1)
	}
}

func (b *MetricBuffer) GetLast(n int) []float64 {
	size := atomic.LoadInt64(&b.size)
	head := atomic.LoadInt64(&b.head)

	if size == 0 {
		return nil
	}

	count := int64(n)
	if count > size {
		count = size
	}

	result := make([]float64, count)
	for i := int64(0); i < count; i++ {
		idx := (head - count + i) % b.capacity
		if idx < 0 {
			idx += b.capacity
		}
		result[i] = b.values[idx]
	}

	return result
}

// ============ Fast Math Functions ============

// meanStdFast computes mean and standard deviation in a single pass
func meanStdFast(vals []float64) (mean, std float64) {
	n := len(vals)
	if n == 0 {
		return 0, 0
	}

	// Welford's online algorithm for numerical stability
	var m, s float64
	for i, v := range vals {
		delta := v - m
		m += delta / float64(i+1)
		s += delta * (v - m)
	}

	mean = m
	if n > 1 {
		std = math.Sqrt(s / float64(n-1))
	}
	return
}

// minMaxFast finds min and max in single pass
func minMaxFast(vals []float64) (min, max float64) {
	if len(vals) == 0 {
		return 0, 0
	}
	min, max = vals[0], vals[0]
	for _, v := range vals[1:] {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	return
}

// percentilePosition returns where value falls in the distribution (0-1)
func percentilePosition(vals []float64, value float64) float64 {
	if len(vals) == 0 {
		return 0.5
	}

	count := 0
	for _, v := range vals {
		if v <= value {
			count++
		}
	}
	return float64(count) / float64(len(vals))
}

// linearSlope computes the slope of a linear regression
func linearSlope(vals []float64) float64 {
	n := float64(len(vals))
	if n < 2 {
		return 0
	}

	var sumX, sumY, sumXY, sumX2 float64
	for i, y := range vals {
		x := float64(i)
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
	}

	denom := n*sumX2 - sumX*sumX
	if denom == 0 {
		return 0
	}

	return (n*sumXY - sumX*sumY) / denom
}

// volatility computes standard deviation of first differences
func volatility(vals []float64) float64 {
	if len(vals) < 2 {
		return 0
	}

	diffs := make([]float64, len(vals)-1)
	for i := 1; i < len(vals); i++ {
		diffs[i-1] = vals[i] - vals[i-1]
	}

	_, std := meanStdFast(diffs)
	return std
}

// sigmoid function
func sigmoid(x float64) float64 {
	return 1 / (1 + math.Exp(-x))
}

// normalCDF approximation
func normalCDF(x float64) float64 {
	// Abramowitz and Stegun approximation
	const a1, a2, a3, a4, a5 = 0.254829592, -0.284496736, 1.421413741, -1.453152027, 1.061405429
	const p = 0.3275911

	sign := 1.0
	if x < 0 {
		sign = -1
		x = -x
	}

	t := 1.0 / (1.0 + p*x)
	y := 1.0 - (((((a5*t+a4)*t)+a3)*t+a2)*t+a1)*t*math.Exp(-x*x)

	return 0.5 * (1.0 + sign*y)
}

func maxFloat(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	m := vals[0]
	for _, v := range vals[1:] {
		if v > m {
			m = v
		}
	}
	return m
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Sort helper for isolation forest
type float64Slice []float64

func (s float64Slice) Len() int           { return len(s) }
func (s float64Slice) Less(i, j int) bool { return s[i] < s[j] }
func (s float64Slice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

func sortFloat64(vals []float64) {
	sort.Sort(float64Slice(vals))
}
