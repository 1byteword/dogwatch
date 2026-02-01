package web

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"dogwatch/internal/knowledge"
)

// KnowledgeHandlers provides HTTP handlers for knowledge objects
type KnowledgeHandlers struct {
	store    *knowledge.Store
	registry *knowledge.Registry
	expander *knowledge.Expander
}

// NewKnowledgeHandlers creates knowledge handlers
func NewKnowledgeHandlers(store *knowledge.Store) *KnowledgeHandlers {
	registry := knowledge.NewRegistry(store)
	return &KnowledgeHandlers{
		store:    store,
		registry: registry,
		expander: knowledge.NewExpander(registry),
	}
}

// GetExpander returns the expander for query integration
func (h *KnowledgeHandlers) GetExpander() *knowledge.Expander {
	return h.expander
}

// GetRegistry returns the registry for direct access
func (h *KnowledgeHandlers) GetRegistry() *knowledge.Registry {
	return h.registry
}

// RegisterRoutes registers knowledge object routes
func (h *KnowledgeHandlers) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/knowledge", h.handleKnowledge)
	mux.HandleFunc("/api/knowledge/", h.handleKnowledgeItem)
	mux.HandleFunc("/api/knowledge/validate", h.handleValidate)
	mux.HandleFunc("/api/knowledge/search", h.handleSearch)
	mux.HandleFunc("/api/knowledge/expand", h.handleExpand)
}

