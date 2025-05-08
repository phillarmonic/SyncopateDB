package datastore

import (
	"encoding/json"
	"fmt"
	"github.com/phillarmonic/syncopate-db/internal/utilities"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/phillarmonic/syncopate-db/internal/common"
)

// Engine provides the core functionality for storing and retrieving data
// and implements the common.DatastoreEngine interface
type Engine struct {
	definitions    map[string]common.EntityDefinition
	entities       map[string]common.Entity // Key format: "entityType:entityID"
	indices        map[string]map[string]map[string][]string
	uniqueIndices  map[string]map[string]map[string]string
	persistence    common.PersistenceProvider
	idGeneratorMgr *IDGeneratorManager
	mu             sync.RWMutex
}

// EngineConfig holds configuration for the data store engine
type EngineConfig struct {
	Persistence       common.PersistenceProvider
	EnablePersistence bool
}

// createEntityKey creates a composite key from an entity type and ID
func createEntityKey(entityType, id string) string {
	return fmt.Sprintf("%s:%s", entityType, id)
}

// parseEntityKey parses a composite key into entity type and ID
func parseEntityKey(key string) (string, string) {
	parts := strings.SplitN(key, ":", 2)
	if len(parts) != 2 {
		return "", key // Fallback for backward compatibility
	}
	return parts[0], parts[1]
}

