package correlation

import (
	"log"
	"sort"
	"sync"
	"time"

	"dogwatch/internal/deploys"
	"dogwatch/internal/incidents"
	"dogwatch/internal/logs"
	"dogwatch/internal/trace"
)

// Engine provides correlation capabilities across all telemetry data
type Engine struct {
	traceStore    *trace.Store
	logStore      *logs.Store
	incidentStore *incidents.Store
	deployStore   *deploys.Store

	// Configuration
	deployCorrelationWindow time.Duration // How far back to look for deploys
	traceCorrelationWindow  time.Duration // How far back/forward to look for traces

	mu sync.RWMutex
}

// NewEngine creates a new correlation engine
func NewEngine() *Engine {
	return &Engine{
		deployCorrelationWindow: 30 * time.Minute,
		traceCorrelationWindow:  5 * time.Minute,
	}
}

// SetTraceStore sets the trace store
func (e *Engine) SetTraceStore(s *trace.Store) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.traceStore = s
}

// SetLogStore sets the log store
func (e *Engine) SetLogStore(s *logs.Store) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.logStore = s
}

// SetIncidentStore sets the incident store
func (e *Engine) SetIncidentStore(s *incidents.Store) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.incidentStore = s
}

// SetDeployStore sets the deploy store
func (e *Engine) SetDeployStore(s *deploys.Store) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.deployStore = s
}

// GetTraceContext returns all data correlated to a trace
func (e *Engine) GetTraceContext(traceID string) (*TraceContext, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	ctx := &TraceContext{
		TraceID: traceID,
	}

	// Get trace and spans
	if e.traceStore != nil {
		detail, err := e.traceStore.GetTrace(traceID)
		if err == nil && detail != nil {
			ctx.Trace = &detail.Trace
			ctx.Spans = detail.Spans
			ctx.Service = detail.ServiceName
			ctx.Duration = detail.DurationMs
			ctx.Status = detail.Status

			// Count errors
			for _, span := range detail.Spans {
				if span.Status == "ERROR" {
					ctx.ErrorCount++
				}
			}
		}
	}

	// Get correlated logs
	if e.logStore != nil {
		logEntries, err := e.logStore.GetByTraceID(traceID)
		if err == nil {
			ctx.Logs = logEntries
		}
	}

	return ctx, nil
}

// GetIncidentContext returns all data correlated to an incident
func (e *Engine) GetIncidentContext(incidentID string) (*IncidentContext, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	ctx := &IncidentContext{}

	// Get the incident
	if e.incidentStore == nil {
		return nil, nil
	}

	incident, err := e.incidentStore.GetIncident(incidentID)
	if err != nil || incident == nil {
		return nil, err
	}
	ctx.Incident = incident

	// Time window for correlations
	incidentTime := incident.CreatedAt
	startWindow := incidentTime.Add(-e.traceCorrelationWindow)
	endWindow := incidentTime.Add(e.traceCorrelationWindow)

	// Get related traces (same service, around incident time)
	if e.traceStore != nil && incident.Service != "" {
		traces, err := e.traceStore.ListTraces(50, incident.Service, time.Since(startWindow))
		if err == nil {
			// Filter to time window
			for _, t := range traces {
				if t.StartTime.After(startWindow) && t.StartTime.Before(endWindow) {
					ctx.RelatedTraces = append(ctx.RelatedTraces, t)
				}
			}
		}
	}

	// Get related logs (same service, around incident time, errors/warnings)
	if e.logStore != nil && incident.Service != "" {
		result, err := e.logStore.Search(logs.SearchQuery{
			Service:   incident.Service,
			StartTime: startWindow,
			EndTime:   endWindow,
			Limit:     100,
		})
		if err == nil {
			ctx.RelatedLogs = result.LogEntries()
		}
	}

	// Get preceding deploys (same service, before incident)
	if e.deployStore != nil && incident.Service != "" {
		deployWindow := incidentTime.Add(-e.deployCorrelationWindow)
		allDeploys, err := e.deployStore.ListByService(incident.Service, 10)
		if err == nil {
			for _, d := range allDeploys {
				if d.Timestamp.After(deployWindow) && d.Timestamp.Before(incidentTime) {
					ctx.PrecedingDeploys = append(ctx.PrecedingDeploys, d)
				}
			}
		}

		// Find probable cause deploy
		if len(ctx.PrecedingDeploys) > 0 {
			ctx.ProbableCause = e.findProbableCauseDeploy(incident, ctx.PrecedingDeploys)
		}
	}

	// Get related incidents (same service, recent)
	if e.incidentStore != nil && incident.Service != "" {
		relatedIncidents, err := e.incidentStore.ListIncidentsByService(incident.Service, 10)
		if err == nil {
			for _, inc := range relatedIncidents {
				if inc.ID != incident.ID {
					ctx.RelatedIncidents = append(ctx.RelatedIncidents, inc)
				}
			}
		}
	}

	// Build timeline
	ctx.Timeline = e.buildIncidentTimeline(incident, ctx)

	return ctx, nil
}

