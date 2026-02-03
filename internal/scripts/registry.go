package scripts

import (
	"sort"
	"strings"
	"sync"
)

// Registry holds all available scripts
type Registry struct {
	mu      sync.RWMutex
	scripts map[string]*Script // keyed by "category/name"
}

// NewRegistry creates a new script registry
func NewRegistry() *Registry {
	return &Registry{
		scripts: make(map[string]*Script),
	}
}

// Register adds a script to the registry
func (r *Registry) Register(s *Script) {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := s.Name
	if !strings.Contains(key, "/") {
		key = s.Category + "/" + s.Name
	}
	r.scripts[key] = s
}

// Get retrieves a script by name (with or without category prefix)
func (r *Registry) Get(name string) *Script {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Try exact match first
	if s, ok := r.scripts[name]; ok {
		return s
	}

	// Try without category prefix
	for key, s := range r.scripts {
		parts := strings.SplitN(key, "/", 2)
		if len(parts) == 2 && parts[1] == name {
			return s
		}
	}

	return nil
}

// List returns all scripts, optionally filtered by category
func (r *Registry) List(category string) []*Script {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*Script
	for _, s := range r.scripts {
		if category == "" || s.Category == category {
			result = append(result, s)
		}
	}

	// Sort by name for consistent output
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result
}

// Categories returns all unique categories with metadata
func (r *Registry) Categories() []CategoryInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	counts := make(map[string]int)
	for _, s := range r.scripts {
		counts[s.Category]++
	}

	categoryMeta := map[string]struct {
		title string
		desc  string
	}{
		"http":     {"HTTP Analysis", "Analyze HTTP traffic, latency, and error patterns"},
		"mysql":    {"MySQL Analysis", "MySQL database performance and query analysis"},
		"postgres": {"PostgreSQL Analysis", "PostgreSQL database performance and query analysis"},
		"redis":    {"Redis Analysis", "Redis performance and command analysis"},
		"k8s":      {"Kubernetes Analysis", "Kubernetes cluster and workload analysis"},
		"security": {"Security Analysis", "Security-focused analysis and threat detection"},
		"service":  {"Service Analysis", "Service-level metrics and dependency analysis"},
		"trace":    {"Trace Analysis", "Distributed tracing analysis"},
		"log":      {"Log Analysis", "Log pattern and error analysis"},
	}

	var result []CategoryInfo
	for cat, count := range counts {
		info := CategoryInfo{
			Name:  cat,
			Count: count,
		}
		if meta, ok := categoryMeta[cat]; ok {
			info.Title = meta.title
			info.Description = meta.desc
		} else {
			info.Title = strings.Title(cat)
			info.Description = cat + " analysis scripts"
		}
		result = append(result, info)
	}

	// Sort by name
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result
}

// DefaultRegistry is the global script registry with built-in scripts
var DefaultRegistry = NewRegistry()

func init() {
	// Register all built-in scripts
	for _, s := range BuiltinScripts {
		DefaultRegistry.Register(s)
	}
}
