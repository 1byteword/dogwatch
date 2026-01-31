package lookout

import (
	"testing"
	"time"
)

func TestNewEngine(t *testing.T) {
	cfg := DefaultConfig()
	engine := NewEngine(cfg)

	if engine == nil {
		t.Fatal("expected non-nil engine")
	}

	if engine.cacheTTL != cfg.CacheTTL {
		t.Errorf("expected cacheTTL %v, got %v", cfg.CacheTTL, engine.cacheTTL)
	}
}

func TestGetOverview_Empty(t *testing.T) {
	engine := NewEngine(DefaultConfig())
	overview := engine.GetOverview()

	if overview == nil {
		t.Fatal("expected non-nil overview")
	}

	if overview.TotalItems != 0 {
		t.Errorf("expected 0 total items, got %d", overview.TotalItems)
	}

	if len(overview.Critical) != 0 {
		t.Errorf("expected 0 critical items, got %d", len(overview.Critical))
	}

	if len(overview.Warning) != 0 {
		t.Errorf("expected 0 warning items, got %d", len(overview.Warning))
	}

	if len(overview.Info) != 0 {
		t.Errorf("expected 0 info items, got %d", len(overview.Info))
	}
}

func TestGetOverview_Cached(t *testing.T) {
	cfg := Config{CacheTTL: 1 * time.Second}
	engine := NewEngine(cfg)

	// First call
	overview1 := engine.GetOverview()

	// Second call should return cached
	overview2 := engine.GetOverview()

	if overview1.Timestamp != overview2.Timestamp {
		t.Error("expected cached overview to have same timestamp")
	}
}

func TestSeverityRank(t *testing.T) {
	tests := []struct {
		severity Severity
		expected int
	}{
		{SeverityCritical, 3},
		{SeverityWarning, 2},
		{SeverityInfo, 1},
		{"unknown", 0},
	}

	for _, tt := range tests {
		result := severityRank(tt.severity)
		if result != tt.expected {
			t.Errorf("severityRank(%s) = %d, expected %d", tt.severity, result, tt.expected)
		}
	}
}

func TestImpactRank(t *testing.T) {
	tests := []struct {
		impact   string
		expected int
	}{
		{"critical", 4},
		{"high", 3},
		{"medium", 2},
		{"low", 1},
		{"unknown", 0},
	}

	for _, tt := range tests {
		result := impactRank(tt.impact)
		if result != tt.expected {
			t.Errorf("impactRank(%s) = %d, expected %d", tt.impact, result, tt.expected)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		duration time.Duration
		expected string
	}{
		{30 * time.Second, "just now"},
		{1 * time.Minute, "1 minute"},
		{5 * time.Minute, "5 minutes"},
		{1 * time.Hour, "1 hour"},
		{3 * time.Hour, "3 hours"},
		{24 * time.Hour, "1 day"},
		{48 * time.Hour, "2 days"},
	}

	for _, tt := range tests {
		result := formatDuration(tt.duration)
		if result != tt.expected {
			t.Errorf("formatDuration(%v) = %q, expected %q", tt.duration, result, tt.expected)
		}
	}
}

func TestEstimateImpact(t *testing.T) {
	tests := []struct {
		score      float64
		isCritical bool
		expected   string
	}{
		{0.95, true, "critical"},
		{0.95, false, "high"},
		{0.85, false, "high"},
		{0.7, false, "medium"},
		{0.5, false, "low"},
	}

	for _, tt := range tests {
		result := estimateImpact(tt.score, tt.isCritical)
		if result != tt.expected {
			t.Errorf("estimateImpact(%.2f, %v) = %q, expected %q", tt.score, tt.isCritical, result, tt.expected)
		}
	}
}

func TestItemTypes(t *testing.T) {
	// Verify type constants are defined correctly
	if TypeAnomaly != "anomaly" {
		t.Errorf("TypeAnomaly = %q, expected %q", TypeAnomaly, "anomaly")
	}
	if TypeAlert != "alert" {
		t.Errorf("TypeAlert = %q, expected %q", TypeAlert, "alert")
	}
	if TypeChange != "change" {
		t.Errorf("TypeChange = %q, expected %q", TypeChange, "change")
	}
	if TypeHealth != "health" {
		t.Errorf("TypeHealth = %q, expected %q", TypeHealth, "health")
	}
}
