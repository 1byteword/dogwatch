package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"dogwatch/internal/catalog"
	"dogwatch/internal/incidents"
	"dogwatch/internal/synthetics"
)

// CatalogHandlers provides HTTP handlers for the service catalog
type CatalogHandlers struct {
	store           *catalog.Store
	incidentStore   *incidents.Store
	syntheticsStore *synthetics.Store
}

// NewCatalogHandlers creates catalog handlers
func NewCatalogHandlers(store *catalog.Store) *CatalogHandlers {
	return &CatalogHandlers{store: store}
}

// SetIncidentStore sets the incident store for context queries
func (h *CatalogHandlers) SetIncidentStore(store *incidents.Store) {
	h.incidentStore = store
}

// SetSyntheticsStore sets the synthetics store for context queries
func (h *CatalogHandlers) SetSyntheticsStore(store *synthetics.Store) {
	h.syntheticsStore = store
}

// RegisterRoutes registers service catalog routes
func (h *CatalogHandlers) RegisterRoutes(mux *http.ServeMux) {
	// Service management
	mux.HandleFunc("/api/catalog/services", h.handleServices)
	mux.HandleFunc("/api/catalog/services/", h.handleService)
	mux.HandleFunc("/api/catalog/services/stats", h.handleServiceStats)

	// Team management
	mux.HandleFunc("/api/catalog/teams", h.handleTeams)
	mux.HandleFunc("/api/catalog/teams/", h.handleTeam)

	// Dependencies
	mux.HandleFunc("/api/catalog/dependencies", h.handleDependencies)
	mux.HandleFunc("/api/catalog/graph", h.handleServiceGraph)

	// Runbooks
	mux.HandleFunc("/api/catalog/runbooks", h.handleRunbooks)
	mux.HandleFunc("/api/catalog/runbooks/", h.handleRunbook)
	mux.HandleFunc("/api/catalog/runbooks/search", h.handleRunbookSearch)
}

