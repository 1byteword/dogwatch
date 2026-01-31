package web

import (
	"encoding/json"
	"html/template"
	"net/http"
	"sort"
	"strconv"
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
	mux.HandleFunc("/api/entities/graph", h.handleEntityGraph)
	mux.HandleFunc("/entities", h.handleEntityExplorer)
	mux.HandleFunc("/entities/map", h.handleEntityMap)
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

// handleEntityGraph returns the entity relationship graph as JSON
func (h *EntityHandlers) handleEntityGraph(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check if requesting a subgraph for a specific entity
	entityID := r.URL.Query().Get("entity")
	depthStr := r.URL.Query().Get("depth")
	depth := 2
	if depthStr != "" {
		if d, err := strconv.Atoi(depthStr); err == nil && d > 0 {
			depth = d
		}
	}

	var graph *entity.Graph
	if entityID != "" {
		graph = h.synthesizer.GetSubgraph(entityID, depth)
	} else {
		graph = h.synthesizer.GetGraph()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(graph)
}

// handleEntityMap serves the visual entity relationship map
func (h *EntityHandlers) handleEntityMap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	graph := h.synthesizer.GetGraph()
	stats := h.synthesizer.GetStats()

	data := struct {
		Graph *entity.Graph
		Stats map[string]interface{}
	}{
		Graph: graph,
		Stats: stats,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl := template.Must(template.New("entitymap").Funcs(template.FuncMap{
		"json": func(v interface{}) string {
			b, _ := json.Marshal(v)
			return string(b)
		},
	}).Parse(entityMapTemplate))
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

const entityMapTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Entity Relationship Map | Dogwatch</title>
    <style>
        :root {
            --bg-primary: #0d1117;
            --bg-secondary: #161b22;
            --bg-tertiary: #21262d;
            --text-primary: #c9d1d9;
            --text-secondary: #8b949e;
            --border-color: #30363d;
            --accent-blue: #58a6ff;
            --accent-green: #3fb950;
            --accent-yellow: #d29922;
            --accent-red: #f85149;
            --accent-purple: #a371f7;
        }

        * { box-sizing: border-box; margin: 0; padding: 0; }

        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: var(--bg-primary);
            color: var(--text-primary);
            height: 100vh;
            overflow: hidden;
        }

        .header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            padding: 16px 24px;
            background: var(--bg-secondary);
            border-bottom: 1px solid var(--border-color);
        }

        .header h1 {
            font-size: 20px;
            font-weight: 600;
        }

        .controls {
            display: flex;
            gap: 12px;
            align-items: center;
        }

        .btn {
            padding: 6px 12px;
            background: var(--bg-tertiary);
            border: 1px solid var(--border-color);
            color: var(--text-primary);
            border-radius: 6px;
            cursor: pointer;
            font-size: 13px;
        }

        .btn:hover { background: var(--bg-secondary); }
        .btn.active { background: var(--accent-blue); border-color: var(--accent-blue); }

        .graph-container {
            height: calc(100vh - 60px);
            position: relative;
        }

        #graph {
            width: 100%;
            height: 100%;
        }

        .legend {
            position: absolute;
            bottom: 20px;
            left: 20px;
            background: var(--bg-secondary);
            border: 1px solid var(--border-color);
            border-radius: 8px;
            padding: 12px;
        }

        .legend-title {
            font-size: 12px;
            color: var(--text-secondary);
            margin-bottom: 8px;
            text-transform: uppercase;
        }

        .legend-item {
            display: flex;
            align-items: center;
            gap: 8px;
            font-size: 12px;
            margin-bottom: 4px;
        }

        .legend-dot {
            width: 12px;
            height: 12px;
            border-radius: 50%;
        }

        .tooltip {
            position: absolute;
            background: var(--bg-secondary);
            border: 1px solid var(--border-color);
            border-radius: 8px;
            padding: 12px;
            pointer-events: none;
            z-index: 100;
            max-width: 300px;
            display: none;
        }

        .tooltip h4 {
            font-size: 14px;
            margin-bottom: 8px;
        }

        .tooltip-row {
            display: flex;
            justify-content: space-between;
            font-size: 12px;
            margin-bottom: 4px;
        }

        .tooltip-label { color: var(--text-secondary); }
        .tooltip-value { color: var(--text-primary); }

        .node-label {
            font-size: 11px;
            fill: var(--text-primary);
            pointer-events: none;
        }

        .edge-label {
            font-size: 9px;
            fill: var(--text-secondary);
        }

        .stats-bar {
            display: flex;
            gap: 24px;
            font-size: 13px;
            color: var(--text-secondary);
        }

        .stats-bar span { color: var(--accent-blue); }
    </style>
