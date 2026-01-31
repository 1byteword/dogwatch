package web

import (
	"encoding/json"
	"html/template"
	"net/http"

	"dogwatch/internal/lookout"
)

// LookoutHandlers provides HTTP handlers for the Lookout feature
type LookoutHandlers struct {
	engine *lookout.Engine
}

// NewLookoutHandlers creates new Lookout handlers
func NewLookoutHandlers(engine *lookout.Engine) *LookoutHandlers {
	return &LookoutHandlers{engine: engine}
}

// RegisterRoutes registers Lookout routes
func (h *LookoutHandlers) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/lookout", h.handleOverview)
	mux.HandleFunc("/api/lookout/overview", h.handleOverview)
	mux.HandleFunc("/lookout", h.handleLookoutPage)
}

// handleOverview returns the Lookout overview as JSON
func (h *LookoutHandlers) handleOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	overview := h.engine.GetOverview()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(overview)
}

// handleLookoutPage serves the Lookout HTML page
func (h *LookoutHandlers) handleLookoutPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	overview := h.engine.GetOverview()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl := template.Must(template.New("lookout").Parse(lookoutTemplate))
	tmpl.Execute(w, overview)
}

const lookoutTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Lookout - What's Different Right Now | Dogwatch</title>
    <style>
        :root {
            --bg-primary: #0d1117;
            --bg-secondary: #161b22;
            --bg-tertiary: #21262d;
            --text-primary: #c9d1d9;
            --text-secondary: #8b949e;
            --text-muted: #6e7681;
            --border-color: #30363d;
            --critical-bg: #490202;
            --critical-border: #f85149;
            --critical-text: #ff7b72;
            --warning-bg: #341a04;
            --warning-border: #d29922;
            --warning-text: #e3b341;
            --info-bg: #0c2d6b;
            --info-border: #58a6ff;
            --info-text: #79c0ff;
            --success-color: #3fb950;
            --purple: #a371f7;
        }

        * { box-sizing: border-box; margin: 0; padding: 0; }

        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: var(--bg-primary);
            color: var(--text-primary);
            min-height: 100vh;
            line-height: 1.5;
        }

        .container { max-width: 1200px; margin: 0 auto; padding: 20px; }

        header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 24px;
            padding-bottom: 16px;
            border-bottom: 1px solid var(--border-color);
        }

        h1 { font-size: 24px; font-weight: 600; }
        .subtitle { color: var(--text-secondary); font-size: 14px; margin-top: 4px; }
        .timestamp { color: var(--text-muted); font-size: 13px; }

        .summary-bar {
            display: flex;
            gap: 24px;
            margin-bottom: 24px;
            padding: 16px;
            background: var(--bg-secondary);
            border-radius: 8px;
            border: 1px solid var(--border-color);
        }

        .summary-item { text-align: center; }
        .summary-value { font-size: 32px; font-weight: 600; }
        .summary-label { font-size: 12px; color: var(--text-secondary); text-transform: uppercase; }
        .summary-value.critical { color: var(--critical-text); }
        .summary-value.warning { color: var(--warning-text); }
        .summary-value.info { color: var(--info-text); }
        .summary-value.healthy { color: var(--success-color); }
        .summary-value.deploys { color: var(--purple); }

        .two-col { display: grid; grid-template-columns: 1fr 320px; gap: 24px; }
        .main-col { min-width: 0; }

        .section { margin-bottom: 24px; }
        .section-header {
            display: flex;
            align-items: center;
            gap: 8px;
            margin-bottom: 12px;
            font-size: 16px;
            font-weight: 600;
        }
        .section-header.critical { color: var(--critical-text); }
        .section-header.warning { color: var(--warning-text); }
        .section-header.info { color: var(--info-text); }
        .section-header.deploys { color: var(--purple); }

        .badge {
            display: inline-flex;
            align-items: center;
            justify-content: center;
            min-width: 20px;
            height: 20px;
            padding: 0 6px;
            font-size: 12px;
            font-weight: 600;
            border-radius: 10px;
        }
        .badge.critical { background: var(--critical-border); color: white; }
        .badge.warning { background: var(--warning-border); color: black; }
        .badge.info { background: var(--info-border); color: black; }
        .badge.deploys { background: var(--purple); color: white; }

        .items { display: flex; flex-direction: column; gap: 8px; }

        .item {
            display: flex;
            padding: 12px 16px;
            background: var(--bg-secondary);
            border-radius: 8px;
            border-left: 4px solid transparent;
        }
        .item.critical { border-left-color: var(--critical-border); background: var(--critical-bg); }
        .item.warning { border-left-color: var(--warning-border); background: var(--warning-bg); }
        .item.info { border-left-color: var(--info-border); background: var(--info-bg); }

        .item-content { flex: 1; }
        .item-header { display: flex; justify-content: space-between; align-items: flex-start; }
        .item-title { font-weight: 600; margin-bottom: 4px; }
        .item-description { font-size: 14px; color: var(--text-secondary); }

        .item-meta {
            display: flex;
            flex-wrap: wrap;
            gap: 12px;
            margin-top: 8px;
            font-size: 12px;
            color: var(--text-muted);
        }
        .item-meta span { display: flex; align-items: center; gap: 4px; }

        .change-badge {
            display: inline-block;
            padding: 2px 6px;
            font-size: 11px;
            font-weight: 600;
            border-radius: 4px;
            margin-left: 8px;
        }
        .change-badge.up { background: rgba(248,81,73,0.2); color: var(--critical-text); }
        .change-badge.down { background: rgba(63,185,80,0.2); color: var(--success-color); }

        .type-badge {
            display: inline-block;
            padding: 2px 8px;
            font-size: 11px;
            font-weight: 500;
            text-transform: uppercase;
            border-radius: 4px;
            background: var(--bg-tertiary);
            color: var(--text-secondary);
            margin-right: 8px;
        }

        .item-actions {
            display: flex;
            gap: 8px;
            margin-top: 10px;
        }

        .action-btn {
            padding: 4px 10px;
            font-size: 11px;
            background: var(--bg-tertiary);
            border: 1px solid var(--border-color);
            color: var(--text-secondary);
            border-radius: 4px;
            cursor: pointer;
            text-decoration: none;
        }
        .action-btn:hover { background: var(--bg-secondary); color: var(--text-primary); }
        .action-btn.primary { background: var(--info-border); color: black; border-color: var(--info-border); }

        .entity-link {
            color: var(--info-text);
            text-decoration: none;
            cursor: pointer;
        }
        .entity-link:hover { text-decoration: underline; }

        .deploy-related {
            display: inline-flex;
            align-items: center;
            gap: 4px;
            padding: 2px 6px;
            background: rgba(163,113,247,0.2);
            border-radius: 4px;
            font-size: 11px;
            color: var(--purple);
        }

        .all-clear {
            text-align: center;
            padding: 48px;
            background: var(--bg-secondary);
            border-radius: 8px;
            border: 1px solid var(--border-color);
        }
        .all-clear .icon { font-size: 64px; margin-bottom: 16px; }
        .all-clear h2 { color: var(--success-color); margin-bottom: 8px; }
        .all-clear p { color: var(--text-secondary); }

        .refresh-btn {
            padding: 8px 16px;
            font-size: 14px;
            background: var(--bg-tertiary);
            border: 1px solid var(--border-color);
            color: var(--text-primary);
            border-radius: 6px;
            cursor: pointer;
        }
        .refresh-btn:hover { background: var(--bg-secondary); }

        .sidebar {
            background: var(--bg-secondary);
            border: 1px solid var(--border-color);
            border-radius: 8px;
            padding: 16px;
            height: fit-content;
        }

        .deploy-item {
            padding: 10px;
            background: var(--bg-tertiary);
            border-radius: 6px;
            margin-bottom: 8px;
        }
        .deploy-item:last-child { margin-bottom: 0; }
        .deploy-service { font-weight: 600; font-size: 13px; }
        .deploy-meta { font-size: 11px; color: var(--text-muted); margin-top: 4px; }
        .deploy-version { color: var(--purple); }

        @media (max-width: 900px) {
            .two-col { grid-template-columns: 1fr; }
            .sidebar { order: -1; }
        }
    </style>
