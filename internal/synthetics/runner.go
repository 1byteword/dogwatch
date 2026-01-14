package synthetics

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"dogwatch/internal/watch"
)

// HealthCallback is called after each check with service health info
type HealthCallback func(serviceID string, passing bool, responseTimeMs float64)

// Runner executes synthetic checks on schedule
type Runner struct {
	store          *Store
	notifier       *watch.Notifier
	healthCallback HealthCallback
	client         *http.Client
	stopChan       chan struct{}
	wg             sync.WaitGroup

	// Track last run time for each check
	lastRun   map[string]time.Time
	lastRunMu sync.RWMutex
}

// NewRunner creates a new synthetic check runner
func NewRunner(store *Store, notifier *watch.Notifier) *Runner {
	return &Runner{
		store:    store,
		notifier: notifier,
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: false},
			},
		},
		stopChan: make(chan struct{}),
		lastRun:  make(map[string]time.Time),
	}
}

// SetHealthCallback sets a callback to be invoked after each check
// with service health information for Service Catalog integration
func (r *Runner) SetHealthCallback(cb HealthCallback) {
	r.healthCallback = cb
}

// Start begins the check runner loop
func (r *Runner) Start() {
	r.wg.Add(1)
	go r.runLoop()
	log.Println("[synthetics] Runner started")
}

// Stop stops the check runner
func (r *Runner) Stop() {
	close(r.stopChan)
	r.wg.Wait()
	log.Println("[synthetics] Runner stopped")
}

func (r *Runner) runLoop() {
	defer r.wg.Done()

	// Check every 5 seconds for checks that need to run
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.stopChan:
			return
		case <-ticker.C:
			r.runDueChecks()
		}
	}
}

func (r *Runner) runDueChecks() {
	checks, err := r.store.ListEnabledChecks()
	if err != nil {
		log.Printf("[synthetics] Failed to list checks: %v", err)
		return
	}

	now := time.Now()

	for _, check := range checks {
		r.lastRunMu.RLock()
		lastRun := r.lastRun[check.ID]
		r.lastRunMu.RUnlock()

		// Check if it's time to run this check
		interval := time.Duration(check.Interval) * time.Second
		if now.Sub(lastRun) >= interval {
			go r.executeCheck(check)

			r.lastRunMu.Lock()
			r.lastRun[check.ID] = now
			r.lastRunMu.Unlock()
		}
	}
}

// RunCheck executes a single check immediately (for testing)
func (r *Runner) RunCheck(checkID string) (*CheckResult, error) {
	check, err := r.store.GetCheck(checkID)
	if err != nil {
		return nil, err
	}
	if check == nil {
		return nil, fmt.Errorf("check not found")
	}

	return r.executeCheckSync(*check), nil
}

func (r *Runner) executeCheck(check Check) {
	result := r.executeCheckSync(check)

	// Record result
	if err := r.store.RecordResult(result); err != nil {
		log.Printf("[synthetics] Failed to record result for %s: %v", check.Name, err)
	}

	// Update check status
	if err := r.store.UpdateCheckStatus(check.ID, result.Status, result.LatencyMs); err != nil {
		log.Printf("[synthetics] Failed to update status for %s: %v", check.Name, err)
	}

	// Update service health via callback if check is linked to a service
	if check.ServiceID != "" && r.healthCallback != nil {
		passing := result.Status == StatusUp
		r.healthCallback(check.ServiceID, passing, float64(result.LatencyMs))
	}

	// Send notification if check failed and has channels configured
	if result.Status != StatusUp && len(check.Channels) > 0 && r.notifier != nil {
		r.sendNotification(check, result)
	}
}

func (r *Runner) executeCheckSync(check Check) *CheckResult {
	result := &CheckResult{
		CheckID:   check.ID,
		Timestamp: time.Now(),
	}

	switch check.Type {
	case CheckHTTP:
		r.executeHTTPCheck(check, result)
	case CheckTCP:
		r.executeTCPCheck(check, result)
	default:
		r.executeHTTPCheck(check, result)
	}

	return result
}

