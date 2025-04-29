package datastore

import (
	"fmt"
	"github.com/phillarmonic/syncopate-db/internal/common"
)

// UpdateEntityType updates an existing entity definition
func (dse *Engine) UpdateEntityType(updatedDef common.EntityDefinition) error {
	// First check if entity type exists
	dse.mu.RLock()
	originalDef, exists := dse.definitions[updatedDef.Name]
	dse.mu.RUnlock()

	if !exists {
		return fmt.Errorf("entity type %s not registered", updatedDef.Name)
	}

	// Validate that no field names start with underscore (reserved for internal use)
	if err := ValidateEntityTypeFields(updatedDef.Fields, true); err != nil {
		return err
	}

	// Perform schema compatibility checks
	migrationPlan, err := dse.createMigrationPlan(originalDef, updatedDef)
	if err != nil {
		return fmt.Errorf("incompatible schema update: %w", err)
	}

	// Add internal fields to the definition
	dse.addInternalFieldDefinitions(&updatedDef)

	// Keep the original ID generator - don't allow changing it
	updatedDef.IDGenerator = originalDef.IDGenerator

	// Now acquire write lock for the update
	dse.mu.Lock()

	// Execute migration plan (transform data if needed)
	if err := dse.executeMigrationPlan(migrationPlan, originalDef, updatedDef); err != nil {
		dse.mu.Unlock()
		return fmt.Errorf("migration failed: %w", err)
	}

	// Update in-memory definition
	dse.definitions[updatedDef.Name] = updatedDef

	// Update indices for new indexed fields
	dse.updateIndicesForSchemaChange(originalDef, updatedDef)

	// Store reference to persistence provider and release lock
	persistenceProvider := dse.persistence
	dse.mu.Unlock()

	// Persist entity type update if persistence is enabled
	if persistenceProvider != nil {
		if err := persistenceProvider.UpdateEntityType(dse, updatedDef); err != nil {
			// If persistence fails, we need to roll back the in-memory changes
			dse.mu.Lock()
			dse.definitions[updatedDef.Name] = originalDef
			dse.mu.Unlock()

			return fmt.Errorf("failed to persist entity type update: %w", err)
		}
	}

	return nil
}

// Add these helper types and methods

// MigrationPlan represents the changes needed for a schema update
type MigrationPlan struct {
	EntityType      string
	AddedFields     []common.FieldDefinition
	RemovedFields   []string
	ChangedFields   []FieldChange
	PropertyChanges []PropertyChange
}

// FieldChange represents a change in field type
type FieldChange struct {
	FieldName    string
	OriginalType string
	NewType      string
}

// PropertyChange represents a change in field properties
type PropertyChange struct {
	FieldName       string
	IndexChanged    bool
	RequiredChanged bool
	NullableChanged bool
	NewIndexed      bool
	NewRequired     bool
	NewNullable     bool
}

// createMigrationPlan analyzes differences between original and updated schemas
// and creates a plan for migrating data
func (dse *Engine) createMigrationPlan(originalDef, updatedDef common.EntityDefinition) (*MigrationPlan, error) {
	plan := &MigrationPlan{
		EntityType:      originalDef.Name,
		AddedFields:     []common.FieldDefinition{},
		RemovedFields:   []string{},
		ChangedFields:   []FieldChange{},
		PropertyChanges: []PropertyChange{},
	}

	// Build maps for quick lookups
	originalFields := make(map[string]common.FieldDefinition)
	for _, field := range originalDef.Fields {
		originalFields[field.Name] = field
	}

	updatedFields := make(map[string]common.FieldDefinition)
	for _, field := range updatedDef.Fields {
		updatedFields[field.Name] = field
	}

	// Find added fields
	for name, field := range updatedFields {
		if _, exists := originalFields[name]; !exists {
			// This is a new field
			plan.AddedFields = append(plan.AddedFields, field)
		}
	}

	// Find removed fields
	for name, field := range originalFields {
		if _, exists := updatedFields[name]; !exists {
			// Skip internal fields - we never remove those
			if !field.Internal {
				plan.RemovedFields = append(plan.RemovedFields, name)
			}
		}
	}

	// Find changed fields
	for name, updatedField := range updatedFields {
		originalField, exists := originalFields[name]
		if exists {
			if originalField.Type != updatedField.Type {
				// Check if the type change is compatible
				if !isCompatibleTypeChange(originalField.Type, updatedField.Type) {
					return nil, fmt.Errorf("incompatible type change for field %s: %s to %s",
						name, originalField.Type, updatedField.Type)
				}

				plan.ChangedFields = append(plan.ChangedFields, FieldChange{
					FieldName:    name,
					OriginalType: originalField.Type,
					NewType:      updatedField.Type,
				})
			}

			// Handle other property changes (indexed, required, nullable)
			if originalField.Required != updatedField.Required ||
				originalField.Nullable != updatedField.Nullable ||
				originalField.Indexed != updatedField.Indexed {

				// Special case: can't make a field required if it wasn't before
				// (existing entities might not have this field)
				if !originalField.Required && updatedField.Required {
					return nil, fmt.Errorf("cannot make field %s required as it may not exist in all entities", name)
				}

				plan.PropertyChanges = append(plan.PropertyChanges, PropertyChange{
					FieldName:       name,
					IndexChanged:    originalField.Indexed != updatedField.Indexed,
					RequiredChanged: originalField.Required != updatedField.Required,
					NullableChanged: originalField.Nullable != updatedField.Nullable,
					NewIndexed:      updatedField.Indexed,
					NewRequired:     updatedField.Required,
					NewNullable:     updatedField.Nullable,
				})
			}
		}
	}

	return plan, nil
}

