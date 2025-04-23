package datastore

import (
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/phillarmonic/syncopate-db/internal/common"
)

// Engine provides the core functionality for storing and retrieving data
// and implements the common.DatastoreEngine interface
type Engine struct {
	definitions    map[string]common.EntityDefinition
	entities       map[string]common.Entity
	indices        map[string]map[string]map[string][]string
	persistence    common.PersistenceProvider
	idGeneratorMgr *IDGeneratorManager
	mu             sync.RWMutex
}

// EngineConfig holds configuration for the data store engine
type EngineConfig struct {
	Persistence       common.PersistenceProvider
	EnablePersistence bool
}

// NewDataStoreEngine creates a new data store engine instance
func NewDataStoreEngine(config ...EngineConfig) *Engine {
	engine := &Engine{
		definitions:    make(map[string]common.EntityDefinition),
		entities:       make(map[string]common.Entity),
		indices:        make(map[string]map[string]map[string][]string),
		idGeneratorMgr: NewIDGeneratorManager(),
	}

	// Apply configuration if provided
	if len(config) > 0 {
		if config[0].EnablePersistence && config[0].Persistence != nil {
			engine.persistence = config[0].Persistence

			// Load data from persistence - this happens before the server starts
			// handling requests, so we don't need to worry about concurrency yet
			if err := engine.persistence.LoadLatestSnapshot(engine); err != nil {
				// Log error but continue
				fmt.Printf("Error loading snapshot: %v\n", err)
			}

			// Apply any WAL entries after the snapshot
			if err := engine.persistence.LoadWAL(engine); err != nil {
				// Log error but continue
				fmt.Printf("Error loading WAL: %v\n", err)
			}

			// Load auto-increment counters (new addition)
			if persistenceWithCounters, ok := engine.persistence.(common.PersistenceWithCounters); ok {
				if err := persistenceWithCounters.LoadCounters(engine); err != nil {
					// Log error but continue
					fmt.Printf("Error loading auto-increment counters: %v\n", err)
				}
			}
		}
	}

	return engine
}

// Close properly shuts down the engine
func (dse *Engine) Close() error {
	dse.mu.Lock()
	defer dse.mu.Unlock()

	if dse.persistence != nil {
		// Instead of using persistence while holding the lock, we release it
		persistenceProvider := dse.persistence
		dse.mu.Unlock()

		// Take a final snapshot without holding the lock
		if err := persistenceProvider.TakeSnapshot(dse); err != nil {
			// Reacquire the lock before returning
			dse.mu.Lock()
			return fmt.Errorf("failed to take final snapshot: %w", err)
		}

		// Close the persistence provider without holding the lock
		err := persistenceProvider.Close()

		// Reacquire the lock before returning
		dse.mu.Lock()
		return err
	}

	return nil
}

// RegisterEntityType registers a new entity type with the data store engine
func (dse *Engine) RegisterEntityType(def common.EntityDefinition) error {
	// First check if entity type already exists without modifying state
	dse.mu.RLock()
	_, exists := dse.definitions[def.Name]
	dse.mu.RUnlock()

	if exists {
		return fmt.Errorf("entity type %s already exists", def.Name)
	}

	// Add internal fields to the definition
	dse.addInternalFieldDefinitions(&def)

	// Set default ID generator if not specified (auto_increment)
	if def.IDGenerator == "" {
		def.IDGenerator = common.IDTypeAutoIncrement
	}

	// Now acquire write lock for modification
	dse.mu.Lock()

	// Double-check existence after acquiring write lock
	if _, exists := dse.definitions[def.Name]; exists {
		dse.mu.Unlock()
		return fmt.Errorf("entity type %s already exists", def.Name)
	}

	// Register the ID generator type for this entity
	dse.idGeneratorMgr.RegisterEntityType(def.Name, def.IDGenerator)

	// Update in-memory state
	dse.definitions[def.Name] = def
	dse.indices[def.Name] = make(map[string]map[string][]string)

	// Initialize indices for indexed fields
	for _, field := range def.Fields {
		if field.Indexed {
			dse.indices[def.Name][field.Name] = make(map[string][]string)
		}
	}

	// Release lock before persistence operation
	dse.mu.Unlock()

	// Persist entity type if persistence is enabled
	var persistErr error
	if dse.persistence != nil {
		persistErr = dse.persistence.RegisterEntityType(dse, def)

		// If persistence fails, we need to clean up the in-memory state
		if persistErr != nil {
			dse.mu.Lock()
			delete(dse.definitions, def.Name)
			delete(dse.indices, def.Name)
			dse.mu.Unlock()

			return fmt.Errorf("failed to persist entity type: %w", persistErr)
		}
	}

	return nil
}

