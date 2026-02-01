package siem

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"dogwatch/internal/security"
)

// Manager manages SIEM export configurations and workers
type Manager struct {
	store         *Store
	securityStore *security.Store

	workers map[string]*ExportWorker
	mu      sync.RWMutex
	ctx     context.Context
	cancel  context.CancelFunc

	// Stats
	stats   map[string]*ExportStats
	statsMu sync.RWMutex
}

// ExportStats tracks export statistics
type ExportStats struct {
	ConfigID        string    `json:"config_id"`
	ConfigName      string    `json:"config_name"`
	EventsExported  int64     `json:"events_exported"`
	EventsFailed    int64     `json:"events_failed"`
	EventsDropped   int64     `json:"events_dropped"`
	LastExportAt    time.Time `json:"last_export_at"`
	LastError       string    `json:"last_error,omitempty"`
	LastErrorAt     time.Time `json:"last_error_at,omitempty"`
	ExportLatencyMs float64   `json:"export_latency_ms"`
	BytesExported   int64     `json:"bytes_exported"`
}

// ExportWorker handles exporting for a single configuration
type ExportWorker struct {
	config    Config
	formatter Formatter
	exporter  Exporter
	queue     chan SecurityEvent
	stats     *ExportStats

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewManager creates a new SIEM manager
func NewManager(store *Store) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		store:   store,
		workers: make(map[string]*ExportWorker),
		stats:   make(map[string]*ExportStats),
		ctx:     ctx,
		cancel:  cancel,
	}
}

// SetSecurityStore sets the security store for event polling
func (m *Manager) SetSecurityStore(store *security.Store) {
	m.securityStore = store
}

// GetWorkerCount returns the number of active export workers
func (m *Manager) GetWorkerCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.workers)
}

// ProcessSecurityAlert processes and exports a security alert
func (m *Manager) ProcessSecurityAlert(alert *security.SecurityAlert) {
	m.EnqueueSecurityAlert(alert)
}

// Start starts the SIEM manager
func (m *Manager) Start() error {
	// Load and start all enabled configurations
	configs, err := m.store.ListConfigs()
	if err != nil {
		return fmt.Errorf("list configs: %w", err)
	}

	for _, config := range configs {
		if config.Enabled {
			if err := m.StartWorker(config); err != nil {
				log.Printf("[siem] failed to start worker for %s: %v", config.Name, err)
			}
		}
	}

	// Start the security event listener
	go m.listenForSecurityEvents()

	return nil
}

// Stop stops the SIEM manager
func (m *Manager) Stop() {
	m.cancel()

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, worker := range m.workers {
		worker.Stop()
	}
	m.workers = make(map[string]*ExportWorker)
}

// StartWorker starts an export worker for a configuration
func (m *Manager) StartWorker(config Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Stop existing worker if any
	if existing, ok := m.workers[config.ID]; ok {
		existing.Stop()
	}

	// Create formatter
	formatter := GetFormatter(string(config.Format))

	// Create exporter
	exporter, err := CreateExporter(config)
	if err != nil {
		return fmt.Errorf("create exporter: %w", err)
	}

	// Create stats
	m.statsMu.Lock()
	stats := &ExportStats{
		ConfigID:   config.ID,
		ConfigName: config.Name,
	}
	m.stats[config.ID] = stats
	m.statsMu.Unlock()

	// Create worker
	ctx, cancel := context.WithCancel(m.ctx)
	worker := &ExportWorker{
		config:    config,
		formatter: formatter,
		exporter:  exporter,
		queue:     make(chan SecurityEvent, 10000),
		stats:     stats,
		ctx:       ctx,
		cancel:    cancel,
	}

	// Start worker goroutines
	worker.Start()

	m.workers[config.ID] = worker
	log.Printf("[siem] started export worker: %s (%s -> %s)", config.Name, config.Format, config.ExporterType)

	return nil
}

// StopWorker stops an export worker
func (m *Manager) StopWorker(configID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if worker, ok := m.workers[configID]; ok {
		worker.Stop()
		delete(m.workers, configID)
		log.Printf("[siem] stopped export worker: %s", configID)
	}
}

// UpdateConfig updates a configuration and restarts its worker if needed
func (m *Manager) UpdateConfig(config Config) error {
	if err := m.store.SaveConfig(config); err != nil {
		return err
	}

	if config.Enabled {
		return m.StartWorker(config)
	} else {
		m.StopWorker(config.ID)
	}

	return nil
}