</head>
<body>
    <div class="container">
        <header>
            <div>
                <h1>🔭 Lookout</h1>
                <div class="subtitle">What's Different Right Now</div>
            </div>
            <div>
                <span class="timestamp">Last updated: {{.Timestamp.Format "15:04:05"}}</span>
                <button class="refresh-btn" onclick="location.reload()">Refresh</button>
            </div>
        </header>

        <div class="summary-bar">
            <div class="summary-item">
                <div class="summary-value critical">{{.CriticalCount}}</div>
                <div class="summary-label">Critical</div>
            </div>
            <div class="summary-item">
                <div class="summary-value warning">{{.WarningCount}}</div>
                <div class="summary-label">Warnings</div>
            </div>
            <div class="summary-item">
                <div class="summary-value info">{{.InfoCount}}</div>
                <div class="summary-label">Info</div>
            </div>
            <div class="summary-item">
                <div class="summary-value deploys">{{.DeployCount}}</div>
                <div class="summary-label">Deploys (1h)</div>
            </div>
            <div class="summary-item" style="margin-left: auto;">
                <div class="summary-value {{if eq .UnhealthyServices 0}}{{if eq .DegradedServices 0}}healthy{{else}}warning{{end}}{{else}}critical{{end}}">
                    {{.TotalServices}} svc
                </div>
                <div class="summary-label">Services</div>
            </div>
        </div>

        {{if eq .TotalItems 0}}
        <div class="all-clear">
            <div class="icon">✅</div>
            <h2>All Clear</h2>
            <p>No anomalies, alerts, or issues detected. Systems operating normally.</p>
        </div>
        {{else}}

        <div class="two-col">
            <div class="main-col">
                {{if .Critical}}
                <div class="section">
                    <div class="section-header critical">
                        🔴 Critical Issues <span class="badge critical">{{.CriticalCount}}</span>
                    </div>
                    <div class="items">
                        {{range .Critical}}
                        <div class="item critical">
                            <div class="item-content">
                                <div class="item-header">
                                    <div class="item-title">
                                        <span class="type-badge">{{.Type}}</span>
                                        {{.Title}}
                                        {{if .ChangePercent}}
                                            {{if gt .ChangePercent 0.0}}
                                            <span class="change-badge up">+{{printf "%.0f" .ChangePercent}}%</span>
                                            {{else if lt .ChangePercent 0.0}}
                                            <span class="change-badge down">{{printf "%.0f" .ChangePercent}}%</span>
                                            {{end}}
                                        {{end}}
                                    </div>
                                </div>
                                <div class="item-description">{{.Description}}</div>
                                <div class="item-meta">
                                    {{if .ServiceName}}
                                        <span>📦 {{if .EntityURL}}<a href="{{.EntityURL}}" class="entity-link">{{.ServiceName}}</a>{{else}}{{.ServiceName}}{{end}}</span>
                                    {{end}}
                                    {{if .MetricName}}<span>📊 {{.MetricName}}</span>{{end}}
                                    <span>⏱️ {{.Duration}}</span>
                                    {{if .Impact}}<span>💥 {{.Impact}}</span>{{end}}
                                    {{if .RelatedDeploys}}<span class="deploy-related">🚀 {{len .RelatedDeploys}} deploy(s)</span>{{end}}
                                </div>
                                <div class="item-actions">
                                    {{if .InvestigateURL}}<a href="{{.InvestigateURL}}" class="action-btn primary">🔍 Investigate</a>{{end}}
                                    {{if .AcknowledgeURL}}<button class="action-btn" onclick="ack('{{.ID}}')">✓ Ack</button>{{end}}
                                    {{if .SilenceURL}}<button class="action-btn" onclick="silence('{{.ID}}')">🔇 Silence</button>{{end}}
                                    {{if .EntityURL}}<a href="/entities" class="action-btn">🗺️ Entity Map</a>{{end}}
                                </div>
                            </div>
                        </div>
                        {{end}}
                    </div>
                </div>
                {{end}}

                {{if .Warning}}
                <div class="section">
                    <div class="section-header warning">
                        ⚠️ Warnings <span class="badge warning">{{.WarningCount}}</span>
                    </div>
                    <div class="items">
                        {{range .Warning}}
                        <div class="item warning">
                            <div class="item-content">
                                <div class="item-header">
                                    <div class="item-title">
                                        <span class="type-badge">{{.Type}}</span>
                                        {{.Title}}
                                        {{if .ChangePercent}}
                                            {{if gt .ChangePercent 0.0}}
                                            <span class="change-badge up">+{{printf "%.0f" .ChangePercent}}%</span>
                                            {{else if lt .ChangePercent 0.0}}
                                            <span class="change-badge down">{{printf "%.0f" .ChangePercent}}%</span>
                                            {{end}}
                                        {{end}}
                                    </div>
                                </div>
                                <div class="item-description">{{.Description}}</div>
                                <div class="item-meta">
                                    {{if .ServiceName}}
                                        <span>📦 {{if .EntityURL}}<a href="{{.EntityURL}}" class="entity-link">{{.ServiceName}}</a>{{else}}{{.ServiceName}}{{end}}</span>
                                    {{end}}
                                    {{if .MetricName}}<span>📊 {{.MetricName}}</span>{{end}}
                                    <span>⏱️ {{.Duration}}</span>
                                    {{if .RelatedDeploys}}<span class="deploy-related">🚀 {{len .RelatedDeploys}} deploy(s)</span>{{end}}
                                </div>
                                <div class="item-actions">
                                    {{if .InvestigateURL}}<a href="{{.InvestigateURL}}" class="action-btn primary">🔍 Investigate</a>{{end}}
                                    {{if .AcknowledgeURL}}<button class="action-btn" onclick="ack('{{.ID}}')">✓ Ack</button>{{end}}
                                    {{if .EntityURL}}<a href="/entities" class="action-btn">🗺️ Entity Map</a>{{end}}
                                </div>
                            </div>
                        </div>
                        {{end}}
                    </div>
                </div>
                {{end}}

                {{if .Info}}
                <div class="section">
                    <div class="section-header info">
                        ℹ️ Info <span class="badge info">{{.InfoCount}}</span>
                    </div>
                    <div class="items">
                        {{range .Info}}
                        <div class="item info">
                            <div class="item-content">
                                <div class="item-title">
                                    <span class="type-badge">{{.Type}}</span>
                                    {{.Title}}
                                </div>
                                <div class="item-description">{{.Description}}</div>
                                <div class="item-meta">
                                    {{if .ServiceName}}<span>📦 {{.ServiceName}}</span>{{end}}
                                    {{if .MetricName}}<span>📊 {{.MetricName}}</span>{{end}}
                                    <span>⏱️ {{.Duration}}</span>
                                </div>
                            </div>
                        </div>
                        {{end}}
                    </div>
                </div>
                {{end}}
            </div>

            <div class="sidebar">
                <div class="section-header deploys">
                    🚀 Recent Deploys <span class="badge deploys">{{.DeployCount}}</span>
                </div>
                {{if .RecentDeploys}}
                    {{range .RecentDeploys}}
                    <div class="deploy-item">
                        <div class="deploy-service">{{.Service}}</div>
                        <div class="deploy-meta">
                            <span class="deploy-version">{{.Version}}</span> · {{.TimeAgo}} · {{.User}}
                            {{if eq .Status "failed"}} · <span style="color: var(--critical-text)">failed</span>{{end}}
                        </div>
                    </div>
                    {{end}}
                {{else}}
                    <div style="color: var(--text-muted); font-size: 13px; padding: 16px 0;">
                        No deploys in the last 2 hours
                    </div>
                {{end}}
            </div>
        </div>

        {{end}}
    </div>

    <script>
        // Auto-refresh every 30 seconds
        setTimeout(() => location.reload(), 30000);

        // Quick action handlers
        function ack(id) {
            fetch('/api/alerting/alerts/' + id + '/acknowledge', { method: 'POST' })
                .then(() => location.reload())
                .catch(e => alert('Failed to acknowledge: ' + e));
        }

        function silence(id) {
            const duration = prompt('Silence duration (e.g., 1h, 30m):', '1h');
            if (duration) {
                fetch('/api/alerting/alerts/' + id + '/silence?duration=' + duration, { method: 'POST' })
                    .then(() => location.reload())
                    .catch(e => alert('Failed to silence: ' + e));
            }
        }
    </script>
</body>
</html>`
