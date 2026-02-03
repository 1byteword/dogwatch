// Package profile provides profile-trace correlation capabilities.
package profile

import (
	"context"
	"strings"
	"time"

	"dogwatch/internal/trace"
)

// Linker correlates profile samples with distributed traces.
type Linker struct {
	profileStore *Store
	traceStore   *trace.Store
}

// NewLinker creates a new profile-trace linker.
func NewLinker(profileStore *Store, traceStore *trace.Store) *Linker {
	return &Linker{
		profileStore: profileStore,
		traceStore:   traceStore,
	}
}

// LinkResult represents the result of linking a profile sample to traces.
type LinkResult struct {
	Sample     *Sample             `json:"sample"`
	Links      []SpanLink          `json:"links"`
	TotalSpans int                 `json:"total_spans"`
}

// SpanLink represents a link between a profile sample and a trace span.
type SpanLink struct {
	TraceID     string    `json:"trace_id"`
	SpanID      string    `json:"span_id"`
	ServiceName string    `json:"service_name"`
	Name        string    `json:"name"`
	StartTime   time.Time `json:"start_time"`
	DurationMs  float64   `json:"duration_ms"`
	Confidence  float64   `json:"confidence"`
	MatchReason string    `json:"match_reason"`
}

// FunctionProfile represents aggregated profile data for a function.
type FunctionProfile struct {
	Function    string      `json:"function"`
	SampleCount uint64      `json:"sample_count"`
	Percent     float64     `json:"percent"`
	Traces      []SpanLink  `json:"traces"`
}

// LinkOptions configures the linking algorithm.
type LinkOptions struct {
	// TimeWindow expands the search window around sample timestamps
	TimeWindow time.Duration
	// MinConfidence filters out links below this threshold
	MinConfidence float64
	// MaxLinks limits the number of links returned per sample
	MaxLinks int
	// IncludeKernelFunctions includes kernel stack frames in matching
	IncludeKernelFunctions bool
}

// DefaultLinkOptions returns sensible defaults for linking.
func DefaultLinkOptions() LinkOptions {
	return LinkOptions{
		TimeWindow:             100 * time.Millisecond,
		MinConfidence:          0.3,
		MaxLinks:               10,
		IncludeKernelFunctions: false,
	}
}

// LinkSampleToTraces finds traces that correlate with a profile sample.
func (l *Linker) LinkSampleToTraces(ctx context.Context, sample *Sample, opts LinkOptions) (*LinkResult, error) {
	result := &LinkResult{
		Sample: sample,
		Links:  []SpanLink{},
	}

	// Calculate time window around the sample
	startTime := sample.Timestamp.Add(-opts.TimeWindow)
	endTime := sample.Timestamp.Add(opts.TimeWindow)

	// First, try to find spans by PID if we have process information
	var spans []trace.Span
	var err error

	if sample.PID > 0 {
		spans, err = l.traceStore.QuerySpansByPIDAndTime(sample.PID, startTime, endTime)
		if err != nil {
			return nil, err
		}
	}

	// If no spans found by PID, fall back to time-based search
	if len(spans) == 0 {
		spans, err = l.traceStore.QuerySpansByTimeRange(startTime, endTime)
		if err != nil {
			return nil, err
		}
	}

	result.TotalSpans = len(spans)

	// Score and filter matches
	for i := range spans {
		span := &spans[i]
		confidence, reason := l.calculateConfidence(sample, span, opts)
		if confidence >= opts.MinConfidence {
			link := SpanLink{
				TraceID:     span.TraceID,
				SpanID:      span.SpanID,
				ServiceName: span.ServiceName,
				Name:        span.Name,
				StartTime:   span.StartTime,
				DurationMs:  span.DurationMs,
				Confidence:  confidence,
				MatchReason: reason,
			}
			result.Links = append(result.Links, link)
		}
	}

	// Sort by confidence and limit results
	sortLinksByConfidence(result.Links)
	if len(result.Links) > opts.MaxLinks {
		result.Links = result.Links[:opts.MaxLinks]
	}

	// Store links in database
	for _, link := range result.Links {
		l.profileStore.RecordLink(&ProfileTraceLink{
			TraceID:      link.TraceID,
			SpanID:       link.SpanID,
			SampleID:     sample.ID,
			FunctionName: l.getPrimaryFunction(sample),
			Confidence:   link.Confidence,
		})
	}

	return result, nil
}