// EnqueueEvent queues an event for export
func (m *Manager) EnqueueEvent(event SecurityEvent) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, worker := range m.workers {
		// Check filter
		if !worker.config.Filter.MatchesFilter(event) {
			continue
		}

		// Try to enqueue, drop if full
		select {
		case worker.queue <- event:
		default:
			worker.stats.EventsDropped++
		}
	}
}

// EnqueueSecurityEvent converts and queues a security event
func (m *Manager) EnqueueSecurityEvent(event *security.SecurityEvent) {
	m.EnqueueEvent(ConvertSecurityEvent(event))
}

// EnqueueSecurityAlert converts and queues a security alert
func (m *Manager) EnqueueSecurityAlert(alert *security.SecurityAlert) {
	m.EnqueueEvent(ConvertSecurityAlert(alert))
}

// GetStats returns export statistics
func (m *Manager) GetStats() map[string]*ExportStats {
	m.statsMu.RLock()
	defer m.statsMu.RUnlock()

	result := make(map[string]*ExportStats)
	for k, v := range m.stats {
		statsCopy := *v
		result[k] = &statsCopy
	}
	return result
}

// GetConfigStats returns stats for a specific config
func (m *Manager) GetConfigStats(configID string) *ExportStats {
	m.statsMu.RLock()
	defer m.statsMu.RUnlock()

	if stats, ok := m.stats[configID]; ok {
		statsCopy := *stats
		return &statsCopy
	}
	return nil
}

// listenForSecurityEvents listens for security events from the security store
func (m *Manager) listenForSecurityEvents() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	var lastEventTime time.Time
	var lastAlertTime time.Time

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			// Poll for new events
			if m.securityStore != nil {
				// Get recent events
				events, err := m.securityStore.ListEvents(security.EventFilter{
					StartTime: lastEventTime,
					Limit:     1000,
				})
				if err == nil && len(events) > 0 {
					for i := range events {
						if events[i].Timestamp.After(lastEventTime) {
							lastEventTime = events[i].Timestamp
						}
						m.EnqueueSecurityEvent(&events[i])
					}
				}

				// Get recent alerts
				alerts, err := m.securityStore.ListAlerts(security.AlertFilter{
					StartTime: lastAlertTime,
					Limit:     1000,
				})
				if err == nil && len(alerts) > 0 {
					for i := range alerts {
						if alerts[i].DetectedAt.After(lastAlertTime) {
							lastAlertTime = alerts[i].DetectedAt
						}
						m.EnqueueSecurityAlert(&alerts[i])
					}
				}
			}
		}
	}
}

// ManualExport triggers a manual export of historical events
func (m *Manager) ManualExport(configID string, startTime, endTime time.Time) (int, error) {
	m.mu.RLock()
	worker, ok := m.workers[configID]
	m.mu.RUnlock()

	if !ok {
		// Load config and create temporary worker
		config, err := m.store.GetConfig(configID)
		if err != nil {
			return 0, fmt.Errorf("get config: %w", err)
		}

		formatter := GetFormatter(string(config.Format))
		exporter, err := CreateExporter(*config)
		if err != nil {
			return 0, fmt.Errorf("create exporter: %w", err)
		}
		defer exporter.Close()

		// Export events
		count := 0
		if m.securityStore != nil {
			events, _ := m.securityStore.ListEvents(security.EventFilter{
				StartTime: startTime,
				EndTime:   endTime,
				Limit:     10000,
			})

			var formatted []FormattedEvent
			for i := range events {
				siemEvent := ConvertSecurityEvent(&events[i])
				if config.Filter.MatchesFilter(siemEvent) {
					formatted = append(formatted, FormattedEvent{
						Original:  siemEvent,
						Formatted: formatter.Format(siemEvent),
						Format:    config.Format,
						Timestamp: events[i].Timestamp,
					})
				}
			}

			if len(formatted) > 0 {
				if err := exporter.Export(formatted); err != nil {
					return 0, fmt.Errorf("export: %w", err)
				}
				count = len(formatted)
			}

			alerts, _ := m.securityStore.ListAlerts(security.AlertFilter{
				StartTime: startTime,
				EndTime:   endTime,
				Limit:     10000,
			})

			formatted = nil
			for i := range alerts {
				siemEvent := ConvertSecurityAlert(&alerts[i])
				if config.Filter.MatchesFilter(siemEvent) {
					formatted = append(formatted, FormattedEvent{
						Original:  siemEvent,
						Formatted: formatter.Format(siemEvent),
						Format:    config.Format,
						Timestamp: alerts[i].DetectedAt,
					})
				}
			}

			if len(formatted) > 0 {
				if err := exporter.Export(formatted); err != nil {
					return count, fmt.Errorf("export alerts: %w", err)
				}
				count += len(formatted)
			}
		}

		return count, nil
	}

	// Use existing worker - queue events directly
	count := 0
	if m.securityStore != nil {
		events, _ := m.securityStore.ListEvents(security.EventFilter{
			StartTime: startTime,
			EndTime:   endTime,
			Limit:     10000,
		})

		for i := range events {
			siemEvent := ConvertSecurityEvent(&events[i])
			if worker.config.Filter.MatchesFilter(siemEvent) {
				select {
				case worker.queue <- siemEvent:
					count++
				default:
				}
			}
		}

		alerts, _ := m.securityStore.ListAlerts(security.AlertFilter{
			StartTime: startTime,
			EndTime:   endTime,
			Limit:     10000,
		})

		for i := range alerts {
			siemEvent := ConvertSecurityAlert(&alerts[i])
			if worker.config.Filter.MatchesFilter(siemEvent) {
				select {
				case worker.queue <- siemEvent:
					count++
				default:
				}
			}
		}
	}

	return count, nil
}

