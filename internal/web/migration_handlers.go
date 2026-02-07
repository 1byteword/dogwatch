package web

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"dogwatch/internal/alerting"
	"dogwatch/internal/dashboard"
	"dogwatch/internal/migration"

	"github.com/google/uuid"
)

var (
	migrationStore        *migration.Store
	migrationDashStore    *dashboard.Store
	migrationAlertManager *alerting.AlertManager
)

// SetMigrationStore sets the migration store for handlers
func SetMigrationStore(store *migration.Store) {
	migrationStore = store
}

// SetMigrationDashboardStore sets the dashboard store for migration
func SetMigrationDashboardStore(store *dashboard.Store) {
	migrationDashStore = store
}

// SetMigrationAlertManager sets the alert manager for migration
func SetMigrationAlertManager(manager *alerting.AlertManager) {
	migrationAlertManager = manager
}

// RegisterMigrationRoutes registers migration API routes
func RegisterMigrationRoutes(mux *http.ServeMux) {
	// Dashboard import
	mux.HandleFunc("/api/migration/datadog/dashboard", handleDatadogDashboardImport)
	mux.HandleFunc("/api/migration/grafana/dashboard", handleGrafanaDashboardImport)

	// Alert import
	mux.HandleFunc("/api/migration/alerts", handleAlertsImport)
	mux.HandleFunc("/api/migration/datadog/alerts", handleDatadogAlertsImport)
	mux.HandleFunc("/api/migration/grafana/alerts", handleGrafanaAlertsImport)
	mux.HandleFunc("/api/migration/prometheus/alerts", handlePrometheusAlertsImport)

	// Reports
	mux.HandleFunc("/api/migration/reports", handleMigrationReports)
	mux.HandleFunc("/api/migration/report/", handleMigrationReport)

	// Stats and info
	mux.HandleFunc("/api/migration/stats", handleMigrationStats)
	mux.HandleFunc("/api/migration/formats", handleMigrationFormats)

	// Preview/dry-run
	mux.HandleFunc("/api/migration/preview", handleMigrationPreview)
}

// handleDatadogDashboardImport handles POST /api/migration/datadog/dashboard
func handleDatadogDashboardImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read request body
	data, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to read request: %v", err), http.StatusBadRequest)
		return
	}

	// Parse options from query params
	opts := migration.DashboardImportOptions{
		SkipUnsupportedWidgets: r.URL.Query().Get("skip_unsupported") == "true",
		DashboardNamePrefix:    r.URL.Query().Get("prefix"),
		SetAsDefault:           r.URL.Query().Get("default") == "true",
	}

	startTime := time.Now()

	// Import dashboard
	importer := migration.NewDatadogImporter(opts)
	converted, result, err := importer.ImportDashboard(data)
	if err != nil {
		http.Error(w, fmt.Sprintf("Import failed: %v", err), http.StatusBadRequest)
		return
	}

	// Save dashboard if store is available
	if migrationDashStore != nil && converted != nil {
		_, err = migrationDashStore.Create(
			converted.Dashboard.Name,
			converted.Dashboard.Layout,
			map[string]dashboard.WidgetConfig{},
			converted.Dashboard.IsDefault,
		)
		if err != nil {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Dashboard import succeeded but save failed: %v", err))
		}
	}

	// Generate and save report
	report := migration.GenerateMigrationReport(
		[]migration.DashboardResult{*result},
		nil,
		migration.PlatformDatadog,
		startTime,
	)

	if migrationStore != nil {
		migrationStore.SaveReport(report)
	}

	respondMigrationJSON(w, map[string]any{
		"success":   result.Success,
		"report_id": report.ID,
		"dashboard": map[string]any{
			"id":                result.TargetID,
			"name":              converted.Dashboard.Name,
			"widgets_total":     result.WidgetsTotal,
			"widgets_converted": result.WidgetsConverted,
			"widgets_skipped":   result.WidgetsSkipped,
		},
		"variables": converted.Variables,
		"warnings":  result.Warnings,
		"duration":  report.Duration.String(),
	})
}