// GetEntityDefinition returns the definition for a specific entity type
func (dse *Engine) GetEntityDefinition(entityType string) (common.EntityDefinition, error) {
	dse.mu.RLock()
	defer dse.mu.RUnlock()

	def, exists := dse.definitions[entityType]
	if !exists {
		return common.EntityDefinition{}, fmt.Errorf("entity type %s not registered", entityType)
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

// prepareEntityForInsert validates and prepares an entity for insertion
// This function requires that the caller holds a read lock
func (dse *Engine) prepareEntityForInsert(entityType string, id string, data map[string]interface{}) (common.Entity, error) {
	// Check if entity type is registered
	if _, exists := dse.definitions[entityType]; !exists {
		return common.Entity{}, fmt.Errorf("entity type %s not registered", entityType)
	}

	// Validate data against entity definition
	if err := dse.validateEntityData(entityType, data); err != nil {
		return common.Entity{}, err
	}

	// Check if ID already exists
	if _, exists := dse.entities[id]; exists {
		return common.Entity{}, fmt.Errorf("entity with ID %s already exists", id)
	}

	// Create the entity
	return common.Entity{
		ID:     id,
		Type:   entityType,
		Fields: data,
	}, nil
}

// Insert adds a new entity to the data store engine with support for ID generation
func (dse *Engine) Insert(entityType string, id string, data map[string]interface{}) error {
	// Check if entity type exists
	dse.mu.RLock()
	_, exists := dse.definitions[entityType]
	dse.mu.RUnlock()

	if !exists {
		return fmt.Errorf("entity type %s not registered", entityType)
	}

	// Handle ID generation if needed
	var generatedID bool
	if id == "" {
		// Generate ID based on entity type's strategy
		var err error
		id, err = dse.idGeneratorMgr.GenerateID(entityType)
		if err != nil {
			return fmt.Errorf("failed to generate ID: %w", err)
		}
		generatedID = true
	} else {
		// Validate provided ID against expected format
		valid, err := dse.idGeneratorMgr.ValidateID(entityType, id)
		if err != nil {
			return fmt.Errorf("failed to validate ID: %w", err)
		}
		if !valid {
			return fmt.Errorf("invalid ID format for entity type %s", entityType)
		}
	}

	// Add internal fields
	dse.addInternalFields(entityType, data)

	// Now validate the data and prepare for insertion
	dse.mu.RLock()
	entity, err := dse.prepareEntityForInsert(entityType, id, data)
	dse.mu.RUnlock()

	if err != nil {
		return err
	}

	// Now acquire write lock and check again to handle race conditions
	dse.mu.Lock()

	// Check again if entity type exists and ID is unique under write lock
	if _, exists := dse.definitions[entityType]; !exists {
		dse.mu.Unlock()
		return fmt.Errorf("entity type %s not registered", entityType)
	}

	if _, exists := dse.entities[id]; exists {
		dse.mu.Unlock()
		if generatedID {
			// If we generated the ID and there's still a collision, something is wrong with our ID generator
			return fmt.Errorf("generated ID %s already exists, this should not happen", id)
		}
		return fmt.Errorf("entity with ID %s already exists", id)
	}

	// Store the entity and update indices
	dse.entities[id] = entity
	dse.updateIndices(entity, true)

	// Store reference to persistence provider and release lock
	persistenceProvider := dse.persistence
	dse.mu.Unlock()

	// Persist entity if persistence is enabled
	if persistenceProvider != nil {
		if err := persistenceProvider.Insert(dse, entityType, id, data); err != nil {
			// If persistence fails, we need to remove the entity from memory
			dse.mu.Lock()
			delete(dse.entities, id)
			// Update indices needs the entity to still be in entities, so we add it back temporarily
			dse.entities[id] = entity
			dse.updateIndices(entity, false)
			delete(dse.entities, id)
			dse.mu.Unlock()

			return fmt.Errorf("failed to persist entity: %w", err)
		}

		// Save auto-increment counter if this is an auto-increment entity type
		if persistenceWithCounters, ok := persistenceProvider.(common.PersistenceWithCounters); ok {
			def, _ := dse.GetEntityDefinition(entityType)
			if def.IDGenerator == common.IDTypeAutoIncrement {
				counter, err := dse.GetAutoIncrementCounter(entityType)
				if err == nil {
					if err := persistenceWithCounters.SaveCounter(entityType, counter); err != nil {
						// Just log the error, don't fail the insert
						fmt.Printf("Error saving auto-increment counter: %v\n", err)
					}
				}
			}
		}
	}
	return nil
}

// Update updates an existing entity in the data store engine
func (dse *Engine) Update(id string, data map[string]interface{}) error {
	// First, read the current state without write lock
	dse.mu.RLock()
	entity, exists := dse.entities[id]
	if !exists {
		dse.mu.RUnlock()
		return fmt.Errorf("entity with ID %s not found", id)
	}

	// Update internal fields
	data["_updated_at"] = time.Now()

	// Ensure _created_at is not modified
	delete(data, "_created_at")

	// Validate the update data using the new function
	if err := dse.validateUpdateData(entity.Type, data); err != nil {
		dse.mu.RUnlock()
		return err
	}

	// Make a deep copy of the original entity for rollback
	originalEntity := common.Entity{
		ID:     entity.ID,
		Type:   entity.Type,
		Fields: make(map[string]interface{}),
	}

	for k, v := range entity.Fields {
		originalEntity.Fields[k] = v
	}

	dse.mu.RUnlock()

	// Now acquire write lock for the update
	dse.mu.Lock()

	// Check again if the entity exists
	entity, exists = dse.entities[id]
	if !exists {
		dse.mu.Unlock()
		return fmt.Errorf("entity with ID %s not found", id)
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

	// Store reference to persistence provider and entity type
	persistenceProvider := dse.persistence
	dse.mu.Unlock()

	// Persist update if persistence is enabled
	if persistenceProvider != nil {
		if err := persistenceProvider.Update(dse, id, data); err != nil {
			// Rollback in-memory state on error
			dse.mu.Lock()
			// Remove updated indices
			entity = dse.entities[id]
			dse.updateIndices(entity, false)

			// Restore original entity
			dse.entities[id] = originalEntity
			dse.updateIndices(originalEntity, true)
			dse.mu.Unlock()

			return fmt.Errorf("failed to persist entity update: %w", err)
		}
	}

	return nil
}

// Delete removes an entity from the data store engine
func (dse *Engine) Delete(id string) error {
	// First read the current state without write lock
	dse.mu.RLock()
	entity, exists := dse.entities[id]
	if !exists {
		dse.mu.RUnlock()
		return fmt.Errorf("entity with ID %s not found", id)
	}

	// Make a copy of the entity for rollback
	originalEntity := common.Entity{
		ID:     entity.ID,
		Type:   entity.Type,
		Fields: make(map[string]interface{}),
	}

	for k, v := range entity.Fields {
		originalEntity.Fields[k] = v
	}

	dse.mu.RUnlock()

	// Now acquire write lock for the deletion
	dse.mu.Lock()

	// Check again if the entity exists
	entity, exists = dse.entities[id]
	if !exists {
		dse.mu.Unlock()
		return fmt.Errorf("entity with ID %s not found", id)
	}

	// Remove index entries
	dse.updateIndices(entity, false)

	// Delete the entity
	delete(dse.entities, id)

	// Store reference to persistence provider and release lock
	persistenceProvider := dse.persistence
	dse.mu.Unlock()

	// Persist deletion if persistence is enabled
	if persistenceProvider != nil {
		if err := persistenceProvider.Delete(dse, id); err != nil {
			// Rollback in-memory state on error
			dse.mu.Lock()
			dse.entities[id] = originalEntity
			dse.updateIndices(originalEntity, true)
			dse.mu.Unlock()

			return fmt.Errorf("failed to persist entity deletion: %w", err)
		}
	}

	return nil
}

// Get retrieves an entity by ID
func (dse *Engine) Get(id string) (common.Entity, error) {
	dse.mu.RLock()
	defer dse.mu.RUnlock()

	entity, exists := dse.entities[id]
	if !exists {
		return common.Entity{}, fmt.Errorf("entity with ID %s not found", id)
	}

	return entity, nil
}

// ForceSnapshot immediately creates a snapshot of the current state
func (dse *Engine) ForceSnapshot() error {
	if dse.persistence == nil {
		return fmt.Errorf("persistence not enabled")
	}

	return dse.persistence.TakeSnapshot(dse)
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
func (dse *Engine) GetAllEntitiesOfType(entityType string) ([]common.Entity, error) {
	dse.mu.RLock()
	defer dse.mu.RUnlock()

	if _, exists := dse.definitions[entityType]; !exists {
		return nil, fmt.Errorf("entity type %s not registered", entityType)
	}

	entities := make([]common.Entity, 0)
	for _, entity := range dse.entities {
		if entity.Type == entityType {
			entities = append(entities, entity)
		}
	}

	return entities, nil
}

// updateIndices adds or removes index entries for an entity
func (dse *Engine) updateIndices(entity common.Entity, add bool) {
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

// ForceGarbageCollection runs garbage collection on the underlying Badger DB
func (dse *Engine) ForceGarbageCollection(discardRatio float64) error {
	if dse.persistence == nil {
		return fmt.Errorf("persistence not enabled")
	}

	// This requires a type assertion which might break the abstraction
	// You might want to add a RunGarbageCollection method to the common.PersistenceProvider interface
	return fmt.Errorf("garbage collection not supported through the interface")
}

// addInternalFields adds necessary internal fields to the entity data
func (dse *Engine) addInternalFields(entityType string, data map[string]interface{}) {
	// Add created_at timestamp if not already present
	if _, exists := data["_created_at"]; !exists {
		data["_created_at"] = time.Now()
	}

	// Add updated_at timestamp
	data["_updated_at"] = time.Now()
}

// addInternalFieldDefinitions adds internal field definitions to an entity type
func (dse *Engine) addInternalFieldDefinitions(def *common.EntityDefinition) {
	// Add _created_at field
	createdAtField := common.FieldDefinition{
		Name:     "_created_at",
		Type:     "datetime",
		Indexed:  true, // Index for efficient sorting
		Required: true,
		Internal: true, // Mark as internal
	}

	// Add _updated_at field
	updatedAtField := common.FieldDefinition{
		Name:     "_updated_at",
		Type:     "datetime",
		Indexed:  true, // Index for efficient filtering
		Required: true,
		Internal: true, // Mark as internal
	}

	def.Fields = append(def.Fields, createdAtField, updatedAtField)
}
