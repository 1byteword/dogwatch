package entity

import (
	"sort"
)

// Graph represents the entity relationship graph
type Graph struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

// GraphNode represents a node in the graph
type GraphNode struct {
	ID          string            `json:"id"`
	Type        Type              `json:"type"`
	Name        string            `json:"name"`
	DisplayName string            `json:"display_name"`
	Health      HealthStatus      `json:"health"`
	Signals     GoldenSignals     `json:"signals"`
	Tags        map[string]string `json:"tags,omitempty"`

	// Layout hints
	Group    string  `json:"group,omitempty"` // For grouping in visualization
	Size     float64 `json:"size"`            // Node size based on importance
	X        float64 `json:"x,omitempty"`     // Optional position
	Y        float64 `json:"y,omitempty"`
}

// GraphEdge represents an edge (relationship) in the graph
type GraphEdge struct {
	ID         string            `json:"id"`
	Source     string            `json:"source"`
	Target     string            `json:"target"`
	Type       RelationType      `json:"type"`
	Label      string            `json:"label,omitempty"`
	Weight     float64           `json:"weight"`    // Edge weight (e.g., call frequency)
	ErrorRate  float64           `json:"error_rate,omitempty"`
	AvgLatency float64           `json:"avg_latency,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// GetGraph returns the full entity relationship graph
func (s *Synthesizer) GetGraph() *Graph {
	s.mu.RLock()
	defer s.mu.RUnlock()

	graph := &Graph{
		Nodes: make([]GraphNode, 0, len(s.entities)),
		Edges: make([]GraphEdge, 0),
	}

	edgeSet := make(map[string]bool) // Track unique edges

	// Build nodes
	for _, e := range s.entities {
		node := GraphNode{
			ID:          e.ID,
			Type:        e.Type,
			Name:        e.Name,
			DisplayName: e.DisplayName,
			Health:      e.Health,
			Signals:     e.GetSignals(),
			Tags:        e.Tags,
			Group:       string(e.Type),
			Size:        calculateNodeSize(e),
		}
		graph.Nodes = append(graph.Nodes, node)

		// Build edges from relationships
		for _, rel := range e.Relationships {
			edgeID := e.ID + "->" + rel.TargetID + ":" + string(rel.Type)
			if edgeSet[edgeID] {
				continue
			}
			edgeSet[edgeID] = true

			edge := GraphEdge{
				ID:         edgeID,
				Source:     e.ID,
				Target:     rel.TargetID,
				Type:       rel.Type,
				Label:      string(rel.Type),
				Weight:     float64(rel.CallCount),
				ErrorRate:  float64(rel.ErrorCount) / float64(max(rel.CallCount, 1)) * 100,
				AvgLatency: rel.AvgLatency,
				Metadata:   rel.Metadata,
			}
			graph.Edges = append(graph.Edges, edge)
		}
	}

	// Sort nodes by type and name for consistent ordering
	sort.Slice(graph.Nodes, func(i, j int) bool {
		if graph.Nodes[i].Type != graph.Nodes[j].Type {
			return graph.Nodes[i].Type < graph.Nodes[j].Type
		}
		return graph.Nodes[i].Name < graph.Nodes[j].Name
	})

	return graph
}

// GetSubgraph returns a subgraph centered on an entity with specified depth
func (s *Synthesizer) GetSubgraph(entityID string, depth int) *Graph {
	if depth <= 0 {
		depth = 2
	}
	if depth > 5 {
		depth = 5 // Limit max depth
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	// Find entities within the specified depth
	visited := make(map[string]bool)
	toVisit := []string{entityID}
	visited[entityID] = true

	for d := 0; d < depth && len(toVisit) > 0; d++ {
		nextVisit := []string{}

		for _, id := range toVisit {
			entity, exists := s.entities[id]
			if !exists {
				continue
			}

			// Add outgoing relationships
			for _, rel := range entity.Relationships {
				if !visited[rel.TargetID] {
					visited[rel.TargetID] = true
					nextVisit = append(nextVisit, rel.TargetID)
				}
			}

			// Add incoming relationships (entities that point to this one)
			for otherId, other := range s.entities {
				if visited[otherId] {
					continue
				}
				for _, rel := range other.Relationships {
					if rel.TargetID == id && !visited[otherId] {
						visited[otherId] = true
						nextVisit = append(nextVisit, otherId)
						break
					}
				}
			}
		}

		toVisit = nextVisit
	}

	// Build subgraph from visited entities
	graph := &Graph{
		Nodes: make([]GraphNode, 0),
		Edges: make([]GraphEdge, 0),
	}

	edgeSet := make(map[string]bool)

	for id := range visited {
		entity, exists := s.entities[id]
		if !exists {
			continue
		}

		node := GraphNode{
			ID:          entity.ID,
			Type:        entity.Type,
			Name:        entity.Name,
			DisplayName: entity.DisplayName,
			Health:      entity.Health,
			Signals:     entity.GetSignals(),
			Tags:        entity.Tags,
			Group:       string(entity.Type),
			Size:        calculateNodeSize(entity),
		}
		graph.Nodes = append(graph.Nodes, node)

		// Add edges only between visited nodes
		for _, rel := range entity.Relationships {
			if !visited[rel.TargetID] {
				continue
			}

			edgeID := entity.ID + "->" + rel.TargetID + ":" + string(rel.Type)
			if edgeSet[edgeID] {
				continue
			}
			edgeSet[edgeID] = true

			edge := GraphEdge{
				ID:         edgeID,
				Source:     entity.ID,
				Target:     rel.TargetID,
				Type:       rel.Type,
				Label:      string(rel.Type),
				Weight:     float64(rel.CallCount),
				ErrorRate:  float64(rel.ErrorCount) / float64(max(rel.CallCount, 1)) * 100,
				AvgLatency: rel.AvgLatency,
			}
			graph.Edges = append(graph.Edges, edge)
		}
	}

	return graph
}

// GetUpstream returns entities that call/connect to this entity
func (s *Synthesizer) GetUpstream(entityID string) []*Entity {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var upstream []*Entity

	for _, e := range s.entities {
		for _, rel := range e.Relationships {
			if rel.TargetID == entityID {
				upstream = append(upstream, e)
				break
			}
		}
	}

	return upstream
}

// GetDownstream returns entities that this entity calls/connects to
func (s *Synthesizer) GetDownstream(entityID string) []*Entity {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entity, exists := s.entities[entityID]
	if !exists {
		return nil
	}

	var downstream []*Entity
	for _, rel := range entity.Relationships {
		if target, exists := s.entities[rel.TargetID]; exists {
			downstream = append(downstream, target)
		}
	}

	return downstream
}

// GetDependencyChain returns the chain of dependencies from source to target
func (s *Synthesizer) GetDependencyChain(sourceID, targetID string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// BFS to find shortest path
	if sourceID == targetID {
		return []string{sourceID}
	}

	visited := make(map[string]bool)
	parent := make(map[string]string)
	queue := []string{sourceID}
	visited[sourceID] = true

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		entity, exists := s.entities[current]
		if !exists {
			continue
		}

		for _, rel := range entity.Relationships {
			if visited[rel.TargetID] {
				continue
			}

			visited[rel.TargetID] = true
			parent[rel.TargetID] = current
			queue = append(queue, rel.TargetID)

			if rel.TargetID == targetID {
				// Reconstruct path
				path := []string{targetID}
				for node := targetID; node != sourceID; {
					node = parent[node]
					path = append([]string{node}, path...)
				}
				return path
			}
		}
	}

	return nil // No path found
}

// calculateNodeSize determines node size based on entity importance
func calculateNodeSize(e *Entity) float64 {
	// Base size
	size := 1.0

	// Size by type
	switch e.Type {
	case TypeService:
		size = 2.0
	case TypeDatabase:
		size = 1.8
	case TypeQueue:
		size = 1.5
	case TypeHost:
		size = 1.5
	case TypeContainer:
		size = 1.0
	case TypeExternalAPI:
		size = 1.3
	}

	// Increase size based on throughput
	if e.Signals.Throughput > 1000 {
		size *= 1.5
	} else if e.Signals.Throughput > 100 {
		size *= 1.2
	}

	// Increase size based on relationships
	relCount := len(e.Relationships)
	if relCount > 10 {
		size *= 1.5
	} else if relCount > 5 {
		size *= 1.2
	}

	return size
}

func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
