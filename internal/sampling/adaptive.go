package sampling

import (
	"hash/fnv"
	"log"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"dogwatch/internal/trace"
)

// AdaptiveSampler dynamically adjusts sampling rates based on traffic volume
// to maintain a target throughput while ensuring all traffic has some representation
type AdaptiveSampler struct {
	config AdaptiveSamplerConfig

	// Global rate
	currentRate float64
	rateMu      sync.RWMutex

	// Per-service rates
	serviceRates   map[string]*serviceRate
	serviceRatesMu sync.RWMutex

	// Traffic tracking
	spanCounter    int64 // Spans seen in current window
	sampledCounter int64 // Spans sampled in current window
	lastAdjustment time.Time
	windowStart    time.Time

	// Historical data for rate calculation
	historicalRates []float64
	historyMu       sync.Mutex

	// Stats
	totalSpans   int64
	sampledSpans int64
	droppedSpans int64

	// Adjustment ticker
	adjustTicker *time.Ticker
	stopCh       chan struct{}
}

type serviceRate struct {
	rate           float64
	spanCount      int64
	sampledCount   int64
	lastSeen       time.Time
	windowStart    time.Time
}

// NewAdaptiveSampler creates a new adaptive sampler
func NewAdaptiveSampler(config AdaptiveSamplerConfig) *AdaptiveSampler {
	as := &AdaptiveSampler{
		config:          config,
		currentRate:     config.MaxSampleRate, // Start at max
		serviceRates:    make(map[string]*serviceRate),
		lastAdjustment:  time.Now(),
		windowStart:     time.Now(),
		historicalRates: make([]float64, 0, 10),
		stopCh:          make(chan struct{}),
	}

	// Start adjustment goroutine
	as.adjustTicker = time.NewTicker(config.AdjustmentInterval)
	go as.adjustmentLoop()

	return as
}

// ShouldSample returns the sampling decision based on current adaptive rate
func (as *AdaptiveSampler) ShouldSample(span *trace.Span) Decision {
	if !as.config.Enabled {
		return DecisionKeep
	}

	atomic.AddInt64(&as.totalSpans, 1)
	atomic.AddInt64(&as.spanCounter, 1)

	// Get the effective rate
	rate := as.getEffectiveRate(span.ServiceName)

	// Apply probabilistic sampling using trace ID for consistency
	if as.shouldSampleByRate(span.TraceID, rate) {
		atomic.AddInt64(&as.sampledSpans, 1)
		atomic.AddInt64(&as.sampledCounter, 1)
		as.recordServiceSample(span.ServiceName, true)
		return DecisionKeep
	}

	atomic.AddInt64(&as.droppedSpans, 1)
	as.recordServiceSample(span.ServiceName, false)
	return DecisionDrop
}

// getEffectiveRate returns the sampling rate to use for a service
func (as *AdaptiveSampler) getEffectiveRate(service string) float64 {
	if !as.config.PerServiceRates {
		as.rateMu.RLock()
		rate := as.currentRate
		as.rateMu.RUnlock()
		return rate
	}

	// Check for service-specific rate
	as.serviceRatesMu.RLock()
	sr, exists := as.serviceRates[service]
	as.serviceRatesMu.RUnlock()

	if exists {
		return sr.rate
	}

	// Use global rate for new services
	as.rateMu.RLock()
	rate := as.currentRate
	as.rateMu.RUnlock()
	return rate
}

// recordServiceSample tracks per-service sampling stats
func (as *AdaptiveSampler) recordServiceSample(service string, sampled bool) {
	if !as.config.PerServiceRates {
		return
	}

	as.serviceRatesMu.Lock()
	defer as.serviceRatesMu.Unlock()

	sr, exists := as.serviceRates[service]
	if !exists {
		as.rateMu.RLock()
		globalRate := as.currentRate
		as.rateMu.RUnlock()

		sr = &serviceRate{
			rate:        globalRate,
			windowStart: time.Now(),
		}
		as.serviceRates[service] = sr
	}

	sr.spanCount++
	sr.lastSeen = time.Now()
	if sampled {
		sr.sampledCount++
	}
}

// shouldSampleByRate uses consistent hashing for deterministic sampling
func (as *AdaptiveSampler) shouldSampleByRate(traceID string, rate float64) bool {
	if rate >= 1.0 {
		return true
	}
	if rate <= 0.0 {
		return false
	}

	h := fnv.New64a()
	h.Write([]byte(traceID))
	hashValue := h.Sum64()

	normalized := float64(hashValue) / float64(^uint64(0))
	return normalized < rate
}

