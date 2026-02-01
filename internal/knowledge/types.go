package knowledge

import (
	"encoding/json"
	"time"
)

// ObjectType defines the type of knowledge object
type ObjectType string

const (
	TypeFieldExtraction ObjectType = "field_extraction"
	TypeLookup          ObjectType = "lookup"
	TypeMacro           ObjectType = "macro"
	TypeSavedSearch     ObjectType = "saved_search"
)

// KnowledgeObject represents a reusable query component
type KnowledgeObject struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`          // Unique name for reference
	Type        ObjectType `json:"type"`          // "field_extraction", "lookup", "macro", "saved_search"
	Definition  string     `json:"definition"`    // The actual content/query (JSON-encoded for complex types)
	Description string     `json:"description"`
	Tags        []string   `json:"tags"`
	Owner       string     `json:"owner"`
	Shared      bool       `json:"shared"` // Available to all users
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// FieldExtraction parses fields from unstructured data
type FieldExtraction struct {
	Pattern      string   `json:"pattern"`       // Regex with named groups
	SourceField  string   `json:"source_field"`  // Field to extract from (default: message)
	TargetFields []string `json:"target_fields"` // Fields to create
}

// Lookup enriches data with external mappings
type Lookup struct {
	Data         map[string]map[string]string `json:"data"`          // key -> field -> value
	KeyField     string                       `json:"key_field"`     // Field to use as lookup key
	OutputFields []string                     `json:"output_fields"` // Fields to add from lookup
}

// Macro represents a reusable query snippet
type Macro struct {
	Expression string   `json:"expression"` // Query fragment
	Args       []string `json:"args"`       // Argument placeholders ($1, $2, etc.)
}

// SavedSearch represents a complete query with optional schedule
type SavedSearch struct {
	Query    string `json:"query"`
	Schedule string `json:"schedule,omitempty"` // cron expression (optional)
	AlertOn  string `json:"alert_on,omitempty"` // condition to alert
}

// ParseFieldExtraction parses a FieldExtraction from a KnowledgeObject
func (ko *KnowledgeObject) ParseFieldExtraction() (*FieldExtraction, error) {
	if ko.Type != TypeFieldExtraction {
		return nil, ErrTypeMismatch
	}
	var fe FieldExtraction
	if err := json.Unmarshal([]byte(ko.Definition), &fe); err != nil {
		return nil, err
	}
	return &fe, nil
}

// ParseLookup parses a Lookup from a KnowledgeObject
func (ko *KnowledgeObject) ParseLookup() (*Lookup, error) {
	if ko.Type != TypeLookup {
		return nil, ErrTypeMismatch
	}
	var l Lookup
	if err := json.Unmarshal([]byte(ko.Definition), &l); err != nil {
		return nil, err
	}
	return &l, nil
}

// ParseMacro parses a Macro from a KnowledgeObject
func (ko *KnowledgeObject) ParseMacro() (*Macro, error) {
	if ko.Type != TypeMacro {
		return nil, ErrTypeMismatch
	}
	var m Macro
	if err := json.Unmarshal([]byte(ko.Definition), &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// ParseSavedSearch parses a SavedSearch from a KnowledgeObject
func (ko *KnowledgeObject) ParseSavedSearch() (*SavedSearch, error) {
	if ko.Type != TypeSavedSearch {
		return nil, ErrTypeMismatch
	}
	var ss SavedSearch
	if err := json.Unmarshal([]byte(ko.Definition), &ss); err != nil {
		return nil, err
	}
	return &ss, nil
}

// SetDefinition encodes and sets the definition based on type
func (ko *KnowledgeObject) SetDefinition(def interface{}) error {
	data, err := json.Marshal(def)
	if err != nil {
		return err
	}
	ko.Definition = string(data)
	return nil
}

// ValidationResult contains validation results
type ValidationResult struct {
	Valid   bool     `json:"valid"`
	Errors  []string `json:"errors,omitempty"`
	Warning []string `json:"warnings,omitempty"`
}
