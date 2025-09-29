package datastore

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/phillarmonic/syncopate-db/internal/common"
	"github.com/phillarmonic/syncopate-db/internal/settings"
)

// normalizeForJoinComparison normalizes values for consistent comparison in joins
// This helps avoid type mismatches between string IDs and numeric values
func (qs *QueryService) normalizeForJoinComparison(value interface{}) interface{} {
	// Debug logging that respects global settings
	debug := func(format string, args ...interface{}) {
		if settings.Config.Debug {
			fmt.Printf("[JOIN DEBUG] "+format+"\n", args...)
		}
	}

	debug("Normalizing value %v of type %T", value, value)

	// Handle string values that might represent numbers
	if strValue, ok := value.(string); ok {
		// Try to convert to int for comparison with numeric fields
		if intValue, err := strconv.Atoi(strValue); err == nil {
			debug("Converted string '%s' to int %d", strValue, intValue)
			return intValue
		}
	}

	// Handle auto-increment IDs which might be stored as strings
	// This is critical for joins between numeric foreign keys and string IDs
	if strValue, ok := value.(string); ok {
		// If it looks like an integer ID (all digits), try to convert
		if match, _ := regexp.MatchString(`^\d+$`, strValue); match {
			if intValue, err := strconv.Atoi(strValue); err == nil {
				debug("Converted ID string '%s' to int %d", strValue, intValue)
				return intValue
			}
		}
	}

	// Convert numeric types to a consistent format
	switch v := value.(type) {
	case int:
		return v
	case int8:
		debug("Converted int8 %d to int %d", v, int(v))
		return int(v)
	case int16:
		debug("Converted int16 %d to int %d", v, int(v))
		return int(v)
	case int32:
		debug("Converted int32 %d to int %d", v, int(v))
		return int(v)
	case int64:
		debug("Converted int64 %d to int %d", v, int(v))
		return int(v)
	case uint:
		debug("Converted uint %d to int %d", v, int(v))
		return int(v)
	case uint8:
		debug("Converted uint8 %d to int %d", v, int(v))
		return int(v)
	case uint16:
		debug("Converted uint16 %d to int %d", v, int(v))
		return int(v)
	case uint32:
		debug("Converted uint32 %d to int %d", v, int(v))
		return int(v)
	case uint64:
		if v <= uint64(^uint(0)>>1) {
			debug("Converted uint64 %d to int %d", v, int(v))
			return int(v)
		}
	case float32:
		debug("Converted float32 %f to float64 %f", v, float64(v))
		return float64(v)
	case float64:
		// If it's a whole number float, convert to int for better matching
		if v == float64(int(v)) {
			debug("Converted whole number float64 %f to int %d", v, int(v))
			return int(v)
		}
		return v
	}

	// For other types, return as is
	debug("Keeping value %v as type %T", value, value)
	return value
}

