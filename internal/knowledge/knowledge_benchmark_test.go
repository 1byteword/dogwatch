package knowledge

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Helper to create a test store with SQLite
func createTestStore(b *testing.B) (*Store, func()) {
	b.Helper()
	tmpDir, err := os.MkdirTemp("", "knowledge_bench_*")
	if err != nil {
		b.Fatalf("failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		b.Fatalf("failed to create store: %v", err)
	}

	cleanup := func() {
		store.Close()
		os.RemoveAll(tmpDir)
	}

	return store, cleanup
}

// Helper to create a test registry
func createTestRegistry(b *testing.B) (*Registry, func()) {
	b.Helper()
	store, cleanup := createTestStore(b)

	// Pre-populate with test data
	macros := []struct {
		name       string
		expression string
		args       []string
	}{
		{"error_filter", "level = 'error' OR level = 'fatal'", nil},
		{"service_filter", "service = '$1'", []string{"service_name"}},
		{"time_range", "timestamp >= now() - $1", []string{"duration"}},
		{"http_errors", "status_code >= 400 AND status_code < 600", nil},
		{"slow_requests", "duration_ms > $1", []string{"threshold"}},
	}

	for _, m := range macros {
		macro := &Macro{Expression: m.expression, Args: m.args}
		def, _ := json.Marshal(macro)
		obj := &KnowledgeObject{
			Name:       m.name,
			Type:       TypeMacro,
			Definition: string(def),
			Owner:      "test",
			Shared:     true,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}
		store.Create(obj)
	}

	// Add field extractions
	extractions := []struct {
		name        string
		pattern     string
		sourceField string
	}{
		{"apache_log", `(?P<ip>\d+\.\d+\.\d+\.\d+) - - \[(?P<timestamp>[^\]]+)\] "(?P<method>\w+) (?P<path>[^ ]+)"`, "message"},
		{"json_fields", `\{[^}]+\}`, "message"},
		{"key_value", `(?P<key>\w+)=(?P<value>[^ ]+)`, "message"},
	}

	for _, e := range extractions {
		fe := &FieldExtraction{Pattern: e.pattern, SourceField: e.sourceField}
		def, _ := json.Marshal(fe)
		obj := &KnowledgeObject{
			Name:       e.name,
			Type:       TypeFieldExtraction,
			Definition: string(def),
			Owner:      "test",
			Shared:     true,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}
		store.Create(obj)
	}

	// Add lookups
	lookupData := make(map[string]map[string]string)
	for i := 0; i < 1000; i++ {
		lookupData[fmt.Sprintf("code_%d", i)] = map[string]string{
			"description": fmt.Sprintf("Description for code %d", i),
			"category":    fmt.Sprintf("category_%d", i%10),
			"severity":    []string{"low", "medium", "high", "critical"}[i%4],
		}
	}
	lookup := &Lookup{
		KeyField:     "error_code",
		OutputFields: []string{"description", "category", "severity"},
		Data:         lookupData,
	}
	def, _ := json.Marshal(lookup)
	obj := &KnowledgeObject{
		Name:       "error_codes",
		Type:       TypeLookup,
		Definition: string(def),
		Owner:      "test",
		Shared:     true,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	store.Create(obj)

	registry := NewRegistry(store)

	return registry, cleanup
}

func BenchmarkMacroExpansion(b *testing.B) {
	registry, cleanup := createTestRegistry(b)
	defer cleanup()

	expander := NewExpander(registry)

	tests := []struct {
		name  string
		query string
	}{
		{"SimpleMacro", "logs | where `error_filter`"},
		{"MacroWithArg", "logs | where `service_filter(api-gateway)`"},
		{"MultipleMacros", "logs | where `error_filter` and `time_range(1h)`"},
		{"NestedMacros", "logs | where `error_filter` | where `http_errors`"},
		{"NoMacros", "logs | where level = 'info'"},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = expander.ExpandMacros(tt.query)
			}
		})
	}
}

