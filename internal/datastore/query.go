package datastore

import (
	"errors"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/phillarmonic/syncopate-db/internal/common"
)

// QueryService handles the querying logic for the data store
type QueryService struct {
	engine *Engine
}

// NewQueryService creates a new query service
func NewQueryService(engine *Engine) *QueryService {
	return &QueryService{
		engine: engine,
	}
}

// Query executes a query against the data store
func (qs *QueryService) Query(options QueryOptions) ([]common.Entity, error) {
	qs.engine.mu.RLock()
	defer qs.engine.mu.RUnlock()

	// Verify entity type
	if _, exists := qs.engine.definitions[options.EntityType]; !exists {
		return nil, errors.New("entity type not registered")
	}

	// Start with all entities of the specified type
	matchingEntities := make([]common.Entity, 0)
	for _, entity := range qs.engine.entities {
		if entity.Type == options.EntityType {
			matchingEntities = append(matchingEntities, entity)
		}
	}

	// Apply filters
	for _, f := range options.Filters {
		// Check if we can use an index for this filter
		def := qs.engine.definitions[options.EntityType]
		isIndexed := false
		for _, fieldDef := range def.Fields {
			if fieldDef.Name == f.Field && fieldDef.Indexed {
				isIndexed = true
				break
			}
		}

		if isIndexed && f.Operator == FilterEq {
			// Use index for equality checks
			strValue := qs.engine.getIndexableValue(f.Value)
			indexedIDs := qs.engine.indices[options.EntityType][f.Field][strValue]

			// Create a map for efficient lookup
			idMap := make(map[string]bool)
			for _, id := range indexedIDs {
				idMap[id] = true
			}

			// Filter entities using the index
			filteredEntities := make([]common.Entity, 0)
			for _, entity := range matchingEntities {
				if idMap[entity.ID] {
					filteredEntities = append(filteredEntities, entity)
				}
			}
			matchingEntities = filteredEntities
		} else if f.Operator == FilterFuzzy {
			// Handle fuzzy search separately
			filteredEntities := make([]common.Entity, 0)
			threshold := 0.7 // Default threshold
			maxDistance := 3 // Default max distance

			if options.FuzzyOpts != nil {
				threshold = options.FuzzyOpts.Threshold
				maxDistance = options.FuzzyOpts.MaxDistance
			}

			searchStr, ok := f.Value.(string)
			if !ok {
				return nil, errors.New("fuzzy search value must be a string")
			}

			for _, entity := range matchingEntities {
				value, exists := entity.Fields[f.Field]
				if !exists {
					continue
				}

				fieldStr, ok := value.(string)
				if !ok {
					continue
				}

				// Use fuzzy matching algorithm
				if qs.fuzzyMatch(fieldStr, searchStr, threshold, maxDistance) {
					filteredEntities = append(filteredEntities, entity)
				}
			}

			matchingEntities = filteredEntities
		} else {
			// No index or non-equality operator, filter manually
			filteredEntities := make([]common.Entity, 0)

			for _, entity := range matchingEntities {
				value, exists := entity.Fields[f.Field]

				if !exists {
					continue
				}

				if qs.matchesFilter(value, f.Operator, f.Value) {
					filteredEntities = append(filteredEntities, entity)
				}
			}

			matchingEntities = filteredEntities
		}
	}

	// Sort results if needed
	if options.OrderBy != "" {
		// Sort the entities
		qs.sortEntities(matchingEntities, options.OrderBy, options.OrderDesc)
	}

	// Apply offset and limit
	if options.Offset >= len(matchingEntities) {
		return []common.Entity{}, nil
	}

	end := len(matchingEntities)
	if options.Limit > 0 && options.Offset+options.Limit < end {
		end = options.Offset + options.Limit
	}

	return matchingEntities[options.Offset:end], nil
}