// findProbableCauseDeploy finds the most likely deploy that caused an incident
func (e *Engine) findProbableCauseDeploy(incident *incidents.Incident, deploys []deploys.Deployment) *DeployCorrelation {
	if len(deploys) == 0 {
		return nil
	}

	// Sort by time (most recent first)
	sort.Slice(deploys, func(i, j int) bool {
		return deploys[i].Timestamp.After(deploys[j].Timestamp)
	})

	// Most recent deploy before incident is most likely cause
	mostRecent := deploys[0]
	timeDelta := incident.CreatedAt.Sub(mostRecent.Timestamp)

	// Calculate confidence based on time proximity and service match
	confidence := 0.5 // Base confidence

	// Higher confidence if deploy was very recent (within 10 minutes)
	if timeDelta < 10*time.Minute {
		confidence += 0.3
	} else if timeDelta < 20*time.Minute {
		confidence += 0.2
	} else if timeDelta < 30*time.Minute {
		confidence += 0.1
	}

	// Higher confidence if same service
	serviceMatch := mostRecent.Service == incident.Service
	if serviceMatch {
		confidence += 0.2
	}

	// Cap at 1.0
	if confidence > 1.0 {
		confidence = 1.0
	}

	reason := "Most recent deployment before incident"
	if timeDelta < 10*time.Minute {
		reason = "Deployment occurred " + timeDelta.Round(time.Minute).String() + " before incident"
	}

	return &DeployCorrelation{
		Deployment:   &mostRecent,
		TimeDelta:    timeDelta,
		Confidence:   confidence,
		Reason:       reason,
		ServiceMatch: serviceMatch,
	}
}

// buildIncidentTimeline builds a chronological timeline of events
func (e *Engine) buildIncidentTimeline(incident *incidents.Incident, ctx *IncidentContext) []CorrelatedEvent {
	var events []CorrelatedEvent

	// Add deploys
	for _, d := range ctx.PrecedingDeploys {
		events = append(events, CorrelatedEvent{
			Type:      "deploy",
			ID:        d.ID,
			Timestamp: d.Timestamp,
			Service:   d.Service,
			Summary:   "Deploy " + d.Version,
			Data:      d,
		})
	}

	// Add incident creation
	events = append(events, CorrelatedEvent{
		Type:      "incident",
		ID:        incident.ID,
		Timestamp: incident.CreatedAt,
		Service:   incident.Service,
		Summary:   incident.Title,
		Severity:  string(incident.Severity),
		Data:      incident,
	})

	// Add error traces
	for _, t := range ctx.RelatedTraces {
		if t.Status == "ERROR" {
			events = append(events, CorrelatedEvent{
				Type:      "trace",
				ID:        t.TraceID,
				Timestamp: t.StartTime,
				Service:   t.ServiceName,
				Summary:   t.Name + " (ERROR)",
				Severity:  "error",
				Data:      t,
			})
		}
	}

	// Add error logs
	for _, l := range ctx.RelatedLogs {
		if l.Level == logs.LevelError || l.Level == logs.LevelFatal {
			events = append(events, CorrelatedEvent{
				Type:      "log",
				ID:        l.ID,
				Timestamp: l.Timestamp,
				Service:   l.Service,
				Summary:   truncate(l.Message, 100),
				Severity:  string(l.Level),
				Data:      l,
			})
		}
	}

	// Sort by timestamp
	sort.Slice(events, func(i, j int) bool {
		return events[i].Timestamp.Before(events[j].Timestamp)
	})

	return events
}

// GetDeployContext returns all data correlated to a deployment
func (e *Engine) GetDeployContext(deployID string) (*DeployContext, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	ctx := &DeployContext{}

	// Get the deployment
	if e.deployStore == nil {
		return nil, nil
	}

	deploy, err := e.deployStore.Get(deployID)
	if err != nil || deploy == nil {
		return nil, err
	}
	ctx.Deployment = deploy

	// Look for incidents after deploy
	if e.incidentStore != nil {
		endWindow := deploy.Timestamp.Add(e.deployCorrelationWindow)
		allIncidents, err := e.incidentStore.ListIncidentsByService(deploy.Service, 20)
		if err == nil {
			for _, inc := range allIncidents {
				if inc.CreatedAt.After(deploy.Timestamp) && inc.CreatedAt.Before(endWindow) {
					ctx.FollowingIncidents = append(ctx.FollowingIncidents, inc)
				}
			}
		}
	}

	// Compare errors before/after deploy
	if e.logStore != nil {
		ctx.ErrorsBeforeAfter = e.compareErrors(deploy.Service, deploy.Timestamp)
	}

	// Determine impact
	ctx.Impact = e.assessDeployImpact(ctx)

	return ctx, nil
}