// handleKnowledge handles GET (list) and POST (create) for knowledge objects
func (h *KnowledgeHandlers) handleKnowledge(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listKnowledge(w, r)
	case http.MethodPost:
		h.createKnowledge(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// listKnowledge lists all knowledge objects with optional filters
func (h *KnowledgeHandlers) listKnowledge(w http.ResponseWriter, r *http.Request) {
	filters := knowledge.ListFilters{}

	// Parse type filter
	if typeFilter := r.URL.Query().Get("type"); typeFilter != "" {
		filters.Type = knowledge.ObjectType(typeFilter)
	}

	// Parse owner filter
	if owner := r.URL.Query().Get("owner"); owner != "" {
		filters.Owner = owner
	}

	// Parse shared filter
	if shared := r.URL.Query().Get("shared"); shared != "" {
		val := shared == "true"
		filters.Shared = &val
	}

	// Parse tags filter
	if tags := r.URL.Query().Get("tags"); tags != "" {
		filters.Tags = strings.Split(tags, ",")
	}

	objects, err := h.store.List(filters)
	if err != nil {
		log.Printf("[knowledge] Failed to list objects: %v", err)
		http.Error(w, "Failed to list knowledge objects", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(objects)
}

// createKnowledge creates a new knowledge object
func (h *KnowledgeHandlers) createKnowledge(w http.ResponseWriter, r *http.Request) {
	var obj knowledge.KnowledgeObject
	if err := json.NewDecoder(r.Body).Decode(&obj); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if obj.Name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}
	if obj.Type == "" {
		http.Error(w, "Type is required", http.StatusBadRequest)
		return
	}
	if obj.Definition == "" {
		http.Error(w, "Definition is required", http.StatusBadRequest)
		return
	}

	// Validate the definition based on type
	validationResult := h.validateDefinition(&obj)
	if !validationResult.Valid {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":      "Invalid definition",
			"validation": validationResult,
		})
		return
	}

	if err := h.store.Create(&obj); err != nil {
		if err == knowledge.ErrDuplicateName {
			http.Error(w, "A knowledge object with this name already exists", http.StatusConflict)
			return
		}
		log.Printf("[knowledge] Failed to create object: %v", err)
		http.Error(w, "Failed to create knowledge object", http.StatusInternalServerError)
		return
	}

	// Invalidate cache
	h.registry.InvalidateCache()

	log.Printf("[knowledge] Created object: %s (%s) type=%s", obj.Name, obj.ID, obj.Type)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(obj)
}

// handleKnowledgeItem handles operations on a single knowledge object
func (h *KnowledgeHandlers) handleKnowledgeItem(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/knowledge/")

	// Handle special endpoints
	if path == "validate" {
		h.handleValidate(w, r)
		return
	}
	if path == "search" {
		h.handleSearch(w, r)
		return
	}
	if path == "expand" {
		h.handleExpand(w, r)
		return
	}

	id := path
	if id == "" {
		http.Error(w, "ID required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.getKnowledge(w, r, id)
	case http.MethodPut:
		h.updateKnowledge(w, r, id)
	case http.MethodDelete:
		h.deleteKnowledge(w, r, id)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// getKnowledge retrieves a single knowledge object
func (h *KnowledgeHandlers) getKnowledge(w http.ResponseWriter, r *http.Request, id string) {
	obj, err := h.store.Get(id)
	if err == knowledge.ErrNotFound {
		// Try by name
		obj, err = h.store.GetByName(id)
	}
	if err == knowledge.ErrNotFound {
		http.Error(w, "Knowledge object not found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("[knowledge] Failed to get object: %v", err)
		http.Error(w, "Failed to get knowledge object", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(obj)
}

// updateKnowledge updates a knowledge object
func (h *KnowledgeHandlers) updateKnowledge(w http.ResponseWriter, r *http.Request, id string) {
	// Get existing object
	existing, err := h.store.Get(id)
	if err == knowledge.ErrNotFound {
		http.Error(w, "Knowledge object not found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("[knowledge] Failed to get object: %v", err)
		http.Error(w, "Failed to get knowledge object", http.StatusInternalServerError)
		return
	}

	// Check if it's a system builtin
	if existing.Owner == "system" {
		http.Error(w, "Cannot modify built-in knowledge objects", http.StatusForbidden)
		return
	}

	var update knowledge.KnowledgeObject
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Preserve ID and update fields
	update.ID = id
	if update.Name == "" {
		update.Name = existing.Name
	}
	if update.Type == "" {
		update.Type = existing.Type
	}
	if update.Definition == "" {
		update.Definition = existing.Definition
	}

	// Validate the definition
	validationResult := h.validateDefinition(&update)
	if !validationResult.Valid {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":      "Invalid definition",
			"validation": validationResult,
		})
		return
	}

	if err := h.store.Update(&update); err != nil {
		if err == knowledge.ErrDuplicateName {
			http.Error(w, "A knowledge object with this name already exists", http.StatusConflict)
			return
		}
		log.Printf("[knowledge] Failed to update object: %v", err)
		http.Error(w, "Failed to update knowledge object", http.StatusInternalServerError)
		return
	}

	// Invalidate cache
	h.registry.InvalidateCache()

	log.Printf("[knowledge] Updated object: %s (%s)", update.Name, update.ID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(update)
}

// deleteKnowledge deletes a knowledge object
func (h *KnowledgeHandlers) deleteKnowledge(w http.ResponseWriter, r *http.Request, id string) {
	// Get existing to check ownership
	existing, err := h.store.Get(id)
	if err == knowledge.ErrNotFound {
		http.Error(w, "Knowledge object not found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("[knowledge] Failed to get object: %v", err)
		http.Error(w, "Failed to get knowledge object", http.StatusInternalServerError)
		return
	}

	// Check if it's a system builtin
	if existing.Owner == "system" {
		http.Error(w, "Cannot delete built-in knowledge objects", http.StatusForbidden)
		return
	}

	if err := h.store.Delete(id); err != nil {
		log.Printf("[knowledge] Failed to delete object: %v", err)
		http.Error(w, "Failed to delete knowledge object", http.StatusInternalServerError)
		return
	}

	// Invalidate cache
	h.registry.InvalidateCache()

	log.Printf("[knowledge] Deleted object: %s", id)

	w.WriteHeader(http.StatusNoContent)
}

// handleValidate validates a knowledge object definition
func (h *KnowledgeHandlers) handleValidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var obj knowledge.KnowledgeObject
	if err := json.NewDecoder(r.Body).Decode(&obj); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	result := h.validateDefinition(&obj)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// validateDefinition validates a knowledge object definition based on its type
func (h *KnowledgeHandlers) validateDefinition(obj *knowledge.KnowledgeObject) *knowledge.ValidationResult {
	switch obj.Type {
	case knowledge.TypeFieldExtraction:
		fe, err := obj.ParseFieldExtraction()
		if err != nil {
			return &knowledge.ValidationResult{
				Valid:  false,
				Errors: []string{"Invalid field extraction definition: " + err.Error()},
			}
		}
		return h.expander.ValidateFieldExtraction(fe)

	case knowledge.TypeMacro:
		m, err := obj.ParseMacro()
		if err != nil {
			return &knowledge.ValidationResult{
				Valid:  false,
				Errors: []string{"Invalid macro definition: " + err.Error()},
			}
		}
		return h.expander.ValidateMacro(m)

	case knowledge.TypeLookup:
		l, err := obj.ParseLookup()
		if err != nil {
			return &knowledge.ValidationResult{
				Valid:  false,
				Errors: []string{"Invalid lookup definition: " + err.Error()},
			}
		}
		return h.expander.ValidateLookup(l)

	case knowledge.TypeSavedSearch:
		_, err := obj.ParseSavedSearch()
		if err != nil {
			return &knowledge.ValidationResult{
				Valid:  false,
				Errors: []string{"Invalid saved search definition: " + err.Error()},
			}
		}
		return &knowledge.ValidationResult{Valid: true}

	default:
		return &knowledge.ValidationResult{
			Valid:  false,
			Errors: []string{"Unknown knowledge object type: " + string(obj.Type)},
		}
	}
}

// handleSearch searches knowledge objects by term
func (h *KnowledgeHandlers) handleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	q := r.URL.Query().Get("q")
	if q == "" {
		http.Error(w, "Search query required", http.StatusBadRequest)
		return
	}

	objects, err := h.store.Search(q)
	if err != nil {
		log.Printf("[knowledge] Search failed: %v", err)
		http.Error(w, "Search failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(objects)
}

// ExpandRequest is the request for macro expansion
type ExpandRequest struct {
	Query string `json:"query"`
}

// ExpandResponse is the response for macro expansion
type ExpandResponse struct {
	Original string `json:"original"`
	Expanded string `json:"expanded"`
	Error    string `json:"error,omitempty"`
}

// handleExpand expands macros in a query
func (h *KnowledgeHandlers) handleExpand(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ExpandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	expanded, err := h.expander.ExpandMacros(req.Query)

	resp := ExpandResponse{
		Original: req.Query,
		Expanded: expanded,
	}
	if err != nil {
		resp.Error = err.Error()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// Global knowledge handlers instance (set in main.go)
var knowledgeHandlers *KnowledgeHandlers

// SetKnowledgeHandlers sets the global knowledge handlers
func SetKnowledgeHandlers(h *KnowledgeHandlers) {
	knowledgeHandlers = h
}

// GetKnowledgeHandlers returns the global knowledge handlers
func GetKnowledgeHandlers() *KnowledgeHandlers {
	return knowledgeHandlers
}
