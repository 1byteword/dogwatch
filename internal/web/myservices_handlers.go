package web

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"time"

	"dogwatch/internal/catalog"
	"dogwatch/internal/deploys"
	"dogwatch/internal/incidents"
	"dogwatch/internal/oncall"
	"dogwatch/internal/rbac"
	"dogwatch/internal/slo"
)

// MyServicesHandlers provides HTTP handlers for the "My Services" developer view
type MyServicesHandlers struct {
	catalogStore    *catalog.Store
	rbacStore       *rbac.Store
	deploysStore    *deploys.Store
	incidentStore   *incidents.Store
	oncallStore     *oncall.Store
	oncallCalc      *oncall.Calculator
	sloStore        *slo.Store
}

// NewMyServicesHandlers creates new MyServices handlers
func NewMyServicesHandlers(
	catalogStore *catalog.Store,
	rbacStore *rbac.Store,
) *MyServicesHandlers {
	return &MyServicesHandlers{
		catalogStore: catalogStore,
		rbacStore:    rbacStore,
	}
}

// SetDeploysStore sets the deploys store
func (h *MyServicesHandlers) SetDeploysStore(store *deploys.Store) {
	h.deploysStore = store
}

// SetIncidentStore sets the incident store
func (h *MyServicesHandlers) SetIncidentStore(store *incidents.Store) {
	h.incidentStore = store
}

// SetOncallStore sets the on-call store and calculator
func (h *MyServicesHandlers) SetOncallStore(store *oncall.Store, calc *oncall.Calculator) {
	h.oncallStore = store
	h.oncallCalc = calc
}

// SetSLOStore sets the SLO store
func (h *MyServicesHandlers) SetSLOStore(store *slo.Store) {
	h.sloStore = store
}

// RegisterRoutes registers My Services routes
func (h *MyServicesHandlers) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/my/services", h.handleMyServicesAPI)
	mux.HandleFunc("/my-services", h.handleMyServicesPage)
}

// MyServicesView represents the My Services dashboard data
type MyServicesView struct {
	UserID      string    `json:"user_id"`
	UserEmail   string    `json:"user_email"`
	UserTeams   []string  `json:"user_teams"`
	Timestamp   time.Time `json:"timestamp"`

	// Summary stats
	TotalServices   int `json:"total_services"`
	Tier1Count      int `json:"tier1_count"`
	Tier2Count      int `json:"tier2_count"`
	Tier3Count      int `json:"tier3_count"`
	HealthyCount    int `json:"healthy_count"`
	DegradedCount   int `json:"degraded_count"`
	UnhealthyCount  int `json:"unhealthy_count"`

	// Grouped data
	Teams []MyTeamServices `json:"teams"`

	// Alerts and incidents
	ActiveIncidents  int `json:"active_incidents"`
	RecentDeploys    int `json:"recent_deploys"`
	SLOsAtRisk       int `json:"slos_at_risk"`
}

// MyTeamServices represents services for a single team
type MyTeamServices struct {
	TeamID   string      `json:"team_id"`
	TeamName string      `json:"team_name"`
	Services []MyService `json:"services"`
}

// MyService represents a service with additional context
type MyService struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	DisplayName     string   `json:"display_name"`
	Description     string   `json:"description"`
	Tier            string   `json:"tier"`
	Health          string   `json:"health"`
	Language        string   `json:"language"`
	Framework       string   `json:"framework"`
	RepoURL         string   `json:"repo_url"`
	Tags            []string `json:"tags"`

	// Contextual data
	RecentDeploys   int    `json:"recent_deploys"`
	ActiveIncidents int    `json:"active_incidents"`
	SLOStatus       string `json:"slo_status"` // "ok", "at_risk", "breached"
	LastDeploy      string `json:"last_deploy"`
	OnCallUser      string `json:"oncall_user"`
}

// handleMyServicesAPI returns JSON for the My Services view
func (h *MyServicesHandlers) handleMyServicesAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	view, err := h.buildMyServicesView(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(view)
}

// handleMyServicesPage serves the HTML page
func (h *MyServicesHandlers) handleMyServicesPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	view, err := h.buildMyServicesView(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl := template.Must(template.New("myservices").Parse(myServicesTemplate))
	tmpl.Execute(w, view)
}

