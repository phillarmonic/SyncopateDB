package persistence

import (
	"sync"
	"time"

	"github.com/phillarmonic/syncopate-db/internal/common"
	"github.com/phillarmonic/syncopate-db/internal/settings"
	"github.com/sirupsen/logrus"
)

// Manager handles integration between the datastore and persistence
type Manager struct {
	engine      common.DatastoreEngine
	persistence *Engine
	logger      *logrus.Logger
	gcTicker    *time.Ticker
	stopGC      chan struct{}
	mu          sync.RWMutex // Protect access to engine
}

// NewManager creates a new persistence manager
func NewManager(config Config) (*Manager, error) {
	// Apply settings from the settings package
	if config.Logger != nil {
		config.Logger.Infof("Using settings from config: WAL=%v, ZSTD=%v",
			settings.Config.EnableWAL, settings.Config.EnableZSTD)
	}

	// Update config with settings
	config.UseCompression = settings.Config.EnableZSTD

	// Initialize persistence engine
	persistenceEngine, err := NewPersistenceEngine(config)
	if err != nil {
		return nil, err
	}

	// The datastore engine will be set later when it's created
	manager := &Manager{
		persistence: persistenceEngine,
		logger:      config.Logger,
		stopGC:      make(chan struct{}),
	}

	// Start automatic garbage collection if enabled
	if config.EnableAutoGC && !settings.Config.Debug {
		manager.StartGarbageCollection(config.GCInterval)
	}

	return manager, nil
}

// SetEngine sets the datastore engine
func (m *Manager) SetEngine(engine common.DatastoreEngine) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.engine = engine
}

// Engine returns the datastore engine
func (m *Manager) Engine() common.DatastoreEngine {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.engine
}

// Close properly shuts down the persistence manager
func (m *Manager) Close() error {
	// Stop garbage collection if running
	m.StopGarbageCollection()

	// Take final snapshot (only if engine is set and not closed)
	m.mu.RLock()
	engine := m.engine
	m.mu.RUnlock()

	if engine != nil {
		if err := m.ForceSnapshot(); err != nil {
			// Only log as warning, don't fail the close operation
			m.logger.Warnf("Failed to take final snapshot on close: %v", err)
		}
	}

	// Close database
	return m.persistence.Close()
}

// ForceSnapshot forces an immediate snapshot
func (m *Manager) ForceSnapshot() error {
	m.mu.RLock()
	engine := m.engine
	m.mu.RUnlock()

	if engine == nil {
		return nil // No engine set yet
	}

	return m.persistence.TakeSnapshot(engine)
}

// RunValueLogGC runs garbage collection on the value log
func (m *Manager) RunValueLogGC(discardRatio float64) error {
	return m.persistence.RunValueLogGC(discardRatio)
}

// StartGarbageCollection starts periodic garbage collection
func (m *Manager) StartGarbageCollection(interval time.Duration) {
	// Skip if debug mode is enabled
	if settings.Config.Debug {
		m.logger.Info("Skipping automatic garbage collection in debug mode")
		return
	}

	// If already running, stop it first
	if m.gcTicker != nil {
		m.StopGarbageCollection()
	}

	m.logger.Info("Starting automatic garbage collection")
	m.gcTicker = time.NewTicker(interval)
	go func() {
		for {
			select {
			case <-m.gcTicker.C:
				m.runGarbageCollectionCycle()
			case <-m.stopGC:
				m.logger.Debug("Stopping garbage collection routine")
				return
			}
		}
	}()
}

// StopGarbageCollection stops the periodic garbage collection
func (m *Manager) StopGarbageCollection() {
	if m.gcTicker != nil {
		m.gcTicker.Stop()
		close(m.stopGC)
		m.gcTicker = nil
		m.stopGC = make(chan struct{})
	}
}

// runGarbageCollectionCycle performs a complete garbage collection cycle
func (m *Manager) runGarbageCollectionCycle() {
	m.logger.Debug("Running Badger value log garbage collection")
	err := m.RunValueLogGC(0.7) // 0.7 is the default discard ratio
	if err == nil {
		// If GC succeeded, run it again to collect more garbage
		m.logger.Debug("Value log GC successful, running again")
		// Add a small delay to give other processes a chance to run
		time.Sleep(500 * time.Millisecond)
		err = m.RunValueLogGC(0.7)
		if err == nil {
			m.logger.Debug("Second value log GC successful")
		} else if err.Error() != "Nothing to discard" {
			m.logger.Warnf("Error during second value log GC: %v", err)
		}
	} else if err.Error() != "Nothing to discard" {
		m.logger.Warnf("02: Error during value log GC: %v", err)
	} else {
		m.logger.Debug("Nothing to discard in garbage collection")
	}
}

// SetSnapshotInterval changes the snapshot interval
func (m *Manager) SetSnapshotInterval(interval time.Duration) {
	m.persistence.snapshotInterval = interval
}

// GetDatabaseSize returns the approximate size of the database in bytes
func (m *Manager) GetDatabaseSize() (int64, error) {
	return m.persistence.getDatabaseSize()
}

// GetStorageStats returns statistics about storage usage
func (m *Manager) GetStorageStats() map[string]interface{} {
	m.mu.RLock()
	engine := m.engine
	m.mu.RUnlock()

	if engine == nil {
		return map[string]interface{}{
			"status": "engine not set",
		}
	}

	stats := map[string]interface{}{
		"entity_types_count": len(engine.ListEntityTypes()),
		"settings": map[string]interface{}{
			"debug":       settings.Config.Debug,
			"enable_wal":  settings.Config.EnableWAL,
			"enable_zstd": settings.Config.EnableZSTD,
			"log_level":   settings.Config.LogLevel,
		},
	}

	// Add entity counts by type
	entityTypeStats := make(map[string]int)
	for _, typeName := range engine.ListEntityTypes() {
		count, err := engine.GetEntityCount(typeName)
		if err == nil {
			entityTypeStats[typeName] = count
		} else {
			entityTypeStats[typeName] = -1 // Error
		}
	}
	stats["entity_counts"] = entityTypeStats

	// Get database size
	dbSize, err := m.GetDatabaseSize()
	if err == nil {
		stats["database_size_bytes"] = dbSize
		stats["database_size_mb"] = float64(dbSize) / (1024 * 1024)
	}

	// Get LSM and value log file counts if possible
	lsmFiles, valueLogFiles, err := m.persistence.getFileStats()
	if err == nil {
		stats["lsm_files_count"] = lsmFiles
		stats["value_log_files_count"] = valueLogFiles
	}

	return stats
}

// Backup creates a backup of the database to the specified writer
func (m *Manager) Backup(path string) error {
	return m.persistence.createBackup(path)
}

// RunCompaction forces compaction of the LSM tree
func (m *Manager) RunCompaction() error {
	// In Badger v4, we can use Flatten() directly without checking for disabled compaction
	return m.persistence.db.Flatten(1)
}

// GetPersistenceProvider returns the persistence provider
func (m *Manager) GetPersistenceProvider() common.PersistenceProvider {
	return m.persistence
}