// executeMigrationPlan applies the migration plan to all entities of the type
func (dse *Engine) executeMigrationPlan(plan *MigrationPlan, originalDef, updatedDef common.EntityDefinition) error {
	// Get all entities of this type
	entitiesToUpdate := make([]common.Entity, 0)

	for key, entity := range dse.entities {
		if entity.Type == plan.EntityType {
			// Use the key variable to avoid the "declared and not used" error
			_ = key
			entitiesToUpdate = append(entitiesToUpdate, entity)
		}
	}

	// Process each entity
	for _, entity := range entitiesToUpdate {
		entityKey := createEntityKey(entity.Type, entity.ID)

		// Skip already removed entities
		if _, exists := dse.entities[entityKey]; !exists {
			continue
		}

		// Remove old index entries
		dse.updateIndices(entity, false)

		// Process field changes
		for _, change := range plan.ChangedFields {
			// Use the change variable to avoid the "declared and not used" error
			_ = change
			if value, exists := entity.Fields[change.FieldName]; exists {
				// Convert value to the new type if needed
				convertedValue, err := convertFieldValue(value, change.OriginalType, change.NewType)
				if err != nil {
					return fmt.Errorf("failed to convert field %s for entity %s: %w",
						change.FieldName, entity.ID, err)
				}
				entity.Fields[change.FieldName] = convertedValue
			}
		}

		// Update the entity
		dse.entities[entityKey] = entity

		// Add new index entries
		dse.updateIndices(entity, true)
	}

	return nil
}

// updateIndicesForSchemaChange updates the index structures for changed field definitions
func (dse *Engine) updateIndicesForSchemaChange(originalDef, updatedDef common.EntityDefinition) {
	// Map for quick lookup of original indexed status
	originalIndexed := make(map[string]bool)
	for _, field := range originalDef.Fields {
		originalIndexed[field.Name] = field.Indexed
	}

	// Process each field in the updated definition
	for _, field := range updatedDef.Fields {
		wasIndexed, exists := originalIndexed[field.Name]

		// If this is a new field or indexing was added, create the index
		if field.Indexed && (!exists || !wasIndexed) {
			if dse.indices[updatedDef.Name] == nil {
				dse.indices[updatedDef.Name] = make(map[string]map[string][]string)
			}

			// Initialize the index for this field
			dse.indices[updatedDef.Name][field.Name] = make(map[string][]string)

			// Populate the index with existing data
			for _, entity := range dse.entities {
				if entity.Type == updatedDef.Name {
					if value, exists := entity.Fields[field.Name]; exists && value != nil {
						strValue := dse.getIndexableValue(value)
						dse.indices[entity.Type][field.Name][strValue] = append(
							dse.indices[entity.Type][field.Name][strValue],
							entity.ID)
					}
				}
			}
		}

		// If indexing was removed, delete the index
		if !field.Indexed && exists && wasIndexed {
			if dse.indices[updatedDef.Name] != nil {
				delete(dse.indices[updatedDef.Name], field.Name)
			}
		}
	}
}

// isCompatibleTypeChange determines if a type change can be performed safely
func isCompatibleTypeChange(oldType, newType string) bool {
	// Allow changing between compatible types
	switch oldType {
	case TypeInteger:
		return newType == TypeFloat // Integer can be safely converted to float
	case TypeString:
		return newType == TypeText // String can be safely converted to text
	case TypeDate:
		return newType == TypeDateTime // Date can be safely converted to datetime
	default:
		return oldType == newType // Otherwise, only allow same type
	}
}

// convertFieldValue converts a field value from one type to another
func convertFieldValue(value interface{}, oldType, newType string) (interface{}, error) {
	switch {
	case oldType == TypeInteger && newType == TypeFloat:
		// Convert integer to float
		switch v := value.(type) {
		case int:
			return float64(v), nil
		case int32:
			return float64(v), nil
		case int64:
			return float64(v), nil
		default:
			return value, nil // Already a compatible type
		}
	case oldType == TypeString && newType == TypeText:
		// No actual conversion needed, just semantic difference
		return value, nil
	case oldType == TypeDate && newType == TypeDateTime:
		// No actual conversion needed for our storage
		return value, nil
	default:
		// For incompatible types (should be caught earlier)
		if oldType != newType {
			return nil, fmt.Errorf("incompatible type conversion: %s to %s", oldType, newType)
		}
		return value, nil
	}
}
