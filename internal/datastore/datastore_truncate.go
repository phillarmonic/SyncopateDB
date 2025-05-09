package datastore

import "github.com/phillarmonic/syncopate-db/internal/common"

// TruncateEntityType removes all entities of a specific type from the datastore
func (dse *Engine) TruncateEntityType(entityType string) error {
	// First check if the entity type exists
	dse.mu.RLock()
	_, exists := dse.definitions[entityType]
	dse.mu.RUnlock()

	if !exists {
		return entityTypeNotFoundError(entityType)
	}

	// Acquire write lock for the deletion operation
	dse.mu.Lock()
	defer dse.mu.Unlock()

	// Count how many entities will be removed for logging
	entitiesRemoved := 0

	// Find all entities of the specified type
	entitiesToRemove := make([]string, 0) // Store keys to remove
	for key, entity := range dse.entities {
		if entity.Type == entityType {
			entitiesToRemove = append(entitiesToRemove, key)
			entitiesRemoved++
		}
	}

	// If there are no entities to remove, just return success
	if entitiesRemoved == 0 {
		return nil
	}

	// Remove index entries for all entities
	for _, entityKey := range entitiesToRemove {
		entity := dse.entities[entityKey]
		// Remove the entity from indices
		dse.updateIndices(entity, false)
		// Remove the entity itself
		delete(dse.entities, entityKey)
	}

	// Store reference to persistence provider to use outside the lock
	persistenceProvider := dse.persistence

	// If persistence is enabled, record the truncate operation
	if persistenceProvider != nil {
		// Release the lock before potentially long-running persistence operation
		dse.mu.Unlock()
		defer dse.mu.Lock() // Re-acquire lock before returning

		err := persistenceProvider.TruncateEntityType(dse, entityType)
		if err != nil {
			// We don't roll back in-memory changes, but log the error
			// Clients should be aware that persistence failed
			return persistenceFailedError(err)
		}
	}

	return nil
}

// TruncateDatabase removes all entities from all entity types
func (dse *Engine) TruncateDatabase() error {
	// Acquire write lock for the entire operation
	dse.mu.Lock()
	defer dse.mu.Unlock()

	// Count how many entities of each type will be removed
	entityTypeCounts := make(map[string]int)
	totalEntities := 0

	// Count all entities by type for logging
	for _, entity := range dse.entities {
		entityTypeCounts[entity.Type]++
		totalEntities++
	}

	// If there are no entities, just return success
	if totalEntities == 0 {
		return nil
	}

	// Clear all entities (but keep entity type definitions)
	dse.entities = make(map[string]common.Entity)

	// Clear all indices
	for entityType := range dse.indices {
		// Initialize the indices for each entity type
		dse.indices[entityType] = make(map[string]map[string][]string)

		// Get the entity definition to reinitialize indices
		def, exists := dse.definitions[entityType]
		if !exists {
			continue // Shouldn't happen but just to be safe
		}

		// Initialize indices for indexed fields
		for _, field := range def.Fields {
			if field.Indexed {
				dse.indices[entityType][field.Name] = make(map[string][]string)
			}
		}
	}

	// Clear all unique indices
	for entityType := range dse.uniqueIndices {
		// Initialize the unique indices for each entity type
		dse.uniqueIndices[entityType] = make(map[string]map[string]string)

		// Get the entity definition to reinitialize unique indices
		def, exists := dse.definitions[entityType]
		if !exists {
			continue // Shouldn't happen but just to be safe
		}

		// Initialize unique indices for unique fields
		for _, field := range def.Fields {
			if field.Unique {
				dse.uniqueIndices[entityType][field.Name] = make(map[string]string)
			}
		}
	}

	// Store reference to persistence provider
	persistenceProvider := dse.persistence

	// Handle persistence outside the lock
	if persistenceProvider != nil {
		// Release the lock before potentially long-running persistence operation
		dse.mu.Unlock()
		defer dse.mu.Lock() // Re-acquire lock before returning

		err := persistenceProvider.TruncateDatabase(dse)
		if err != nil {
			// We don't roll back in-memory changes, but log the error
			return persistenceFailedError(err)
		}
	}

	return nil
}
