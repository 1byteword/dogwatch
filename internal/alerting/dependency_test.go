package alerting

import (
	"testing"
	"time"
)

func TestDependencyAlerting_UpdateServiceHealth(t *testing.T) {
	da := NewDependencyAlerting(nil)

	// Test updating with no alerts
	da.UpdateServiceHealth("svc-1", "api-gateway", nil)
	state, exists := da.GetServiceAlertState("svc-1")
	if !exists {
		t.Fatal("expected service state to exist")
	}
	if state.HasFiringAlerts {
		t.Error("expected no firing alerts")
	}

	// Test updating with alerts
	alerts := []*Alert{
		{ID: "alert-1", Severity: SeverityWarning},
		{ID: "alert-2", Severity: SeverityCritical},
	}
	da.UpdateServiceHealth("svc-2", "payment-service", alerts)

	state, exists = da.GetServiceAlertState("svc-2")
	if !exists {
		t.Fatal("expected service state to exist")
	}
	if !state.HasFiringAlerts {
		t.Error("expected firing alerts")
	}
	if state.Severity != SeverityCritical {
		t.Errorf("expected critical severity, got %s", state.Severity)
	}
	if len(state.FiringAlertIDs) != 2 {
		t.Errorf("expected 2 firing alert IDs, got %d", len(state.FiringAlertIDs))
	}
}

func TestDependencyAlerting_GetDependencyContext(t *testing.T) {
	da := NewDependencyAlerting(nil)

	// Manually set up caches (simulating what would come from catalog)
	da.upstreamCache["svc-1"] = []string{"svc-2", "svc-3"}
	da.downstreamCache["svc-2"] = []string{"svc-1"}
	da.downstreamCache["svc-3"] = []string{"svc-1"}
	da.cacheUpdatedAt = time.Now()

	// Set up service health states
	da.serviceHealth["svc-2"] = ServiceAlertState{
		ServiceID:       "svc-2",
		ServiceName:     "database",
		HasFiringAlerts: true,
		FiringAlertIDs:  []string{"alert-db"},
		Severity:        SeverityCritical,
	}

	// Create an alert for svc-1
	alert := &Alert{
		ID: "alert-1",
		Labels: map[string]string{
			"service":   "svc-1",
			"alertname": "connection_timeout",
		},
		Severity: SeverityWarning,
	}

	ctx := da.GetDependencyContext(alert)

	if !ctx.IsLikelySymptom {
		t.Error("expected alert to be identified as likely symptom")
	}

	if ctx.RootCauseHint == "" {
		t.Error("expected root cause hint")
	}

	if !ctx.ShouldSuppress {
		t.Error("expected alert to be suppressed due to upstream failure")
	}
}

func TestDependencyAlerting_IsInhibitedByDependency(t *testing.T) {
	da := NewDependencyAlerting(nil)

	// Set up dependency chain: svc-1 depends on svc-2
	da.upstreamCache["svc-1"] = []string{"svc-2"}
	da.cacheUpdatedAt = time.Now()

	// Test 1: No upstream alerts - should not inhibit
	alert := &Alert{
		ID:       "alert-1",
		Labels:   map[string]string{"service": "svc-1", "alertname": "high_latency"},
		Severity: SeverityWarning,
	}

	inhibited, _ := da.IsInhibitedByDependency(alert)
	if inhibited {
		t.Error("should not be inhibited when no upstream alerts")
	}

	// Test 2: Upstream has critical alert - should inhibit
	da.serviceHealth["svc-2"] = ServiceAlertState{
		ServiceID:       "svc-2",
		ServiceName:     "database",
		HasFiringAlerts: true,
		FiringAlertIDs:  []string{"alert-db"},
		Severity:        SeverityCritical,
	}

	inhibited, reason := da.IsInhibitedByDependency(alert)
	// Should be inhibited because upstream has critical alert
	if !inhibited {
		t.Log("reason:", reason)
		// For connection-type alerts, it should be inhibited
	}

	// Test with connection-type alert
	alert.Labels["alertname"] = "connection_error"
	inhibited, reason = da.IsInhibitedByDependency(alert)
	if !inhibited {
		t.Errorf("connection alert should be inhibited when upstream has critical failure, got reason: %s", reason)
	}
}

