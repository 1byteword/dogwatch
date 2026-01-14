package web

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Pagination constants
const (
	DefaultPageSize = 50
	MaxPageSize     = 500
	DefaultCursor   = ""
)

// PaginationParams holds parsed pagination parameters
type PaginationParams struct {
	Limit  int    // Number of items to return
	Offset int    // Number of items to skip (for offset pagination)
	Cursor string // Cursor for cursor-based pagination
	Sort   string // Sort field
	Order  string // Sort order (asc/desc)
}

// PaginatedResponse is a generic paginated response wrapper
type PaginatedResponse struct {
	Data       interface{}     `json:"data"`
	Pagination PaginationMeta  `json:"pagination"`
}

// PaginationMeta contains pagination metadata
type PaginationMeta struct {
	Total      int    `json:"total"`                 // Total number of items (if known)
	Limit      int    `json:"limit"`                 // Items per page
	Offset     int    `json:"offset,omitempty"`      // Current offset (offset pagination)
	NextCursor string `json:"next_cursor,omitempty"` // Cursor for next page
	PrevCursor string `json:"prev_cursor,omitempty"` // Cursor for previous page
	HasMore    bool   `json:"has_more"`              // Whether there are more items
}

// CursorData holds encoded cursor information
type CursorData struct {
	ID        string    `json:"id,omitempty"`
	Timestamp time.Time `json:"ts,omitempty"`
	Offset    int       `json:"off,omitempty"`
	Sort      string    `json:"s,omitempty"`
}

// ParsePaginationParams extracts pagination parameters from request
func ParsePaginationParams(r *http.Request) PaginationParams {
	q := r.URL.Query()

	params := PaginationParams{
		Limit:  DefaultPageSize,
		Offset: 0,
		Cursor: q.Get("cursor"),
		Sort:   q.Get("sort"),
		Order:  strings.ToLower(q.Get("order")),
	}

	// Parse limit
	if limitStr := q.Get("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil && limit > 0 {
			params.Limit = limit
		}
	}
	// Also support "page_size" parameter
	if pageSizeStr := q.Get("page_size"); pageSizeStr != "" {
		if pageSize, err := strconv.Atoi(pageSizeStr); err == nil && pageSize > 0 {
			params.Limit = pageSize
		}
	}

	// Cap limit to max
	if params.Limit > MaxPageSize {
		params.Limit = MaxPageSize
	}

	// Parse offset
	if offsetStr := q.Get("offset"); offsetStr != "" {
		if offset, err := strconv.Atoi(offsetStr); err == nil && offset >= 0 {
			params.Offset = offset
		}
	}
	// Also support "page" parameter (1-indexed)
	if pageStr := q.Get("page"); pageStr != "" {
		if page, err := strconv.Atoi(pageStr); err == nil && page > 0 {
			params.Offset = (page - 1) * params.Limit
		}
	}

	// Validate sort order
	if params.Order != "asc" && params.Order != "desc" {
		params.Order = "desc" // Default to descending (most recent first)
	}

	return params
}

// EncodeCursor encodes cursor data to a string
func EncodeCursor(data CursorData) string {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(jsonData)
}

// DecodeCursor decodes a cursor string to cursor data
func DecodeCursor(cursor string) (*CursorData, error) {
	if cursor == "" {
		return nil, nil
	}

	jsonData, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return nil, fmt.Errorf("invalid cursor encoding")
	}

	var data CursorData
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return nil, fmt.Errorf("invalid cursor data")
	}

	return &data, nil
}

// NewPaginatedResponse creates a paginated response
func NewPaginatedResponse(data interface{}, total, limit, offset int, hasMore bool) *PaginatedResponse {
	return &PaginatedResponse{
		Data: data,
		Pagination: PaginationMeta{
			Total:   total,
			Limit:   limit,
			Offset:  offset,
			HasMore: hasMore,
		},
	}
}

// NewCursorPaginatedResponse creates a cursor-based paginated response
func NewCursorPaginatedResponse(data interface{}, total, limit int, nextCursor, prevCursor string, hasMore bool) *PaginatedResponse {
	return &PaginatedResponse{
		Data: data,
		Pagination: PaginationMeta{
			Total:      total,
			Limit:      limit,
			NextCursor: nextCursor,
			PrevCursor: prevCursor,
			HasMore:    hasMore,
		},
	}
}

// WritePaginatedJSON writes a paginated JSON response
func WritePaginatedJSON(w http.ResponseWriter, data interface{}, total, limit, offset int, hasMore bool) {
	resp := NewPaginatedResponse(data, total, limit, offset, hasMore)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// PaginateSlice applies pagination to an in-memory slice
// Returns the paginated slice and metadata
func PaginateSlice[T any](items []T, params PaginationParams) ([]T, PaginationMeta) {
	total := len(items)

	// Apply offset
	start := params.Offset
	if start > total {
		start = total
	}

	// Apply limit
	end := start + params.Limit
	if end > total {
		end = total
	}

	hasMore := end < total

	return items[start:end], PaginationMeta{
		Total:   total,
		Limit:   params.Limit,
		Offset:  params.Offset,
		HasMore: hasMore,
	}
}

// SQLPaginationClause returns SQL LIMIT/OFFSET clause
func SQLPaginationClause(params PaginationParams) string {
	return fmt.Sprintf("LIMIT %d OFFSET %d", params.Limit, params.Offset)
}

// SQLOrderClause returns SQL ORDER BY clause
func SQLOrderClause(params PaginationParams, allowedFields map[string]string) string {
	// Map requested sort field to actual column
	column := "created_at" // default
	if params.Sort != "" {
		if col, ok := allowedFields[params.Sort]; ok {
			column = col
		}
	}

	order := "DESC"
	if params.Order == "asc" {
		order = "ASC"
	}

	return fmt.Sprintf("ORDER BY %s %s", column, order)
}

// LinkHeader builds a Link header for pagination (RFC 5988)
func LinkHeader(baseURL string, params PaginationParams, total int) string {
	var links []string

	// Calculate pages
	currentPage := (params.Offset / params.Limit) + 1
	totalPages := (total + params.Limit - 1) / params.Limit

	// First page
	links = append(links, fmt.Sprintf(`<%s?page=1&limit=%d>; rel="first"`, baseURL, params.Limit))

	// Previous page
	if currentPage > 1 {
		links = append(links, fmt.Sprintf(`<%s?page=%d&limit=%d>; rel="prev"`, baseURL, currentPage-1, params.Limit))
	}

	// Next page
	if currentPage < totalPages {
		links = append(links, fmt.Sprintf(`<%s?page=%d&limit=%d>; rel="next"`, baseURL, currentPage+1, params.Limit))
	}

	// Last page
	if totalPages > 0 {
		links = append(links, fmt.Sprintf(`<%s?page=%d&limit=%d>; rel="last"`, baseURL, totalPages, params.Limit))
	}

	return strings.Join(links, ", ")
}

// SetPaginationHeaders sets pagination-related HTTP headers
func SetPaginationHeaders(w http.ResponseWriter, baseURL string, params PaginationParams, total int) {
	w.Header().Set("X-Total-Count", strconv.Itoa(total))
	w.Header().Set("X-Page-Size", strconv.Itoa(params.Limit))

	if total > 0 {
		currentPage := (params.Offset / params.Limit) + 1
		totalPages := (total + params.Limit - 1) / params.Limit
		w.Header().Set("X-Page", strconv.Itoa(currentPage))
		w.Header().Set("X-Total-Pages", strconv.Itoa(totalPages))
	}

	if baseURL != "" {
		w.Header().Set("Link", LinkHeader(baseURL, params, total))
	}
}
