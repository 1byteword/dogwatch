package knowledge

import "errors"

var (
	// ErrNotFound indicates the knowledge object was not found
	ErrNotFound = errors.New("knowledge object not found")

	// ErrTypeMismatch indicates the object type doesn't match the expected type
	ErrTypeMismatch = errors.New("knowledge object type mismatch")

	// ErrDuplicateName indicates an object with the same name already exists
	ErrDuplicateName = errors.New("knowledge object with this name already exists")

	// ErrInvalidDefinition indicates the definition is invalid
	ErrInvalidDefinition = errors.New("invalid knowledge object definition")

	// ErrInvalidRegex indicates the regex pattern is invalid
	ErrInvalidRegex = errors.New("invalid regex pattern")

	// ErrMacroNotFound indicates a referenced macro was not found
	ErrMacroNotFound = errors.New("macro not found")

	// ErrCircularReference indicates a circular reference in macros
	ErrCircularReference = errors.New("circular macro reference detected")

	// ErrLookupKeyNotFound indicates the lookup key field was not found
	ErrLookupKeyNotFound = errors.New("lookup key field not found in row")
)