// buildMyServicesView constructs the view for a user
func (h *MyServicesHandlers) buildMyServicesView(r *http.Request) (*MyServicesView, error) {
	// Get user from query params or auth context
	userID := r.URL.Query().Get("user_id")
	userEmail := r.URL.Query().Get("email")

	view := &MyServicesView{
		UserID:    userID,
		UserEmail: userEmail,
		Timestamp: time.Now(),
		Teams:     []MyTeamServices{},
	}

	if h.catalogStore == nil || h.rbacStore == nil {
		return view, nil
	}

	// Get user's team memberships
	var teamIDs []string
	if userID != "" {
		if user, err := h.rbacStore.GetUser(userID); err == nil && user != nil {
			teamIDs = user.TeamIDs
			view.UserEmail = user.Email
		}
	} else if userEmail != "" {
		// Try to find user by email
		users, err := h.rbacStore.ListUsers("default")
		if err == nil {
			for _, u := range users {
				if u.Email == userEmail {
					teamIDs = u.TeamIDs
					view.UserID = u.ID
					break
				}
			}
		}
	}

	// If no specific user, show all teams (for demo/testing)
	if len(teamIDs) == 0 {
		teams, err := h.catalogStore.ListTeams("default")
		if err == nil {
			for _, t := range teams {
				teamIDs = append(teamIDs, t.ID)
			}
		}
	}

	view.UserTeams = teamIDs

	// Build services by team
	teamMap := make(map[string]*MyTeamServices)

	for _, teamID := range teamIDs {
		// Get team info
		team, err := h.catalogStore.GetTeam(teamID)
		teamName := teamID
		if err == nil && team != nil {
			teamName = team.Name
		}

		teamMap[teamID] = &MyTeamServices{
			TeamID:   teamID,
			TeamName: teamName,
			Services: []MyService{},
		}

		// Get services for this team
		services, err := h.catalogStore.ListServices("default", catalog.ServiceFilters{
			TeamID: teamID,
		})
		if err != nil {
			continue
		}

		for _, svc := range services {
			myService := h.enrichService(svc)
			teamMap[teamID].Services = append(teamMap[teamID].Services, myService)

			// Update stats
			view.TotalServices++
			switch svc.Tier {
			case "tier1":
				view.Tier1Count++
			case "tier2":
				view.Tier2Count++
			default:
				view.Tier3Count++
			}
			switch svc.Health {
			case catalog.HealthHealthy:
				view.HealthyCount++
			case catalog.HealthDegraded:
				view.DegradedCount++
			case catalog.HealthUnhealthy:
				view.UnhealthyCount++
			}

			view.RecentDeploys += myService.RecentDeploys
			view.ActiveIncidents += myService.ActiveIncidents
			if myService.SLOStatus == "at_risk" || myService.SLOStatus == "breached" {
				view.SLOsAtRisk++
			}
		}
	}

	// Convert map to sorted slice
	for _, ts := range teamMap {
		// Sort services by tier then name
		sort.Slice(ts.Services, func(i, j int) bool {
			if ts.Services[i].Tier != ts.Services[j].Tier {
				return ts.Services[i].Tier < ts.Services[j].Tier
			}
			return ts.Services[i].Name < ts.Services[j].Name
		})
		view.Teams = append(view.Teams, *ts)
	}

	// Sort teams by name
	sort.Slice(view.Teams, func(i, j int) bool {
		return view.Teams[i].TeamName < view.Teams[j].TeamName
	})

	return view, nil
}

