package anomaly

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// Service manages anomaly detection for all metrics
type Service struct {
	detector    *Detector
	config      Config
	dataDir     string
	running     atomic.Bool
	stopCh      chan struct{}

	// Metric subscriptions
	subscriptions sync.Map // map[string]bool - metrics to monitor

	// Recent anomalies for API
	recentAnomalies   []AnomalyResult
	recentAnomaliesMu sync.RWMutex
	maxRecentAnomalies int

	// Callbacks
	onAnomaly func(AnomalyResult)

	// Stats
	totalProcessed atomic.Int64
	totalAnomalies atomic.Int64

	mu sync.RWMutex
}

// ServiceConfig for the anomaly service
type ServiceConfig struct {
	DataDir            string  `json:"data_dir"`
	AnomalyThreshold   float64 `json:"anomaly_threshold"`
	CriticalThreshold  float64 `json:"critical_threshold"`
	Algorithm          string  `json:"algorithm"`
	XGBoostModelPath   string  `json:"xgboost_model_path"`
	MaxRecentAnomalies int     `json:"max_recent_anomalies"`
}

// NewService creates a new anomaly detection service
func NewService(cfg ServiceConfig) (*Service, error) {
	detectorConfig := DefaultConfig()

	if cfg.AnomalyThreshold > 0 {
		detectorConfig.AnomalyThreshold = cfg.AnomalyThreshold
	}
	if cfg.CriticalThreshold > 0 {
		detectorConfig.CriticalThreshold = cfg.CriticalThreshold
	}
	if cfg.Algorithm != "" {
		detectorConfig.Algorithm = cfg.Algorithm
	}
	if cfg.XGBoostModelPath != "" {
		detectorConfig.XGBoostModelPath = cfg.XGBoostModelPath
	}

	detector, err := NewDetector(detectorConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create detector: %w", err)
	}

	maxRecent := cfg.MaxRecentAnomalies
	if maxRecent <= 0 {
		maxRecent = 1000
	}

	s := &Service{
		detector:           detector,
		config:             detectorConfig,
		dataDir:            cfg.DataDir,
		stopCh:             make(chan struct{}),
		recentAnomalies:    make([]AnomalyResult, 0, maxRecent),
		maxRecentAnomalies: maxRecent,
	}

	// Set up callback to store anomalies
	detector.SetAnomalyCallback(s.handleAnomaly)

	// Auto-subscribe to common metrics
	s.Subscribe("cpu_percent")
	s.Subscribe("memory_percent")
	s.Subscribe("disk_io_read")
	s.Subscribe("disk_io_write")
	s.Subscribe("network_rx_bytes")
	s.Subscribe("network_tx_bytes")
	s.Subscribe("http_request_latency_p99")
	s.Subscribe("http_error_rate")
	s.Subscribe("tcp_connection_count")

	return s, nil
}

// Start begins the anomaly detection service
func (s *Service) Start() error {
	if s.running.Load() {
		return nil
	}
	s.running.Store(true)

	log.Printf("[anomaly] Service started (algorithm: %s, threshold: %.2f)",
		s.config.Algorithm, s.config.AnomalyThreshold)

	// Load any saved models
	if s.dataDir != "" {
		s.loadModels()
	}

	return nil
}

// Stop stops the anomaly detection service
func (s *Service) Stop() {
	if !s.running.Load() {
		return
	}
	s.running.Store(false)
	close(s.stopCh)

	// Save models
	if s.dataDir != "" {
		s.saveModels()
	}

	log.Printf("[anomaly] Service stopped (processed: %d, anomalies: %d)",
		s.totalProcessed.Load(), s.totalAnomalies.Load())
}

// Subscribe adds a metric to be monitored
func (s *Service) Subscribe(metricName string) {
	s.subscriptions.Store(metricName, true)
}

// Unsubscribe removes a metric from monitoring
func (s *Service) Unsubscribe(metricName string) {
	s.subscriptions.Delete(metricName)
}

// IsSubscribed checks if a metric is being monitored
func (s *Service) IsSubscribed(metricName string) bool {
	_, ok := s.subscriptions.Load(metricName)
	return ok
}

// Push sends a metric value to the anomaly detector
func (s *Service) Push(metricName string, value float64, timestamp time.Time) *AnomalyResult {
	if !s.running.Load() {
		return nil
	}

	// Check if subscribed
	if _, ok := s.subscriptions.Load(metricName); !ok {
		return nil
	}

	s.totalProcessed.Add(1)
	return s.detector.Push(metricName, value, timestamp)
}

