package persistence

import (
	"github.com/phillarmonic/syncopate-db/internal/datastore"
	"github.com/sirupsen/logrus"
)

// Manager handles integration between the datastore and persistence
type Manager struct {
	engine      *datastore.Engine
	persistence *Engine
	logger      *logrus.Logger
}

// NewManager creates a new persistence manager
func NewManager(config Config) (*Manager, error) {
	// Initialize persistence engine
	persistenceEngine, err := NewPersistenceEngine(config)
	if err != nil {
		return nil, err
	}

	// Create datastore engine with persistence enabled
	datastoreEngine := datastore.NewDataStoreEngine(datastore.EngineConfig{
		Persistence:       persistenceEngine,
		EnablePersistence: true,
	})

	return &Manager{
		engine:      datastoreEngine,
		persistence: persistenceEngine,
		logger:      config.Logger,
	}, nil
}

// Engine returns the datastore engine
func (m *Manager) Engine() *datastore.Engine {
	return m.engine
}

// Close properly shuts down the persistence manager
func (m *Manager) Close() error {
	return m.engine.Close()
}

// ForceSnapshot forces an immediate snapshot
func (m *Manager) ForceSnapshot() error {
	return m.engine.ForceSnapshot()
}

// SetSnapshotInterval changes the snapshot interval
func (m *Manager) SetSnapshotInterval(seconds int) {
	// Implementation would need to be added to the persistence Engine
}

// GetStorageStats returns statistics about storage usage
func (m *Manager) GetStorageStats() map[string]interface{} {
	// This would need to be implemented by checking the database
	return map[string]interface{}{
		"entities_count": len(m.engine.ListEntityTypes()),
		// Additional stats would be added here
	}
}
