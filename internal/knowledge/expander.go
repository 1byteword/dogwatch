package knowledge

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// Expander handles query expansion using knowledge objects
type Expander struct {
	registry *Registry
}

// NewExpander creates a new query expander
func NewExpander(registry *Registry) *Expander {
	return &Expander{registry: registry}
}

// ExpandMacros expands all macro references in a DQL query
// Macros are referenced as `macro_name` or `macro_name(arg1, arg2)`
func (e *Expander) ExpandMacros(query string) (string, error) {
	expanded, err := e.expandMacrosRecursive(query, make(map[string]bool))
	if err != nil {
		return "", err
	}
	return expanded, nil
}

func (e *Expander) expandMacrosRecursive(query string, seen map[string]bool) (string, error) {
	// Find macro references: `macro_name` or `macro_name(args)`
	// Pattern matches: `name` or `name(arg1, arg2, ...)`
	macroPattern := regexp.MustCompile("`([a-zA-Z_][a-zA-Z0-9_]*)(?:\\(([^)]+)\\))?`")

	result := macroPattern.ReplaceAllStringFunc(query, func(match string) string {
		// Extract macro name and args
		submatches := macroPattern.FindStringSubmatch(match)
		macroName := submatches[1]
		argsStr := ""
		if len(submatches) > 2 {
			argsStr = submatches[2]
		}

		// Check for circular reference
		if seen[macroName] {
			return match // Return original to prevent infinite loop
		}

		// Get macro from registry
		obj, err := e.registry.GetMacro(macroName)
		if err != nil {
			return match // Return original if not found
		}

		macro, err := obj.ParseMacro()
		if err != nil {
			return match
		}

		// Substitute arguments
		expanded := macro.Expression
		if argsStr != "" {
			args := parseArgs(argsStr)
			for i, arg := range args {
				placeholder := fmt.Sprintf("$%d", i+1)
				expanded = strings.ReplaceAll(expanded, placeholder, arg)

				// Also support named args from macro definition
				if i < len(macro.Args) {
					namedPlaceholder := "$" + macro.Args[i]
					expanded = strings.ReplaceAll(expanded, namedPlaceholder, arg)
				}
			}
		}

		return expanded
	})

	// Check if any macros were expanded
	if result != query {
		// Mark current macros as seen
		matches := macroPattern.FindAllStringSubmatch(query, -1)
		for _, m := range matches {
			seen[m[1]] = true
		}

		// Recursively expand
		return e.expandMacrosRecursive(result, seen)
	}

	return result, nil
}

// parseArgs splits a comma-separated argument string
func parseArgs(argsStr string) []string {
	if argsStr == "" {
		return nil
	}

	args := strings.Split(argsStr, ",")
	result := make([]string, len(args))
	for i, arg := range args {
		result[i] = strings.TrimSpace(arg)
	}
	return result
}

// Row represents a query result row
type Row map[string]interface{}

// ApplyFieldExtraction applies a field extraction to log results
func (e *Expander) ApplyFieldExtraction(extractionName string, rows []Row) ([]Row, error) {
	obj, err := e.registry.GetFieldExtraction(extractionName)
	if err != nil {
		return nil, err
	}

	fe, err := obj.ParseFieldExtraction()
	if err != nil {
		return nil, err
	}

	// Compile the regex pattern
	re, err := regexp.Compile(fe.Pattern)
	if err != nil {
		return nil, ErrInvalidRegex
	}

	names := re.SubexpNames()

	// Apply extraction to each row
	result := make([]Row, len(rows))
	for i, row := range rows {
		newRow := make(Row)
		for k, v := range row {
			newRow[k] = v
		}

		// Get source field
		sourceField := fe.SourceField
		if sourceField == "" {
			sourceField = "message"
		}

		if sourceVal, ok := row[sourceField]; ok {
			if sourceStr, ok := sourceVal.(string); ok {
				// Handle JSON extraction specially
				if extractionName == "json_extract" {
					e.extractJSON(sourceStr, newRow)
				} else {
					// Apply regex extraction
					match := re.FindStringSubmatch(sourceStr)
					if match != nil {
						for j, name := range names {
							if j > 0 && name != "" {
								newRow[name] = match[j]
							}
						}
					}
				}
			}
		}

		result[i] = newRow
	}

	return result, nil
}

// extractJSON extracts JSON fields from a string
func (e *Expander) extractJSON(s string, row Row) {
	// Find JSON object in string
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start == -1 || end == -1 || end <= start {
		return
	}

	jsonStr := s[start : end+1]
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return
	}

	// Flatten into row
	for k, v := range data {
		row[k] = v
	}
}

