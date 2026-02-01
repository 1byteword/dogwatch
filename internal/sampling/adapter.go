package sampling

import (
	"dogwatch/internal/trace"
)

// OTLPSamplerAdapter adapts the sampling.Manager to the otlp.SpanSampler interface
type OTLPSamplerAdapter struct {
	manager *Manager
}

// NewOTLPSamplerAdapter creates a new adapter
func NewOTLPSamplerAdapter(manager *Manager) *OTLPSamplerAdapter {
	return &OTLPSamplerAdapter{manager: manager}
}

// ShouldSample implements the otlp.SpanSampler interface
// Returns true if the span should be kept
func (a *OTLPSamplerAdapter) ShouldSample(span *trace.Span) bool {
	if a.manager == nil {
		return true // Keep if no manager
	}

	decision := a.manager.ProcessSpan(span)

	// Keep if decision is Keep or Defer (tail sampling handles deferred)
	return decision == DecisionKeep || decision == DecisionDefer
}