// enrichService adds contextual data to a service
func (h *MyServicesHandlers) enrichService(svc *catalog.Service) MyService {
	ms := MyService{
		ID:          svc.ID,
		Name:        svc.Name,
		DisplayName: svc.DisplayName,
		Description: svc.Description,
		Tier:        string(svc.Tier),
		Health:      string(svc.Health),
		Language:    svc.Language,
		Framework:   svc.Framework,
		RepoURL:     svc.RepoURL,
		Tags:        svc.Tags,
		SLOStatus:   "ok",
	}

	if ms.DisplayName == "" {
		ms.DisplayName = ms.Name
	}

	// Get recent deploys
	if h.deploysStore != nil {
		deployList, err := h.deploysStore.ListByService(svc.Name, 10)
		if err == nil {
			weekAgo := time.Now().Add(-7 * 24 * time.Hour)
			for _, d := range deployList {
				if d.Timestamp.After(weekAgo) {
					ms.RecentDeploys++
				}
			}
			if len(deployList) > 0 {
				ms.LastDeploy = formatTimeAgo(deployList[0].Timestamp)
			}
		}
	}

	// Get active incidents
	if h.incidentStore != nil {
		incidentList, err := h.incidentStore.ListIncidentsByService(svc.ID, 10)
		if err == nil {
			for _, inc := range incidentList {
				if inc.Status == "open" || inc.Status == "investigating" || inc.Status == "identified" {
					ms.ActiveIncidents++
				}
			}
		}
	}

	// Get current on-call user
	if h.oncallCalc != nil && svc.OnCallID != "" {
		entry, err := h.oncallCalc.GetCurrentOnCall(svc.OnCallID)
		if err == nil && entry != nil {
			ms.OnCallUser = entry.User.Name
			if ms.OnCallUser == "" {
				ms.OnCallUser = entry.User.Email
			}
		}
	}

	// Get SLO status from latest snapshot
	if h.sloStore != nil && svc.SLOID != "" {
		snapshots, err := h.sloStore.GetSnapshots(svc.SLOID, time.Hour, 1)
		if err == nil && len(snapshots) > 0 {
			snap := snapshots[0]
			// Check based on status
			switch snap.Status {
			case slo.StatusBreached:
				ms.SLOStatus = "breached"
			case slo.StatusAtRisk:
				ms.SLOStatus = "at_risk"
			default:
				// Also check budget remaining (negative = breached)
				if snap.BudgetRemaining < 0 {
					ms.SLOStatus = "breached"
				}
			}
		}
	}

	return ms
}

func formatTimeAgo(t time.Time) string {
	d := time.Since(t)
	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		mins := int(d.Minutes())
		if mins == 1 {
			return "1 min ago"
		}
		return fmt.Sprintf("%d mins ago", mins)
	}
	if d < 24*time.Hour {
		hours := int(d.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	}
	days := int(d.Hours() / 24)
	if days == 1 {
		return "1 day ago"
	}
	return fmt.Sprintf("%d days ago", days)
}

const myServicesTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>My Services | Dogwatch</title>
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
            --accent-purple: #a371f7;
        }

        * { box-sizing: border-box; margin: 0; padding: 0; }

        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: var(--bg-primary);
            color: var(--text-primary);
            min-height: 100vh;
            line-height: 1.5;
        }

        .container { max-width: 1400px; margin: 0 auto; padding: 24px; }

        header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 24px;
            padding-bottom: 16px;
            border-bottom: 1px solid var(--border-color);
        }

        h1 { font-size: 24px; font-weight: 600; display: flex; align-items: center; gap: 12px; }
        .subtitle { color: var(--text-secondary); font-size: 14px; margin-top: 4px; }

        .stats-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
            gap: 16px;
            margin-bottom: 32px;
        }

        .stat-card {
            background: var(--bg-secondary);
            border: 1px solid var(--border-color);
            border-radius: 8px;
            padding: 16px;
            text-align: center;
        }

        .stat-value {
            font-size: 32px;
            font-weight: 700;
            line-height: 1;
        }

        .stat-label {
            font-size: 12px;
            color: var(--text-secondary);
            text-transform: uppercase;
            margin-top: 8px;
        }

        .stat-value.green { color: var(--accent-green); }
        .stat-value.yellow { color: var(--accent-yellow); }
        .stat-value.red { color: var(--accent-red); }
        .stat-value.blue { color: var(--accent-blue); }
        .stat-value.purple { color: var(--accent-purple); }

        .team-section {
            margin-bottom: 32px;
        }

        .team-header {
            display: flex;
            align-items: center;
            gap: 12px;
            margin-bottom: 16px;
            font-size: 18px;
            font-weight: 600;
            color: var(--accent-blue);
        }

        .team-badge {
            background: var(--bg-tertiary);
            padding: 4px 10px;
            border-radius: 12px;
            font-size: 12px;
            color: var(--text-secondary);
        }

        .services-grid {
            display: grid;
            grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
            gap: 16px;
        }

        .service-card {
            background: var(--bg-secondary);
            border: 1px solid var(--border-color);
            border-radius: 8px;
            padding: 16px;
            transition: border-color 0.2s;
        }

        .service-card:hover {
            border-color: var(--accent-blue);
        }

        .service-header {
            display: flex;
            justify-content: space-between;
            align-items: flex-start;
            margin-bottom: 12px;
        }

        .service-name {
            font-size: 16px;
            font-weight: 600;
        }

        .service-desc {
            font-size: 13px;
            color: var(--text-secondary);
            margin-bottom: 12px;
            display: -webkit-box;
            -webkit-line-clamp: 2;
            -webkit-box-orient: vertical;
            overflow: hidden;
        }

        .tier-badge {
            padding: 2px 8px;
            border-radius: 4px;
            font-size: 11px;
            font-weight: 600;
            text-transform: uppercase;
        }

        .tier-badge.tier1 { background: rgba(248,81,73,0.2); color: var(--accent-red); }
        .tier-badge.tier2 { background: rgba(210,153,34,0.2); color: var(--accent-yellow); }
        .tier-badge.tier3 { background: rgba(139,148,158,0.2); color: var(--text-secondary); }

        .health-indicator {
            display: inline-flex;
            align-items: center;
            gap: 6px;
            font-size: 12px;
            padding: 4px 8px;
            border-radius: 4px;
            margin-right: 8px;
        }

        .health-indicator.healthy { background: rgba(63,185,80,0.15); color: var(--accent-green); }
        .health-indicator.degraded { background: rgba(210,153,34,0.15); color: var(--accent-yellow); }
        .health-indicator.unhealthy { background: rgba(248,81,73,0.15); color: var(--accent-red); }
        .health-indicator.unknown { background: rgba(139,148,158,0.15); color: var(--text-secondary); }

        .health-dot {
            width: 8px;
            height: 8px;
            border-radius: 50%;
        }

        .health-dot.healthy { background: var(--accent-green); }
        .health-dot.degraded { background: var(--accent-yellow); }
        .health-dot.unhealthy { background: var(--accent-red); }
        .health-dot.unknown { background: var(--text-muted); }

        .service-meta {
            display: flex;
            flex-wrap: wrap;
            gap: 12px;
            font-size: 12px;
            color: var(--text-muted);
            margin-top: 12px;
            padding-top: 12px;
            border-top: 1px solid var(--border-color);
        }

        .meta-item {
            display: flex;
            align-items: center;
            gap: 4px;
        }

        .meta-item.warning { color: var(--accent-yellow); }
        .meta-item.danger { color: var(--accent-red); }

        .service-actions {
            display: flex;
            gap: 8px;
            margin-top: 12px;
        }

        .action-btn {
            padding: 6px 12px;
            font-size: 12px;
            background: var(--bg-tertiary);
            border: 1px solid var(--border-color);
            color: var(--text-secondary);
            border-radius: 4px;
            cursor: pointer;
            text-decoration: none;
            transition: all 0.2s;
        }

        .action-btn:hover {
            background: var(--bg-secondary);
            color: var(--text-primary);
            border-color: var(--accent-blue);
        }

        .action-btn.primary {
            background: var(--accent-blue);
            border-color: var(--accent-blue);
            color: white;
        }

        .empty-state {
            text-align: center;
            padding: 64px 32px;
            background: var(--bg-secondary);
            border-radius: 8px;
            border: 1px solid var(--border-color);
        }

        .empty-state h2 {
            font-size: 20px;
            margin-bottom: 8px;
        }

        .empty-state p {
            color: var(--text-secondary);
            margin-bottom: 16px;
        }

        .nav-links {
            display: flex;
            gap: 16px;
        }

        .nav-link {
            color: var(--text-secondary);
            text-decoration: none;
            font-size: 14px;
            padding: 8px 12px;
            border-radius: 6px;
            transition: all 0.2s;
        }

        .nav-link:hover {
            background: var(--bg-tertiary);
            color: var(--text-primary);
        }

        .refresh-btn {
            padding: 8px 16px;
            font-size: 14px;
            background: var(--bg-tertiary);
            border: 1px solid var(--border-color);
            color: var(--text-primary);
            border-radius: 6px;
            cursor: pointer;
        }

        .refresh-btn:hover {
            background: var(--bg-secondary);
        }

        @media (max-width: 768px) {
            .stats-grid {
                grid-template-columns: repeat(2, 1fr);
            }
            .services-grid {
                grid-template-columns: 1fr;
            }
        }
    </style>