// NewDataStoreEngine creates a new data store engine instance
func NewDataStoreEngine(config ...EngineConfig) *Engine {
	engine := &Engine{
		definitions:    make(map[string]common.EntityDefinition),
		entities:       make(map[string]common.Entity),
		indices:        make(map[string]map[string]map[string][]string),
		uniqueIndices:  make(map[string]map[string]map[string]string),
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

			// Load auto-increment counters
			if persistenceWithCounters, ok := engine.persistence.(common.PersistenceWithCounters); ok {
				if err := persistenceWithCounters.LoadCounters(engine); err != nil {
					// Log error but continue
					fmt.Printf("Error loading auto-increment counters: %v\n", err)
				}
			}

			// Load deleted IDs for auto-increment generators
			if persistenceWithDeletedIDs, ok := engine.persistence.(common.PersistenceWithDeletedIDs); ok {
				if err := persistenceWithDeletedIDs.LoadDeletedIDs(engine); err != nil {
					// Log error but continue
					fmt.Printf("Error loading deleted IDs: %v\n", err)
				}
			}
		}
	}

	engine.EnsureAutoIncrementCounterAboveExistingIDs()

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

	// Validate that no field names start with underscore (reserved for internal use)
	// Pass false to validate all fields, since we haven't added our internal fields yet
	if err := ValidateEntityTypeFields(def.Fields, true); err != nil {
		return err
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

	// Initialize unique indices for unique fields
	dse.uniqueIndices[def.Name] = make(map[string]map[string]string)

	// Initialize indices for indexed fields and unique indices for unique fields
	for _, field := range def.Fields {
		if field.Indexed {
			dse.indices[def.Name][field.Name] = make(map[string][]string)
		}

		if field.Unique {
			// Unique fields should also be indexed for performance
			if !field.Indexed {
				field.Indexed = true
				dse.indices[def.Name][field.Name] = make(map[string][]string)
			}

			// Initialize the unique index map
			dse.uniqueIndices[def.Name][field.Name] = make(map[string]string)
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
			delete(dse.uniqueIndices, def.Name)
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

	// Check if ID already exists using the composite key
	entityKey := createEntityKey(entityType, id)
	if _, exists := dse.entities[entityKey]; exists {
		return common.Entity{}, fmt.Errorf("entity with ID %s already exists for entity type %s", id, entityType)
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

	// Create composite key
	entityKey := createEntityKey(entityType, id)

	// Now acquire write lock and check again to handle race conditions
	dse.mu.Lock()

	// Check again if entity type exists and ID is unique under write lock
	if _, exists := dse.definitions[entityType]; !exists {
		dse.mu.Unlock()
		return fmt.Errorf("entity type %s not registered", entityType)
	}

	if _, exists := dse.entities[entityKey]; exists {
		dse.mu.Unlock()
		if generatedID {
			// If we generated the ID and there's still a collision, something is wrong with our ID generator
			return fmt.Errorf("generated ID %s already exists for entity type %s, this should not happen", id, entityType)
		}
		return fmt.Errorf("entity with ID %s already exists for entity type %s", id, entityType)
	}

	// Check uniqueness constraints AFTER acquiring write lock
	if err := dse.validateUniqueness(entityType, data, ""); err != nil {
		dse.mu.Unlock()
		return err
	}

	// Store the entity and update indices
	dse.entities[entityKey] = entity
	dse.updateIndices(entity, true)

	// Store reference to persistence provider and release lock
	persistenceProvider := dse.persistence
	dse.mu.Unlock()

	// Persist entity if persistence is enabled
	if persistenceProvider != nil {
		if err := persistenceProvider.Insert(dse, entityType, id, data); err != nil {
			// If persistence fails, we need to remove the entity from memory
			dse.mu.Lock()
			delete(dse.entities, entityKey)
			// Update indices needs the entity to still be in entities, so we add it back temporarily
			dse.entities[entityKey] = entity
			dse.updateIndices(entity, false)
			delete(dse.entities, entityKey)
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
func (dse *Engine) Update(entityType string, id string, data map[string]interface{}) error {
	// First, read the current state without write lock
	dse.mu.RLock()

	// Try to find the entity using the composite key format
	entityKey := createEntityKey(entityType, id)
	entity, exists := dse.entities[entityKey]

	// If not found by composite key, try backward compatibility approach
	if !exists {
		// Try to find an entity with matching ID and type through iteration
		for key, e := range dse.entities {
			_, entityID := parseEntityKey(key)
			if entityID == id && e.Type == entityType {
				entity = e
				entityKey = key
				exists = true
				break
			}
		}
	}

	if !exists {
		dse.mu.RUnlock()
		return fmt.Errorf("entity with ID %s and type %s not found", id, entityType)
	}

	// Update internal fields
	data["_updated_at"] = time.Now()

	// Ensure _created_at is not modified
	delete(data, "_created_at")

	// Validate the update data against the correct entity type
	if err := dse.validateUpdateData(entityType, id, data); err != nil {
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

	// Check again if the entity exists under write lock
	entity, exists = dse.entities[entityKey]
	if !exists {
		dse.mu.Unlock()
		return fmt.Errorf("entity with ID %s and type %s not found", id, entityType)
	}

	// Check uniqueness constraints AFTER acquiring write lock
	// First, create a merged map of current fields + update fields to check the final state
	mergedFields := make(map[string]interface{})
	for k, v := range entity.Fields {
		mergedFields[k] = v
	}
	for k, v := range data {
		mergedFields[k] = v
	}

	if err := dse.validateUniqueness(entityType, mergedFields, id); err != nil {
		dse.mu.Unlock()
		return err
	}

	// Remove old index entries
	dse.updateIndices(entity, false)

	// Update entity
	for k, v := range data {
		entity.Fields[k] = v
	}
	dse.entities[entityKey] = entity

	// Add new index entries
	dse.updateIndices(entity, true)

	// Store reference to persistence provider
	persistenceProvider := dse.persistence
	dse.mu.Unlock()

	// Persist update if persistence is enabled
	if persistenceProvider != nil {
		if err := persistenceProvider.Update(dse, entityType, id, data); err != nil {
			// Rollback in-memory state on error
			dse.mu.Lock()
			// Remove updated indices
			entity = dse.entities[entityKey]
			dse.updateIndices(entity, false)

			// Restore original entity
			dse.entities[entityKey] = originalEntity
			dse.updateIndices(originalEntity, true)
			dse.mu.Unlock()

			return fmt.Errorf("failed to persist entity update: %w", err)
		}
	}

	return nil
}

// Delete removes an entity from the data store engine
func (dse *Engine) Delete(entityTypeToDelete string, id string) error {
	// First read the current state without write lock
	dse.mu.RLock()

	var entityToProcess common.Entity
	var actualEntityKey string // The key in dse.entities map, e.g., "posts:1"
	var entityFound bool

	// Priority 1: Try with the precise composite key "{entityTypeToDelete}:{id}"
	preciseKey := createEntityKey(entityTypeToDelete, id)
	if e, ok := dse.entities[preciseKey]; ok {
		entityToProcess = e
		actualEntityKey = preciseKey
		entityFound = true
	}

	// Priority 2: Fallback for potential legacy keys or general search if preciseKey fails.
	if !entityFound {
		// Check if `id` itself is a key (legacy) and if its type matches.
		if e, ok := dse.entities[id]; ok && e.Type == entityTypeToDelete {
			entityToProcess = e
			actualEntityKey = id // id here is the key
			entityFound = true
		} else {
			// Iterate to find a composite key that matches ID and Type.
			for keyInMap, e := range dse.entities {
				_, idFromKey := parseEntityKey(keyInMap)
				if idFromKey == id && e.Type == entityTypeToDelete {
					entityToProcess = e
					actualEntityKey = keyInMap
					entityFound = true
					break // Found the correct entity matching ID and Type
				}
			}
		}
	}

	if !entityFound {
		dse.mu.RUnlock()
		return fmt.Errorf("entity with ID %s and type %s not found", id, entityTypeToDelete)
	}

	// Make a copy of the entity for persistence and rollback
	originalEntity := common.Entity{
		ID:     entityToProcess.ID,
		Type:   entityToProcess.Type,
		Fields: make(map[string]interface{}),
	}

	for k, v := range entityToProcess.Fields {
		originalEntity.Fields[k] = v
	}

	// Store the entity type for persistence
	entityTypeForPersistence := entityToProcess.Type

	dse.mu.RUnlock()

	// Get entity definition to determine ID type
	idGeneratorType, err := dse.GetIDGeneratorType(entityTypeToDelete)
	if err == nil && idGeneratorType == common.IDTypeAutoIncrement {
		// For auto-increment, mark this ID as deleted to prevent reuse
		generator, err := dse.idGeneratorMgr.GetGenerator(entityTypeToDelete)
		if err == nil {
			if autoGen, ok := generator.(*AutoIncrementGenerator); ok {
				autoGen.MarkIDDeleted(entityTypeToDelete, id)

				// Save the updated deleted IDs list if persistence is enabled
				if dse.persistence != nil {
					if persistenceWithDeletedIDs, ok := dse.persistence.(common.PersistenceWithDeletedIDs); ok {
						deletedIDs := autoGen.SaveDeletedIDs(entityTypeToDelete)
						if err := persistenceWithDeletedIDs.SaveDeletedIDs(entityTypeToDelete, deletedIDs); err != nil {
							// Just log the error, don't fail the delete
							fmt.Printf("Error saving deleted IDs: %v\n", err)
						}
					}
				}
			}
		}
	}

	// Now acquire write lock for the deletion
	dse.mu.Lock()

	// Check again if the entity exists
	currentEntityInMap, currentExists := dse.entities[actualEntityKey]
	if !currentExists {
		dse.mu.Unlock()
		return fmt.Errorf("entity with ID %s and type %s not found (disappeared before write lock)", id, entityTypeToDelete)
	}
	// Paranoia check: ensure the entity we're about to delete is still the one we expect.
	if currentEntityInMap.ID != id || currentEntityInMap.Type != entityTypeToDelete {
		dse.mu.Unlock()
		return fmt.Errorf("entity integrity check failed before deletion for ID %s, type %s. Found ID %s, type %s at key %s",
			id, entityTypeToDelete, currentEntityInMap.ID, currentEntityInMap.Type, actualEntityKey)
	}

	// Remove index entries
	dse.updateIndices(currentEntityInMap, false)

	// Delete the entity
	delete(dse.entities, actualEntityKey)

	// Store reference to persistence provider and release lock
	persistenceProvider := dse.persistence
	dse.mu.Unlock()

	// Persist deletion if persistence is enabled
	if persistenceProvider != nil {
		// Pass the simple `id` and `entityTypeForPersistence`
		if err := persistenceProvider.Delete(dse, id, entityTypeForPersistence); err != nil {
			// Rollback in-memory state on error
			dse.mu.Lock()
			dse.entities[actualEntityKey] = originalEntity
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

	// Try direct lookup first (backward compatibility)
	entity, exists := dse.entities[id]
	if exists {
		return entity, nil
	}

	// If not found, check for composite keys that match the ID
	// We can recognize the key format we're looking for
	// but we need to be more careful than before

	// First, look for composite keys (type:id format)
	for key, entity := range dse.entities {
		_, entityID := parseEntityKey(key)
		if entityID == id {
			return entity, nil
		}
	}

	return common.Entity{}, fmt.Errorf("entity with ID %s not found", id)
}

func (dse *Engine) GetByType(id string, entityType string) (common.Entity, error) {
	dse.mu.RLock()
	defer dse.mu.RUnlock()

	// Try the composite key first
	entityKey := createEntityKey(entityType, id)
	entity, exists := dse.entities[entityKey]
	if exists {
		return entity, nil
	}

	// For backward compatibility, try just the ID but then verify the type
	entity, err := dse.Get(id)
	if err == nil && entity.Type == entityType {
		return entity, nil
	}

	return common.Entity{}, fmt.Errorf("entity with ID %s and type %s not found", id, entityType)
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

	// Iterate through all entities and filter by type
	// For both legacy and new composite keys
	for _, entity := range dse.entities {
		if entity.Type == entityType {
			entities = append(entities, entity)
		}
	}

	return entities, nil
}

// updateIndices adds or removes index entries for an entity
func (dse *Engine) updateIndices(entity common.Entity, add bool) {
	// Original index update logic
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

	// Also update unique indices
	dse.updateUniqueIndices(entity, add)
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
	// Check if internal fields already exist
	createdAtExists := false
	updatedAtExists := false

	for _, field := range def.Fields {
		if field.Name == "_created_at" {
			createdAtExists = true
		}
		if field.Name == "_updated_at" {
			updatedAtExists = true
		}
	}

	// Add _created_at field if it doesn't exist
	if !createdAtExists {
		createdAtField := common.FieldDefinition{
			Name:     "_created_at",
			Type:     "datetime",
			Indexed:  true, // Index for efficient sorting
			Required: true,
			Internal: true, // Mark as internal
		}
		def.Fields = append(def.Fields, createdAtField)
	}

	// Add _updated_at field if it doesn't exist
	if !updatedAtExists {
		updatedAtField := common.FieldDefinition{
			Name:     "_updated_at",
			Type:     "datetime",
			Indexed:  true, // Index for efficient filtering
			Required: true,
			Internal: true, // Mark as internal
		}
		def.Fields = append(def.Fields, updatedAtField)
	}
}

// DebugInspectEntities provides direct access to the entities map for debugging purposes
// This should only be used in development environments
func (dse *Engine) DebugInspectEntities(inspector func(map[string]common.Entity)) {
	dse.mu.RLock()
	defer dse.mu.RUnlock()

	// Create a copy of the map to avoid exposing the internal map directly
	entitiesCopy := make(map[string]common.Entity, len(dse.entities))
	for k, v := range dse.entities {
		entitiesCopy[k] = v
	}

	inspector(entitiesCopy)
}

// MarkIDAsDeleted marks an ID as deleted so it won't be reused
func (dse *Engine) MarkIDAsDeleted(entityType string, id string) error {
	generator, err := dse.idGeneratorMgr.GetGenerator(entityType)
	if err != nil {
		return err
	}

	// Only apply to auto-increment generators
	if generator.Type() != common.IDTypeAutoIncrement {
		return nil
	}

	autoGen, ok := generator.(*AutoIncrementGenerator)
	if !ok {
		return fmt.Errorf("expected AutoIncrementGenerator, got %T", generator)
	}

	autoGen.MarkIDDeleted(entityType, id)
	return nil
}

// GetDeletedIDs gets the set of deleted IDs for an entity type
func (dse *Engine) GetDeletedIDs(entityType string) (map[string]bool, error) {
	generator, err := dse.idGeneratorMgr.GetGenerator(entityType)
	if err != nil {
		return nil, err
	}

	// Only apply to auto-increment generators
	if generator.Type() != common.IDTypeAutoIncrement {
		return make(map[string]bool), nil
	}

	autoGen, ok := generator.(*AutoIncrementGenerator)
	if !ok {
		return nil, fmt.Errorf("expected AutoIncrementGenerator, got %T", generator)
	}

	return autoGen.SaveDeletedIDs(entityType), nil
}

// LoadDeletedIDs loads a set of deleted IDs for an entity type
func (dse *Engine) LoadDeletedIDs(entityType string, deletedIDs map[string]bool) error {
	generator, err := dse.idGeneratorMgr.GetGenerator(entityType)
	if err != nil {
		return err
	}

	// Only apply to auto-increment generators
	if generator.Type() != common.IDTypeAutoIncrement {
		return nil
	}

	autoGen, ok := generator.(*AutoIncrementGenerator)
	if !ok {
		return fmt.Errorf("expected AutoIncrementGenerator, got %T", generator)
	}

	autoGen.LoadDeletedIDs(entityType, deletedIDs)
	return nil
}

// MigrateToCompositeKeys migrates the existing entity storage from flat keys to composite keys
// This function should be called during startup if there's a need to migrate older data
func (dse *Engine) MigrateToCompositeKeys() {
	dse.mu.Lock()
	defer dse.mu.Unlock()

	// Create a temp map to hold migrated entities
	migratedEntities := make(map[string]common.Entity)

	// Process all entities
	for key, entity := range dse.entities {
		// Check if this is already a composite key
		if strings.Contains(key, ":") {
			migratedEntities[key] = entity
			continue
		}

		// This is an old-style key, create a composite key
		newKey := createEntityKey(entity.Type, entity.ID)
		migratedEntities[newKey] = entity
	}

	// Replace the old map with the migrated one
	dse.entities = migratedEntities
}

// Add this function to validate uniqueness constraints
func (dse *Engine) validateUniqueness(entityType string, data map[string]interface{}, entityID string) error {
	def, exists := dse.definitions[entityType]
	if !exists {
		return fmt.Errorf("entity type %s not registered", entityType)
	}

	// Create a map to track which fields need uniqueness validation
	uniqueFields := make(map[string]bool)
	for _, fieldDef := range def.Fields {
		if fieldDef.Unique {
			uniqueFields[fieldDef.Name] = true
		}
	}

	// If no unique fields, return early
	if len(uniqueFields) == 0 {
		return nil
	}

	// For each unique field in the incoming data, check using the unique index
	for fieldName := range uniqueFields {
		value, exists := data[fieldName]
		if !exists || value == nil {
			// Skip fields that don't exist in the data or are null
			continue
		}

		// Get string representation of the value
		indexValue := uniqueIndexKey(value)

		// Check if this value exists in the unique index
		if dse.uniqueIndices[entityType] != nil &&
			dse.uniqueIndices[entityType][fieldName] != nil {
			if existingID, exists := dse.uniqueIndices[entityType][fieldName][indexValue]; exists {
				// If the existing ID is not the entity being updated, it's a conflict
				if existingID != entityID {
					return fmt.Errorf("unique constraint violation: field '%s' with value '%v' already exists in entity ID '%s'",
						fieldName, utilities.FormatValueForDisplay(value), existingID)
				}
			}
		}
	}

	return nil
}

func uniqueIndexKey(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", v)
	case float32, float64:
		return fmt.Sprintf("%g", v)
	case bool:
		return fmt.Sprintf("%t", v)
	case time.Time:
		return v.Format(time.RFC3339)
	default:
		// For complex types, use JSON serialization
		bytes, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(bytes)
	}
}

// updateUniqueIndices updates the unique indices for an entity
func (dse *Engine) updateUniqueIndices(entity common.Entity, add bool) {
	// Get entity definition
	def := dse.definitions[entity.Type]

	// Iterate through fields marked as unique
	for _, fieldDef := range def.Fields {
		if fieldDef.Unique {
			// Get the field value
			value, exists := entity.Fields[fieldDef.Name]
			if !exists || value == nil {
				continue // Skip nil or non-existent values
			}

			// Get string representation of the value
			indexValue := uniqueIndexKey(value)

			// Initialize the unique index map for this entity type and field if needed
			if add {
				if dse.uniqueIndices[entity.Type] == nil {
					dse.uniqueIndices[entity.Type] = make(map[string]map[string]string)
				}
				if dse.uniqueIndices[entity.Type][fieldDef.Name] == nil {
					dse.uniqueIndices[entity.Type][fieldDef.Name] = make(map[string]string)
				}

				// Add to unique index (value -> entity ID)
				dse.uniqueIndices[entity.Type][fieldDef.Name][indexValue] = entity.ID
			} else {
				// Remove from unique index
				if dse.uniqueIndices[entity.Type] != nil &&
					dse.uniqueIndices[entity.Type][fieldDef.Name] != nil {
					delete(dse.uniqueIndices[entity.Type][fieldDef.Name], indexValue)
				}
			}
		}
	}
}