// ExportWorker methods

// Start starts the export worker
func (w *ExportWorker) Start() {
	w.wg.Add(1)
	go w.run()
}

// Stop stops the export worker
func (w *ExportWorker) Stop() {
	w.cancel()
	w.wg.Wait()
	w.exporter.Close()
}

// run is the main export loop
func (w *ExportWorker) run() {
	defer w.wg.Done()

	var batch []SecurityEvent
	var flushTimer *time.Timer

	if w.config.Batching.Enabled {
		flushTimer = time.NewTimer(w.config.Batching.FlushInterval)
	}

	flush := func() {
		if len(batch) == 0 {
			return
		}

		start := time.Now()
		formatted := make([]FormattedEvent, len(batch))
		for i, event := range batch {
			formatted[i] = FormattedEvent{
				Original:  event,
				Formatted: w.formatter.Format(event),
				Format:    w.config.Format,
				Timestamp: event.Timestamp,
			}
		}

		err := w.exportWithRetry(formatted)
		latency := time.Since(start).Seconds() * 1000

		w.stats.ExportLatencyMs = latency
		w.stats.LastExportAt = time.Now()

		if err != nil {
			w.stats.EventsFailed += int64(len(batch))
			w.stats.LastError = err.Error()
			w.stats.LastErrorAt = time.Now()
			log.Printf("[siem] export failed: %v", err)
		} else {
			w.stats.EventsExported += int64(len(batch))
			for _, f := range formatted {
				w.stats.BytesExported += int64(len(f.Formatted))
			}
		}

		batch = batch[:0]
	}

	for {
		select {
		case <-w.ctx.Done():
			flush()
			return

		case event := <-w.queue:
			batch = append(batch, event)
			if w.config.Batching.Enabled && len(batch) >= w.config.Batching.MaxEvents {
				flush()
				if flushTimer != nil {
					flushTimer.Reset(w.config.Batching.FlushInterval)
				}
			}
			if !w.config.Batching.Enabled {
				flush()
			}

		case <-func() <-chan time.Time {
			if flushTimer != nil {
				return flushTimer.C
			}
			return nil
		}():
			flush()
			flushTimer.Reset(w.config.Batching.FlushInterval)
		}
	}
}

// exportWithRetry exports events with retry logic
func (w *ExportWorker) exportWithRetry(events []FormattedEvent) error {
	var lastErr error
	backoff := w.config.Retry.InitialBackoff

	for attempt := 0; attempt <= w.config.Retry.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-w.ctx.Done():
				return w.ctx.Err()
			case <-time.After(backoff):
			}
			backoff = time.Duration(float64(backoff) * w.config.Retry.Multiplier)
			if backoff > w.config.Retry.MaxBackoff {
				backoff = w.config.Retry.MaxBackoff
			}
		}

		if err := w.exporter.Export(events); err != nil {
			lastErr = err
			continue
		}
		return nil
	}

	return lastErr
}
