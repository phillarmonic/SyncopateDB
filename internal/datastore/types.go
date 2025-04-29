package datastore

import "github.com/phillarmonic/syncopate-db/internal/common"

// Field types supported by the data store
const (
	TypeBoolean  = "boolean"
	TypeDate     = "date"
	TypeDateTime = "datetime"
	TypeString   = "string"
	TypeText     = "text"
	TypeJSON     = "json"
	TypeInteger  = "integer"
	TypeFloat    = "float"
)

// Filter types for queries
const (
	FilterEq               = "eq"
	FilterNeq              = "neq"
	FilterGt               = "gt"
	FilterGte              = "gte"
	FilterLt               = "lt"
	FilterLte              = "lte"
	FilterContains         = "contains"
	FilterStartsWith       = "startswith"
	FilterEndsWith         = "endswith"
	FilterIn               = "in"
	FilterFuzzy            = "fuzzy"
	FilterArrayContains    = "array_contains"     // Check if array contains a specific value
	FilterArrayContainsAny = "array_contains_any" // Check if array contains any of the specified values
	FilterArrayContainsAll = "array_contains_all" // Check if array contains all the specified values
)

// QueryOptions defines parameters for running a query
type QueryOptions struct {
	EntityType string              `json:"entityType"`
	Filters    []QueryFilter       `json:"filters"`
	Limit      int                 `json:"limit"`
	Offset     int                 `json:"offset"`
	OrderBy    string              `json:"orderBy"`
	OrderDesc  bool                `json:"orderDesc"`
	FuzzyOpts  *FuzzySearchOptions `json:"fuzzyOpts,omitempty"`
}

// QueryFilter represents a filter condition
type QueryFilter struct {
	Field    string      `json:"field"`
	Operator string      `json:"operator"`
	Value    interface{} `json:"value"`
}

// FuzzySearchOptions defines parameters for fuzzy searching
type FuzzySearchOptions struct {
	Threshold   float64 `json:"threshold"`   // Similarity threshold (0.0-1.0)
	MaxDistance int     `json:"maxDistance"` // Maximum edit distance for Levenshtein
}

// PaginatedResponse represents a paginated result of entities
type PaginatedResponse struct {
	Total      int             `json:"total"`
	Count      int             `json:"count"`
	Limit      int             `json:"limit"`
	Offset     int             `json:"offset"`
	HasMore    bool            `json:"hasMore"`
	EntityType string          `json:"entityType"`
	Data       []common.Entity `json:"data"`
}