// compareErrors compares error counts before and after a timestamp
func (e *Engine) compareErrors(service string, timestamp time.Time) *ErrorComparison {
	window := 15 * time.Minute

	beforeStart := timestamp.Add(-window)
	afterEnd := timestamp.Add(window)

	var beforeCount, afterCount int

	// Count errors before
	beforeResult, err := e.logStore.Search(logs.SearchQuery{
		Service:   service,
		Level:     logs.LevelError,
		StartTime: beforeStart,
		EndTime:   timestamp,
		Limit:     1000,
	})
	if err == nil {
		beforeCount = beforeResult.TotalCount
	}

	// Count errors after
	afterResult, err := e.logStore.Search(logs.SearchQuery{
		Service:   service,
		Level:     logs.LevelError,
		StartTime: timestamp,
		EndTime:   afterEnd,
		Limit:     1000,
	})
	if err == nil {
		afterCount = afterResult.TotalCount
	}

	comparison := &ErrorComparison{
		Before:     beforeCount,
		After:      afterCount,
		BeforeRate: float64(beforeCount) / window.Minutes(),
		AfterRate:  float64(afterCount) / window.Minutes(),
	}

	if beforeCount > 0 {
		comparison.ChangePercent = float64(afterCount-beforeCount) / float64(beforeCount) * 100
	} else if afterCount > 0 {
		comparison.ChangePercent = 100 // Went from 0 to something
	}

	return comparison
}

// assessDeployImpact determines the impact of a deployment
func (e *Engine) assessDeployImpact(ctx *DeployContext) string {
	// Negative if incidents followed
	if len(ctx.FollowingIncidents) > 0 {
		return "negative"
	}

	// Negative if errors increased significantly
	if ctx.ErrorsBeforeAfter != nil {
		if ctx.ErrorsBeforeAfter.ChangePercent > 50 {
			return "negative"
		}
		if ctx.ErrorsBeforeAfter.ChangePercent < -20 {
			return "positive"
		}
	}

	return "neutral"
}

// GetServiceTimeline returns all events for a service in a time range
func (e *Engine) GetServiceTimeline(service string, start, end time.Time) (*ServiceTimeline, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	timeline := &ServiceTimeline{
		Service:   service,
		StartTime: start,
		EndTime:   end,
		Summary:   &TimelineSummary{},
	}

	var events []CorrelatedEvent

	// Get traces
	if e.traceStore != nil {
		traces, err := e.traceStore.ListTraces(200, service, time.Since(start))
		if err == nil {
			for _, t := range traces {
				if t.StartTime.After(start) && t.StartTime.Before(end) {
					events = append(events, CorrelatedEvent{
						Type:      "trace",
						ID:        t.TraceID,
						Timestamp: t.StartTime,
						Service:   t.ServiceName,
						Summary:   t.Name,
						Severity:  t.Status,
					})
					timeline.Summary.TraceCount++
				}
			}
		}
	}

	// Get logs
	if e.logStore != nil {
		result, err := e.logStore.Search(logs.SearchQuery{
			Service:   service,
			StartTime: start,
			EndTime:   end,
			Limit:     500,
		})
		if err == nil {
			for _, l := range result.Entries {
				events = append(events, CorrelatedEvent{
					Type:      "log",
					ID:        l.ID,
					Timestamp: l.Timestamp,
					Service:   l.Service,
					Summary:   truncate(l.Message, 80),
					Severity:  string(l.Level),
				})
				timeline.Summary.LogCount++
				if l.Level == logs.LevelError || l.Level == logs.LevelFatal {
					timeline.Summary.ErrorLogCount++
				}
			}
		}
	}

	// Get incidents
	if e.incidentStore != nil {
		allIncidents, err := e.incidentStore.ListIncidentsByService(service, 50)
		if err == nil {
			for _, inc := range allIncidents {
				if inc.CreatedAt.After(start) && inc.CreatedAt.Before(end) {
					events = append(events, CorrelatedEvent{
						Type:      "incident",
						ID:        inc.ID,
						Timestamp: inc.CreatedAt,
						Service:   inc.Service,
						Summary:   inc.Title,
						Severity:  string(inc.Severity),
					})
					timeline.Summary.IncidentCount++
				}
			}
		}
	}

	// Get deploys
	if e.deployStore != nil {
		allDeploys, err := e.deployStore.ListByService(service, 50)
		if err == nil {
			for _, d := range allDeploys {
				if d.Timestamp.After(start) && d.Timestamp.Before(end) {
					events = append(events, CorrelatedEvent{
						Type:      "deploy",
						ID:        d.ID,
						Timestamp: d.Timestamp,
						Service:   d.Service,
						Summary:   "Deploy " + d.Version,
					})
					timeline.Summary.DeployCount++
				}
			}
		}
	}

	// Sort by timestamp
	sort.Slice(events, func(i, j int) bool {
		return events[i].Timestamp.Before(events[j].Timestamp)
	})

	timeline.Events = events
	timeline.Summary.TotalEvents = len(events)

	return timeline, nil
}