// Levenshtein calculates the Levenshtein distance between two strings
func (qs *QueryService) levenshteinDistance(s1, s2 string) int {
	s1 = strings.ToLower(s1)
	s2 = strings.ToLower(s2)

	if len(s1) == 0 {
		return len(s2)
	}
	if len(s2) == 0 {
		return len(s1)
	}

	// Create two work vectors of integer distances
	v0 := make([]int, len(s2)+1)
	v1 := make([]int, len(s2)+1)

	// Initialize v0 (the previous row of distances)
	for i := 0; i <= len(s2); i++ {
		v0[i] = i
	}

	// Calculate each row of the matrix
	for i := 0; i < len(s1); i++ {
		// First element of v1 is always i+1
		v1[0] = i + 1

		// Calculate the rest of the row
		for j := 0; j < len(s2); j++ {
			deletionCost := v0[j+1] + 1
			insertionCost := v1[j] + 1

			substitutionCost := v0[j]
			if s1[i] != s2[j] {
				substitutionCost++
			}

			v1[j+1] = min(deletionCost, min(insertionCost, substitutionCost))
		}

		// Swap v0 and v1
		v0, v1 = v1, v0
	}

	// The last element of v0 contains the answer
	return v0[len(s2)]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// fuzzyMatch determines if two strings match within a threshold using Levenshtein distance
func (qs *QueryService) fuzzyMatch(s1, s2 string, threshold float64, maxDistance int) bool {
	s1 = strings.ToLower(s1)
	s2 = strings.ToLower(s2)

	// Check for contains for better results
	if strings.Contains(s1, s2) || strings.Contains(s2, s1) {
		return true
	}

	// Check word by word for multi-word strings
	words1 := strings.Fields(s1)
	words2 := strings.Fields(s2)

	if len(words1) > 1 || len(words2) > 1 {
		// For multi-word strings, check if enough words match
		matches := 0
		for _, w1 := range words1 {
			for _, w2 := range words2 {
				distance := qs.levenshteinDistance(w1, w2)
				maxLen := max(len(w1), len(w2))
				if maxLen > 0 {
					similarity := 1.0 - float64(distance)/float64(maxLen)
					if similarity >= threshold {
						matches++
						break
					}
				}
			}
		}

		requiredMatches := min(len(words1), len(words2)) / 2
		if requiredMatches == 0 {
			requiredMatches = 1
		}

		return matches >= requiredMatches
	}

	// For single words, use Levenshtein distance directly
	distance := qs.levenshteinDistance(s1, s2)
	if distance > maxDistance {
		return false
	}

	maxLen := max(len(s1), len(s2))
	if maxLen == 0 {
		return true // Both strings are empty
	}

	similarity := 1.0 - float64(distance)/float64(maxLen)
	return similarity >= threshold
}

// matchesFilter checks if a value matches a filter condition
func (qs *QueryService) matchesFilter(value interface{}, operator string, filterValue interface{}) bool {
	switch operator {
	case FilterEq:
		return reflect.DeepEqual(value, filterValue)

	case FilterNeq:
		return !reflect.DeepEqual(value, filterValue)

	case FilterGt, FilterGte, FilterLt, FilterLte:
		return qs.compareValues(value, operator, filterValue)

	case FilterContains:
		// Handle arrays - check if the array contains the filter value
		if valueArr, ok := value.([]interface{}); ok {
			for _, v := range valueArr {
				// For string array elements, try string contains check
				if strItem, ok := v.(string); ok {
					if strFilter, ok := filterValue.(string); ok {
						if strings.Contains(strings.ToLower(strItem), strings.ToLower(strFilter)) {
							return true
						}
					} else if reflect.DeepEqual(v, filterValue) {
						return true
					}
				} else if reflect.DeepEqual(v, filterValue) {
					// For non-string elements, check equality
					return true
				}
			}
			return false
		}

		// Original string contains check
		strValue, ok1 := value.(string)
		strFilter, ok2 := filterValue.(string)
		if ok1 && ok2 {
			return strings.Contains(strings.ToLower(strValue), strings.ToLower(strFilter))
		}
		return false

	case FilterStartsWith:
		strValue, ok1 := value.(string)
		strFilter, ok2 := filterValue.(string)
		if ok1 && ok2 {
			return strings.HasPrefix(strings.ToLower(strValue), strings.ToLower(strFilter))
		}
		return false

	case FilterEndsWith:
		strValue, ok1 := value.(string)
		strFilter, ok2 := filterValue.(string)
		if ok1 && ok2 {
			return strings.HasSuffix(strings.ToLower(strValue), strings.ToLower(strFilter))
		}
		return false

	case FilterIn:
		sliceFilter, ok := filterValue.([]interface{})
		if !ok {
			return false
		}

		// Check if any value in the filter slice equals the field value
		for _, v := range sliceFilter {
			if reflect.DeepEqual(value, v) {
				return true
			}
		}

		// If field value is an array, check if any element matches any filter value
		if valueArr, ok := value.([]interface{}); ok {
			for _, fieldVal := range valueArr {
				for _, filterVal := range sliceFilter {
					if reflect.DeepEqual(fieldVal, filterVal) {
						return true
					}
				}
			}
		}

		return false

	case FilterArrayContains:
		// Check if an array field contains the specific value
		valueArr, ok := value.([]interface{})
		if !ok {
			return false // Not an array field
		}

		for _, v := range valueArr {
			if reflect.DeepEqual(v, filterValue) {
				return true
			}
		}
		return false

	case FilterArrayContainsAny:
		// Check if an array field contains any of the values in the filter array
		valueArr, ok1 := value.([]interface{})
		filterArr, ok2 := filterValue.([]interface{})

		if !ok1 || !ok2 {
			return false // Either not an array field or not an array filter
		}

		for _, fieldVal := range valueArr {
			for _, filterVal := range filterArr {
				if reflect.DeepEqual(fieldVal, filterVal) {
					return true
				}
			}
		}
		return false

	case FilterArrayContainsAll:
		// Check if an array field contains all values in the filter array
		valueArr, ok1 := value.([]interface{})
		filterArr, ok2 := filterValue.([]interface{})

		if !ok1 || !ok2 {
			return false // Either not an array field or not an array filter
		}

		// For each filter value, check if it exists in the field array
		for _, filterVal := range filterArr {
			found := false
			for _, fieldVal := range valueArr {
				if reflect.DeepEqual(fieldVal, filterVal) {
					found = true
					break
				}
			}
			if !found {
				return false // One required value wasn't found
			}
		}
		return true // All values were found

	default:
		return false
	}
}

// compareValues compares two values for inequality operators
func (qs *QueryService) compareValues(left interface{}, operator string, right interface{}) bool {
	// Handle numeric comparisons
	switch lv := left.(type) {
	case int, int8, int16, int32, int64, float32, float64:
		var leftFloat, rightFloat float64

		// Convert left to float64
		switch v := left.(type) {
		case int:
			leftFloat = float64(v)
		case int8:
			leftFloat = float64(v)
		case int16:
			leftFloat = float64(v)
		case int32:
			leftFloat = float64(v)
		case int64:
			leftFloat = float64(v)
		case float32:
			leftFloat = float64(v)
		case float64:
			leftFloat = v
		}

		// Convert right to float64
		switch v := right.(type) {
		case int:
			rightFloat = float64(v)
		case int8:
			rightFloat = float64(v)
		case int16:
			rightFloat = float64(v)
		case int32:
			rightFloat = float64(v)
		case int64:
			rightFloat = float64(v)
		case float32:
			rightFloat = float64(v)
		case float64:
			rightFloat = v
		default:
			return false
		}

		switch operator {
		case FilterGt:
			return leftFloat > rightFloat
		case FilterGte:
			return leftFloat >= rightFloat
		case FilterLt:
			return leftFloat < rightFloat
		case FilterLte:
			return leftFloat <= rightFloat
		default:
			return false
		}

	case string:
		rightStr, ok := right.(string)
		if !ok {
			return false
		}

		switch operator {
		case FilterGt:
			return lv > rightStr
		case FilterGte:
			return lv >= rightStr
		case FilterLt:
			return lv < rightStr
		case FilterLte:
			return lv <= rightStr
		default:
			return false
		}

	case time.Time:
		var rightTime time.Time

		switch rt := right.(type) {
		case time.Time:
			rightTime = rt
		case string:
			parsedTime, err := time.Parse(time.RFC3339, rt)
			if err != nil {
				return false
			}
			rightTime = parsedTime
		default:
			return false
		}

		switch operator {
		case FilterGt:
			return lv.After(rightTime)
		case FilterGte:
			return lv.After(rightTime) || lv.Equal(rightTime)
		case FilterLt:
			return lv.Before(rightTime)
		case FilterLte:
			return lv.Before(rightTime) || lv.Equal(rightTime)
		default:
			return false
		}
	}

	return false
}

// sortEntities sorts a slice of entities by the specified field
func (qs *QueryService) sortEntities(entities []common.Entity, field string, descending bool) {
	sort.Slice(entities, func(i, j int) bool {
		valI, existsI := entities[i].Fields[field]
		valJ, existsJ := entities[j].Fields[field]

		// Handle cases where fields don't exist
		if !existsI && !existsJ {
			return false
		}
		if !existsI {
			return !descending
		}
		if !existsJ {
			return descending
		}

		// Compare based on type
		switch vi := valI.(type) {
		case string:
			if vj, ok := valJ.(string); ok {
				if descending {
					return vi > vj
				}
				return vi < vj
			}

		case time.Time:
			if vj, ok := valJ.(time.Time); ok {
				if descending {
					return vi.After(vj)
				}
				return vi.Before(vj)
			}

		case int, int8, int16, int32, int64, float32, float64:
			var floatI, floatJ float64

			// Convert to float64 for comparison
			switch v := valI.(type) {
			case int:
				floatI = float64(v)
			case int8:
				floatI = float64(v)
			case int16:
				floatI = float64(v)
			case int32:
				floatI = float64(v)
			case int64:
				floatI = float64(v)
			case float32:
				floatI = float64(v)
			case float64:
				floatI = v
			}

			switch v := valJ.(type) {
			case int:
				floatJ = float64(v)
			case int8:
				floatJ = float64(v)
			case int16:
				floatJ = float64(v)
			case int32:
				floatJ = float64(v)
			case int64:
				floatJ = float64(v)
			case float32:
				floatJ = float64(v)
			case float64:
				floatJ = v
			default:
				return false
			}

			if descending {
				return floatI > floatJ
			}
			return floatI < floatJ

		case bool:
			if vj, ok := valJ.(bool); ok {
				// false comes before true in ascending order
				if descending {
					return vi && !vj
				}
				return !vi && vj
			}
		}

		// Default sorting for incomparable types (should rarely happen)
		return !descending
	})
}

// ExecutePaginatedQuery executes a query and returns a paginated response
func (qs *QueryService) ExecutePaginatedQuery(options QueryOptions) (*PaginatedResponse, error) {
	// Set the default sort (internal) field if none is specified
	if options.OrderBy == "" {
		options.OrderBy = "_created_at"
		// Default to ascending order (oldest first)
		options.OrderDesc = false
	}

	// Execute the query with filters but without pagination limits
	// This gets us all matching results for accurate counting
	queryOptionsForCount := options
	queryOptionsForCount.Offset = 0
	queryOptionsForCount.Limit = 0 // No limit to get all matches for counting

	allMatchingResults, err := qs.Query(queryOptionsForCount)
	if err != nil {
		return nil, err
	}

	// Total filtered count is the number of matches after all filters are applied
	totalFilteredCount := len(allMatchingResults)

	// Now execute the actual query with pagination
	results, err := qs.Query(options)
	if err != nil {
		return nil, err
	}

	return &PaginatedResponse{
		Data:       results,
		Total:      totalFilteredCount, // Use filtered count instead of total entity count
		Count:      len(results),
		Limit:      options.Limit,
		Offset:     options.Offset,
		HasMore:    options.Offset+len(results) < totalFilteredCount,
		EntityType: options.EntityType,
	}, nil
}

// filterInternalFields removes internal fields from entity data
func (qs *QueryService) filterInternalFields(entity common.Entity) common.Entity {
	// Get the entity definition
	def, err := qs.engine.GetEntityDefinition(entity.Type)
	if err != nil {
		// If we can't get the definition, return the entity as is
		return entity
	}

	// Create a map of internal field names for quick lookup
	internalFields := make(map[string]bool)
	for _, field := range def.Fields {
		if field.Internal {
			internalFields[field.Name] = true
		}
	}

	// Create a filtered copy of the entity
	filteredEntity := common.Entity{
		ID:     entity.ID,
		Type:   entity.Type,
		Fields: make(map[string]interface{}),
	}

	// Copy only non-internal fields
	for name, value := range entity.Fields {
		if !internalFields[name] {
			filteredEntity.Fields[name] = value
		}
	}

	return filteredEntity
}
