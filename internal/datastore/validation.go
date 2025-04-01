package datastore

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"
)

// validateEntityData validates entity data against its definition
func (dse *Engine) validateEntityData(entityType string, data map[string]interface{}) error {
	def, exists := dse.definitions[entityType]
	if !exists {
		return fmt.Errorf("entity type %s not registered", entityType)
	}

	// Check for required fields and type validation
	for _, fieldDef := range def.Fields {
		value, exists := data[fieldDef.Name]

		if fieldDef.Required && !exists {
			return fmt.Errorf("required field %s is missing", fieldDef.Name)
		}

		if exists {
			if err := validateFieldType(fieldDef.Type, value); err != nil {
				return fmt.Errorf("field %s: %w", fieldDef.Name, err)
			}
		}
	}

	return nil
}

// validateFieldType validates that a value matches the expected type
func validateFieldType(fieldType string, value interface{}) error {
	if value == nil {
		return nil // Allowing nil values for all types
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
	default:
		return fmt.Errorf("unsupported field type: %s", fieldType)
	}

	return nil
}
