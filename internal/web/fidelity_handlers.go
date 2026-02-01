package web

import (
	"encoding/json"
	"io"
	"net/http"

	"dogwatch/internal/migration"
)

// RegisterFidelityRoutes registers migration fidelity API routes
func RegisterFidelityRoutes(mux *http.ServeMux) {
	// Preview with fidelity analysis
	mux.HandleFunc("/api/migration/fidelity/preview", handleFidelityPreview)

	// Analyze existing migration
	mux.HandleFunc("/api/migration/fidelity/analyze", handleFidelityAnalyze)

	// Template variable conversion
	mux.HandleFunc("/api/migration/fidelity/variables", handleVariableConversion)

	// Composite monitor analysis
	mux.HandleFunc("/api/migration/fidelity/composite", handleCompositeAnalysis)
}

// handleFidelityPreview provides detailed migration preview with fidelity scoring
// POST /api/migration/fidelity/preview
func handleFidelityPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	data, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request: "+err.Error(), http.StatusBadRequest)
		return
	}

	preview, err := migration.GenerateMigrationPreview(data)
	if err != nil {
		http.Error(w, "Preview generation failed: "+err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(preview)
}

// handleFidelityAnalyze analyzes fidelity of a migration
// POST /api/migration/fidelity/analyze
func handleFidelityAnalyze(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	data, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request: "+err.Error(), http.StatusBadRequest)
		return
	}

	format := migration.DetectFormat(data)

	analyzer := migration.NewFidelityAnalyzer()

	// Process based on format
	var converted *migration.ConvertedDashboard
	var result *migration.DashboardResult

	switch format {
	case migration.PlatformDatadog:
		importer := migration.NewDatadogImporter(migration.DashboardImportOptions{
			SkipUnsupportedWidgets: true,
		})
		converted, result, err = importer.ImportDashboard(data)
		if err != nil {
			// Try as monitors
			monitors, results, err := importer.ImportMonitors(data, migration.AlertImportOptions{})
			if err != nil {
				http.Error(w, "Failed to parse input: "+err.Error(), http.StatusBadRequest)
				return
			}

			// Return alert analysis
			response := map[string]interface{}{
				"format":    "datadog",
				"type":      "alerts",
				"count":     len(monitors),
				"results":   results,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
			return
		}

	case migration.PlatformGrafana:
		importer := migration.NewGrafanaImporter(migration.DashboardImportOptions{
			SkipUnsupportedWidgets: true,
		})
		converted, result, err = importer.ImportDashboard(data)
		if err != nil {
			http.Error(w, "Failed to parse Grafana dashboard: "+err.Error(), http.StatusBadRequest)
			return
		}

	case migration.PlatformPrometheus:
		// Prometheus is alerts only
		importer := migration.NewAlertImporter()
		alerts, alertResults, err := importer.ImportPrometheusRules(data, migration.AlertImportOptions{})
		if err != nil {
			http.Error(w, "Failed to parse Prometheus rules: "+err.Error(), http.StatusBadRequest)
			return
		}

		response := map[string]interface{}{
			"format":  "prometheus",
			"type":    "alerts",
			"count":   len(alerts),
			"results": alertResults,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
		return

	default:
		http.Error(w, "Unknown format", http.StatusBadRequest)
		return
	}

	// Calculate fidelity score
	score := analyzer.CalculateFidelityScore(nil, converted, result)

	response := map[string]interface{}{
		"format":         format,
		"type":           "dashboard",
		"fidelity_score": score,
		"dashboard": map[string]interface{}{
			"name":              converted.Dashboard.Name,
			"widgets_total":     result.WidgetsTotal,
			"widgets_converted": result.WidgetsConverted,
			"widgets_skipped":   result.WidgetsSkipped,
		},
		"variables": converted.Variables,
		"warnings":  result.Warnings,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleVariableConversion handles template variable conversion analysis
// POST /api/migration/fidelity/variables
func handleVariableConversion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	data, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request: "+err.Error(), http.StatusBadRequest)
		return
	}

	format := migration.DetectFormat(data)

	var enhancedVars []migration.EnhancedTemplateVariable

	switch format {
	case migration.PlatformDatadog:
		importer := migration.NewDatadogImporter(migration.DashboardImportOptions{})
		dash, err := importer.ParseDashboard(data)
		if err != nil {
			http.Error(w, "Failed to parse Datadog dashboard: "+err.Error(), http.StatusBadRequest)
			return
		}
		enhancedVars = migration.ParseDatadogTemplateVariables(dash.TemplateVariables)

	case migration.PlatformGrafana:
		importer := migration.NewGrafanaImporter(migration.DashboardImportOptions{})
		dash, err := importer.ParseDashboard(data)
		if err != nil {
			http.Error(w, "Failed to parse Grafana dashboard: "+err.Error(), http.StatusBadRequest)
			return
		}
		enhancedVars = migration.ParseGrafanaTemplateVariables(dash.Templating.List)

	default:
		http.Error(w, "Unsupported format for variable analysis", http.StatusBadRequest)
		return
	}

	// Analyze chaining and dependencies
	varDeps := make(map[string][]string)
	for _, v := range enhancedVars {
		if len(v.ChainedVars) > 0 {
			varDeps[v.Name] = v.ChainedVars
		}
	}

	response := map[string]interface{}{
		"format":       format,
		"variables":    enhancedVars,
		"count":        len(enhancedVars),
		"dependencies": varDeps,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleCompositeAnalysis analyzes Datadog composite monitors
// POST /api/migration/fidelity/composite
func handleCompositeAnalysis(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	data, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request: "+err.Error(), http.StatusBadRequest)
		return
	}

	importer := migration.NewDatadogImporter(migration.DashboardImportOptions{})
	monitors, err := importer.ParseMonitors(data)
	if err != nil {
		http.Error(w, "Failed to parse monitors: "+err.Error(), http.StatusBadRequest)
		return
	}

	var composites []map[string]interface{}

	for _, m := range monitors {
		if m.Type == "composite" {
			composite, err := migration.ParseDatadogCompositeMonitor(&m)
			if err != nil {
				continue
			}

			composites = append(composites, map[string]interface{}{
				"id":           composite.ID,
				"name":         composite.Name,
				"expression":   composite.Expression,
				"sub_monitors": composite.SubMonitors,
				"logic_tree":   composite.LogicTree,
			})
		}
	}

	response := map[string]interface{}{
		"total_monitors":      len(monitors),
		"composite_monitors":  len(composites),
		"composites":          composites,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
