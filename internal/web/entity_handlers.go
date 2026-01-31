package web

import (
	"encoding/json"
	"html/template"
	"net/http"
	"sort"
	"strings"

	"dogwatch/internal/entity"
)

// EntityHandlers provides HTTP handlers for the Entity Synthesis feature
type EntityHandlers struct {
	synthesizer *entity.Synthesizer
}

// NewEntityHandlers creates new Entity handlers
func NewEntityHandlers(synthesizer *entity.Synthesizer) *EntityHandlers {
	return &EntityHandlers{synthesizer: synthesizer}
}

// RegisterRoutes registers Entity routes
func (h *EntityHandlers) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/entities", h.handleListEntities)
	mux.HandleFunc("/api/entities/", h.handleGetEntity)
	mux.HandleFunc("/api/entities/stats", h.handleEntityStats)
	mux.HandleFunc("/entities", h.handleEntityExplorer)
}

// handleListEntities returns all entities
func (h *EntityHandlers) handleListEntities(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse query params
	typeFilter := entity.Type(r.URL.Query().Get("type"))
	healthFilter := r.URL.Query().Get("health")
	search := strings.ToLower(r.URL.Query().Get("search"))

	entities := h.synthesizer.ListEntities(typeFilter)

	// Apply filters
	var filtered []*entity.Entity
	for _, e := range entities {
		if healthFilter != "" && string(e.Health) != healthFilter {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(e.Name), search) &&
			!strings.Contains(strings.ToLower(e.DisplayName), search) {
			continue
		}
		filtered = append(filtered, e)
	}

	// Sort by type, then name
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].Type != filtered[j].Type {
			return filtered[i].Type < filtered[j].Type
		}
		return filtered[i].Name < filtered[j].Name
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"entities": filtered,
		"total":    len(filtered),
	})
}

// handleGetEntity returns a single entity with related entities
func (h *EntityHandlers) handleGetEntity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract entity ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/entities/")
	if path == "" {
		http.Error(w, "Entity ID required", http.StatusBadRequest)
		return
	}

	// Handle related entities endpoint
	if strings.HasSuffix(path, "/related") {
		entityID := strings.TrimSuffix(path, "/related")
		related := h.synthesizer.GetRelatedEntities(entityID)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"entity_id": entityID,
			"related":   related,
		})
		return
	}

	e := h.synthesizer.GetEntity(path)
	if e == nil {
		http.Error(w, "Entity not found", http.StatusNotFound)
		return
	}

	// Get related entities
	related := h.synthesizer.GetRelatedEntities(e.ID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"entity":  e,
		"related": related,
	})
}

// handleEntityStats returns synthesizer statistics
func (h *EntityHandlers) handleEntityStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stats := h.synthesizer.GetStats()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// handleEntityExplorer serves the entity explorer HTML page
func (h *EntityHandlers) handleEntityExplorer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	entities := h.synthesizer.ListEntities("")
	stats := h.synthesizer.GetStats()

	// Group by type
	grouped := make(map[entity.Type][]*entity.Entity)
	for _, e := range entities {
		grouped[e.Type] = append(grouped[e.Type], e)
	}

	// Sort within groups
	for _, list := range grouped {
		sort.Slice(list, func(i, j int) bool {
			return list[i].Name < list[j].Name
		})
	}

	data := struct {
		Entities map[entity.Type][]*entity.Entity
		Stats    map[string]interface{}
		Total    int
	}{
		Entities: grouped,
		Stats:    stats,
		Total:    len(entities),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl := template.Must(template.New("entities").Funcs(template.FuncMap{
		"typeIcon": typeIcon,
		"healthColor": healthColor,
	}).Parse(entityExplorerTemplate))
	tmpl.Execute(w, data)
}

func typeIcon(t entity.Type) string {
	switch t {
	case entity.TypeService:
		return "🔧"
	case entity.TypeHost:
		return "🖥️"
	case entity.TypeContainer:
		return "📦"
	case entity.TypeDatabase:
		return "🗄️"
	case entity.TypeQueue:
		return "📬"
	case entity.TypeExternalAPI:
		return "🌐"
	case entity.TypeLoadBalancer:
		return "⚖️"
	default:
		return "📌"
	}
}

func healthColor(h entity.HealthStatus) string {
	switch h {
	case entity.HealthHealthy:
		return "#3fb950"
	case entity.HealthDegraded:
		return "#d29922"
	case entity.HealthUnhealthy:
		return "#f85149"
	default:
		return "#8b949e"
	}
}

const entityExplorerTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Entity Explorer | Dogwatch</title>
    <style>
        :root {
            --bg-primary: #0d1117;
            --bg-secondary: #161b22;
            --bg-tertiary: #21262d;
            --text-primary: #c9d1d9;
            --text-secondary: #8b949e;
            --text-muted: #6e7681;
            --border-color: #30363d;
            --accent-blue: #58a6ff;
            --accent-green: #3fb950;
            --accent-yellow: #d29922;
            --accent-red: #f85149;
        }

        * { box-sizing: border-box; margin: 0; padding: 0; }

        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: var(--bg-primary);
            color: var(--text-primary);
            min-height: 100vh;
            line-height: 1.5;
        }

        .container {
            max-width: 1400px;
            margin: 0 auto;
            padding: 24px;
        }

        header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 24px;
            padding-bottom: 16px;
            border-bottom: 1px solid var(--border-color);
        }

        h1 { font-size: 24px; font-weight: 600; }
        h2 { font-size: 18px; font-weight: 600; margin-bottom: 16px; }

        .stats-row {
            display: flex;
            gap: 16px;
            margin-bottom: 24px;
        }

        .stat-card {
            background: var(--bg-secondary);
            border: 1px solid var(--border-color);
            border-radius: 8px;
            padding: 16px;
            text-align: center;
            min-width: 100px;
        }

        .stat-value {
            font-size: 28px;
            font-weight: 600;
            color: var(--accent-blue);
        }

        .stat-label {
            font-size: 12px;
            color: var(--text-secondary);
            text-transform: uppercase;
        }

        .type-section {
            margin-bottom: 24px;
        }

        .type-header {
            display: flex;
            align-items: center;
            gap: 8px;
            margin-bottom: 12px;
        }

        .type-icon {
            font-size: 20px;
        }

        .type-count {
            color: var(--text-muted);
            font-weight: normal;
            font-size: 14px;
        }

        .entity-grid {
            display: grid;
            grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
            gap: 12px;
        }

        .entity-card {
            background: var(--bg-secondary);
            border: 1px solid var(--border-color);
            border-radius: 8px;
            padding: 16px;
            transition: border-color 0.2s;
        }

        .entity-card:hover {
            border-color: var(--accent-blue);
        }

        .entity-header {
            display: flex;
            justify-content: space-between;
            align-items: flex-start;
            margin-bottom: 12px;
        }

        .entity-name {
            font-weight: 600;
            font-size: 15px;
        }

        .entity-display-name {
            font-size: 12px;
            color: var(--text-secondary);
        }

        .health-indicator {
            width: 10px;
            height: 10px;
            border-radius: 50%;
            flex-shrink: 0;
        }

        .signals {
            display: grid;
            grid-template-columns: repeat(2, 1fr);
            gap: 8px;
            margin-bottom: 12px;
        }

        .signal {
            background: var(--bg-tertiary);
            padding: 8px;
            border-radius: 4px;
        }

        .signal-label {
            font-size: 10px;
            color: var(--text-muted);
            text-transform: uppercase;
        }

        .signal-value {
            font-size: 14px;
            font-weight: 500;
        }

        .relationships {
            font-size: 12px;
            color: var(--text-secondary);
        }

        .rel-count {
            color: var(--accent-blue);
        }

        .empty-state {
            text-align: center;
            padding: 48px;
            color: var(--text-secondary);
        }

        .search-box {
            padding: 8px 16px;
            background: var(--bg-tertiary);
            border: 1px solid var(--border-color);
            color: var(--text-primary);
            border-radius: 6px;
            font-size: 14px;
            width: 250px;
        }

        .search-box:focus {
            outline: none;
            border-color: var(--accent-blue);
        }
    </style>
</head>
<body>
    <div class="container">
        <header>
            <div>
                <h1>🔍 Entity Explorer</h1>
                <div style="color: var(--text-secondary); font-size: 14px;">
                    Auto-discovered infrastructure entities
                </div>
            </div>
            <input type="text" class="search-box" placeholder="Search entities..." id="search">
        </header>

        <div class="stats-row">
            <div class="stat-card">
                <div class="stat-value">{{.Total}}</div>
                <div class="stat-label">Total Entities</div>
            </div>
            {{range $type, $count := .Stats.by_type}}
            <div class="stat-card">
                <div class="stat-value">{{$count}}</div>
                <div class="stat-label">{{$type}}</div>
            </div>
            {{end}}
        </div>

        {{if eq .Total 0}}
        <div class="empty-state">
            <h2>No entities discovered yet</h2>
            <p>Entities are automatically discovered from traces, metrics, and Kubernetes.</p>
            <p>Start sending telemetry data to see entities appear here.</p>
        </div>
        {{else}}

        {{range $type, $entities := .Entities}}
        <div class="type-section">
            <h2 class="type-header">
                <span class="type-icon">{{typeIcon $type}}</span>
                {{$type}}
                <span class="type-count">({{len $entities}})</span>
            </h2>
            <div class="entity-grid">
                {{range $entities}}
                <div class="entity-card" data-name="{{.Name}} {{.DisplayName}}">
                    <div class="entity-header">
                        <div>
                            <div class="entity-name">{{.Name}}</div>
                            {{if ne .Name .DisplayName}}
                            <div class="entity-display-name">{{.DisplayName}}</div>
                            {{end}}
                        </div>
                        <div class="health-indicator" style="background: {{healthColor .Health}}" title="{{.Health}}"></div>
                    </div>
                    <div class="signals">
                        <div class="signal">
                            <div class="signal-label">Throughput</div>
                            <div class="signal-value">{{printf "%.1f" .Signals.Throughput}} {{.Signals.ThroughputUnit}}</div>
                        </div>
                        <div class="signal">
                            <div class="signal-label">Error Rate</div>
                            <div class="signal-value">{{printf "%.2f" .Signals.ErrorRate}}%</div>
                        </div>
                        <div class="signal">
                            <div class="signal-label">P99 Latency</div>
                            <div class="signal-value">{{printf "%.0f" .Signals.LatencyP99}}ms</div>
                        </div>
                        <div class="signal">
                            <div class="signal-label">Saturation</div>
                            <div class="signal-value">{{printf "%.0f" .Signals.Saturation}}% {{.Signals.SaturationUnit}}</div>
                        </div>
                    </div>
                    {{if .Relationships}}
                    <div class="relationships">
                        <span class="rel-count">{{len .Relationships}}</span> relationships
                    </div>
                    {{end}}
                </div>
                {{end}}
            </div>
        </div>
        {{end}}

        {{end}}
    </div>

    <script>
        // Simple search filter
        document.getElementById('search').addEventListener('input', function(e) {
            const query = e.target.value.toLowerCase();
            document.querySelectorAll('.entity-card').forEach(card => {
                const name = card.dataset.name.toLowerCase();
                card.style.display = name.includes(query) ? '' : 'none';
            });
        });
    </script>
</body>
</html>`