// PushBatch efficiently pushes multiple metrics
func (s *Service) PushBatch(metrics map[string]float64, timestamp time.Time) map[string]*AnomalyResult {
	results := make(map[string]*AnomalyResult)

	for name, value := range metrics {
		if result := s.Push(name, value, timestamp); result != nil {
			results[name] = result
		}
	}

	return results
}

// handleAnomaly is called when an anomaly is detected
func (s *Service) handleAnomaly(result AnomalyResult) {
	s.totalAnomalies.Add(1)

	// Store in recent list
	s.recentAnomaliesMu.Lock()
	s.recentAnomalies = append(s.recentAnomalies, result)
	if len(s.recentAnomalies) > s.maxRecentAnomalies {
		s.recentAnomalies = s.recentAnomalies[1:]
	}
	s.recentAnomaliesMu.Unlock()

	// Log
	level := "ANOMALY"
	if result.IsCritical {
		level = "CRITICAL"
	}
	log.Printf("[anomaly] %s: %s = %.2f (score: %.3f) - %s",
		level, result.MetricName, result.Value, result.Score, result.Reason)

	// Call external callback if set
	s.mu.RLock()
	callback := s.onAnomaly
	s.mu.RUnlock()

	if callback != nil {
		callback(result)
	}
}

// SetAnomalyCallback sets an external callback for anomalies
func (s *Service) SetAnomalyCallback(fn func(AnomalyResult)) {
	s.mu.Lock()
	s.onAnomaly = fn
	s.mu.Unlock()
}

// GetRecentAnomalies returns recent anomalies
func (s *Service) GetRecentAnomalies(limit int) []AnomalyResult {
	s.recentAnomaliesMu.RLock()
	defer s.recentAnomaliesMu.RUnlock()

	if limit <= 0 || limit > len(s.recentAnomalies) {
		limit = len(s.recentAnomalies)
	}

	// Return most recent first
	result := make([]AnomalyResult, limit)
	for i := 0; i < limit; i++ {
		result[i] = s.recentAnomalies[len(s.recentAnomalies)-1-i]
	}

	return result
}

// GetMetricStats returns stats for a specific metric
func (s *Service) GetMetricStats(metricName string) map[string]float64 {
	return s.detector.GetMetricStats(metricName)
}

// GetStats returns overall service stats
func (s *Service) GetStats() map[string]interface{} {
	subscribed := 0
	s.subscriptions.Range(func(_, _ interface{}) bool {
		subscribed++
		return true
	})

	return map[string]interface{}{
		"running":          s.running.Load(),
		"algorithm":        s.config.Algorithm,
		"threshold":        s.config.AnomalyThreshold,
		"total_processed":  s.totalProcessed.Load(),
		"total_anomalies":  s.totalAnomalies.Load(),
		"subscribed_metrics": subscribed,
		"recent_anomalies": len(s.recentAnomalies),
	}
}

// TrainModel trains the isolation forest on collected data
func (s *Service) TrainModel(metricName string) error {
	return s.detector.TrainIsolationForest(metricName)
}

// LoadXGBoostModel loads an XGBoost model from file
func (s *Service) LoadXGBoostModel(path string) error {
	return s.detector.LoadXGBoostModel(path)
}

// ExportTrainingData exports data for external model training
func (s *Service) ExportTrainingData(metricName, path string) error {
	return s.detector.ExportTrainingData(metricName, path)
}

// loadModels loads saved models from disk
func (s *Service) loadModels() {
	if s.dataDir == "" {
		return
	}

	// Try to load XGBoost model
	xgbPath := filepath.Join(s.dataDir, "anomaly_model.xgb")
	if _, err := os.Stat(xgbPath); err == nil {
		if err := s.detector.LoadXGBoostModel(xgbPath); err != nil {
			log.Printf("[anomaly] Failed to load XGBoost model: %v", err)
		}
	}
}

// saveModels saves models to disk
func (s *Service) saveModels() {
	if s.dataDir == "" {
		return
	}

	// Save stats
	statsPath := filepath.Join(s.dataDir, "anomaly_stats.json")
	stats := s.GetStats()

	f, err := os.Create(statsPath)
	if err != nil {
		log.Printf("[anomaly] Failed to save stats: %v", err)
		return
	}
	defer f.Close()

	json.NewEncoder(f).Encode(stats)
}

// GetSubscribedMetrics returns list of subscribed metrics
func (s *Service) GetSubscribedMetrics() []string {
	var metrics []string
	s.subscriptions.Range(func(key, _ interface{}) bool {
		metrics = append(metrics, key.(string))
		return true
	})
	return metrics
}
