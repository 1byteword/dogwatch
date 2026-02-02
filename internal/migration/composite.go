package migration

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// CompositeQueryParser parses Datadog composite monitor queries
type CompositeQueryParser struct {
	// monitorIDMap maps Datadog monitor IDs to dogwatch rule IDs
	monitorIDMap map[int64]string
}

// NewCompositeQueryParser creates a new composite query parser
func NewCompositeQueryParser() *CompositeQueryParser {
	return &CompositeQueryParser{
		monitorIDMap: make(map[int64]string),
	}
}

// RegisterMonitorMapping registers a mapping from Datadog monitor ID to dogwatch rule ID
func (p *CompositeQueryParser) RegisterMonitorMapping(datadogID int64, dogwatchID string) {
	p.monitorIDMap[datadogID] = dogwatchID
}

// ParsedCompositeQuery represents a parsed composite query
type ParsedCompositeQuery struct {
	Expression     string   // Normalized expression for dogwatch (e.g., "a AND b")
	SubRuleIDs     []string // Dogwatch rule IDs referenced
	UnresolvedRefs []int64  // Datadog monitor IDs that couldn't be resolved
	Warnings       []string // Parsing warnings
}

// ParseCompositeQuery parses a Datadog composite monitor query
// Datadog format examples:
//   - Simple: monitors.123
//   - Negation: !monitors.123
//   - AND: monitors.123 && monitors.456
//   - OR: monitors.123 || monitors.456
//   - Mixed: (monitors.123 && monitors.456) || !monitors.789
func (p *CompositeQueryParser) ParseCompositeQuery(query string) (*ParsedCompositeQuery, error) {
	result := &ParsedCompositeQuery{
		SubRuleIDs: []string{},
	}

	// Extract all monitor references
	monitorRefs := extractMonitorRefs(query)
	if len(monitorRefs) == 0 {
		return nil, fmt.Errorf("no monitor references found in composite query: %s", query)
	}

	// Map monitor IDs to rule IDs and build substitution map
	substitutions := make(map[string]string)
	for _, ref := range monitorRefs {
		if ruleID, ok := p.monitorIDMap[ref.ID]; ok {
			substitutions[ref.Original] = ruleID
			result.SubRuleIDs = append(result.SubRuleIDs, ruleID)
		} else {
			// Monitor not found - create placeholder
			placeholder := fmt.Sprintf("unresolved_%d", ref.ID)
			substitutions[ref.Original] = placeholder
			result.UnresolvedRefs = append(result.UnresolvedRefs, ref.ID)
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Monitor %d not found in import batch - rule will need manual update", ref.ID))
		}
	}

	// Convert expression to dogwatch format
	expr := convertCompositeExpression(query, substitutions)
	result.Expression = expr

	return result, nil
}

// MonitorRef represents a reference to a monitor in a composite query
type MonitorRef struct {
	Original string // Original text (e.g., "monitors.123")
	ID       int64  // Parsed monitor ID
	Negated  bool   // Whether the reference is negated
}