// handleGrafanaDashboardImport handles POST /api/migration/grafana/dashboard
func handleGrafanaDashboardImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	data, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to read request: %v", err), http.StatusBadRequest)
		return
	}

	opts := migration.DashboardImportOptions{
		SkipUnsupportedWidgets: r.URL.Query().Get("skip_unsupported") == "true",
		DashboardNamePrefix:    r.URL.Query().Get("prefix"),
		SetAsDefault:           r.URL.Query().Get("default") == "true",
	}

	startTime := time.Now()

	importer := migration.NewGrafanaImporter(opts)
	converted, result, err := importer.ImportDashboard(data)
	if err != nil {
		http.Error(w, fmt.Sprintf("Import failed: %v", err), http.StatusBadRequest)
		return
	}

	// Save dashboard
	if migrationDashStore != nil && converted != nil {
		_, err = migrationDashStore.Create(
			converted.Dashboard.Name,
			converted.Dashboard.Layout,
			map[string]dashboard.WidgetConfig{},
			converted.Dashboard.IsDefault,
		)
		if err != nil {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Dashboard import succeeded but save failed: %v", err))
		}
	}

	report := migration.GenerateMigrationReport(
		[]migration.DashboardResult{*result},
		nil,
		migration.PlatformGrafana,
		startTime,
	)

	if migrationStore != nil {
		migrationStore.SaveReport(report)
	}

	respondMigrationJSON(w, map[string]any{
		"success":   result.Success,
		"report_id": report.ID,
		"dashboard": map[string]any{
			"id":                result.TargetID,
			"name":              converted.Dashboard.Name,
			"widgets_total":     result.WidgetsTotal,
			"widgets_converted": result.WidgetsConverted,
			"widgets_skipped":   result.WidgetsSkipped,
		},
		"variables":   converted.Variables,
		"annotations": converted.Annotations,
		"warnings":    result.Warnings,
		"duration":    report.Duration.String(),
	})
}

// handleAlertsImport handles POST /api/migration/alerts (auto-detect format)
func handleAlertsImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	data, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to read request: %v", err), http.StatusBadRequest)
		return
	}

	opts := parseAlertImportOptions(r)

	importer := migration.NewAlertImporter()
	alerts, result, err := importer.ImportAlerts(data, opts)
	if err != nil {
		http.Error(w, fmt.Sprintf("Import failed: %v", err), http.StatusBadRequest)
		return
	}

	// Save alerts to alert manager
	savedCount := 0
	if migrationAlertManager != nil {
		for _, alert := range alerts {
			if err := migrationAlertManager.Store.CreateRule(alert.Rule); err == nil {
				savedCount++
			} else {
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("Failed to save alert '%s': %v", alert.Rule.Name, err))
			}
		}
	}

	// Save report
	if migrationStore != nil {
		report := &migration.MigrationReport{
			ID:             result.ID,
			Source:         result.Source,
			StartedAt:      result.CreatedAt,
			CompletedAt:    time.Now(),
			Duration:       result.Duration,
			AlertsTotal:    result.ItemsImported + result.ItemsFailed,
			AlertsImported: result.ItemsImported,
			AlertsFailed:   result.ItemsFailed,
			Warnings:       result.Warnings,
			Errors:         result.Errors,
		}
		migrationStore.SaveReport(report)
	}

	respondMigrationJSON(w, map[string]any{
		"success":   result.Success,
		"report_id": result.ID,
		"source":    result.Source,
		"alerts": map[string]int{
			"total":    result.ItemsImported + result.ItemsFailed,
			"imported": result.ItemsImported,
			"failed":   result.ItemsFailed,
			"saved":    savedCount,
		},
		"warnings": result.Warnings,
		"errors":   result.Errors,
		"duration": result.Duration.String(),
	})
}

// handleDatadogAlertsImport handles POST /api/migration/datadog/alerts
func handleDatadogAlertsImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	data, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to read request: %v", err), http.StatusBadRequest)
		return
	}

	opts := parseAlertImportOptions(r)
	startTime := time.Now()

	importer := migration.NewDatadogImporter(migration.DashboardImportOptions{})
	alerts, results, err := importer.ImportMonitors(data, opts)
	if err != nil {
		http.Error(w, fmt.Sprintf("Import failed: %v", err), http.StatusBadRequest)
		return
	}

	// Save alerts
	savedCount := saveAlerts(alerts, results)

	report := migration.GenerateMigrationReport(nil, results, migration.PlatformDatadog, startTime)
	if migrationStore != nil {
		migrationStore.SaveReport(report)
	}

	var warnings []string
	for _, r := range results {
		warnings = append(warnings, r.Warnings...)
	}

	respondMigrationJSON(w, map[string]any{
		"success":   report.AlertsFailed == 0,
		"report_id": report.ID,
		"alerts": map[string]int{
			"total":    len(alerts),
			"imported": report.AlertsImported,
			"failed":   report.AlertsFailed,
			"saved":    savedCount,
		},
		"warnings": warnings,
		"duration": report.Duration.String(),
	})
}