func (r *Runner) executeHTTPCheck(check Check, result *CheckResult) {
	// Create request
	req, err := http.NewRequest(check.Method, check.URL, strings.NewReader(check.Body))
	if err != nil {
		result.Status = StatusDown
		result.Error = fmt.Sprintf("Invalid request: %v", err)
		return
	}

	// Add headers
	for k, v := range check.Headers {
		req.Header.Set(k, v)
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "Dogwatch-Synthetics/1.0")
	}

	// Create client with specific timeout
	client := &http.Client{
		Timeout: time.Duration(check.Timeout) * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: false},
		},
	}

	// Execute request
	start := time.Now()
	resp, err := client.Do(req)
	elapsed := time.Since(start)
	result.LatencyMs = elapsed.Milliseconds()

	if err != nil {
		result.Status = StatusDown
		result.Error = err.Error()
		return
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode

	// Read body (limited)
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 10*1024))
	result.Body = string(body)

	// Run assertions
	result.Status = StatusUp
	for _, assertion := range check.Assertions {
		if !r.checkAssertion(assertion, resp, result.Body, result.LatencyMs) {
			result.Status = StatusDown
			result.Error = fmt.Sprintf("Assertion failed: %s %s %s", assertion.Type, assertion.Operator, assertion.Value)
			break
		}
	}

	// Default assertion: 2xx status code
	if len(check.Assertions) == 0 && (resp.StatusCode < 200 || resp.StatusCode >= 300) {
		result.Status = StatusDown
		result.Error = fmt.Sprintf("Unexpected status code: %d", resp.StatusCode)
	}
}

func (r *Runner) executeTCPCheck(check Check, result *CheckResult) {
	// For TCP checks, we just try to connect
	start := time.Now()

	// Parse URL to get host:port
	url := check.URL
	if !strings.Contains(url, "://") {
		url = "tcp://" + url
	}

	// Use net.DialTimeout for TCP checks
	conn, err := (&http.Transport{}).DialContext(
		nil, "tcp", strings.TrimPrefix(url, "tcp://"),
	)
	elapsed := time.Since(start)
	result.LatencyMs = elapsed.Milliseconds()

	if err != nil {
		result.Status = StatusDown
		result.Error = err.Error()
		return
	}
	conn.Close()

	result.Status = StatusUp
}

func (r *Runner) checkAssertion(a Assertion, resp *http.Response, body string, latencyMs int64) bool {
	switch a.Type {
	case "status_code":
		expected, _ := strconv.Atoi(a.Value)
		return compareInt(resp.StatusCode, expected, a.Operator)

	case "body_contains":
		if a.Operator == "contains" || a.Operator == "equals" {
			return strings.Contains(body, a.Value)
		}
		if a.Operator == "not_contains" {
			return !strings.Contains(body, a.Value)
		}

	case "response_time":
		expected, _ := strconv.ParseInt(a.Value, 10, 64)
		return compareInt64(latencyMs, expected, a.Operator)

	case "header":
		headerVal := resp.Header.Get(a.Target)
		if a.Operator == "equals" {
			return headerVal == a.Value
		}
		if a.Operator == "contains" {
			return strings.Contains(headerVal, a.Value)
		}
		if a.Operator == "exists" {
			return headerVal != ""
		}
	}

	return true
}

func compareInt(actual, expected int, op string) bool {
	switch op {
	case "equals", "eq":
		return actual == expected
	case "not_equals", "ne":
		return actual != expected
	case "less_than", "lt":
		return actual < expected
	case "greater_than", "gt":
		return actual > expected
	case "less_than_or_equals", "lte":
		return actual <= expected
	case "greater_than_or_equals", "gte":
		return actual >= expected
	}
	return actual == expected
}

func compareInt64(actual, expected int64, op string) bool {
	switch op {
	case "equals", "eq":
		return actual == expected
	case "less_than", "lt":
		return actual < expected
	case "greater_than", "gt":
		return actual > expected
	case "less_than_or_equals", "lte":
		return actual <= expected
	case "greater_than_or_equals", "gte":
		return actual >= expected
	}
	return actual <= expected // default: response time should be less than or equal
}

func (r *Runner) sendNotification(check Check, result *CheckResult) {
	if r.notifier == nil {
		return
	}

	// Get channels from watch store (reusing watch infrastructure)
	// For now, just log the failure
	log.Printf("[synthetics] ALERT: %s is %s - %s", check.Name, result.Status, result.Error)

	// TODO: Integrate with watch.Notifier to send to configured channels
}
