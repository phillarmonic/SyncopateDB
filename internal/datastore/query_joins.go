package datastore

import (
	"fmt"
	"github.com/phillarmonic/syncopate-db/internal/common"
	"github.com/sirupsen/logrus"
	"regexp"
	"strconv"
	"strings"
)

// normalizeForJoinComparison normalizes values for consistent comparison in joins
// This helps avoid type mismatches between string IDs and numeric values
func (qs *QueryService) normalizeForJoinComparison(value interface{}) interface{} {
	// Simple debug logging for development
	debugEnabled := false // Set to true to enable logs
	debug := func(format string, args ...interface{}) {
		if debugEnabled {
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
	// Use direct debug logging
	var logger *logrus.Logger

	// We can create a new logger for debugging
	logger = logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	// Optionally set a specific formatter for debugging
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	// Log debugging info if we have a logger
	logDebug := func(format string, args ...interface{}) {
		fmt.Printf("[JOIN DEBUG] "+format+"\n", args...)
	}

	// The default join type is inner if not specified
	if join.Type == "" {
		join.Type = "inner"
	}

	// Default select strategy is "first" if not specified
	if join.SelectStrategy == "" {
		join.SelectStrategy = "first"
	}

	logDebug("Starting join: %s -> %s (local: %s, foreign: %s, as: %s)",
		join.EntityType, join.ForeignField, join.LocalField, join.As)

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
				logDebug("Foreign field %s not found in entity %s", join.ForeignField, entity.ID)
				continue
			}
		}

		// Normalize the foreign value for consistent comparison
		foreignKey := qs.normalizeForJoinComparison(foreignValue)
		logDebug("Normalized foreign key from %v to %v for entity %s", foreignValue, foreignKey, entity.ID)
		targetMap[foreignKey] = append(targetMap[foreignKey], entity)
	}

	logDebug("Built target map with %d unique keys", len(targetMap))

	// Process each main entity
	matchCount := 0
	noValueCount := 0
	noMatchCount := 0

	for i := range entities {
		localValue, exists := entities[i].Fields[join.LocalField]
		if !exists {
			logDebug("Local field %s not found in entity %s", join.LocalField, entities[i].ID)
			noValueCount++
			if join.Type == "inner" {
				// For inner joins, remove entities that don't have the join field
				entities[i].Fields["_excluded"] = true
				logDebug("Marking entity %s for exclusion (inner join, missing local field)", entities[i].ID)
			}
			continue
		}

		// Debug the local value
		logDebug("Entity %s has local value %v (type: %T) for field %s",
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
			if join.Type == "inner" {
				// For inner joins, remove entities that don't have matches
				entities[i].Fields["_excluded"] = true
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
			entities[i].Fields[join.As] = qs.filterJoinFields(matches[0], join.IncludeFields, join.ExcludeFields)
		case "all":
			// Select all matches
			logDebug("Using 'all' strategy: selecting all %d matches for entity %s", len(matches), entities[i].ID)
			joinedEntities := make([]map[string]interface{}, len(matches))
			for j, match := range matches {
				joinedEntities[j] = qs.filterJoinFields(match, join.IncludeFields, join.ExcludeFields)
			}
			entities[i].Fields[join.As] = joinedEntities
		}
	}

	logDebug("Join summary: %d entities processed, %d matches found, %d with no local value, %d with no matches",
		len(entities), matchCount, noValueCount, noMatchCount)

	// For inner joins, remove the excluded entities
	if join.Type == "inner" {
		initialCount := len(entities)
		filteredEntities := make([]common.Entity, 0, len(entities))
		for _, entity := range entities {
			if _, excluded := entity.Fields["_excluded"]; !excluded {
				filteredEntities = append(filteredEntities, entity)
			}
		}
		// Update the original slice (possible because we're modifying the original slice)
		copy(entities, filteredEntities)
		// Truncate the slice to the new length
		finalCount := len(filteredEntities)
		entities = entities[:finalCount]
		logDebug("Inner join: filtered from %d to %d entities", initialCount, finalCount)
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