// handleGrafanaAlertsImport handles POST /api/migration/grafana/alerts
func handleGrafanaAlertsImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	data, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to read request: %v", err), http.StatusBadRequest)
		return
	}

	opts := parseAlertImportOptions(r)
	startTime := time.Now()

	importer := migration.NewGrafanaImporter(migration.DashboardImportOptions{})
	alerts, results, err := importer.ImportAlertRules(data, opts)
	if err != nil {
		http.Error(w, fmt.Sprintf("Import failed: %v", err), http.StatusBadRequest)
		return
	}

	savedCount := saveAlerts(alerts, results)

	report := migration.GenerateMigrationReport(nil, results, migration.PlatformGrafana, startTime)
	if migrationStore != nil {
		migrationStore.SaveReport(report)
	}

	var warnings []string
	for _, r := range results {
		warnings = append(warnings, r.Warnings...)
	}

	respondMigrationJSON(w, map[string]any{
		"success":   report.AlertsFailed == 0,
		"report_id": report.ID,
		"alerts": map[string]int{
			"total":    len(alerts),
			"imported": report.AlertsImported,
			"failed":   report.AlertsFailed,
			"saved":    savedCount,
		},
		"warnings": warnings,
		"duration": report.Duration.String(),
	})
}

// handlePrometheusAlertsImport handles POST /api/migration/prometheus/alerts
func handlePrometheusAlertsImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	data, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to read request: %v", err), http.StatusBadRequest)
		return
	}

	opts := parseAlertImportOptions(r)
	startTime := time.Now()

	importer := migration.NewAlertImporter()
	alerts, results, err := importer.ImportPrometheusRules(data, opts)
	if err != nil {
		http.Error(w, fmt.Sprintf("Import failed: %v", err), http.StatusBadRequest)
		return
	}

	savedCount := saveAlerts(alerts, results)

	report := migration.GenerateMigrationReport(nil, results, migration.PlatformPrometheus, startTime)
	if migrationStore != nil {
		migrationStore.SaveReport(report)
	}

	var warnings []string
	for _, r := range results {
		warnings = append(warnings, r.Warnings...)
	}

	respondMigrationJSON(w, map[string]any{
		"success":   report.AlertsFailed == 0,
		"report_id": report.ID,
		"alerts": map[string]int{
			"total":    len(alerts),
			"imported": report.AlertsImported,
			"failed":   report.AlertsFailed,
			"saved":    savedCount,
		},
		"warnings": warnings,
		"duration": report.Duration.String(),
	})
}

// handleMigrationReports handles GET /api/migration/reports
func handleMigrationReports(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if migrationStore == nil {
		http.Error(w, "Migration store not configured", http.StatusServiceUnavailable)
		return
	}

	// Parse filters
	source := r.URL.Query().Get("source")
	limit := 100

	var reports []*migration.MigrationReport
	var err error

	if source != "" {
		reports, err = migrationStore.GetReportsBySource(migration.SourcePlatform(source), limit)
	} else {
		reports, err = migrationStore.ListReports(limit)
	}

	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to list reports: %v", err), http.StatusInternalServerError)
		return
	}

	respondMigrationJSON(w, reports)
}

