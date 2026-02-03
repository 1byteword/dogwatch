package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"dogwatch/internal/query"
	"dogwatch/internal/scripts"
)

var scriptsRunner *scripts.Runner

// SetScriptsRunner configures the scripts runner for API handlers
func SetScriptsRunner(runner *scripts.Runner) {
	scriptsRunner = runner
}

// InitScriptsRunner creates a scripts runner from the query executor
func InitScriptsRunner(executor *query.Executor) *scripts.Runner {
	runner := scripts.NewRunner(executor, scripts.DefaultRegistry)
	SetScriptsRunner(runner)
	return runner
}

// RegisterScriptsRoutes registers script-related API endpoints
func RegisterScriptsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/scripts", handleScriptsList)
	mux.HandleFunc("/api/scripts/categories", handleScriptsCategories)
	mux.HandleFunc("/api/scripts/", handleScriptsRoute)
}

// handleScriptsList lists all scripts or scripts in a category
func handleScriptsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	category := r.URL.Query().Get("category")

	var scriptsList []*scripts.Script
	if scriptsRunner != nil {
		scriptsList = scriptsRunner.ListScripts(category)
	} else {
		scriptsList = scripts.DefaultRegistry.List(category)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"scripts": scriptsList,
		"count":   len(scriptsList),
	})
}

// handleScriptsCategories lists all script categories
func handleScriptsCategories(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var categories []scripts.CategoryInfo
	if scriptsRunner != nil {
		categories = scriptsRunner.ListCategories()
	} else {
		categories = scripts.DefaultRegistry.Categories()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"categories": categories,
	})
}

// handleScriptsRoute handles /api/scripts/{category}/{name} or /api/scripts/{category}/{name}/run
func handleScriptsRoute(w http.ResponseWriter, r *http.Request) {
	// Parse path: /api/scripts/{category}/{name}[/run]
	path := strings.TrimPrefix(r.URL.Path, "/api/scripts/")
	parts := strings.Split(path, "/")

	if len(parts) < 2 {
		// List scripts in category
		category := parts[0]
		if category == "" {
			http.Error(w, "Category required", http.StatusBadRequest)
			return
		}

		var scriptsList []*scripts.Script
		if scriptsRunner != nil {
			scriptsList = scriptsRunner.ListScripts(category)
		} else {
			scriptsList = scripts.DefaultRegistry.List(category)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"category": category,
			"scripts":  scriptsList,
			"count":    len(scriptsList),
		})
		return
	}

	scriptName := parts[0] + "/" + parts[1]
	isRun := len(parts) >= 3 && parts[2] == "run"

	if isRun {
		handleScriptRun(w, r, scriptName)
		return
	}

	// Get script details
	handleScriptGet(w, r, scriptName)
}

// handleScriptGet returns details about a specific script
func handleScriptGet(w http.ResponseWriter, r *http.Request, name string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var script *scripts.Script
	if scriptsRunner != nil {
		script = scriptsRunner.GetScript(name)
	} else {
		script = scripts.DefaultRegistry.Get(name)
	}

	if script == nil {
		http.Error(w, "Script not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(script)
}

// handleScriptRun executes a script
func handleScriptRun(w http.ResponseWriter, r *http.Request, name string) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if scriptsRunner == nil {
		http.Error(w, "Scripts runner not configured", http.StatusServiceUnavailable)
		return
	}

	// Parse parameters from query string or JSON body
	params := make(map[string]string)

	if r.Method == http.MethodPost {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			for k, v := range body {
				switch val := v.(type) {
				case string:
					params[k] = val
				case float64:
					params[k] = formatParamValue(val)
				case int:
					params[k] = formatParamValue(float64(val))
				case bool:
					if val {
						params[k] = "true"
					} else {
						params[k] = "false"
					}
				}
			}
		}
	}

	// Also accept query parameters (overrides body)
	for k, v := range r.URL.Query() {
		if len(v) > 0 {
			params[k] = v[0]
		}
	}

	// Run script
	result, err := scriptsRunner.RunScript(r.Context(), name, params)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func formatParamValue(f float64) string {
	if f == float64(int64(f)) {
		return fmt.Sprintf("%.0f", f)
	}
	return fmt.Sprintf("%g", f)
}
