package knowledge

import (
	"sync"
	"time"
)

// Registry provides in-memory caching of knowledge objects for query expansion
type Registry struct {
	store   *Store
	mu      sync.RWMutex
	macros  map[string]*KnowledgeObject
	lookups map[string]*KnowledgeObject
	fields  map[string]*KnowledgeObject
	lastLoad time.Time
	cacheTTL time.Duration
}

// NewRegistry creates a new knowledge registry
func NewRegistry(store *Store) *Registry {
	r := &Registry{
		store:    store,
		macros:   make(map[string]*KnowledgeObject),
		lookups:  make(map[string]*KnowledgeObject),
		fields:   make(map[string]*KnowledgeObject),
		cacheTTL: 5 * time.Minute,
	}
	r.Reload()
	return r
}

// Reload refreshes the cache from the store
func (r *Registry) Reload() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Load macros
	macros, err := r.store.GetAllMacros()
	if err != nil {
		return err
	}
	r.macros = make(map[string]*KnowledgeObject)
	for _, m := range macros {
		r.macros[m.Name] = m
	}

	// Load field extractions
	fields, err := r.store.GetAllFieldExtractions()
	if err != nil {
		return err
	}
	r.fields = make(map[string]*KnowledgeObject)
	for _, f := range fields {
		r.fields[f.Name] = f
	}

	// Load lookups
	lookups, err := r.store.GetAllLookups()
	if err != nil {
		return err
	}
	r.lookups = make(map[string]*KnowledgeObject)
	for _, l := range lookups {
		r.lookups[l.Name] = l
	}

	r.lastLoad = time.Now()
	return nil
}

// checkCache ensures the cache is fresh
func (r *Registry) checkCache() {
	r.mu.RLock()
	needsReload := time.Since(r.lastLoad) > r.cacheTTL
	r.mu.RUnlock()

	if needsReload {
		r.Reload()
	}
}

// GetMacro returns a macro by name
func (r *Registry) GetMacro(name string) (*KnowledgeObject, error) {
	r.checkCache()
	r.mu.RLock()
	defer r.mu.RUnlock()

	obj, ok := r.macros[name]
	if !ok {
		return nil, ErrMacroNotFound
	}
	return obj, nil
}

// GetFieldExtraction returns a field extraction by name
func (r *Registry) GetFieldExtraction(name string) (*KnowledgeObject, error) {
	r.checkCache()
	r.mu.RLock()
	defer r.mu.RUnlock()

	obj, ok := r.fields[name]
	if !ok {
		return nil, ErrNotFound
	}
	return obj, nil
}

// GetLookup returns a lookup by name
func (r *Registry) GetLookup(name string) (*KnowledgeObject, error) {
	r.checkCache()
	r.mu.RLock()
	defer r.mu.RUnlock()

	obj, ok := r.lookups[name]
	if !ok {
		return nil, ErrNotFound
	}
	return obj, nil
}

// ListMacros returns all macros
func (r *Registry) ListMacros() []*KnowledgeObject {
	r.checkCache()
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*KnowledgeObject, 0, len(r.macros))
	for _, m := range r.macros {
		result = append(result, m)
	}
	return result
}

// ListFieldExtractions returns all field extractions
func (r *Registry) ListFieldExtractions() []*KnowledgeObject {
	r.checkCache()
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*KnowledgeObject, 0, len(r.fields))
	for _, f := range r.fields {
		result = append(result, f)
	}
	return result
}

// ListLookups returns all lookups
func (r *Registry) ListLookups() []*KnowledgeObject {
	r.checkCache()
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*KnowledgeObject, 0, len(r.lookups))
	for _, l := range r.lookups {
		result = append(result, l)
	}
	return result
}

// InvalidateCache forces a reload on next access
func (r *Registry) InvalidateCache() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastLoad = time.Time{}
}

// SetCacheTTL sets the cache time-to-live
func (r *Registry) SetCacheTTL(ttl time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cacheTTL = ttl
}
