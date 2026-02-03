package scripts

import (
	"bytes"
	"context"
	"testing"
	"time"
)

func TestScriptParseParameters(t *testing.T) {
	script := &Script{
		Name:     "test/script",
		Category: "test",
		Parameters: []Parameter{
			{Name: "threshold", Type: "int", Default: 100},
			{Name: "limit", Type: "int", Default: 20},
			{Name: "duration", Type: "duration", Default: "1h"},
			{Name: "name", Type: "string", Default: "default"},
		},
	}

	tests := []struct {
		name    string
		params  map[string]string
		wantErr bool
		check   func(t *testing.T, parsed ParsedParams)
	}{
		{
			name:   "defaults only",
			params: map[string]string{},
			check: func(t *testing.T, parsed ParsedParams) {
				if parsed["threshold"] != 100 {
					t.Errorf("expected threshold=100, got %v", parsed["threshold"])
				}
				if parsed["limit"] != 20 {
					t.Errorf("expected limit=20, got %v", parsed["limit"])
				}
			},
		},
		{
			name:   "override threshold",
			params: map[string]string{"threshold": "500"},
			check: func(t *testing.T, parsed ParsedParams) {
				if parsed["threshold"] != 500 {
					t.Errorf("expected threshold=500, got %v", parsed["threshold"])
				}
			},
		},
		{
			name:   "parse duration",
			params: map[string]string{"duration": "30m"},
			check: func(t *testing.T, parsed ParsedParams) {
				d, ok := parsed["duration"].(time.Duration)
				if !ok {
					t.Errorf("expected duration to be time.Duration, got %T", parsed["duration"])
					return
				}
				if d != 30*time.Minute {
					t.Errorf("expected 30m, got %v", d)
				}
			},
		},
		{
			name:    "unknown parameter",
			params:  map[string]string{"unknown": "value"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := script.ParseParameters(tt.params)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseParameters() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.check != nil && err == nil {
				tt.check(t, parsed)
			}
		})
	}
}

func TestScriptExpandQuery(t *testing.T) {
	script := &Script{
		Name:  "test/script",
		Query: "SELECT * FROM traces WHERE duration_ms > {{.threshold}} LIMIT {{.limit}}",
		Parameters: []Parameter{
			{Name: "threshold", Type: "int", Default: 100},
			{Name: "limit", Type: "int", Default: 20},
		},
	}

	params := ParsedParams{
		"threshold": 500,
		"limit":     50,
	}

	expanded := script.ExpandQuery(params)
	expected := "SELECT * FROM traces WHERE duration_ms > 500 LIMIT 50"
	if expanded != expected {
		t.Errorf("ExpandQuery() = %q, want %q", expanded, expected)
	}
}

func TestRegistryOperations(t *testing.T) {
	registry := NewRegistry()

	// Register scripts
	registry.Register(&Script{
		Name:        "http/slow_requests",
		Category:    "http",
		Title:       "Slow HTTP Requests",
		Description: "Find slow requests",
	})
	registry.Register(&Script{
		Name:        "mysql/slow_queries",
		Category:    "mysql",
		Title:       "Slow MySQL Queries",
		Description: "Find slow queries",
	})

	// Test Get
	script := registry.Get("http/slow_requests")
	if script == nil {
		t.Error("Get() returned nil for existing script")
	}
	if script.Title != "Slow HTTP Requests" {
		t.Errorf("Get() returned wrong script, got title %q", script.Title)
	}

	// Test Get without category prefix
	script = registry.Get("slow_queries")
	if script == nil {
		t.Error("Get() returned nil when looking up without category prefix")
	}

	// Test Get non-existent
	script = registry.Get("nonexistent/script")
	if script != nil {
		t.Error("Get() returned non-nil for non-existent script")
	}

	// Test List all
	scripts := registry.List("")
	if len(scripts) != 2 {
		t.Errorf("List() returned %d scripts, want 2", len(scripts))
	}

	// Test List by category
	scripts = registry.List("http")
	if len(scripts) != 1 {
		t.Errorf("List(http) returned %d scripts, want 1", len(scripts))
	}

	// Test Categories
	categories := registry.Categories()
	if len(categories) != 2 {
		t.Errorf("Categories() returned %d categories, want 2", len(categories))
	}
}

func TestDefaultRegistryHasScripts(t *testing.T) {
	scripts := DefaultRegistry.List("")
	if len(scripts) == 0 {
		t.Error("DefaultRegistry has no scripts registered")
	}

	// Verify some expected scripts exist
	expectedScripts := []string{
		"http/slow_requests",
		"mysql/slow_queries",
		"k8s/pod_restarts",
		"security/outbound_connections",
	}

	for _, name := range expectedScripts {
		s := DefaultRegistry.Get(name)
		if s == nil {
			t.Errorf("Expected script %q not found in DefaultRegistry", name)
		}
	}
}

