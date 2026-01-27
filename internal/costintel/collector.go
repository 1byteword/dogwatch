package costintel

import (
	"log"
	"sync"
	"time"
)

// UsageProvider is an interface for collecting usage metrics
type UsageProvider interface {
	CollectUsage() UsageMetrics
}

// Collector periodically collects cost estimates
type Collector struct {
	store         *Store
	calculator    *Calculator
	usageProvider UsageProvider
	interval      time.Duration

	mu       sync.Mutex
	running  bool
	stopCh   chan struct{}
	doneCh   chan struct{}
}

// NewCollector creates a new cost collector
func NewCollector(store *Store, usageProvider UsageProvider, interval time.Duration) *Collector {
	if interval < time.Minute {
		interval = time.Hour // Default to hourly collection
	}

	return &Collector{
		store:         store,
		calculator:    NewCalculator(),
		usageProvider: usageProvider,
		interval:      interval,
		stopCh:        make(chan struct{}),
		doneCh:        make(chan struct{}),
	}
}

// Start begins periodic cost collection
func (c *Collector) Start() {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return
	}
	c.running = true
	c.stopCh = make(chan struct{})
	c.doneCh = make(chan struct{})
	c.mu.Unlock()

	go c.run()
}

// Stop stops the collector
func (c *Collector) Stop() {
	c.mu.Lock()
	if !c.running {
		c.mu.Unlock()
		return
	}
	c.running = false
	close(c.stopCh)
	c.mu.Unlock()

	<-c.doneCh
}

func (c *Collector) run() {
	defer close(c.doneCh)

	// Collect immediately on start
	c.collect()

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.collect()
		case <-c.stopCh:
			return
		}
	}
}

func (c *Collector) collect() {
	if c.usageProvider == nil {
		return
	}

	usage := c.usageProvider.CollectUsage()
	comparison := c.calculator.Compare(usage)

	if err := c.store.RecordComparison(comparison); err != nil {
		log.Printf("Failed to record cost estimates: %v", err)
	}
}

// CollectNow forces an immediate collection
func (c *Collector) CollectNow() error {
	if c.usageProvider == nil {
		return nil
	}

	usage := c.usageProvider.CollectUsage()
	comparison := c.calculator.Compare(usage)
	return c.store.RecordComparison(comparison)
}

// GetStore returns the underlying store
func (c *Collector) GetStore() *Store {
	return c.store
}

// GetCalculator returns the calculator
func (c *Collector) GetCalculator() *Calculator {
	return c.calculator
}