// calculateConfidence scores how likely a span is related to a profile sample.
func (l *Linker) calculateConfidence(sample *Sample, span *trace.Span, opts LinkOptions) (float64, string) {
	var confidence float64
	var reasons []string

	// Time overlap scoring (0.0 - 0.3)
	timeScore := l.calculateTimeOverlap(sample.Timestamp, span.StartTime, span.DurationMs, opts.TimeWindow)
	confidence += timeScore * 0.3
	if timeScore > 0.5 {
		reasons = append(reasons, "time_overlap")
	}

	// PID match scoring (0.0 - 0.4)
	if sample.PID > 0 && span.ProcessID > 0 && sample.PID == span.ProcessID {
		confidence += 0.4
		reasons = append(reasons, "pid_match")
	} else if sample.TGID > 0 && span.ProcessID > 0 && sample.TGID == span.ProcessID {
		confidence += 0.35
		reasons = append(reasons, "tgid_match")
	}

	// Function name matching (0.0 - 0.3)
	funcScore, funcMatch := l.calculateFunctionMatch(sample, span, opts)
	confidence += funcScore * 0.3
	if funcMatch != "" {
		reasons = append(reasons, "func:"+funcMatch)
	}

	// Comm/service name correlation
	if sample.Comm != "" && span.ServiceName != "" {
		if strings.Contains(strings.ToLower(span.ServiceName), strings.ToLower(sample.Comm)) ||
			strings.Contains(strings.ToLower(sample.Comm), strings.ToLower(span.ServiceName)) {
			confidence += 0.1
			reasons = append(reasons, "service_match")
		}
	}

	// Cap at 1.0
	if confidence > 1.0 {
		confidence = 1.0
	}

	reason := strings.Join(reasons, ",")
	if reason == "" {
		reason = "time_proximity"
	}

	return confidence, reason
}

// calculateTimeOverlap returns a score based on how close the sample is to the span.
func (l *Linker) calculateTimeOverlap(sampleTime, spanStart time.Time, spanDurationMs float64, window time.Duration) float64 {
	spanEnd := spanStart.Add(time.Duration(spanDurationMs * float64(time.Millisecond)))

	// Check if sample is within span duration
	if sampleTime.After(spanStart) && sampleTime.Before(spanEnd) {
		return 1.0
	}

	// Calculate distance to span
	var distance time.Duration
	if sampleTime.Before(spanStart) {
		distance = spanStart.Sub(sampleTime)
	} else {
		distance = sampleTime.Sub(spanEnd)
	}

	// Score based on proximity within window
	if distance > window {
		return 0.0
	}

	return 1.0 - float64(distance)/float64(window)
}

// calculateFunctionMatch checks if any stack functions match span operation.
func (l *Linker) calculateFunctionMatch(sample *Sample, span *trace.Span, opts LinkOptions) (float64, string) {
	operationLower := strings.ToLower(span.Name)

	// Check user stack functions
	for _, fn := range sample.UserStack {
		fnLower := strings.ToLower(fn)
		if strings.Contains(fnLower, operationLower) || strings.Contains(operationLower, fnLower) {
			return 1.0, fn
		}
		// Check for common patterns
		if matchesHTTPPattern(fn, span.Name) {
			return 0.8, fn
		}
		if matchesDBPattern(fn, span.Name) {
			return 0.8, fn
		}
	}

	// Check kernel stack if enabled
	if opts.IncludeKernelFunctions {
		for _, fn := range sample.KernelStack {
			fnLower := strings.ToLower(fn)
			if strings.Contains(fnLower, operationLower) {
				return 0.7, fn
			}
		}
	}

	return 0.0, ""
}

// matchesHTTPPattern checks if a function looks like HTTP handling.
func matchesHTTPPattern(fn, operation string) bool {
	httpKeywords := []string{"http", "handler", "serve", "request", "response", "route"}
	fnLower := strings.ToLower(fn)
	opLower := strings.ToLower(operation)

	for _, kw := range httpKeywords {
		if strings.Contains(fnLower, kw) && strings.Contains(opLower, kw) {
			return true
		}
	}
	// Check for HTTP methods
	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH"}
	for _, m := range methods {
		if strings.Contains(operation, m) && (strings.Contains(fnLower, "http") || strings.Contains(fnLower, "handler")) {
			return true
		}
	}
	return false
}

// matchesDBPattern checks if a function looks like database operation.
func matchesDBPattern(fn, operation string) bool {
	dbKeywords := []string{"sql", "query", "exec", "prepare", "mysql", "postgres", "redis", "mongo"}
	fnLower := strings.ToLower(fn)
	opLower := strings.ToLower(operation)

	for _, kw := range dbKeywords {
		if strings.Contains(fnLower, kw) && strings.Contains(opLower, kw) {
			return true
		}
	}
	return false
}

// getPrimaryFunction returns the most significant function from a sample's stack.
func (l *Linker) getPrimaryFunction(sample *Sample) string {
	// Prefer user stack top-of-stack (most specific)
	if len(sample.UserStack) > 0 {
		return sample.UserStack[0]
	}
	if len(sample.KernelStack) > 0 {
		return sample.KernelStack[0]
	}
	return ""
}