func TestOutputFormatterTable(t *testing.T) {
	result := &Result{
		Script: &Script{
			Name: "test/script",
			Columns: []Column{
				{Name: "service", Type: "string"},
				{Name: "count", Type: "int"},
			},
		},
		Columns: []string{"service", "count"},
		Rows: []map[string]interface{}{
			{"service": "api", "count": 100},
			{"service": "web", "count": 50},
		},
		RowCount: 2,
		Duration: 100 * time.Millisecond,
	}

	var buf bytes.Buffer
	formatter := NewFormatter(&buf, FormatTable)
	err := formatter.Format(result)
	if err != nil {
		t.Fatalf("Format() error: %v", err)
	}

	output := buf.String()
	if output == "" {
		t.Error("Format() produced empty output")
	}
	if !bytes.Contains(buf.Bytes(), []byte("api")) {
		t.Error("Format() output missing expected value 'api'")
	}
	if !bytes.Contains(buf.Bytes(), []byte("100")) {
		t.Error("Format() output missing expected value '100'")
	}
}

func TestOutputFormatterJSON(t *testing.T) {
	result := &Result{
		Script: &Script{Name: "test/script"},
		Rows: []map[string]interface{}{
			{"service": "api", "count": 100},
		},
		RowCount: 1,
		Duration: 100 * time.Millisecond,
	}

	var buf bytes.Buffer
	formatter := NewFormatter(&buf, FormatJSON)
	err := formatter.Format(result)
	if err != nil {
		t.Fatalf("Format() error: %v", err)
	}

	output := buf.String()
	if !bytes.Contains(buf.Bytes(), []byte(`"script"`)) {
		t.Error("JSON output missing 'script' field")
	}
	if !bytes.Contains(buf.Bytes(), []byte(`"rows"`)) {
		t.Error("JSON output missing 'rows' field")
	}
	if output == "" {
		t.Error("Format() produced empty output")
	}
}

func TestOutputFormatterCSV(t *testing.T) {
	result := &Result{
		Script: &Script{
			Name: "test/script",
			Columns: []Column{
				{Name: "service", Type: "string"},
				{Name: "count", Type: "int"},
			},
		},
		Columns: []string{"service", "count"},
		Rows: []map[string]interface{}{
			{"service": "api", "count": 100},
			{"service": "web", "count": 50},
		},
		RowCount: 2,
		Duration: 100 * time.Millisecond,
	}

	var buf bytes.Buffer
	formatter := NewFormatter(&buf, FormatCSV)
	err := formatter.Format(result)
	if err != nil {
		t.Fatalf("Format() error: %v", err)
	}

	output := buf.String()
	if !bytes.Contains(buf.Bytes(), []byte("service,count")) {
		t.Error("CSV output missing header row")
	}
	if !bytes.Contains(buf.Bytes(), []byte("api,100")) {
		t.Error("CSV output missing data row")
	}
	if output == "" {
		t.Error("Format() produced empty output")
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
		wantErr  bool
	}{
		{"100ms", 100 * time.Millisecond, false},
		{"5s", 5 * time.Second, false},
		{"30m", 30 * time.Minute, false},
		{"1h", time.Hour, false},
		{"2d", 48 * time.Hour, false},
		{"500", 500 * time.Millisecond, false},
		{"invalid", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			d, err := parseDuration(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseDuration(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && d != tt.expected {
				t.Errorf("parseDuration(%q) = %v, want %v", tt.input, d, tt.expected)
			}
		})
	}
}

func TestRunnerListScripts(t *testing.T) {
	runner := NewRunner(nil, DefaultRegistry)

	// List all
	scripts := runner.ListScripts("")
	if len(scripts) == 0 {
		t.Error("ListScripts() returned empty list")
	}

	// List by category
	httpScripts := runner.ListScripts("http")
	for _, s := range httpScripts {
		if s.Category != "http" {
			t.Errorf("ListScripts(http) returned script with category %q", s.Category)
		}
	}

	// List categories
	categories := runner.ListCategories()
	if len(categories) == 0 {
		t.Error("ListCategories() returned empty list")
	}
}

func TestRunnerRunWithoutExecutor(t *testing.T) {
	runner := NewRunner(nil, DefaultRegistry)

	_, err := runner.Run(context.Background(), "http/slow_requests", RunOptions{})
	if err == nil {
		t.Error("Run() should fail without executor configured")
	}
}
