package scripts

import (
	"fmt"
	"strings"
	"time"
)

// Script defines an analysis script that can be run via CLI or API
type Script struct {
	Name        string      `json:"name"`        // "mysql/slow_queries"
	Category    string      `json:"category"`    // "mysql", "http", "k8s", "security"
	Title       string      `json:"title"`       // "Slow MySQL Queries"
	Description string      `json:"description"` // Human-readable description
	Query       string      `json:"query"`       // WatchQL query template
	Parameters  []Parameter `json:"parameters"`  // Configurable parameters
	Columns     []Column    `json:"columns"`     // Expected output columns
}

// Parameter defines a configurable parameter for a script
type Parameter struct {
	Name        string      `json:"name"`        // "threshold"
	Type        string      `json:"type"`        // "duration", "int", "string", "float"
	Default     interface{} `json:"default"`     // 100 * time.Millisecond or "100ms"
	Description string      `json:"description"` // "Minimum query duration"
	Required    bool        `json:"required"`
}

// Column defines an expected output column
type Column struct {
	Name string `json:"name"` // "query"
	Type string `json:"type"` // "string", "duration", "int", "float"
}

// ParsedParams holds parsed parameter values ready for query execution
type ParsedParams map[string]interface{}

// ParseParameters parses and validates user-provided parameters against script definition
func (s *Script) ParseParameters(provided map[string]string) (ParsedParams, error) {
	result := make(ParsedParams)

	// First, apply defaults
	for _, p := range s.Parameters {
		result[p.Name] = p.Default
	}

	// Then override with provided values
	for key, value := range provided {
		param := s.findParam(key)
		if param == nil {
			return nil, fmt.Errorf("unknown parameter: %s", key)
		}

		parsed, err := parseValue(value, param.Type)
		if err != nil {
			return nil, fmt.Errorf("invalid value for %s: %w", key, err)
		}
		result[key] = parsed
	}

	// Check required parameters
	for _, p := range s.Parameters {
		if p.Required && result[p.Name] == nil {
			return nil, fmt.Errorf("required parameter missing: %s", p.Name)
		}
	}

	return result, nil
}

func (s *Script) findParam(name string) *Parameter {
	for i := range s.Parameters {
		if s.Parameters[i].Name == name {
			return &s.Parameters[i]
		}
	}
	return nil
}

func parseValue(value string, paramType string) (interface{}, error) {
	switch paramType {
	case "duration":
		return parseDuration(value)
	case "int":
		var i int
		_, err := fmt.Sscanf(value, "%d", &i)
		return i, err
	case "float":
		var f float64
		_, err := fmt.Sscanf(value, "%f", &f)
		return f, err
	case "string":
		return value, nil
	case "bool":
		return value == "true" || value == "1" || value == "yes", nil
	default:
		return value, nil
	}
}

func parseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)

	// Handle common formats
	if strings.HasSuffix(s, "ms") {
		return time.ParseDuration(s)
	}
	if strings.HasSuffix(s, "s") && !strings.HasSuffix(s, "ms") {
		return time.ParseDuration(s)
	}
	if strings.HasSuffix(s, "m") && !strings.HasSuffix(s, "ms") {
		return time.ParseDuration(s)
	}
	if strings.HasSuffix(s, "h") {
		return time.ParseDuration(s)
	}
	if strings.HasSuffix(s, "d") {
		var days int
		_, err := fmt.Sscanf(s, "%dd", &days)
		if err != nil {
			return 0, err
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}

	// Try standard parsing
	d, err := time.ParseDuration(s)
	if err != nil {
		// Try parsing as milliseconds
		var ms int
		_, err := fmt.Sscanf(s, "%d", &ms)
		if err != nil {
			return 0, fmt.Errorf("invalid duration: %s", s)
		}
		return time.Duration(ms) * time.Millisecond, nil
	}
	return d, nil
}

// ExpandQuery replaces template parameters in the query with actual values
func (s *Script) ExpandQuery(params ParsedParams) string {
	query := s.Query

	for key, value := range params {
		placeholder := "{{." + key + "}}"
		var strValue string

		switch v := value.(type) {
		case time.Duration:
			strValue = fmt.Sprintf("%d", v.Milliseconds())
		case int:
			strValue = fmt.Sprintf("%d", v)
		case int64:
			strValue = fmt.Sprintf("%d", v)
		case float64:
			strValue = fmt.Sprintf("%f", v)
		case string:
			strValue = fmt.Sprintf("'%s'", v)
		default:
			strValue = fmt.Sprintf("%v", v)
		}

		query = strings.ReplaceAll(query, placeholder, strValue)
	}

	return query
}

// CategoryInfo provides metadata about a script category
type CategoryInfo struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Count       int    `json:"count"`
}
