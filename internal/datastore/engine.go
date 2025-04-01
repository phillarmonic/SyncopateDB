package datastore

import (
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"
)

// Engine provides the core functionality for storing and retrieving data
type Engine struct {
	definitions map[string]EntityDefinition
	entities    map[string]Entity
	indices     map[string]map[string]map[string][]string // entityType -> fieldName -> fieldValue -> []entityIDs
	mu          sync.RWMutex
}

// NewDataStoreEngine creates a new data store engine instance
func NewDataStoreEngine() *Engine {
	return &Engine{
		definitions: make(map[string]EntityDefinition),
		entities:    make(map[string]Entity),
		indices:     make(map[string]map[string]map[string][]string),
	}
}

// RegisterEntityType registers a new entity type with the data store engine
func (dse *Engine) RegisterEntityType(def EntityDefinition) error {
	dse.mu.Lock()
	defer dse.mu.Unlock()

	if _, exists := dse.definitions[def.Name]; exists {
		return fmt.Errorf("entity type %s already exists", def.Name)
	}

	dse.definitions[def.Name] = def
	dse.indices[def.Name] = make(map[string]map[string][]string)

	// Initialize indices for indexed fields
	for _, field := range def.Fields {
		if field.Indexed {
			dse.indices[def.Name][field.Name] = make(map[string][]string)
		}
	}

	return nil
}

// GetEntityDefinition returns the definition for a specific entity type
func (dse *Engine) GetEntityDefinition(entityType string) (EntityDefinition, error) {
	dse.mu.RLock()
	defer dse.mu.RUnlock()

	def, exists := dse.definitions[entityType]
	if !exists {
		return EntityDefinition{}, fmt.Errorf("entity type %s not registered", entityType)
	}

	return def, nil
}

// ListEntityTypes returns a list of all registered entity types
func (dse *Engine) ListEntityTypes() []string {
	dse.mu.RLock()
	defer dse.mu.RUnlock()

	types := make([]string, 0, len(dse.definitions))
	for typeName := range dse.definitions {
		types = append(types, typeName)
	}
	sort.Strings(types)
	return types
}

// Insert adds a new entity to the data store engine
func (dse *Engine) Insert(entityType string, id string, data map[string]interface{}) error {
	dse.mu.Lock()
	defer dse.mu.Unlock()

	// Check if entity type is registered
	if _, exists := dse.definitions[entityType]; !exists {
		return fmt.Errorf("entity type %s not registered", entityType)
	}

	// Validate data against entity definition
	if err := dse.validateEntityData(entityType, data); err != nil {
		return err
	}

	// Check if ID already exists
	if _, exists := dse.entities[id]; exists {
		return fmt.Errorf("entity with ID %s already exists", id)
	}

	// Create and store the entity
	entity := Entity{
		ID:     id,
		Type:   entityType,
		Fields: data,
	}
	dse.entities[id] = entity

	// Update indices for indexed fields
	dse.updateIndices(entity, true)

	return nil
}

// Update updates an existing entity in the data store engine
func (dse *Engine) Update(id string, data map[string]interface{}) error {
	dse.mu.Lock()
	defer dse.mu.Unlock()

	entity, exists := dse.entities[id]
	if !exists {
		return fmt.Errorf("entity with ID %s not found", id)
	}

	// Validate data against entity definition
	if err := dse.validateEntityData(entity.Type, data); err != nil {
		return err
	}

	// Remove old index entries
	dse.updateIndices(entity, false)

	// Update entity
	for k, v := range data {
		entity.Fields[k] = v
	}
	dse.entities[id] = entity

	// Add new index entries
	dse.updateIndices(entity, true)

	return nil
}

// Delete removes an entity from the data store engine
func (dse *Engine) Delete(id string) error {
	dse.mu.Lock()
	defer dse.mu.Unlock()

	entity, exists := dse.entities[id]
	if !exists {
		return fmt.Errorf("entity with ID %s not found", id)
	}

	// Remove index entries
	dse.updateIndices(entity, false)

	// Delete the entity
	delete(dse.entities, id)

	return nil
}

// Get retrieves an entity by ID
func (dse *Engine) Get(id string) (Entity, error) {
	dse.mu.RLock()
	defer dse.mu.RUnlock()

	entity, exists := dse.entities[id]
	if !exists {
		return Entity{}, fmt.Errorf("entity with ID %s not found", id)
	}

	return entity, nil
}

// GetEntityCount returns the count of entities of a specific type
func (dse *Engine) GetEntityCount(entityType string) (int, error) {
	dse.mu.RLock()
	defer dse.mu.RUnlock()

	if _, exists := dse.definitions[entityType]; !exists {
		return 0, fmt.Errorf("entity type %s not registered", entityType)
	}

	count := 0
	for _, entity := range dse.entities {
		if entity.Type == entityType {
			count++
		}
	}

	return count, nil
}

// GetAllEntitiesOfType retrieves all entities of a specific type
func (dse *Engine) GetAllEntitiesOfType(entityType string) ([]Entity, error) {
	dse.mu.RLock()
	defer dse.mu.RUnlock()

	if _, exists := dse.definitions[entityType]; !exists {
		return nil, fmt.Errorf("entity type %s not registered", entityType)
	}

	entities := make([]Entity, 0)
	for _, entity := range dse.entities {
		if entity.Type == entityType {
			entities = append(entities, entity)
		}
	}

	return entities, nil
}

// updateIndices adds or removes index entries for an entity
func (dse *Engine) updateIndices(entity Entity, add bool) {
	def := dse.definitions[entity.Type]
	for _, fieldDef := range def.Fields {
		if fieldDef.Indexed {
			if value, exists := entity.Fields[fieldDef.Name]; exists && value != nil {
				strValue := dse.getIndexableValue(value)

				if add {
					// Add to index
					if dse.indices[entity.Type][fieldDef.Name] == nil {
						dse.indices[entity.Type][fieldDef.Name] = make(map[string][]string)
					}
					dse.indices[entity.Type][fieldDef.Name][strValue] = append(dse.indices[entity.Type][fieldDef.Name][strValue], entity.ID)
				} else {
					// Remove from index
					ids := dse.indices[entity.Type][fieldDef.Name][strValue]
					for i, id := range ids {
						if id == entity.ID {
							dse.indices[entity.Type][fieldDef.Name][strValue] = append(ids[:i], ids[i+1:]...)
							break
						}
					}

					// Clean up empty slices
					if len(dse.indices[entity.Type][fieldDef.Name][strValue]) == 0 {
						delete(dse.indices[entity.Type][fieldDef.Name], strValue)
					}
				}
			}
		}
	}
}

// getIndexableValue converts a value to a string for indexing
func (dse *Engine) getIndexableValue(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	case time.Time:
		return v.Format(time.RFC3339)
	case bool:
		if v {
			return "true"
		}
		return "false"
	case map[string]interface{}, []interface{}:
		bytes, _ := json.Marshal(v)
		return string(bytes)
	default:
		return fmt.Sprintf("%v", v)
	}
}