// handleServices handles GET/POST for services list
func (h *CatalogHandlers) handleServices(w http.ResponseWriter, r *http.Request) {
	orgID := r.Header.Get("X-Org-ID")
	if orgID == "" {
		orgID = "default"
	}

	switch r.Method {
	case http.MethodGet:
		filters := catalog.ServiceFilters{
			TeamID:    r.URL.Query().Get("team_id"),
			Tier:      catalog.ServiceTier(r.URL.Query().Get("tier")),
			Lifecycle: catalog.ServiceLifecycle(r.URL.Query().Get("lifecycle")),
			Health:    catalog.ServiceHealth(r.URL.Query().Get("health")),
		}

		services, err := h.store.ListServices(orgID, filters)
		if err != nil {
			http.Error(w, "Failed to list services", http.StatusInternalServerError)
			return
		}

		respondJSON(w, services)

	case http.MethodPost:
		var svc catalog.Service
		if err := json.NewDecoder(r.Body).Decode(&svc); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		svc.ID = fmt.Sprintf("svc_%d", time.Now().UnixNano())
		svc.OrgID = orgID

		if err := h.store.CreateService(&svc); err != nil {
			if strings.Contains(err.Error(), "UNIQUE constraint") {
				http.Error(w, "Service with this name already exists", http.StatusConflict)
				return
			}
			http.Error(w, "Failed to create service", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
		respondJSON(w, svc)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleService handles GET/PUT/DELETE for single service
func (h *CatalogHandlers) handleService(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/catalog/services/")

	// Handle subpaths
	if strings.Contains(path, "/") {
		parts := strings.SplitN(path, "/", 2)
		serviceID := parts[0]
		subpath := parts[1]

		switch subpath {
		case "dependencies":
			h.handleServiceDependencies(w, r, serviceID)
		case "runbooks":
			h.handleServiceRunbooks(w, r, serviceID)
		case "health":
			h.handleServiceHealth(w, r, serviceID)
		case "context":
			h.handleServiceContext(w, r, serviceID)
		default:
			http.Error(w, "Not found", http.StatusNotFound)
		}
		return
	}

	id := path
	if id == "" || id == "stats" {
		if id == "stats" {
			h.handleServiceStats(w, r)
			return
		}
		http.Error(w, "Service ID required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		svc, err := h.store.GetService(id)
		if err != nil {
			http.Error(w, "Service not found", http.StatusNotFound)
			return
		}
		respondJSON(w, svc)

	case http.MethodPut:
		var svc catalog.Service
		if err := json.NewDecoder(r.Body).Decode(&svc); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		svc.ID = id

		if err := h.store.UpdateService(&svc); err != nil {
			http.Error(w, "Failed to update service", http.StatusInternalServerError)
			return
		}
		respondJSON(w, svc)

	case http.MethodDelete:
		if err := h.store.DeleteService(id); err != nil {
			http.Error(w, "Failed to delete service", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleServiceStats returns aggregate service statistics
func (h *CatalogHandlers) handleServiceStats(w http.ResponseWriter, r *http.Request) {
	orgID := r.Header.Get("X-Org-ID")
	if orgID == "" {
		orgID = "default"
	}

	stats, err := h.store.GetServiceStats(orgID)
	if err != nil {
		http.Error(w, "Failed to get stats", http.StatusInternalServerError)
		return
	}

	respondJSON(w, stats)
}

// handleServiceDependencies handles GET/POST for service dependencies
func (h *CatalogHandlers) handleServiceDependencies(w http.ResponseWriter, r *http.Request, serviceID string) {
	switch r.Method {
	case http.MethodGet:
		upstream, downstream, err := h.store.GetDependencies(serviceID)
		if err != nil {
			http.Error(w, "Failed to get dependencies", http.StatusInternalServerError)
			return
		}

		respondJSON(w, map[string]interface{}{
			"upstream":   upstream,
			"downstream": downstream,
		})

	case http.MethodPost:
		var dep catalog.Dependency
		if err := json.NewDecoder(r.Body).Decode(&dep); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		dep.ID = fmt.Sprintf("dep_%d", time.Now().UnixNano())
		dep.SourceService = serviceID

		if err := h.store.AddDependency(&dep); err != nil {
			http.Error(w, "Failed to add dependency", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
		respondJSON(w, dep)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleServiceRunbooks handles GET for service runbooks
func (h *CatalogHandlers) handleServiceRunbooks(w http.ResponseWriter, r *http.Request, serviceID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	runbooks, err := h.store.GetRunbooksForService(serviceID)
	if err != nil {
		http.Error(w, "Failed to get runbooks", http.StatusInternalServerError)
		return
	}

	respondJSON(w, runbooks)
}

// handleServiceHealth handles POST to update service health
func (h *CatalogHandlers) handleServiceHealth(w http.ResponseWriter, r *http.Request, serviceID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Health catalog.ServiceHealth `json:"health"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.store.UpdateServiceHealth(serviceID, req.Health); err != nil {
		http.Error(w, "Failed to update health", http.StatusInternalServerError)
		return
	}

	respondJSON(w, map[string]string{"status": "updated"})
}

// ServiceContext provides full context about a service including all linked resources
type ServiceContext struct {
	Service      *catalog.Service      `json:"service"`
	Team         *catalog.Team         `json:"team,omitempty"`
	Upstream     []*catalog.Dependency `json:"upstream_dependencies"`
	Downstream   []*catalog.Dependency `json:"downstream_dependencies"`
	Runbooks     []*catalog.Runbook    `json:"runbooks"`
	Incidents    []incidents.Incident  `json:"recent_incidents,omitempty"`
	SynthChecks  []synthetics.Check    `json:"synthetic_checks,omitempty"`
}

// handleServiceContext returns full service context with all linked resources
func (h *CatalogHandlers) handleServiceContext(w http.ResponseWriter, r *http.Request, serviceID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get service
	svc, err := h.store.GetService(serviceID)
	if err != nil || svc == nil {
		http.Error(w, "Service not found", http.StatusNotFound)
		return
	}

	ctx := &ServiceContext{
		Service: svc,
	}

	// Get team if assigned
	if svc.TeamID != "" {
		team, _ := h.store.GetTeam(svc.TeamID)
		ctx.Team = team
	}

	// Get dependencies
	ctx.Upstream, ctx.Downstream, _ = h.store.GetDependencies(serviceID)

	// Get runbooks
	ctx.Runbooks, _ = h.store.GetRunbooksForService(serviceID)

	// Get recent incidents if store available
	if h.incidentStore != nil {
		ctx.Incidents, _ = h.incidentStore.ListIncidentsByService(serviceID, 10)
	}

	// Get synthetic checks if store available
	if h.syntheticsStore != nil {
		ctx.SynthChecks, _ = h.syntheticsStore.ListChecksByService(serviceID)
	}

	respondJSON(w, ctx)
}

// handleTeams handles GET/POST for teams list
func (h *CatalogHandlers) handleTeams(w http.ResponseWriter, r *http.Request) {
	orgID := r.Header.Get("X-Org-ID")
	if orgID == "" {
		orgID = "default"
	}

	switch r.Method {
	case http.MethodGet:
		teams, err := h.store.ListTeams(orgID)
		if err != nil {
			http.Error(w, "Failed to list teams", http.StatusInternalServerError)
			return
		}
		respondJSON(w, teams)

	case http.MethodPost:
		var team catalog.Team
		if err := json.NewDecoder(r.Body).Decode(&team); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		team.ID = fmt.Sprintf("team_%d", time.Now().UnixNano())
		team.OrgID = orgID

		if err := h.store.CreateTeam(&team); err != nil {
			http.Error(w, "Failed to create team", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
		respondJSON(w, team)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleTeam handles GET/PUT/DELETE for single team
func (h *CatalogHandlers) handleTeam(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/catalog/teams/")
	if id == "" {
		http.Error(w, "Team ID required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		team, err := h.store.GetTeam(id)
		if err != nil {
			http.Error(w, "Team not found", http.StatusNotFound)
			return
		}
		respondJSON(w, team)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleDependencies handles POST for adding dependencies
func (h *CatalogHandlers) handleDependencies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var dep catalog.Dependency
	if err := json.NewDecoder(r.Body).Decode(&dep); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	dep.ID = fmt.Sprintf("dep_%d", time.Now().UnixNano())

	if err := h.store.AddDependency(&dep); err != nil {
		http.Error(w, "Failed to add dependency", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	respondJSON(w, dep)
}

// handleServiceGraph returns the full service dependency graph
func (h *CatalogHandlers) handleServiceGraph(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	orgID := r.Header.Get("X-Org-ID")
	if orgID == "" {
		orgID = "default"
	}

	graph, err := h.store.GetServiceGraph(orgID)
	if err != nil {
		http.Error(w, "Failed to get service graph", http.StatusInternalServerError)
		return
	}

	respondJSON(w, graph)
}

// handleRunbooks handles GET/POST for runbooks list
func (h *CatalogHandlers) handleRunbooks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var rb catalog.Runbook
		if err := json.NewDecoder(r.Body).Decode(&rb); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		rb.ID = fmt.Sprintf("rb_%d", time.Now().UnixNano())

		if err := h.store.CreateRunbook(&rb); err != nil {
			http.Error(w, "Failed to create runbook", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
		respondJSON(w, rb)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleRunbook handles GET/PUT/DELETE for single runbook
func (h *CatalogHandlers) handleRunbook(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/catalog/runbooks/")
	if id == "" {
		http.Error(w, "Runbook ID required", http.StatusBadRequest)
		return
	}

	// Handle search subpath
	if id == "search" {
		h.handleRunbookSearch(w, r)
		return
	}

	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

// handleRunbookSearch searches for runbooks matching alert tags
func (h *CatalogHandlers) handleRunbookSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	orgID := r.Header.Get("X-Org-ID")
	if orgID == "" {
		orgID = "default"
	}

	tagsParam := r.URL.Query().Get("tags")
	if tagsParam == "" {
		http.Error(w, "tags parameter required", http.StatusBadRequest)
		return
	}

	tags := strings.Split(tagsParam, ",")

	runbooks, err := h.store.FindRunbookByAlertTags(orgID, tags)
	if err != nil {
		http.Error(w, "Failed to search runbooks", http.StatusInternalServerError)
		return
	}

	respondJSON(w, runbooks)
}

// respondJSON writes a JSON response
func respondJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}