</head>
<body>
    <div class="container">
        <header>
            <div>
                <h1>
                    <span style="font-size: 28px;">🔧</span>
                    My Services
                </h1>
                <div class="subtitle">Your teams' services at a glance</div>
            </div>
            <div class="nav-links">
                <a href="/" class="nav-link">Dashboard</a>
                <a href="/my-oncall" class="nav-link">My On-Call</a>
                <a href="/lookout" class="nav-link">Lookout</a>
                <button class="refresh-btn" onclick="location.reload()">Refresh</button>
            </div>
        </header>

        <div class="stats-grid">
            <div class="stat-card">
                <div class="stat-value blue">{{.TotalServices}}</div>
                <div class="stat-label">Total Services</div>
            </div>
            <div class="stat-card">
                <div class="stat-value green">{{.HealthyCount}}</div>
                <div class="stat-label">Healthy</div>
            </div>
            <div class="stat-card">
                <div class="stat-value {{if gt .DegradedCount 0}}yellow{{else}}green{{end}}">{{.DegradedCount}}</div>
                <div class="stat-label">Degraded</div>
            </div>
            <div class="stat-card">
                <div class="stat-value {{if gt .UnhealthyCount 0}}red{{else}}green{{end}}">{{.UnhealthyCount}}</div>
                <div class="stat-label">Unhealthy</div>
            </div>
            <div class="stat-card">
                <div class="stat-value {{if gt .ActiveIncidents 0}}red{{else}}green{{end}}">{{.ActiveIncidents}}</div>
                <div class="stat-label">Active Incidents</div>
            </div>
            <div class="stat-card">
                <div class="stat-value purple">{{.RecentDeploys}}</div>
                <div class="stat-label">Deploys (7d)</div>
            </div>
            <div class="stat-card">
                <div class="stat-value {{if gt .SLOsAtRisk 0}}yellow{{else}}green{{end}}">{{.SLOsAtRisk}}</div>
                <div class="stat-label">SLOs at Risk</div>
            </div>
        </div>

        {{if eq .TotalServices 0}}
        <div class="empty-state">
            <h2>No Services Found</h2>
            <p>You're not assigned to any teams with services, or no services have been created yet.</p>
            <a href="/api/catalog/services" class="action-btn primary">View All Services</a>
        </div>
        {{else}}
        {{range .Teams}}
        {{if .Services}}
        <div class="team-section">
            <div class="team-header">
                <span>{{.TeamName}}</span>
                <span class="team-badge">{{len .Services}} services</span>
            </div>
            <div class="services-grid">
                {{range .Services}}
                <div class="service-card">
                    <div class="service-header">
                        <div>
                            <div class="service-name">{{.DisplayName}}</div>
                        </div>
                        <span class="tier-badge {{.Tier}}">{{.Tier}}</span>
                    </div>
                    {{if .Description}}
                    <div class="service-desc">{{.Description}}</div>
                    {{end}}
                    <div>
                        <span class="health-indicator {{.Health}}">
                            <span class="health-dot {{.Health}}"></span>
                            {{.Health}}
                        </span>
                        {{if .Language}}
                        <span style="font-size: 12px; color: var(--text-muted);">{{.Language}}</span>
                        {{end}}
                    </div>
                    <div class="service-meta">
                        {{if gt .ActiveIncidents 0}}
                        <span class="meta-item danger">🚨 {{.ActiveIncidents}} incident{{if gt .ActiveIncidents 1}}s{{end}}</span>
                        {{end}}
                        {{if gt .RecentDeploys 0}}
                        <span class="meta-item">🚀 {{.RecentDeploys}} deploy{{if gt .RecentDeploys 1}}s{{end}} (7d)</span>
                        {{end}}
                        {{if .LastDeploy}}
                        <span class="meta-item">📅 {{.LastDeploy}}</span>
                        {{end}}
                        {{if .OnCallUser}}
                        <span class="meta-item">📟 {{.OnCallUser}}</span>
                        {{end}}
                        {{if eq .SLOStatus "at_risk"}}
                        <span class="meta-item warning">⚠️ SLO at risk</span>
                        {{else if eq .SLOStatus "breached"}}
                        <span class="meta-item danger">🔴 SLO breached</span>
                        {{end}}
                    </div>
                    <div class="service-actions">
                        <a href="/api/catalog/services/{{.ID}}" class="action-btn">Details</a>
                        <a href="/api/traces?service={{.Name}}" class="action-btn">Traces</a>
                        <a href="/api/logs?service={{.Name}}" class="action-btn">Logs</a>
                        {{if .RepoURL}}
                        <a href="{{.RepoURL}}" class="action-btn" target="_blank">Repo</a>
                        {{end}}
                    </div>
                </div>
                {{end}}
            </div>
        </div>
        {{end}}
        {{end}}
        {{end}}
    </div>

    <script>
        // Auto-refresh every 60 seconds
        setTimeout(() => location.reload(), 60000);
    </script>
</body>
</html>`