func BenchmarkFieldExtraction(b *testing.B) {
	registry, cleanup := createTestRegistry(b)
	defer cleanup()

	expander := NewExpander(registry)

	// Sample log rows
	rows := []Row{
		{"message": `192.168.1.100 - - [01/Jan/2024:10:00:00 +0000] "GET /api/users" 200`},
		{"message": `10.0.0.50 - - [01/Jan/2024:10:00:01 +0000] "POST /api/orders" 201`},
		{"message": `172.16.0.1 - - [01/Jan/2024:10:00:02 +0000] "DELETE /api/items/123" 404`},
	}

	b.Run("ApacheLog", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = expander.ApplyFieldExtraction("apache_log", rows)
		}
	})

	b.Run("KeyValue", func(b *testing.B) {
		kvRows := []Row{
			{"message": "user=john status=active role=admin"},
			{"message": "error=timeout service=api duration=5000"},
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = expander.ApplyFieldExtraction("key_value", kvRows)
		}
	})
}

func BenchmarkFieldExtractionByRowCount(b *testing.B) {
	registry, cleanup := createTestRegistry(b)
	defer cleanup()

	expander := NewExpander(registry)

	rowCounts := []int{10, 100, 1000, 10000}

	for _, count := range rowCounts {
		rows := make([]Row, count)
		for i := 0; i < count; i++ {
			rows[i] = Row{
				"message": fmt.Sprintf(`192.168.1.%d - - [01/Jan/2024:10:00:%02d +0000] "GET /api/item/%d" 200`, i%256, i%60, i),
			}
		}

		b.Run(fmt.Sprintf("%dRows", count), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = expander.ApplyFieldExtraction("apache_log", rows)
			}
		})
	}
}

func BenchmarkLookupEnrichment(b *testing.B) {
	registry, cleanup := createTestRegistry(b)
	defer cleanup()

	expander := NewExpander(registry)

	// Rows with error codes to look up
	rows := make([]Row, 100)
	for i := 0; i < 100; i++ {
		rows[i] = Row{
			"error_code": fmt.Sprintf("code_%d", i*10),
			"message":    "Some error occurred",
		}
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = expander.ApplyLookup("error_codes", rows)
	}
}

func BenchmarkLookupEnrichmentBySize(b *testing.B) {
	store, cleanup := createTestStore(b)
	defer cleanup()

	lookupSizes := []int{100, 1000, 10000}

	for _, size := range lookupSizes {
		// Create lookup with specified size
		lookupData := make(map[string]map[string]string)
		for i := 0; i < size; i++ {
			lookupData[fmt.Sprintf("key_%d", i)] = map[string]string{
				"value1": fmt.Sprintf("val1_%d", i),
				"value2": fmt.Sprintf("val2_%d", i),
			}
		}
		lookup := &Lookup{
			KeyField:     "lookup_key",
			OutputFields: []string{"value1", "value2"},
			Data:         lookupData,
		}
		def, _ := json.Marshal(lookup)
		obj := &KnowledgeObject{
			Name:       fmt.Sprintf("lookup_%d", size),
			Type:       TypeLookup,
			Definition: string(def),
			Owner:      "test",
			Shared:     true,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}
		store.Create(obj)

		registry := NewRegistry(store)
		expander := NewExpander(registry)

		// Test rows that will find matches
		rows := make([]Row, 50)
		for i := 0; i < 50; i++ {
			rows[i] = Row{
				"lookup_key": fmt.Sprintf("key_%d", i*size/50),
			}
		}

		b.Run(fmt.Sprintf("LookupSize%d", size), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = expander.ApplyLookup(fmt.Sprintf("lookup_%d", size), rows)
			}
		})
	}
}

func BenchmarkRegistryLookup(b *testing.B) {
	registry, cleanup := createTestRegistry(b)
	defer cleanup()

	b.Run("GetMacro", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = registry.GetMacro("error_filter")
		}
	})

	b.Run("GetFieldExtraction", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = registry.GetFieldExtraction("apache_log")
		}
	})

	b.Run("GetLookup", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = registry.GetLookup("error_codes")
		}
	})

	b.Run("GetMacroNotFound", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = registry.GetMacro("nonexistent")
		}
	})
}

func BenchmarkRegistryList(b *testing.B) {
	registry, cleanup := createTestRegistry(b)
	defer cleanup()

	b.Run("ListMacros", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = registry.ListMacros()
		}
	})

	b.Run("ListFieldExtractions", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = registry.ListFieldExtractions()
		}
	})

	b.Run("ListLookups", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = registry.ListLookups()
		}
	})
}

