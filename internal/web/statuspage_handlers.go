package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"dogwatch/internal/statuspage"
)

// StatusPageHandlers provides HTTP handlers for status pages
type StatusPageHandlers struct {
	store *statuspage.Store
}

// NewStatusPageHandlers creates status page handlers
func NewStatusPageHandlers(store *statuspage.Store) *StatusPageHandlers {
	return &StatusPageHandlers{store: store}
}

// RegisterRoutes registers status page routes
func (h *StatusPageHandlers) RegisterRoutes(mux *http.ServeMux) {
	// Status page management (authenticated)
	mux.HandleFunc("/api/statuspages", h.handleStatusPages)
	mux.HandleFunc("/api/statuspages/", h.handleStatusPage)

	// Component management
	mux.HandleFunc("/api/statuspage/components", h.handleComponents)
	mux.HandleFunc("/api/statuspage/components/", h.handleComponent)

	// Incident management
	mux.HandleFunc("/api/statuspage/incidents", h.handleIncidents)
	mux.HandleFunc("/api/statuspage/incidents/", h.handleIncident)

	// Public status page endpoint
	mux.HandleFunc("/status/", h.handlePublicStatusPage)
}

// handleStatusPages handles GET/POST for status pages list
func (h *StatusPageHandlers) handleStatusPages(w http.ResponseWriter, r *http.Request) {
	orgID := r.Header.Get("X-Org-ID")
	if orgID == "" {
		orgID = "default"
	}

	switch r.Method {
	case http.MethodGet:
		pages, err := h.store.ListStatusPages(orgID)
		if err != nil {
			http.Error(w, "Failed to list status pages", http.StatusInternalServerError)
			return
		}
		writeJSON(w, pages)

	case http.MethodPost:
		var page statuspage.StatusPage
		if err := json.NewDecoder(r.Body).Decode(&page); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		page.ID = fmt.Sprintf("sp_%d", time.Now().UnixNano())
		page.OrgID = orgID

		// Generate slug from name if not provided
		if page.Slug == "" {
			page.Slug = generateSlug(page.Name)
		}

		if err := h.store.CreateStatusPage(&page); err != nil {
			http.Error(w, "Failed to create status page", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
		writeJSON(w, page)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleStatusPage handles GET/PUT/DELETE for single status page
func (h *StatusPageHandlers) handleStatusPage(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/statuspages/")
	if id == "" {
		http.Error(w, "Status page ID required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		page, err := h.store.GetStatusPage(id)
		if err != nil {
			http.Error(w, "Status page not found", http.StatusNotFound)
			return
		}
		writeJSON(w, page)

	case http.MethodPut:
		var page statuspage.StatusPage
		if err := json.NewDecoder(r.Body).Decode(&page); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		page.ID = id

		if err := h.store.UpdateStatusPage(&page); err != nil {
			http.Error(w, "Failed to update status page", http.StatusInternalServerError)
			return
		}
		writeJSON(w, page)

	case http.MethodDelete:
		if err := h.store.DeleteStatusPage(id); err != nil {
			http.Error(w, "Failed to delete status page", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleComponents handles GET/POST for components list
func (h *StatusPageHandlers) handleComponents(w http.ResponseWriter, r *http.Request) {
	orgID := r.Header.Get("X-Org-ID")
	if orgID == "" {
		orgID = "default"
	}

	switch r.Method {
	case http.MethodGet:
		components, err := h.store.ListComponents(orgID)
		if err != nil {
			http.Error(w, "Failed to list components", http.StatusInternalServerError)
			return
		}
		writeJSON(w, components)

	case http.MethodPost:
		var comp statuspage.Component
		if err := json.NewDecoder(r.Body).Decode(&comp); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		comp.ID = fmt.Sprintf("comp_%d", time.Now().UnixNano())
		comp.OrgID = orgID
		comp.Enabled = true
		comp.Public = true

		if err := h.store.CreateComponent(&comp); err != nil {
			http.Error(w, "Failed to create component", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
		writeJSON(w, comp)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleComponent handles GET/PUT/DELETE for single component
func (h *StatusPageHandlers) handleComponent(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/statuspage/components/")

	// Handle status update subpath
	if strings.HasSuffix(id, "/status") {
		h.handleComponentStatus(w, r, strings.TrimSuffix(id, "/status"))
		return
	}

	if id == "" {
		http.Error(w, "Component ID required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		comp, err := h.store.GetComponent(id)
		if err != nil {
			http.Error(w, "Component not found", http.StatusNotFound)
			return
		}
		writeJSON(w, comp)

	case http.MethodPut:
		var comp statuspage.Component
		if err := json.NewDecoder(r.Body).Decode(&comp); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		comp.ID = id

		if err := h.store.UpdateComponent(&comp); err != nil {
			http.Error(w, "Failed to update component", http.StatusInternalServerError)
			return
		}
		writeJSON(w, comp)

	case http.MethodDelete:
		if err := h.store.DeleteComponent(id); err != nil {
			http.Error(w, "Failed to delete component", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleComponentStatus handles POST to update component status
func (h *StatusPageHandlers) handleComponentStatus(w http.ResponseWriter, r *http.Request, componentID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Status       statuspage.ServiceStatus `json:"status"`
		ResponseTime float64                  `json:"response_time_ms"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.store.RecordStatus(componentID, req.Status, req.ResponseTime); err != nil {
		http.Error(w, "Failed to record status", http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{"status": "recorded"})
}

// handleIncidents handles GET/POST for incidents list
func (h *StatusPageHandlers) handleIncidents(w http.ResponseWriter, r *http.Request) {
	orgID := r.Header.Get("X-Org-ID")
	if orgID == "" {
		orgID = "default"
	}

	switch r.Method {
	case http.MethodGet:
		includeResolved := r.URL.Query().Get("include_resolved") == "true"
		incidents, err := h.store.ListIncidents(orgID, 50, includeResolved)
		if err != nil {
			http.Error(w, "Failed to list incidents", http.StatusInternalServerError)
			return
		}
		writeJSON(w, incidents)

	case http.MethodPost:
		var inc statuspage.StatusIncident
		if err := json.NewDecoder(r.Body).Decode(&inc); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		inc.ID = fmt.Sprintf("sinc_%d", time.Now().UnixNano())
		inc.OrgID = orgID
		if inc.Status == "" {
			inc.Status = statuspage.IncidentInvestigating
		}

		// Add initial update
		inc.Updates = []statuspage.IncidentUpdate{
			{
				ID:        fmt.Sprintf("upd_%d", time.Now().UnixNano()),
				Status:    inc.Status,
				Message:   inc.Message,
				CreatedAt: time.Now(),
			},
		}

		if err := h.store.CreateIncident(&inc); err != nil {
			http.Error(w, "Failed to create incident", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
		writeJSON(w, inc)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleIncident handles GET/PUT/DELETE for single incident
func (h *StatusPageHandlers) handleIncident(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/statuspage/incidents/")

	// Handle update subpath
	if strings.Contains(path, "/update") {
		parts := strings.Split(path, "/")
		if len(parts) >= 2 {
			h.handleIncidentUpdate(w, r, parts[0])
			return
		}
	}

	id := path
	if id == "" {
		http.Error(w, "Incident ID required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		inc, err := h.store.GetIncident(id)
		if err != nil {
			http.Error(w, "Incident not found", http.StatusNotFound)
			return
		}
		writeJSON(w, inc)

	case http.MethodPut:
		var inc statuspage.StatusIncident
		if err := json.NewDecoder(r.Body).Decode(&inc); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		inc.ID = id

		if err := h.store.UpdateIncident(&inc); err != nil {
			http.Error(w, "Failed to update incident", http.StatusInternalServerError)
			return
		}
		writeJSON(w, inc)

	case http.MethodDelete:
		http.Error(w, "Incidents cannot be deleted", http.StatusMethodNotAllowed)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleIncidentUpdate handles POST to add an incident update
func (h *StatusPageHandlers) handleIncidentUpdate(w http.ResponseWriter, r *http.Request, incidentID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Status  statuspage.IncidentStatus `json:"status"`
		Message string                    `json:"message"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Get existing incident
	inc, err := h.store.GetIncident(incidentID)
	if err != nil {
		http.Error(w, "Incident not found", http.StatusNotFound)
		return
	}

	// Add update
	update := statuspage.IncidentUpdate{
		ID:        fmt.Sprintf("upd_%d", time.Now().UnixNano()),
		Status:    req.Status,
		Message:   req.Message,
		CreatedAt: time.Now(),
	}
	inc.Updates = append(inc.Updates, update)
	inc.Status = req.Status

	// Mark resolved if applicable
	if req.Status == statuspage.IncidentResolved {
		now := time.Now()
		inc.ResolvedAt = &now
	}

	if err := h.store.UpdateIncident(inc); err != nil {
		http.Error(w, "Failed to update incident", http.StatusInternalServerError)
		return
	}

	writeJSON(w, inc)
}

// handlePublicStatusPage serves the public status page
func (h *StatusPageHandlers) handlePublicStatusPage(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimPrefix(r.URL.Path, "/status/")
	if slug == "" {
		http.Error(w, "Status page slug required", http.StatusBadRequest)
		return
	}

	// Handle API endpoint for status page data
	if strings.HasSuffix(slug, "/api") {
		h.handlePublicStatusPageAPI(w, r, strings.TrimSuffix(slug, "/api"))
		return
	}

	// Serve the status page HTML
	page, err := h.store.GetStatusPageBySlug(slug)
	if err != nil {
		http.Error(w, "Status page not found", http.StatusNotFound)
		return
	}

	if !page.Public {
		http.Error(w, "Status page not found", http.StatusNotFound)
		return
	}

	// Return status page data as JSON for SPA rendering
	h.handlePublicStatusPageAPI(w, r, slug)
}

// handlePublicStatusPageAPI returns status page data as JSON
func (h *StatusPageHandlers) handlePublicStatusPageAPI(w http.ResponseWriter, r *http.Request, slug string) {
	page, err := h.store.GetStatusPageBySlug(slug)
	if err != nil {
		http.Error(w, "Status page not found", http.StatusNotFound)
		return
	}

	if !page.Public {
		http.Error(w, "Status page not found", http.StatusNotFound)
		return
	}

	// Get components
	var components []*statuspage.Component
	for _, compID := range page.ComponentIDs {
		comp, err := h.store.GetComponent(compID)
		if err == nil && comp.Public {
			components = append(components, comp)
		}
	}

	// Get active incidents
	incidents, _ := h.store.ListIncidents(page.OrgID, 10, false)

	// Calculate overall status
	overallStatus := h.store.GetOverallStatus(page.OrgID)

	response := map[string]interface{}{
		"page":           page,
		"components":     components,
		"incidents":      incidents,
		"overall_status": overallStatus,
		"updated_at":     time.Now(),
	}

	writeJSON(w, response)
}

// generateSlug generates a URL-friendly slug from a name
func generateSlug(name string) string {
	slug := strings.ToLower(name)
	slug = strings.ReplaceAll(slug, " ", "-")
	// Remove any non-alphanumeric characters except hyphens
	var result strings.Builder
	for _, c := range slug {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			result.WriteRune(c)
		}
	}
	return result.String()
}

// writeJSON is a helper to write JSON response
func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}