// sortLinksByConfidence sorts links in descending order of confidence.
func sortLinksByConfidence(links []SpanLink) {
	for i := 0; i < len(links); i++ {
		for j := i + 1; j < len(links); j++ {
			if links[j].Confidence > links[i].Confidence {
				links[i], links[j] = links[j], links[i]
			}
		}
	}
}

// GetTracesForHotspot finds traces related to a CPU hotspot function.
func (l *Linker) GetTracesForHotspot(ctx context.Context, functionName string, start, end time.Time, opts LinkOptions) (*FunctionProfile, error) {
	// Find samples containing this function
	samples, err := l.profileStore.QueryByFunction(functionName, start, end)
	if err != nil {
		return nil, err
	}

	profile := &FunctionProfile{
		Function: functionName,
		Traces:   []SpanLink{},
	}

	// Calculate sample statistics
	var totalCount uint64
	for _, s := range samples {
		totalCount += s.Count
	}
	profile.SampleCount = totalCount

	// Find related traces for each sample
	seenTraces := make(map[string]bool)
	for _, sample := range samples {
		result, err := l.LinkSampleToTraces(ctx, sample, opts)
		if err != nil {
			continue
		}

		for _, link := range result.Links {
			key := link.TraceID + ":" + link.SpanID
			if !seenTraces[key] {
				seenTraces[key] = true
				profile.Traces = append(profile.Traces, link)
			}
		}

		// Limit total traces
		if len(profile.Traces) >= opts.MaxLinks*10 {
			break
		}
	}

	// Sort by confidence
	sortLinksByConfidence(profile.Traces)
	if len(profile.Traces) > opts.MaxLinks*3 {
		profile.Traces = profile.Traces[:opts.MaxLinks*3]
	}

	return profile, nil
}

// GetProfilesForTrace finds profile samples related to a trace.
func (l *Linker) GetProfilesForTrace(ctx context.Context, traceID string) ([]*Sample, error) {
	// Get stored links
	links, err := l.profileStore.GetLinksForTrace(traceID)
	if err != nil {
		return nil, err
	}

	samples := make([]*Sample, 0, len(links))
	seen := make(map[int64]bool)

	for _, link := range links {
		if seen[link.SampleID] {
			continue
		}
		seen[link.SampleID] = true

		sample, err := l.profileStore.GetSampleByID(link.SampleID)
		if err != nil {
			continue
		}
		samples = append(samples, sample)
	}

	return samples, nil
}

// GetProfilesForSpan finds profile samples related to a specific span.
func (l *Linker) GetProfilesForSpan(ctx context.Context, traceID, spanID string) ([]*Sample, error) {
	links, err := l.profileStore.GetLinksForSpan(traceID, spanID)
	if err != nil {
		return nil, err
	}

	samples := make([]*Sample, 0, len(links))
	for _, link := range links {
		sample, err := l.profileStore.GetSampleByID(link.SampleID)
		if err != nil {
			continue
		}
		samples = append(samples, sample)
	}

	return samples, nil
}

// AutoLink runs background correlation for recent data.
func (l *Linker) AutoLink(ctx context.Context, lookback time.Duration) (int, error) {
	end := time.Now()
	start := end.Add(-lookback)

	// Get recent samples
	samples, err := l.profileStore.QueryByTimeRange(start, end)
	if err != nil {
		return 0, err
	}

	opts := DefaultLinkOptions()
	linked := 0

	for _, sample := range samples {
		select {
		case <-ctx.Done():
			return linked, ctx.Err()
		default:
		}

		result, err := l.LinkSampleToTraces(ctx, sample, opts)
		if err != nil {
			continue
		}

		if len(result.Links) > 0 {
			linked++
		}
	}

	return linked, nil
}

// Stats returns linking statistics.
type LinkStats struct {
	TotalSamples      int64 `json:"total_samples"`
	TotalLinks        int64 `json:"total_links"`
	AvgLinksPerSample float64 `json:"avg_links_per_sample"`
	HighConfidenceLinks int64 `json:"high_confidence_links"`
}

// GetStats returns linking statistics.
func (l *Linker) GetStats(ctx context.Context) (*LinkStats, error) {
	stats := &LinkStats{}

	// Count samples
	row := l.profileStore.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM profile_samples")
	row.Scan(&stats.TotalSamples)

	// Count links
	row = l.profileStore.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM profile_trace_links")
	row.Scan(&stats.TotalLinks)

	// Count high confidence links
	row = l.profileStore.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM profile_trace_links WHERE confidence >= 0.7")
	row.Scan(&stats.HighConfidenceLinks)

	if stats.TotalSamples > 0 {
		stats.AvgLinksPerSample = float64(stats.TotalLinks) / float64(stats.TotalSamples)
	}

	return stats, nil
}