// adjustmentLoop periodically recalculates sampling rates
func (as *AdaptiveSampler) adjustmentLoop() {
	for {
		select {
		case <-as.adjustTicker.C:
			as.adjustRates()
		case <-as.stopCh:
			return
		}
	}
}

// adjustRates recalculates the sampling rate based on recent traffic
func (as *AdaptiveSampler) adjustRates() {
	windowDuration := time.Since(as.windowStart).Seconds()
	if windowDuration < 1 {
		windowDuration = 1
	}

	spanCount := atomic.SwapInt64(&as.spanCounter, 0)
	sampledCount := atomic.SwapInt64(&as.sampledCounter, 0)

	// Calculate current throughput (traces per second)
	currentTPS := float64(spanCount) / windowDuration

	// Calculate desired rate to hit target
	var desiredRate float64
	if currentTPS > 0 {
		desiredRate = as.config.TargetTracesPerSecond / currentTPS
	} else {
		desiredRate = as.config.MaxSampleRate
	}

	// Clamp to min/max
	desiredRate = math.Max(as.config.MinSampleRate, math.Min(as.config.MaxSampleRate, desiredRate))

	// Apply smoothing to avoid oscillation
	as.rateMu.Lock()
	oldRate := as.currentRate
	newRate := oldRate + (desiredRate-oldRate)*as.config.SmoothingFactor
	newRate = math.Max(as.config.MinSampleRate, math.Min(as.config.MaxSampleRate, newRate))
	as.currentRate = newRate
	as.rateMu.Unlock()

	// Update per-service rates if enabled
	if as.config.PerServiceRates {
		as.adjustServiceRates(windowDuration)
	}

	// Store historical rate for analysis
	as.historyMu.Lock()
	as.historicalRates = append(as.historicalRates, newRate)
	if len(as.historicalRates) > 60 { // Keep last hour of data (assuming 1-minute intervals)
		as.historicalRates = as.historicalRates[1:]
	}
	as.historyMu.Unlock()

	// Reset window
	as.windowStart = time.Now()
	as.lastAdjustment = time.Now()

	// Log adjustment
	var actualRate float64
	if spanCount > 0 {
		actualRate = float64(sampledCount) / float64(spanCount)
	}
	log.Printf("[adaptive-sampler] Adjusted rate: %.4f -> %.4f (target TPS: %.1f, actual TPS: %.1f, actual rate: %.4f)",
		oldRate, newRate, as.config.TargetTracesPerSecond, currentTPS, actualRate)
}

// adjustServiceRates updates per-service sampling rates
func (as *AdaptiveSampler) adjustServiceRates(windowDuration float64) {
	as.serviceRatesMu.Lock()
	defer as.serviceRatesMu.Unlock()

	as.rateMu.RLock()
	globalRate := as.currentRate
	as.rateMu.RUnlock()

	// Calculate total traffic and per-service contribution
	var totalSpans int64
	for _, sr := range as.serviceRates {
		totalSpans += sr.spanCount
	}

	if totalSpans == 0 {
		return
	}

	// Target spans per service (proportional to traffic)
	targetTotal := as.config.TargetTracesPerSecond * windowDuration

	for service, sr := range as.serviceRates {
		// Skip stale services
		if time.Since(sr.lastSeen) > 5*time.Minute {
			delete(as.serviceRates, service)
			continue
		}

		// Calculate service's share of the target
		trafficShare := float64(sr.spanCount) / float64(totalSpans)
		serviceTarget := targetTotal * trafficShare

		// Calculate needed rate
		var desiredRate float64
		if sr.spanCount > 0 {
			desiredRate = serviceTarget / float64(sr.spanCount)
		} else {
			desiredRate = globalRate
		}

		// Clamp and smooth
		desiredRate = math.Max(as.config.MinSampleRate, math.Min(as.config.MaxSampleRate, desiredRate))
		newRate := sr.rate + (desiredRate-sr.rate)*as.config.SmoothingFactor
		newRate = math.Max(as.config.MinSampleRate, math.Min(as.config.MaxSampleRate, newRate))

		sr.rate = newRate
		sr.spanCount = 0
		sr.sampledCount = 0
		sr.windowStart = time.Now()
	}
}

// GetStats returns current sampler statistics
func (as *AdaptiveSampler) GetStats() SamplerStats {
	total := atomic.LoadInt64(&as.totalSpans)
	sampled := atomic.LoadInt64(&as.sampledSpans)
	dropped := atomic.LoadInt64(&as.droppedSpans)

	as.rateMu.RLock()
	rate := as.currentRate
	as.rateMu.RUnlock()

	return SamplerStats{
		TotalSpans:   total,
		SampledSpans: sampled,
		DroppedSpans: dropped,
		CurrentRate:  rate,
	}
}

