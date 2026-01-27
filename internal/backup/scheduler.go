package backup

import (
	"fmt"
	"log"
	"sync"
	"time"
)

// Scheduler manages automatic backups
type Scheduler struct {
	mu       sync.Mutex
	config   SchedulerConfig
	ticker   *time.Ticker
	stopCh   chan struct{}
	running  bool
	lastRun  time.Time
	lastErr  error
	history  []BackupRun
}

// SchedulerConfig configures automatic backups
type SchedulerConfig struct {
	Enabled   bool              `json:"enabled"`
	DataDir   string            `json:"data_dir"`
	OutputDir string            `json:"output_dir"`
	Interval  time.Duration     `json:"interval"`   // How often to backup
	Retention RetentionPolicy   `json:"retention"`
	Compress  bool              `json:"compress"`
	OnError   func(error)       // Optional error callback
	OnSuccess func(*BackupRun)  // Optional success callback
}

// DefaultSchedulerConfig returns sensible defaults
func DefaultSchedulerConfig(dataDir string) SchedulerConfig {
	return SchedulerConfig{
		Enabled:   false,
		DataDir:   dataDir,
		OutputDir: dataDir,
		Interval:  24 * time.Hour, // Daily backups
		Retention: DefaultRetentionPolicy(),
		Compress:  true,
	}
}

// BackupRun records a backup execution
type BackupRun struct {
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
	Success     bool      `json:"success"`
	Path        string    `json:"path,omitempty"`
	Size        int64     `json:"size,omitempty"`
	FileCount   int       `json:"file_count,omitempty"`
	Error       string    `json:"error,omitempty"`
	Retention   *RetentionResult `json:"retention,omitempty"`
}

// NewScheduler creates a new backup scheduler
func NewScheduler(config SchedulerConfig) *Scheduler {
	return &Scheduler{
		config:  config,
		history: make([]BackupRun, 0, 100),
	}
}

// Start begins the scheduler
func (s *Scheduler) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("scheduler already running")
	}

	if !s.config.Enabled {
		log.Printf("[backup] Scheduler disabled")
		return nil
	}

	if s.config.Interval < time.Minute {
		return fmt.Errorf("backup interval too short: %v (minimum 1 minute)", s.config.Interval)
	}

	s.stopCh = make(chan struct{})
	s.ticker = time.NewTicker(s.config.Interval)
	s.running = true

	go s.run()

	log.Printf("[backup] Scheduler started (interval: %v)", s.config.Interval)
	return nil
}

// Stop stops the scheduler
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return
	}

	close(s.stopCh)
	s.ticker.Stop()
	s.running = false
	log.Printf("[backup] Scheduler stopped")
}

// IsRunning returns whether the scheduler is active
func (s *Scheduler) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// GetStatus returns scheduler status
func (s *Scheduler) GetStatus() SchedulerStatus {
	s.mu.Lock()
	defer s.mu.Unlock()

	status := SchedulerStatus{
		Enabled:  s.config.Enabled,
		Running:  s.running,
		Interval: s.config.Interval,
		LastRun:  s.lastRun,
	}

	if s.lastErr != nil {
		status.LastError = s.lastErr.Error()
	}

	if s.running && !s.lastRun.IsZero() {
		status.NextRun = s.lastRun.Add(s.config.Interval)
	}

	// Include recent history
	histLen := len(s.history)
	if histLen > 10 {
		status.RecentRuns = s.history[histLen-10:]
	} else {
		status.RecentRuns = s.history
	}

	return status
}

// SchedulerStatus provides scheduler information
type SchedulerStatus struct {
	Enabled    bool          `json:"enabled"`
	Running    bool          `json:"running"`
	Interval   time.Duration `json:"interval"`
	LastRun    time.Time     `json:"last_run,omitempty"`
	NextRun    time.Time     `json:"next_run,omitempty"`
	LastError  string        `json:"last_error,omitempty"`
	RecentRuns []BackupRun   `json:"recent_runs,omitempty"`
}

// RunNow triggers an immediate backup
func (s *Scheduler) RunNow() (*BackupRun, error) {
	return s.executeBackup()
}

// UpdateConfig updates scheduler configuration
func (s *Scheduler) UpdateConfig(config SchedulerConfig) error {
	s.mu.Lock()
	wasRunning := s.running
	s.mu.Unlock()

	// Stop if running
	if wasRunning {
		s.Stop()
	}

	// Update config
	s.mu.Lock()
	s.config = config
	s.mu.Unlock()

	// Restart if was running and still enabled
	if wasRunning && config.Enabled {
		return s.Start()
	}

	return nil
}

func (s *Scheduler) run() {
	// Run immediately on start
	s.executeBackup()

	for {
		select {
		case <-s.ticker.C:
			s.executeBackup()
		case <-s.stopCh:
			return
		}
	}
}

func (s *Scheduler) executeBackup() (*BackupRun, error) {
	run := BackupRun{
		StartedAt: time.Now(),
	}

	// Create backup
	result, err := Create(BackupOptions{
		DataDir:    s.config.DataDir,
		OutputPath: "", // Auto-generate
		Compress:   s.config.Compress,
	})

	if err != nil {
		run.Success = false
		run.Error = err.Error()
		run.CompletedAt = time.Now()

		s.mu.Lock()
		s.lastRun = run.StartedAt
		s.lastErr = err
		s.history = append(s.history, run)
		if len(s.history) > 100 {
			s.history = s.history[1:]
		}
		s.mu.Unlock()

		if s.config.OnError != nil {
			s.config.OnError(err)
		}

		log.Printf("[backup] Scheduled backup failed: %v", err)
		return &run, err
	}

	run.Success = true
	run.Path = result.Path
	run.Size = result.Size
	run.FileCount = result.FileCount

	// Apply retention policy
	if s.config.Retention.MaxBackups > 0 || s.config.Retention.MaxAge > 0 {
		retentionResult, retentionErr := ApplyRetention(s.config.OutputDir, s.config.Retention)
		if retentionErr != nil {
			log.Printf("[backup] Retention policy error: %v", retentionErr)
		} else if len(retentionResult.Deleted) > 0 {
			run.Retention = retentionResult
			log.Printf("[backup] Retention: deleted %d old backups, freed %s",
				len(retentionResult.Deleted), FormatSize(retentionResult.BytesFreed))
		}
	}

	run.CompletedAt = time.Now()

	s.mu.Lock()
	s.lastRun = run.StartedAt
	s.lastErr = nil
	s.history = append(s.history, run)
	if len(s.history) > 100 {
		s.history = s.history[1:]
	}
	s.mu.Unlock()

	if s.config.OnSuccess != nil {
		s.config.OnSuccess(&run)
	}

	log.Printf("[backup] Scheduled backup complete: %s (%s, %d files)",
		result.Path, FormatSize(result.Size), result.FileCount)

	return &run, nil
}