</head>
<body>
    <div class="header">
        <h1>🗺️ Entity Relationship Map</h1>
        <div class="stats-bar">
            <div><span>{{len .Graph.Nodes}}</span> entities</div>
            <div><span>{{len .Graph.Edges}}</span> relationships</div>
        </div>
        <div class="controls">
            <button class="btn" onclick="zoomIn()">+ Zoom In</button>
            <button class="btn" onclick="zoomOut()">- Zoom Out</button>
            <button class="btn" onclick="resetView()">Reset</button>
            <a href="/entities" class="btn">List View</a>
        </div>
    </div>

    <div class="graph-container">
        <svg id="graph"></svg>

        <div class="legend">
            <div class="legend-title">Entity Types</div>
            <div class="legend-item"><div class="legend-dot" style="background: #58a6ff"></div> Service</div>
            <div class="legend-item"><div class="legend-dot" style="background: #a371f7"></div> Host</div>
            <div class="legend-item"><div class="legend-dot" style="background: #3fb950"></div> Container</div>
            <div class="legend-item"><div class="legend-dot" style="background: #d29922"></div> Database</div>
            <div class="legend-item"><div class="legend-dot" style="background: #f85149"></div> Queue</div>
            <div class="legend-item"><div class="legend-dot" style="background: #8b949e"></div> External API</div>
        </div>

        <div class="tooltip" id="tooltip"></div>
    </div>

    <script>
        const graphData = {{.Graph | json}};

        const typeColors = {
            'SERVICE': '#58a6ff',
            'HOST': '#a371f7',
            'CONTAINER': '#3fb950',
            'DATABASE': '#d29922',
            'QUEUE': '#f85149',
            'EXTERNAL_API': '#8b949e',
            'LOAD_BALANCER': '#79c0ff',
            'CUSTOM': '#6e7681'
        };

        const healthColors = {
            'healthy': '#3fb950',
            'degraded': '#d29922',
            'unhealthy': '#f85149',
            'unknown': '#6e7681'
        };

        const svg = document.getElementById('graph');
        const tooltip = document.getElementById('tooltip');
        const width = svg.clientWidth;
        const height = svg.clientHeight;

        let scale = 1;
        let translateX = width / 2;
        let translateY = height / 2;

        // Create SVG groups
        svg.innerHTML = ` + "`" + `
            <defs>
                <marker id="arrowhead" markerWidth="10" markerHeight="7" refX="10" refY="3.5" orient="auto">
                    <polygon points="0 0, 10 3.5, 0 7" fill="#6e7681"/>
                </marker>
            </defs>
            <g id="main-group">
                <g id="edges"></g>
                <g id="nodes"></g>
            </g>
        ` + "`" + `;

        const mainGroup = document.getElementById('main-group');
        const edgesGroup = document.getElementById('edges');
        const nodesGroup = document.getElementById('nodes');

        // Simple force-directed layout
        const nodes = graphData.nodes || [];
        const edges = graphData.edges || [];

        // Initialize positions
        nodes.forEach((node, i) => {
            const angle = (2 * Math.PI * i) / nodes.length;
            const radius = Math.min(width, height) * 0.35;
            node.x = Math.cos(angle) * radius;
            node.y = Math.sin(angle) * radius;
            node.vx = 0;
            node.vy = 0;
        });

        // Create node map for edge lookup
        const nodeMap = {};
        nodes.forEach(n => nodeMap[n.id] = n);

        // Run force simulation
        function simulate() {
            const repulsion = 5000;
            const attraction = 0.01;
            const damping = 0.9;

            // Repulsion between all nodes
            for (let i = 0; i < nodes.length; i++) {
                for (let j = i + 1; j < nodes.length; j++) {
                    const dx = nodes[j].x - nodes[i].x;
                    const dy = nodes[j].y - nodes[i].y;
                    const dist = Math.sqrt(dx * dx + dy * dy) || 1;
                    const force = repulsion / (dist * dist);
                    const fx = (dx / dist) * force;
                    const fy = (dy / dist) * force;
                    nodes[i].vx -= fx;
                    nodes[i].vy -= fy;
                    nodes[j].vx += fx;
                    nodes[j].vy += fy;
                }
            }

            // Attraction along edges
            edges.forEach(edge => {
                const source = nodeMap[edge.source];
                const target = nodeMap[edge.target];
                if (!source || !target) return;

                const dx = target.x - source.x;
                const dy = target.y - source.y;
                const dist = Math.sqrt(dx * dx + dy * dy) || 1;
                const force = dist * attraction;
                const fx = (dx / dist) * force;
                const fy = (dy / dist) * force;
                source.vx += fx;
                source.vy += fy;
                target.vx -= fx;
                target.vy -= fy;
            });

            // Apply velocities
            nodes.forEach(node => {
                node.vx *= damping;
                node.vy *= damping;
                node.x += node.vx;
                node.y += node.vy;
            });
        }

        // Run simulation iterations
        for (let i = 0; i < 100; i++) simulate();

        // Draw edges
        edges.forEach(edge => {
            const source = nodeMap[edge.source];
            const target = nodeMap[edge.target];
            if (!source || !target) return;

            const line = document.createElementNS('http://www.w3.org/2000/svg', 'line');
            line.setAttribute('x1', source.x);
            line.setAttribute('y1', source.y);
            line.setAttribute('x2', target.x);
            line.setAttribute('y2', target.y);
            line.setAttribute('stroke', '#30363d');
            line.setAttribute('stroke-width', Math.max(1, Math.min(edge.weight / 100, 4)));
            line.setAttribute('marker-end', 'url(#arrowhead)');
            edgesGroup.appendChild(line);
        });

        // Draw nodes
        nodes.forEach(node => {
            const g = document.createElementNS('http://www.w3.org/2000/svg', 'g');
            g.setAttribute('transform', ` + "`" + `translate(${node.x}, ${node.y})` + "`" + `);
            g.style.cursor = 'pointer';

            const circle = document.createElementNS('http://www.w3.org/2000/svg', 'circle');
            circle.setAttribute('r', node.size * 8);
            circle.setAttribute('fill', typeColors[node.type] || '#6e7681');
            circle.setAttribute('stroke', healthColors[node.health] || '#6e7681');
            circle.setAttribute('stroke-width', 3);

            const text = document.createElementNS('http://www.w3.org/2000/svg', 'text');
            text.setAttribute('y', node.size * 8 + 14);
            text.setAttribute('text-anchor', 'middle');
            text.setAttribute('class', 'node-label');
            text.textContent = node.name.length > 15 ? node.name.slice(0, 15) + '...' : node.name;

            g.appendChild(circle);
            g.appendChild(text);

            // Tooltip events
            g.addEventListener('mouseenter', (e) => {
                tooltip.innerHTML = ` + "`" + `
                    <h4>${node.display_name}</h4>
                    <div class="tooltip-row"><span class="tooltip-label">Type:</span><span class="tooltip-value">${node.type}</span></div>
                    <div class="tooltip-row"><span class="tooltip-label">Health:</span><span class="tooltip-value">${node.health}</span></div>
                    <div class="tooltip-row"><span class="tooltip-label">Throughput:</span><span class="tooltip-value">${node.signals.throughput.toFixed(1)} ${node.signals.throughput_unit}</span></div>
                    <div class="tooltip-row"><span class="tooltip-label">Error Rate:</span><span class="tooltip-value">${node.signals.error_rate.toFixed(2)}%</span></div>
                    <div class="tooltip-row"><span class="tooltip-label">P99 Latency:</span><span class="tooltip-value">${node.signals.latency_p99.toFixed(0)}ms</span></div>
                ` + "`" + `;
                tooltip.style.display = 'block';
                tooltip.style.left = (e.pageX + 10) + 'px';
                tooltip.style.top = (e.pageY + 10) + 'px';
            });

            g.addEventListener('mouseleave', () => {
                tooltip.style.display = 'none';
            });

            g.addEventListener('click', () => {
                window.location.href = '/api/entities/' + encodeURIComponent(node.id);
            });

            nodesGroup.appendChild(g);
        });

        // Apply initial transform
        updateTransform();

        function updateTransform() {
            mainGroup.setAttribute('transform', ` + "`" + `translate(${translateX}, ${translateY}) scale(${scale})` + "`" + `);
        }

        function zoomIn() { scale *= 1.2; updateTransform(); }
        function zoomOut() { scale /= 1.2; updateTransform(); }
        function resetView() { scale = 1; translateX = width / 2; translateY = height / 2; updateTransform(); }

        // Mouse drag
        let isDragging = false;
        let lastX, lastY;

        svg.addEventListener('mousedown', (e) => {
            isDragging = true;
            lastX = e.clientX;
            lastY = e.clientY;
        });

        svg.addEventListener('mousemove', (e) => {
            if (!isDragging) return;
            translateX += e.clientX - lastX;
            translateY += e.clientY - lastY;
            lastX = e.clientX;
            lastY = e.clientY;
            updateTransform();
        });

        svg.addEventListener('mouseup', () => isDragging = false);
        svg.addEventListener('mouseleave', () => isDragging = false);

        // Mouse wheel zoom
        svg.addEventListener('wheel', (e) => {
            e.preventDefault();
            const factor = e.deltaY > 0 ? 0.9 : 1.1;
            scale *= factor;
            scale = Math.max(0.1, Math.min(scale, 5));
            updateTransform();
        });
    </script>
</body>
</html>`