func BenchmarkCacheReload(b *testing.B) {
	registry, cleanup := createTestRegistry(b)
	defer cleanup()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = registry.Reload()
	}
}

func BenchmarkConcurrentRegistryAccess(b *testing.B) {
	registry, cleanup := createTestRegistry(b)
	defer cleanup()

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			switch i % 4 {
			case 0:
				_, _ = registry.GetMacro("error_filter")
			case 1:
				_, _ = registry.GetFieldExtraction("apache_log")
			case 2:
				_, _ = registry.GetLookup("error_codes")
			case 3:
				_ = registry.ListMacros()
			}
			i++
		}
	})
}

func BenchmarkApplyAllFieldExtractions(b *testing.B) {
	registry, cleanup := createTestRegistry(b)
	defer cleanup()

	expander := NewExpander(registry)

	rows := make([]Row, 50)
	for i := 0; i < 50; i++ {
		rows[i] = Row{
			"message": fmt.Sprintf(`192.168.1.%d - - [01/Jan/2024:10:00:%02d +0000] "GET /api/item/%d" user=test_%d status=ok`, i%256, i%60, i, i),
		}
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = expander.ApplyAllFieldExtractions(rows)
	}
}

func BenchmarkMacroValidation(b *testing.B) {
	registry, cleanup := createTestRegistry(b)
	defer cleanup()

	expander := NewExpander(registry)

	tests := []struct {
		name  string
		macro *Macro
	}{
		{"ValidSimple", &Macro{Expression: "level = 'error'", Args: nil}},
		{"ValidWithArgs", &Macro{Expression: "service = $1 AND status = $2", Args: []string{"service", "status"}}},
		{"UnusedArg", &Macro{Expression: "level = 'error'", Args: []string{"unused"}}},
		{"EmptyExpression", &Macro{Expression: "", Args: nil}},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = expander.ValidateMacro(tt.macro)
			}
		})
	}
}

func BenchmarkFieldExtractionValidation(b *testing.B) {
	registry, cleanup := createTestRegistry(b)
	defer cleanup()

	expander := NewExpander(registry)

	tests := []struct {
		name string
		fe   *FieldExtraction
	}{
		{"ValidWithGroups", &FieldExtraction{Pattern: `(?P<ip>\d+\.\d+\.\d+\.\d+)`, SourceField: "message"}},
		{"ValidNoGroups", &FieldExtraction{Pattern: `\d+\.\d+\.\d+\.\d+`, SourceField: "message"}},
		{"InvalidRegex", &FieldExtraction{Pattern: `[invalid`, SourceField: "message"}},
		{"EmptySourceField", &FieldExtraction{Pattern: `(?P<field>\w+)`, SourceField: ""}},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = expander.ValidateFieldExtraction(tt.fe)
			}
		})
	}
}

func BenchmarkLookupValidation(b *testing.B) {
	registry, cleanup := createTestRegistry(b)
	defer cleanup()

	expander := NewExpander(registry)

	lookup := &Lookup{
		KeyField:     "code",
		OutputFields: []string{"description", "severity"},
		Data: map[string]map[string]string{
			"E001": {"description": "Error 1", "severity": "high"},
			"E002": {"description": "Error 2", "severity": "low"},
		},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = expander.ValidateLookup(lookup)
	}
}

func BenchmarkKnowledgeObjectParsing(b *testing.B) {
	macroObj := &KnowledgeObject{
		Type:       TypeMacro,
		Definition: `{"expression":"level = 'error'","args":["field"]}`,
	}

	feObj := &KnowledgeObject{
		Type:       TypeFieldExtraction,
		Definition: `{"pattern":"(?P<ip>\\d+\\.\\d+\\.\\d+\\.\\d+)","source_field":"message"}`,
	}

	lookupObj := &KnowledgeObject{
		Type:       TypeLookup,
		Definition: `{"key_field":"code","output_fields":["desc"],"data":{"k1":{"desc":"v1"}}}`,
	}

	b.Run("ParseMacro", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = macroObj.ParseMacro()
		}
	})

	b.Run("ParseFieldExtraction", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = feObj.ParseFieldExtraction()
		}
	})

	b.Run("ParseLookup", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = lookupObj.ParseLookup()
		}
	})
}