// executeJoin performs a join operation between the main entities and a target entity type
func (qs *QueryService) executeJoin(entities []common.Entity, join JoinOptions) error {
	// Use proper debug logging that respects the global debug setting
	logDebug := func(format string, args ...interface{}) {
		// Only log if debug mode is enabled in settings
		if settings.Config.Debug {
			fmt.Printf("[JOIN DEBUG] "+format+"\n", args...)
		}
	}

	// Handle backward compatibility and set defaults
	joinType := join.JoinType
	if joinType == "" {
		joinType = join.Type // Legacy field
	}
	if joinType == "" {
		joinType = JoinTypeInner // Default to inner join
	}

	resultField := join.ResultField
	if resultField == "" {
		resultField = join.As // Legacy field
	}
	if resultField == "" {
		resultField = join.EntityType // Default to entity type name
	}

	// Default select strategy is "first" if not specified
	if join.SelectStrategy == "" {
		join.SelectStrategy = "first"
	}

	logDebug("Starting join: %s -> %s (local: %s, foreign: %s, result: %s, type: %s)",
		join.EntityType, join.ForeignField, join.LocalField, resultField, joinType)

	// Execute a query to get the target entities
	targetOpts := QueryOptions{
		EntityType: join.EntityType,
		Filters:    join.Filters,
		Limit:      0, // No limit for joins
	}

	logDebug("Executing query for target entities of type: %s", join.EntityType)
	targetEntities, err := qs.Query(targetOpts)
	if err != nil {
		logDebug("Error querying join target entities: %v", err)
		return fmt.Errorf("error querying join target entities: %w", err)
	}
	logDebug("Found %d target entities", len(targetEntities))

	// Create a map for quick lookups
	targetMap := make(map[interface{}][]common.Entity)
	for _, entity := range targetEntities {
		foreignValue, exists := entity.Fields[join.ForeignField]
		if !exists {
			// If ForeignField is "id", try using the entity ID directly
			if join.ForeignField == "id" {
				foreignValue = entity.ID
				logDebug("Using entity ID as foreign field for entity %s", entity.ID)
			} else {
				logDebug("Foreign field '%s' not found in entity %s", join.ForeignField, entity.ID)
				continue
			}
		}

		// Normalize the foreign value for consistent comparison
		foreignKey := qs.normalizeForJoinComparison(foreignValue)
		logDebug("Normalized foreign key from %v to %v for entity %s", foreignValue, foreignKey, entity.ID)
		targetMap[foreignKey] = append(targetMap[foreignKey], entity)
	}

	logDebug("Built target map with %d unique keys", len(targetMap))

	// Create a temporary map to hold join results
	joinResults := make([]map[string]interface{}, len(entities))
	excludedEntities := make([]bool, len(entities))

	// Process each main entity
	matchCount := 0
	noValueCount := 0
	noMatchCount := 0

	for i := range entities {
		// Initialize the join results map for this entity
		joinResults[i] = make(map[string]interface{})

		localValue, exists := entities[i].Fields[join.LocalField]
		if !exists {
			logDebug("Local field '%s' not found in entity %s", join.LocalField, entities[i].ID)
			noValueCount++
			if joinType == JoinTypeInner {
				// For inner joins, mark entities that don't have the join field for exclusion
				excludedEntities[i] = true
				logDebug("Marking entity %s for exclusion (inner join, missing local field)", entities[i].ID)
			}
			continue
		}

		// Debug the local value
		logDebug("Entity '%s' has local value '%v' (type: %T) for field '%s'",
			entities[i].ID, localValue, localValue, join.LocalField)

		// Normalize the local value for consistent comparison
		normalizedLocalValue := qs.normalizeForJoinComparison(localValue)
		logDebug("Normalized local value from %v to %v for entity %s",
			localValue, normalizedLocalValue, entities[i].ID)

		// Get matching target entities
		matches, found := targetMap[normalizedLocalValue]
		if !found || len(matches) == 0 {
			logDebug("No matches found for entity %s with normalized local value %v",
				entities[i].ID, normalizedLocalValue)
			noMatchCount++
			if joinType == JoinTypeInner {
				// For inner joins, mark entities that don't have matches for exclusion
				excludedEntities[i] = true
				logDebug("Marking entity %s for exclusion (inner join, no matches)", entities[i].ID)
			}
			continue
		}

		logDebug("Found %d matches for entity %s with normalized local value %v",
			len(matches), entities[i].ID, normalizedLocalValue)
		matchCount++

		// Process matches based on the select strategy
		switch join.SelectStrategy {
		case "first":
			// Just select the first match
			logDebug("Using 'first' strategy: selecting first match for entity %s", entities[i].ID)
			joinResults[i][resultField] = qs.filterJoinFields(matches[0], join.IncludeFields, join.ExcludeFields)
		case "all":
			// Select all matches
			logDebug("Using 'all' strategy: selecting all %d matches for entity %s", len(matches), entities[i].ID)
			joinedEntities := make([]map[string]interface{}, len(matches))
			for j, match := range matches {
				joinedEntities[j] = qs.filterJoinFields(match, join.IncludeFields, join.ExcludeFields)
			}
			joinResults[i][resultField] = joinedEntities
		}
	}

	logDebug("Join summary: %d entities processed, %d matches found, %d with no local value, %d with no matches",
		len(entities), matchCount, noValueCount, noMatchCount)

	// For inner joins, create a new filtered list
	if joinType == JoinTypeInner {
		initialCount := len(entities)
		finalEntities := make([]common.Entity, 0, len(entities))

		for i, entity := range entities {
			if !excludedEntities[i] {
				// Create a deep copy of the entity to avoid modifying the original
				entityCopy := common.Entity{
					ID:     entity.ID,
					Type:   entity.Type,
					Fields: make(map[string]interface{}),
				}

				// Copy all fields
				for k, v := range entity.Fields {
					entityCopy.Fields[k] = v
				}

				// Now add join data to our copy
				for k, v := range joinResults[i] {
					entityCopy.Fields[k] = v
				}

				finalEntities = append(finalEntities, entityCopy)
			}
		}

		// Replace the original slice with our filtered copies
		for i := 0; i < len(finalEntities) && i < len(entities); i++ {
			entities[i] = finalEntities[i]
		}

		// Update the slice length by re-slicing
		if len(finalEntities) < len(entities) {
			// Clear the remaining elements to avoid memory leaks
			for i := len(finalEntities); i < len(entities); i++ {
				entities[i] = common.Entity{}
			}
		}

		logDebug("Inner join: filtered from %d to %d entities", initialCount, len(finalEntities))
	} else {
		// For outer joins, apply the join results to copies of the original entities
		for i := range entities {
			// Create a deep copy of the entity
			entityCopy := common.Entity{
				ID:     entities[i].ID,
				Type:   entities[i].Type,
				Fields: make(map[string]interface{}),
			}

			// Copy all fields
			for k, v := range entities[i].Fields {
				entityCopy.Fields[k] = v
			}

			// Add join data to our copy
			for k, v := range joinResults[i] {
				entityCopy.Fields[k] = v
			}

			// Replace the original entity with our modified copy
			entities[i] = entityCopy
		}
	}

	return nil
}

// filterJoinFields creates a filtered map of entity fields based on include/exclude lists
func (qs *QueryService) filterJoinFields(entity common.Entity, includeFields, excludeFields []string) map[string]interface{} {
	result := make(map[string]interface{})

	// Copy ID by default
	result["id"] = entity.ID

	// If include fields is empty, include all fields
	if len(includeFields) == 0 {
		// Copy all fields except those in exclude list
		for key, value := range entity.Fields {
			if !containsString(excludeFields, key) && !strings.HasPrefix(key, "_") {
				result[key] = value
			}
		}
	} else {
		// Only copy fields in the include list
		for _, field := range includeFields {
			if value, exists := entity.Fields[field]; exists && !containsString(excludeFields, field) {
				result[field] = value
			}
		}
	}

	// Convert ID for auto_increment entity types
	// Get entity definition to check ID generator type
	def, err := qs.engine.GetEntityDefinition(entity.Type)
	if err == nil && def.IDGenerator == common.IDTypeAutoIncrement {
		// Try to convert ID to int
		if id, err := strconv.Atoi(entity.ID); err == nil {
			result["id"] = id
		}
	}

	return result
}

// containsString checks if a string is in a slice
func containsString(slice []string, str string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}