// ApplyLookup enriches rows with lookup data
func (e *Expander) ApplyLookup(lookupName string, rows []Row) ([]Row, error) {
	obj, err := e.registry.GetLookup(lookupName)
	if err != nil {
		return nil, err
	}

	lookup, err := obj.ParseLookup()
	if err != nil {
		return nil, err
	}

	// Apply lookup to each row
	result := make([]Row, len(rows))
	for i, row := range rows {
		newRow := make(Row)
		for k, v := range row {
			newRow[k] = v
		}

		// Get key field value
		if keyVal, ok := row[lookup.KeyField]; ok {
			keyStr := fmt.Sprintf("%v", keyVal)

			// Look up data
			if lookupData, ok := lookup.Data[keyStr]; ok {
				for field, value := range lookupData {
					newRow[field] = value
				}
			}
		}

		result[i] = newRow
	}

	return result, nil
}

// ApplyAllFieldExtractions applies all applicable field extractions to rows
func (e *Expander) ApplyAllFieldExtractions(rows []Row) []Row {
	extractions := e.registry.ListFieldExtractions()

	for _, ext := range extractions {
		fe, err := ext.ParseFieldExtraction()
		if err != nil {
			continue
		}

		// Compile pattern
		re, err := regexp.Compile(fe.Pattern)
		if err != nil {
			continue
		}

		names := re.SubexpNames()
		sourceField := fe.SourceField
		if sourceField == "" {
			sourceField = "message"
		}

		// Try to apply to each row
		for i, row := range rows {
			if sourceVal, ok := row[sourceField]; ok {
				if sourceStr, ok := sourceVal.(string); ok {
					match := re.FindStringSubmatch(sourceStr)
					if match != nil {
						// This extraction matches, apply it
						for j, name := range names {
							if j > 0 && name != "" {
								rows[i][name] = match[j]
							}
						}
					}
				}
			}
		}
	}

	return rows
}

// ValidateFieldExtraction validates a field extraction definition
func (e *Expander) ValidateFieldExtraction(fe *FieldExtraction) *ValidationResult {
	result := &ValidationResult{Valid: true}

	// Validate regex pattern
	_, err := regexp.Compile(fe.Pattern)
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("Invalid regex pattern: %v", err))
	}

	// Check for named groups
	if fe.Pattern != "" && !strings.Contains(fe.Pattern, "(?P<") {
		result.Warning = append(result.Warning, "Pattern has no named capture groups. Use (?P<name>...) syntax to extract fields.")
	}

	// Check source field
	if fe.SourceField == "" {
		result.Warning = append(result.Warning, "No source field specified, will use 'message' as default")
	}

	return result
}

// ValidateMacro validates a macro definition
func (e *Expander) ValidateMacro(m *Macro) *ValidationResult {
	result := &ValidationResult{Valid: true}

	// Check expression is not empty
	if strings.TrimSpace(m.Expression) == "" {
		result.Valid = false
		result.Errors = append(result.Errors, "Macro expression cannot be empty")
	}

	// Check for arg placeholders
	for i, arg := range m.Args {
		placeholder := fmt.Sprintf("$%d", i+1)
		namedPlaceholder := "$" + arg
		if !strings.Contains(m.Expression, placeholder) && !strings.Contains(m.Expression, namedPlaceholder) {
			result.Warning = append(result.Warning, fmt.Sprintf("Argument '%s' is declared but not used in expression", arg))
		}
	}

	// Check for undeclared placeholders
	placeholderPattern := regexp.MustCompile(`\$(\d+|\w+)`)
	matches := placeholderPattern.FindAllStringSubmatch(m.Expression, -1)
	for _, match := range matches {
		placeholder := match[1]
		// Check if it's a numeric placeholder
		if placeholder[0] >= '0' && placeholder[0] <= '9' {
			// Numeric placeholder
			found := false
			for range m.Args {
				found = true
				break
			}
			if !found && len(m.Args) == 0 {
				result.Warning = append(result.Warning, fmt.Sprintf("Placeholder $%s found but no arguments declared", placeholder))
			}
		}
	}

	return result
}

// ValidateLookup validates a lookup definition
func (e *Expander) ValidateLookup(l *Lookup) *ValidationResult {
	result := &ValidationResult{Valid: true}

	// Check key field
	if l.KeyField == "" {
		result.Valid = false
		result.Errors = append(result.Errors, "Lookup key field cannot be empty")
	}

	// Check output fields
	if len(l.OutputFields) == 0 {
		result.Warning = append(result.Warning, "No output fields specified")
	}

	// Check data
	if len(l.Data) == 0 {
		result.Warning = append(result.Warning, "Lookup data is empty")
	}

	// Validate that output fields exist in data
	for key, fields := range l.Data {
		for _, outField := range l.OutputFields {
			if _, ok := fields[outField]; !ok {
				result.Warning = append(result.Warning, fmt.Sprintf("Output field '%s' missing in data for key '%s'", outField, key))
			}
		}
		break // Just check first entry
	}

	return result
}