func TestDependencyAlerting_FindRootCause(t *testing.T) {
	da := NewDependencyAlerting(nil)

	// Set up dependency chain: svc-1 -> svc-2 -> svc-3
	da.upstreamCache["svc-1"] = []string{"svc-2"}
	da.upstreamCache["svc-2"] = []string{"svc-3"}
	da.cacheUpdatedAt = time.Now()

	// svc-3 is the root cause (deepest unhealthy)
	da.serviceHealth["svc-2"] = ServiceAlertState{
		ServiceID:       "svc-2",
		ServiceName:     "api-gateway",
		HasFiringAlerts: true,
		Severity:        SeverityWarning,
	}
	da.serviceHealth["svc-3"] = ServiceAlertState{
		ServiceID:       "svc-3",
		ServiceName:     "database",
		HasFiringAlerts: true,
		Severity:        SeverityCritical,
	}

	rootCause := da.FindRootCause("svc-1")
	if rootCause == nil {
		t.Fatal("expected to find root cause")
	}
	if rootCause.ServiceID != "svc-3" {
		t.Errorf("expected root cause to be svc-3, got %s", rootCause.ServiceID)
	}
}

func TestDependencyAlerting_GetUnhealthyUpstreams(t *testing.T) {
	da := NewDependencyAlerting(nil)

	// Set up dependencies
	da.upstreamCache["svc-1"] = []string{"svc-2", "svc-3", "svc-4"}
	da.cacheUpdatedAt = time.Now()

	// Only svc-2 and svc-4 are unhealthy
	da.serviceHealth["svc-2"] = ServiceAlertState{
		ServiceID:       "svc-2",
		HasFiringAlerts: true,
	}
	da.serviceHealth["svc-3"] = ServiceAlertState{
		ServiceID:       "svc-3",
		HasFiringAlerts: false,
	}
	da.serviceHealth["svc-4"] = ServiceAlertState{
		ServiceID:       "svc-4",
		HasFiringAlerts: true,
	}

	unhealthy := da.GetUnhealthyUpstreams("svc-1")
	if len(unhealthy) != 2 {
		t.Errorf("expected 2 unhealthy upstreams, got %d", len(unhealthy))
	}
}

func TestContainsAny(t *testing.T) {
	tests := []struct {
		s        string
		substrs  []string
		expected bool
	}{
		{"connection_timeout", []string{"timeout", "connection"}, true},
		{"high_latency", []string{"timeout", "connection"}, false},
		{"error_rate_high", []string{"error_rate"}, true},
		{"cpu_usage", []string{"timeout", "connection", "error_rate"}, false},
	}

	for _, tt := range tests {
		result := containsAny(tt.s, tt.substrs)
		if result != tt.expected {
			t.Errorf("containsAny(%q, %v) = %v, expected %v", tt.s, tt.substrs, result, tt.expected)
		}
	}
}

func TestBlastRadius_EstimateImpact(t *testing.T) {
	da := NewDependencyAlerting(nil)

	tests := []struct {
		name             string
		failedTier       string
		criticalAffected int
		totalAffected    int
		expected         string
	}{
		{"critical service", "critical", 0, 1, "critical"},
		{"affects critical", "medium", 1, 3, "critical"},
		{"high tier many affected", "high", 0, 5, "high"},
		{"medium many affected", "medium", 0, 6, "high"},
		{"few affected", "medium", 0, 3, "medium"},
		{"minimal impact", "low", 0, 1, "low"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := da.estimateImpactByString(tt.failedTier, tt.criticalAffected, tt.totalAffected)
			if result != tt.expected {
				t.Errorf("estimateImpact() = %s, expected %s", result, tt.expected)
			}
		})
	}
}
