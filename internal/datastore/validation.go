package datastore

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/phillarmonic/syncopate-db/internal/common"
)

// validateEntityData validates entity data against its definition
func (dse *Engine) validateEntityData(entityType string, data map[string]interface{}) error {
	def, exists := dse.definitions[entityType]
	if !exists {
		return fmt.Errorf("entity type %s not registered", entityType)
	}

	// Check for fields that start with underscore (reserved for internal use)
	for fieldName := range data {
		// Skip validation for internal fields that are added by the system
		// We can identify these by checking if they're in the entity definition with Internal=true
		isInternalField := false
		for _, fieldDef := range def.Fields {
			if fieldDef.Name == fieldName && fieldDef.Internal {
				isInternalField = true
				break
			}
		}

		if !isInternalField && strings.HasPrefix(fieldName, "_") {
			return fmt.Errorf("field name '%s' is not allowed: names starting with underscore are reserved for internal use", fieldName)
		}
	}

	// Check for required fields and type validation
	for _, fieldDef := range def.Fields {
		value, exists := data[fieldDef.Name]

		if fieldDef.Required && !exists {
			return fmt.Errorf("required field '%s' is missing", fieldDef.Name)
		}

		if exists {
			// Check for null value
			if value == nil {
				if !fieldDef.Nullable {
					return fmt.Errorf("field '%s' cannot be null", fieldDef.Name)
				}
				continue // Skip type validation for null values
			}

			if err := validateFieldType(fieldDef.Type, value); err != nil {
				return fmt.Errorf("field '%s': %w", fieldDef.Name, err)
			}
		}
	}

	// Validate uniqueness constraints (pass empty string for entityID as this is a new entity)
	if err := dse.validateUniqueness(entityType, data, ""); err != nil {
		return err
	}

	return nil
}

// validateFieldType validates that a value matches the expected type
func validateFieldType(fieldType string, value interface{}) error {
	if value == nil {
		return nil // Allowing nil values if the field is nullable
	}

	switch fieldType {
	case TypeBoolean:
		if _, ok := value.(bool); !ok {
			return errors.New("value is not a boolean")
		}
	case TypeDate, TypeDateTime:
		switch v := value.(type) {
		case time.Time:
			// Valid time.Time
		case string:
			// Try to parse string as time
			_, err := time.Parse(time.RFC3339, v)
			if err != nil {
				return errors.New("invalid date/time format")
			}
		default:
			return errors.New("value is not a valid date/time")
		}
	case TypeString, TypeText:
		if _, ok := value.(string); !ok {
			return errors.New("value is not a string")
		}
	case TypeJSON:
		switch v := value.(type) {
		case string:
			// Try to parse as JSON
			var js interface{}
			if err := json.Unmarshal([]byte(v), &js); err != nil {
				return errors.New("invalid JSON format")
			}
		case map[string]interface{}, []interface{}:
			// Already a parsed JSON structure
		default:
			return errors.New("value is not a valid JSON")
		}
	case TypeInteger:
		switch reflect.TypeOf(value).Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			// Valid integer type
		default:
			if f, ok := value.(float64); ok {
				// JSON numbers are decoded as float64, check if it's an integer
				if f == float64(int(f)) {
					// It's an integer value
					return nil
				}
			}
			return errors.New("value is not an integer")
		}
	case TypeFloat:
		switch reflect.TypeOf(value).Kind() {
		case reflect.Float32, reflect.Float64:
			// Valid float type
		default:
			// Check if integer that can be treated as float
			switch reflect.TypeOf(value).Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				return nil
			}
			return errors.New("value is not a float")
		}
	case TypeArray:
		if _, ok := value.([]interface{}); !ok {
			return errors.New("value is not an array")
		}
	case TypeObject:
		if _, ok := value.(map[string]interface{}); !ok {
			return errors.New("value is not an object")
		}
	default:
		return fmt.Errorf("unsupported field type: %s", fieldType)
	}

	return nil
}

// On update of entity data, validate the data against the defined schema.
func (dse *Engine) validateUpdateData(entityType string, entityID string, data map[string]interface{}) error {
	def, exists := dse.definitions[entityType]
	if !exists {
		return fmt.Errorf("entity type '%s' not registered", entityType)
	}

	// Check for fields that start with underscore (reserved for internal use)
	for fieldName := range data {
		// Skip validation for internal fields that are added by the system
		// We can identify these by checking if they're in the entity definition with Internal=true
		isInternalField := false
		for _, fieldDef := range def.Fields {
			if fieldDef.Name == fieldName && fieldDef.Internal {
				isInternalField = true
				break
			}
		}

		if !isInternalField && strings.HasPrefix(fieldName, "_") {
			return fmt.Errorf("field name '%s' is not allowed: names starting with underscore are reserved for internal use", fieldName)
		}
	}

	// Only validate types of fields being updated
	for fieldName, value := range data {
		// Find field definition
		var fieldDef *common.FieldDefinition
		for i := range def.Fields {
			if def.Fields[i].Name == fieldName {
				fieldDef = &def.Fields[i]
				break
			}
		}

		if fieldDef == nil {
			return fmt.Errorf("field '%s' does not exist in entity type %s", fieldName, entityType)
		}

		// Check for null value
		if value == nil {
			if !fieldDef.Nullable {
				return fmt.Errorf("field '%s' cannot be null", fieldName)
			}
			continue // Skip type validation for null values
		}

		// Validate field type
		if err := validateFieldType(fieldDef.Type, value); err != nil {
			return fmt.Errorf("field '%s': %w", fieldName, err)
		}
	}

	// Check uniqueness for updates
	// For updates, we need to check uniqueness but exclude the entity being updated
	if err := dse.validateUniqueness(entityType, data, entityID); err != nil {
		return err
	}

	return nil
}

// ValidateEntityTypeFields validates field definitions in an entity type
// to ensure no field name starts with an underscore (reserved for internal use)
// The skipInternal parameter allows internal fields to bypass this validation
func ValidateEntityTypeFields(fields []common.FieldDefinition, skipInternal bool) error {
	for _, field := range fields {
		// Skip validation for fields already marked as internal
		if field.Internal {
			continue
		}

		// If a field starts with underscore, mark it as internal and skip validation
		if strings.HasPrefix(field.Name, "_") {
			if skipInternal {
				continue
			}
			return fmt.Errorf("field name '%s' is not allowed: names starting with underscore are reserved for internal use", field.Name)
		}
	}
	return nil
}