// GetCurrentRate returns the current global sampling rate
func (as *AdaptiveSampler) GetCurrentRate() float64 {
	as.rateMu.RLock()
	defer as.rateMu.RUnlock()
	return as.currentRate
}

// GetServiceRates returns a map of service-specific rates
func (as *AdaptiveSampler) GetServiceRates() map[string]float64 {
	as.serviceRatesMu.RLock()
	defer as.serviceRatesMu.RUnlock()

	result := make(map[string]float64)
	for service, sr := range as.serviceRates {
		result[service] = sr.rate
	}
	return result
}

// SetRate manually sets the global sampling rate
func (as *AdaptiveSampler) SetRate(rate float64) {
	rate = math.Max(as.config.MinSampleRate, math.Min(as.config.MaxSampleRate, rate))

	as.rateMu.Lock()
	as.currentRate = rate
	as.rateMu.Unlock()
}

// SetServiceRate manually sets the rate for a specific service
func (as *AdaptiveSampler) SetServiceRate(service string, rate float64) {
	rate = math.Max(as.config.MinSampleRate, math.Min(as.config.MaxSampleRate, rate))

	as.serviceRatesMu.Lock()
	sr, exists := as.serviceRates[service]
	if !exists {
		sr = &serviceRate{
			windowStart: time.Now(),
		}
		as.serviceRates[service] = sr
	}
	sr.rate = rate
	sr.lastSeen = time.Now()
	as.serviceRatesMu.Unlock()
}

// UpdateConfig updates the sampler configuration
func (as *AdaptiveSampler) UpdateConfig(config AdaptiveSamplerConfig) {
	as.rateMu.Lock()
	as.config = config

	// Clamp current rate to new bounds
	as.currentRate = math.Max(config.MinSampleRate, math.Min(config.MaxSampleRate, as.currentRate))
	as.rateMu.Unlock()

	// Update adjustment interval
	as.adjustTicker.Reset(config.AdjustmentInterval)
}

// GetConfig returns the current configuration
func (as *AdaptiveSampler) GetConfig() AdaptiveSamplerConfig {
	as.rateMu.RLock()
	defer as.rateMu.RUnlock()
	return as.config
}

// GetHistoricalRates returns recent rate adjustments
func (as *AdaptiveSampler) GetHistoricalRates() []float64 {
	as.historyMu.Lock()
	defer as.historyMu.Unlock()
	return append([]float64{}, as.historicalRates...)
}

// GetRateStatistics returns statistics about rate adjustments
func (as *AdaptiveSampler) GetRateStatistics() RateStatistics {
	as.historyMu.Lock()
	rates := append([]float64{}, as.historicalRates...)
	as.historyMu.Unlock()

	if len(rates) == 0 {
		as.rateMu.RLock()
		current := as.currentRate
		as.rateMu.RUnlock()
		return RateStatistics{
			Current: current,
			Min:     current,
			Max:     current,
			Mean:    current,
		}
	}

	var sum, min, max float64
	min = rates[0]
	max = rates[0]

	for _, r := range rates {
		sum += r
		if r < min {
			min = r
		}
		if r > max {
			max = r
		}
	}

	mean := sum / float64(len(rates))

	// Calculate standard deviation
	var variance float64
	for _, r := range rates {
		variance += (r - mean) * (r - mean)
	}
	stddev := math.Sqrt(variance / float64(len(rates)))

	as.rateMu.RLock()
	current := as.currentRate
	as.rateMu.RUnlock()

	return RateStatistics{
		Current:   current,
		Min:       min,
		Max:       max,
		Mean:      mean,
		StdDev:    stddev,
		Samples:   len(rates),
	}
}

// RateStatistics provides statistics about rate adjustments
type RateStatistics struct {
	Current float64 `json:"current"`
	Min     float64 `json:"min"`
	Max     float64 `json:"max"`
	Mean    float64 `json:"mean"`
	StdDev  float64 `json:"std_dev"`
	Samples int     `json:"samples"`
}

// Stop stops the adaptive sampler adjustment goroutine
func (as *AdaptiveSampler) Stop() {
	close(as.stopCh)
	as.adjustTicker.Stop()
}

// ForceAdjust immediately triggers a rate adjustment
func (as *AdaptiveSampler) ForceAdjust() {
	as.adjustRates()
}