// FindDeployIncidentCorrelations finds all deploy->incident correlations
func (e *Engine) FindDeployIncidentCorrelations(since time.Duration) ([]DeployCorrelation, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if e.deployStore == nil || e.incidentStore == nil {
		return nil, nil
	}

	var correlations []DeployCorrelation

	// Get recent deploys
	deploys, err := e.deployStore.List(100)
	if err != nil {
		return nil, err
	}

	cutoff := time.Now().Add(-since)

	for _, deploy := range deploys {
		if deploy.Timestamp.Before(cutoff) {
			continue
		}

		// Look for incidents after this deploy
		incidents, err := e.incidentStore.ListIncidentsByService(deploy.Service, 20)
		if err != nil {
			continue
		}

		endWindow := deploy.Timestamp.Add(e.deployCorrelationWindow)

		for _, inc := range incidents {
			if inc.CreatedAt.After(deploy.Timestamp) && inc.CreatedAt.Before(endWindow) {
				timeDelta := inc.CreatedAt.Sub(deploy.Timestamp)

				// Calculate confidence
				confidence := 0.5
				if timeDelta < 10*time.Minute {
					confidence = 0.9
				} else if timeDelta < 20*time.Minute {
					confidence = 0.7
				}

				correlations = append(correlations, DeployCorrelation{
					Deployment:   &deploy,
					TimeDelta:    timeDelta,
					Confidence:   confidence,
					Reason:       "Incident started " + timeDelta.Round(time.Minute).String() + " after deploy",
					ServiceMatch: deploy.Service == inc.Service,
				})
			}
		}
	}

	// Sort by confidence (highest first)
	sort.Slice(correlations, func(i, j int) bool {
		return correlations[i].Confidence > correlations[j].Confidence
	})

	return correlations, nil
}

// GetAlertContext returns context for an alert (logs, traces around trigger time)
func (e *Engine) GetAlertContext(alertID string, service string, triggeredAt time.Time) (*AlertContext, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	ctx := &AlertContext{
		AlertID:     alertID,
		Service:     service,
		TriggeredAt: triggeredAt,
	}

	window := e.traceCorrelationWindow
	startWindow := triggeredAt.Add(-window)
	endWindow := triggeredAt.Add(window)

	// Get traces around trigger time
	if e.traceStore != nil && service != "" {
		traces, err := e.traceStore.ListTraces(100, service, time.Since(startWindow))
		if err == nil {
			for _, t := range traces {
				if t.StartTime.After(startWindow) && t.StartTime.Before(endWindow) {
					ctx.TriggerTraces = append(ctx.TriggerTraces, t)
				}
			}
		}
	}

	// Get logs around trigger time
	if e.logStore != nil {
		result, err := e.logStore.Search(logs.SearchQuery{
			Service:   service,
			StartTime: startWindow,
			EndTime:   endWindow,
			Limit:     100,
		})
		if err == nil {
			ctx.TriggerLogs = result.LogEntries()
		}
	}

	// Get recent deploys
	if e.deployStore != nil && service != "" {
		deploys, err := e.deployStore.ListByService(service, 5)
		if err == nil {
			for _, d := range deploys {
				if d.Timestamp.Before(triggeredAt) && d.Timestamp.After(triggeredAt.Add(-e.deployCorrelationWindow)) {
					ctx.RecentDeploys = append(ctx.RecentDeploys, d)
				}
			}
		}
	}

	return ctx, nil
}

// CorrelateLogToTrace finds the trace associated with a log entry
func (e *Engine) CorrelateLogToTrace(logEntry *logs.LogEntry) (*TraceContext, error) {
	if logEntry.TraceID == "" {
		return nil, nil
	}
	return e.GetTraceContext(logEntry.TraceID)
}

// Helper function to truncate strings
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// OnIncidentCreated is called when an incident is created to find correlations
func (e *Engine) OnIncidentCreated(incident *incidents.Incident) {
	ctx, err := e.GetIncidentContext(incident.ID)
	if err != nil {
		return
	}

	if ctx.ProbableCause != nil {
		log.Printf("[correlation] Incident %s may be caused by deploy %s (confidence: %.0f%%)",
			incident.ID, ctx.ProbableCause.Deployment.ID, ctx.ProbableCause.Confidence*100)
	}
}