// handleMigrationReport handles GET/DELETE /api/migration/report/:id
func handleMigrationReport(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/migration/report/")
	if id == "" {
		http.Error(w, "Report ID required", http.StatusBadRequest)
		return
	}

	if migrationStore == nil {
		http.Error(w, "Migration store not configured", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		report, err := migrationStore.GetReport(id)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to get report: %v", err), http.StatusInternalServerError)
			return
		}
		if report == nil {
			http.Error(w, "Report not found", http.StatusNotFound)
			return
		}
		respondMigrationJSON(w, report)

	case http.MethodDelete:
		if err := migrationStore.DeleteReport(id); err != nil {
			http.Error(w, fmt.Sprintf("Failed to delete report: %v", err), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleMigrationStats handles GET /api/migration/stats
func handleMigrationStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if migrationStore == nil {
		http.Error(w, "Migration store not configured", http.StatusServiceUnavailable)
		return
	}

	stats, err := migrationStore.GetStats()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get stats: %v", err), http.StatusInternalServerError)
		return
	}

	respondMigrationJSON(w, stats)
}

// handleMigrationFormats handles GET /api/migration/formats
func handleMigrationFormats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	respondMigrationJSON(w, migration.GetSupportedFormats())
}

// handleMigrationPreview handles POST /api/migration/preview (dry-run)
func handleMigrationPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	data, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to read request: %v", err), http.StatusBadRequest)
		return
	}

	// Detect format
	format := migration.DetectFormat(data)

	// Get import type from query param
	importType := r.URL.Query().Get("type") // dashboard, alert
	if importType == "" {
		importType = "auto"
	}

	preview := map[string]any{
		"format":      format,
		"import_type": importType,
		"items":       []any{},
		"warnings":    []string{},
	}

	switch format {
	case migration.PlatformDatadog:
		if importType == "dashboard" || importType == "auto" {
			importer := migration.NewDatadogImporter(migration.DashboardImportOptions{})
			if dash, err := importer.ParseDashboard(data); err == nil {
				preview["items"] = append(preview["items"].([]any), map[string]any{
					"type":           "dashboard",
					"name":           dash.Title,
					"widget_count":   len(dash.Widgets),
					"variable_count": len(dash.TemplateVariables),
				})
			}
		}
		if importType == "alert" || importType == "auto" {
			importer := migration.NewDatadogImporter(migration.DashboardImportOptions{})
			if monitors, err := importer.ParseMonitors(data); err == nil {
				for _, m := range monitors {
					preview["items"] = append(preview["items"].([]any), map[string]any{
						"type":       "alert",
						"name":       m.Name,
						"alert_type": m.Type,
					})
				}
			}
		}

	case migration.PlatformGrafana:
		if importType == "dashboard" || importType == "auto" {
			importer := migration.NewGrafanaImporter(migration.DashboardImportOptions{})
			if dash, err := importer.ParseDashboard(data); err == nil {
				preview["items"] = append(preview["items"].([]any), map[string]any{
					"type":           "dashboard",
					"name":           dash.Title,
					"panel_count":    len(dash.Panels),
					"variable_count": len(dash.Templating.List),
				})
			}
		}
		if importType == "alert" || importType == "auto" {
			importer := migration.NewGrafanaImporter(migration.DashboardImportOptions{})
			if groups, err := importer.ParseAlertRules(data); err == nil {
				for _, g := range groups {
					for _, rule := range g.Rules {
						preview["items"] = append(preview["items"].([]any), map[string]any{
							"type":  "alert",
							"name":  rule.Title,
							"group": g.Name,
						})
					}
				}
			}
		}

	case migration.PlatformPrometheus:
		if rules, err := migration.ParsePrometheusRules(data); err == nil {
			for _, g := range rules.Groups {
				for _, rule := range g.Rules {
					if rule.Alert != "" {
						preview["items"] = append(preview["items"].([]any), map[string]any{
							"type":  "alert",
							"name":  rule.Alert,
							"group": g.Name,
						})
					}
				}
			}
		}

	default:
		preview["warnings"] = []string{"Unknown format. Supported: Datadog, Grafana, Prometheus"}
	}

	respondMigrationJSON(w, preview)
}

// Helper functions

func parseAlertImportOptions(r *http.Request) migration.AlertImportOptions {
	return migration.AlertImportOptions{
		EnableImportedAlerts:  r.URL.Query().Get("enable") == "true",
		AlertNamePrefix:       r.URL.Query().Get("prefix"),
		OverwriteExisting:     r.URL.Query().Get("overwrite") == "true",
		DefaultNotifyChannels: strings.Split(r.URL.Query().Get("channels"), ","),
	}
}

func saveAlerts(alerts []*migration.ConvertedAlert, results []migration.AlertResult) int {
	if migrationAlertManager == nil {
		return 0
	}

	savedCount := 0
	for i, alert := range alerts {
		if !results[i].Success {
			continue
		}
		// Ensure unique ID
		alert.Rule.ID = uuid.New().String()
		if err := migrationAlertManager.Store.CreateRule(alert.Rule); err == nil {
			savedCount++
		}
	}
	return savedCount
}

func respondMigrationJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}