// extractMonitorRefs extracts all monitor references from a composite query
func extractMonitorRefs(query string) []MonitorRef {
	var refs []MonitorRef

	// Pattern matches: monitors.123, monitors[123], monitor:123
	// Also handles negation: !monitors.123
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(!?)monitors\.(\d+)`),
		regexp.MustCompile(`(!?)monitors\[(\d+)\]`),
		regexp.MustCompile(`(!?)monitor:(\d+)`),
	}

	seen := make(map[int64]bool)

	for _, pattern := range patterns {
		matches := pattern.FindAllStringSubmatch(query, -1)
		for _, match := range matches {
			if len(match) < 3 {
				continue
			}

			id, err := strconv.ParseInt(match[2], 10, 64)
			if err != nil {
				continue
			}

			if seen[id] {
				continue
			}
			seen[id] = true

			refs = append(refs, MonitorRef{
				Original: match[0],
				ID:       id,
				Negated:  match[1] == "!",
			})
		}
	}

	return refs
}

// convertCompositeExpression converts a Datadog composite expression to dogwatch format
func convertCompositeExpression(query string, substitutions map[string]string) string {
	expr := query

	// Sort keys by length descending to replace longer matches first
	// (prevents partial replacements, e.g., "!monitors.123" before "monitors.123")
	var keys []string
	for k := range substitutions {
		keys = append(keys, k)
	}
	// Sort by length descending
	for i := 0; i < len(keys)-1; i++ {
		for j := i + 1; j < len(keys); j++ {
			if len(keys[j]) > len(keys[i]) {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}

	// Replace monitor references with rule IDs
	for _, original := range keys {
		ruleID := substitutions[original]

		// Check if this is a negated reference (Original starts with !)
		if strings.HasPrefix(original, "!") {
			// Negated ref in substitutions map
			expr = strings.ReplaceAll(expr, original, fmt.Sprintf("NOT %s", ruleID))
		} else {
			// Non-negated ref: also handle !original in the expression
			expr = strings.ReplaceAll(expr, "!"+original, fmt.Sprintf("NOT %s", ruleID))
			expr = strings.ReplaceAll(expr, original, ruleID)
		}
	}

	// Convert operators
	expr = strings.ReplaceAll(expr, "&&", " AND ")
	expr = strings.ReplaceAll(expr, "||", " OR ")

	// Clean up whitespace
	expr = regexp.MustCompile(`\s+`).ReplaceAllString(expr, " ")
	expr = strings.TrimSpace(expr)

	return expr
}

// CompositeMonitorContext manages monitor ID mappings during batch import
type CompositeMonitorContext struct {
	parser           *CompositeQueryParser
	pendingComposite []*PendingCompositeMonitor
}

// PendingCompositeMonitor represents a composite monitor waiting for resolution
type PendingCompositeMonitor struct {
	DatadogMonitor *DatadogMonitor
	TargetRuleID   string
	OriginalQuery  string
}

// NewCompositeMonitorContext creates a new composite monitor context
func NewCompositeMonitorContext() *CompositeMonitorContext {
	return &CompositeMonitorContext{
		parser: NewCompositeQueryParser(),
	}
}

// RegisterMonitor registers a converted monitor for composite reference resolution
func (c *CompositeMonitorContext) RegisterMonitor(datadogID int64, dogwatchID string) {
	c.parser.RegisterMonitorMapping(datadogID, dogwatchID)
}

// AddPendingComposite adds a composite monitor to be resolved later
func (c *CompositeMonitorContext) AddPendingComposite(monitor *DatadogMonitor, ruleID string) {
	c.pendingComposite = append(c.pendingComposite, &PendingCompositeMonitor{
		DatadogMonitor: monitor,
		TargetRuleID:   ruleID,
		OriginalQuery:  monitor.Query,
	})
}

// ResolvePendingComposites resolves all pending composite monitors
// Returns the updates needed for each rule
func (c *CompositeMonitorContext) ResolvePendingComposites() []CompositeResolution {
	var resolutions []CompositeResolution

	for _, pending := range c.pendingComposite {
		parsed, err := c.parser.ParseCompositeQuery(pending.OriginalQuery)
		if err != nil {
			resolutions = append(resolutions, CompositeResolution{
				RuleID:   pending.TargetRuleID,
				Success:  false,
				Error:    err.Error(),
				Warnings: []string{fmt.Sprintf("Failed to parse composite query: %s", pending.OriginalQuery)},
			})
			continue
		}

		resolutions = append(resolutions, CompositeResolution{
			RuleID:     pending.TargetRuleID,
			Success:    true,
			Expression: parsed.Expression,
			SubRuleIDs: parsed.SubRuleIDs,
			Warnings:   parsed.Warnings,
		})
	}

	return resolutions
}

// CompositeResolution contains the resolution result for a composite monitor
type CompositeResolution struct {
	RuleID     string   `json:"rule_id"`
	Success    bool     `json:"success"`
	Expression string   `json:"expression,omitempty"`
	SubRuleIDs []string `json:"sub_rule_ids,omitempty"`
	Warnings   []string `json:"warnings,omitempty"`
	Error      string   `json:"error,omitempty"`
}

// IsCompositeMonitor checks if a Datadog monitor is a composite monitor
func IsCompositeMonitor(monitor *DatadogMonitor) bool {
	if monitor.Type == "composite" {
		return true
	}

	// Also check if query contains monitor references
	if strings.Contains(monitor.Query, "monitors.") ||
		strings.Contains(monitor.Query, "monitors[") ||
		strings.Contains(monitor.Query, "monitor:") {
		return true
	}

	return false
}

// ExtractCompositeOperator extracts the primary operator from a composite expression
func ExtractCompositeOperator(query string) string {
	// Count operators to determine the primary one
	andCount := strings.Count(query, "&&")
	orCount := strings.Count(query, "||")

	if andCount > 0 && orCount > 0 {
		return "mixed"
	} else if andCount > 0 {
		return "and"
	} else if orCount > 0 {
		return "or"
	}

	return "single"
}
