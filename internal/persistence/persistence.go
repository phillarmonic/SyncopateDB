package persistence

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/klauspost/compress/zstd"
	"github.com/phillarmonic/syncopate-db/internal/common"
	"github.com/sirupsen/logrus"
)

// Operation types for WAL entries
const (
	OpRegisterEntityType = iota + 1
	OpInsertEntity
	OpUpdateEntity
	OpDeleteEntity
)

// WALEntry represents a write-ahead log entry
type WALEntry struct {
	Timestamp  int64
	Operation  int
	EntityType string
	EntityID   string
	Data       []byte // Compressed serialized data
}

// Engine implements disk persistence for the datastore
type Engine struct {
	db               *badger.DB
	path             string
	compressor       *zstd.Encoder
	decompressor     *zstd.Decoder
	entityCache      *LRUCache
	logger           *logrus.Logger
	mu               sync.RWMutex
	syncWAL          bool
	snapshotInterval time.Duration
	stopSnapshot     chan struct{}
	snapshotTicker   *time.Ticker
}

// Config holds configuration for the persistence engine
type Config struct {
	Path             string
	CacheSize        int
	SyncWrites       bool
	SnapshotInterval time.Duration
	Logger           *logrus.Logger
	EncryptionKey    []byte
	EnableAutoGC     bool
	GCInterval       time.Duration
}

func init() {
	// Register types for gob encoding/decoding
	gob.Register([]interface{}{})
	gob.Register(map[string]interface{}{})
	gob.Register(map[interface{}]interface{}{})
	gob.Register(time.Time{})
}

// DefaultConfig returns a default configuration
func DefaultConfig() Config {
	return Config{
		Path:             "./data",
		CacheSize:        10000,
		SyncWrites:       true,
		SnapshotInterval: 10 * time.Minute,
		Logger:           logrus.New(),
		EnableAutoGC:     true,
		GCInterval:       5 * time.Minute,
	}
}

// NewPersistenceEngine creates a new persistence engine
func NewPersistenceEngine(config Config) (*Engine, error) {
	// Create data directory if it doesn't exist
	if err := os.MkdirAll(config.Path, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	// Initialize Badger DB
	badgerOpts := badger.DefaultOptions(config.Path).
		WithSyncWrites(config.SyncWrites).
		WithLogger(config.Logger)

	// Add encryption if key is provided
	if len(config.EncryptionKey) > 0 {
		badgerOpts = badgerOpts.
			WithEncryptionKey(config.EncryptionKey)
		// Note: WithIndexCache was removed in Badger v4
	}

	db, err := badger.Open(badgerOpts)

	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Initialize zstd compressor and decompressor
	compressor, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create compressor: %w", err)
	}

	decompressor, err := zstd.NewReader(nil)
	if err != nil {
		db.Close()
		compressor.Close()
		return nil, fmt.Errorf("failed to create decompressor: %w", err)
	}

	engine := &Engine{
		db:               db,
		path:             config.Path,
		compressor:       compressor,
		decompressor:     decompressor,
		entityCache:      NewLRUCache(config.CacheSize),
		logger:           config.Logger,
		syncWAL:          config.SyncWrites,
		snapshotInterval: config.SnapshotInterval,
		stopSnapshot:     make(chan struct{}),
	}

	// Start snapshot routine
	if config.SnapshotInterval > 0 {
		engine.startSnapshotRoutine()
	}

	return engine, nil
}

// Close closes the persistence engine
func (pe *Engine) Close() error {
	pe.mu.Lock()

	// Stop the snapshot routine if running
	if pe.snapshotTicker != nil {
		pe.snapshotTicker.Stop()
		close(pe.stopSnapshot)
	}

	// Get references to resources that need to be closed
	compressor := pe.compressor
	decompressor := pe.decompressor
	db := pe.db

	// Clear references before releasing lock
	pe.compressor = nil
	pe.decompressor = nil
	pe.db = nil

	pe.mu.Unlock()

	// Close resources without holding the lock
	compressor.Close()
	decompressor.Close()
	return db.Close()
}

// Compress compresses data using zstd
func (pe *Engine) Compress(data []byte) []byte {
	return pe.compressor.EncodeAll(data, nil)
}

// Decompress decompresses data using zstd
func (pe *Engine) Decompress(data []byte) ([]byte, error) {
	return pe.decompressor.DecodeAll(data, nil)
}

// WriteWALEntry writes an operation to the write-ahead log
func (pe *Engine) WriteWALEntry(op int, entityType, entityID string, data []byte) error {
	// Create WAL entry outside of the lock
	entry := WALEntry{
		Timestamp:  time.Now().UnixNano(),
		Operation:  op,
		EntityType: entityType,
		EntityID:   entityID,
		Data:       pe.Compress(data),
	}

	// Serialize entry
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(entry); err != nil {
		return fmt.Errorf("failed to encode WAL entry: %w", err)
	}

	// Create the key
	key := fmt.Sprintf("wal:%d:%s:%s", entry.Timestamp, entityType, entityID)

	// No need to lock for this DB operation - Badger handles its own thread safety
	return pe.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(key), buf.Bytes())
	})
}

// LoadWAL loads all WAL entries and applies them to the in-memory store
// This should only be called during initialization before the server starts
func (pe *Engine) LoadWAL(store common.DatastoreEngine) error {
	// No need for RLock here since this is called during initialization

	return pe.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte("wal:")

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()

			err := item.Value(func(val []byte) error {
				var entry WALEntry
				buf := bytes.NewBuffer(val)

				if err := gob.NewDecoder(buf).Decode(&entry); err != nil {
					return fmt.Errorf("failed to decode WAL entry: %w", err)
				}

				// Decompress data
				data, err := pe.Decompress(entry.Data)
				if err != nil {
					return fmt.Errorf("failed to decompress WAL data: %w", err)
				}

				// Apply operation to the store
				return pe.applyOperation(store, entry.Operation, entry.EntityType, entry.EntityID, data)
			})

			if err != nil {
				return err
			}
		}

		return nil
	})
}

// applyOperation applies a WAL operation to the datastore
// This is only called during initialization
func (pe *Engine) applyOperation(store common.DatastoreEngine, op int, entityType, entityID string, data []byte) error {
	switch op {
	case OpRegisterEntityType:
		var def common.EntityDefinition
		if err := gob.NewDecoder(bytes.NewBuffer(data)).Decode(&def); err != nil {
			return err
		}

		// Check if the entity type already exists before trying to register it
		existingDef, err := store.GetEntityDefinition(def.Name)
		if err == nil {
			// Entity type already exists, compare the definitions
			if compareEntityDefinitions(existingDef, def) {
				// Definitions are identical, skip registration
				pe.logger.Debugf("Entity type %s already exists during WAL replay with identical definition, skipping registration", def.Name)
				return nil
			} else {
				// Definitions are different, log a warning with details
				pe.logger.Warnf("Entity type %s already exists during WAL replay with DIFFERENT definition. This may indicate a schema change that wasn't properly migrated.", def.Name)
				pe.logger.Warnf("Existing fields: %v, New fields: %v", existingDef.Fields, def.Fields)
				// Continue with existing definition - don't try to re-register
				return nil
			}
		}

		return store.RegisterEntityType(def)

	case OpInsertEntity:
		var fields map[string]interface{}
		if err := gob.NewDecoder(bytes.NewBuffer(data)).Decode(&fields); err != nil {
			return err
		}
		return store.Insert(entityType, entityID, fields)

	case OpUpdateEntity:
		var fields map[string]interface{}
		if err := gob.NewDecoder(bytes.NewBuffer(data)).Decode(&fields); err != nil {
			return err
		}
		return store.Update(entityID, fields)

	case OpDeleteEntity:
		return store.Delete(entityID)

	default:
		return fmt.Errorf("unknown operation: %d", op)
	}
}

// TakeSnapshot creates a full snapshot of the current state
func (pe *Engine) TakeSnapshot(store common.DatastoreEngine) error {
	// Get the current timestamp and create keys outside the lock
	timestamp := time.Now().UnixNano()
	snapshotKey := fmt.Sprintf("snapshot:%d", timestamp)

	// Get entity types and definitions without locking persistence engine
	entityTypes := store.ListEntityTypes()

	// Create a buffer for serialization
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)

	// First, write entity definitions
	if err := enc.Encode(len(entityTypes)); err != nil {
		return fmt.Errorf("failed to encode entity type count: %w", err)
	}

	for _, typeName := range entityTypes {
		def, err := store.GetEntityDefinition(typeName)
		if err != nil {
			return fmt.Errorf("failed to get entity definition: %w", err)
		}

		if err := enc.Encode(def); err != nil {
			return fmt.Errorf("failed to encode entity definition: %w", err)
		}

		// Get and write all entities of this type
		entities, err := store.GetAllEntitiesOfType(typeName)
		if err != nil {
			return fmt.Errorf("failed to get entities: %w", err)
		}

		if err := enc.Encode(len(entities)); err != nil {
			return fmt.Errorf("failed to encode entity count: %w", err)
		}

		for _, entity := range entities {
			if err := enc.Encode(entity); err != nil {
				return fmt.Errorf("failed to encode entity: %w", err)
			}
		}
	}

	// Compress the snapshot
	compressedData := pe.Compress(buf.Bytes())

	// Write the snapshot to the database - Badger handles its own thread safety
	return pe.db.Update(func(txn *badger.Txn) error {
		// Store the snapshot
		if err := txn.Set([]byte(snapshotKey), compressedData); err != nil {
			return err
		}

		// Update the latest snapshot pointer
		latestKey := []byte("latest_snapshot")
		latestValue := make([]byte, 8)
		binary.LittleEndian.PutUint64(latestValue, uint64(timestamp))

		return txn.Set(latestKey, latestValue)
	})
}

// LoadLatestSnapshot loads the most recent snapshot
// This should only be called during initialization
func (pe *Engine) LoadLatestSnapshot(store common.DatastoreEngine) error {
	var snapshotKey string

	// Find the latest snapshot key - Badger handles its own thread safety
	err := pe.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("latest_snapshot"))
		if err == badger.ErrKeyNotFound {
			return nil // No snapshot yet
		}
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			timestamp := binary.LittleEndian.Uint64(val)
			snapshotKey = fmt.Sprintf("snapshot:%d", timestamp)
			return nil
		})
	})

	if err != nil {
		return err
	}

	if snapshotKey == "" {
		// No snapshot exists
		return nil
	}

	// Load the snapshot - Badger handles its own thread safety
	return pe.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(snapshotKey))
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			// Decompress the snapshot
			data, err := pe.Decompress(val)
			if err != nil {
				return fmt.Errorf("failed to decompress snapshot: %w", err)
			}

			// Deserialize and load into the store
			buf := bytes.NewBuffer(data)
			dec := gob.NewDecoder(buf)

			// Read entity definitions
			var typeCount int
			if err := dec.Decode(&typeCount); err != nil {
				return fmt.Errorf("failed to decode entity type count: %w", err)
			}

			for i := 0; i < typeCount; i++ {
				var def common.EntityDefinition
				if err := dec.Decode(&def); err != nil {
					return fmt.Errorf("failed to decode entity definition: %w", err)
				}

				if err := store.RegisterEntityType(def); err != nil {
					return fmt.Errorf("failed to register entity type: %w", err)
				}

				// Read entities for this type
				var entityCount int
				if err := dec.Decode(&entityCount); err != nil {
					return fmt.Errorf("failed to decode entity count: %w", err)
				}

				for j := 0; j < entityCount; j++ {
					var entity common.Entity
					if err := dec.Decode(&entity); err != nil {
						return fmt.Errorf("failed to decode entity: %w", err)
					}

					if err := store.Insert(entity.Type, entity.ID, entity.Fields); err != nil {
						return fmt.Errorf("failed to insert entity: %w", err)
					}
				}
			}

			return nil
		})
	})
}

// startSnapshotRoutine starts the periodic snapshot routine
func (pe *Engine) startSnapshotRoutine() {
	pe.snapshotTicker = time.NewTicker(pe.snapshotInterval)
	go func() {
		for {
			select {
			case <-pe.snapshotTicker.C:
				// We need a reference to the datastore to snapshot it
				// This will be provided when the snapshot is triggered from the Manager
				pe.logger.Debug("Snapshot interval reached, waiting for snapshot to be triggered")
			case <-pe.stopSnapshot:
				pe.logger.Debug("Stopping snapshot routine")
				return
			}
		}
	}()
}

// RegisterEntityType registers a new entity type and persists the definition
func (pe *Engine) RegisterEntityType(store common.DatastoreEngine, def common.EntityDefinition) error {
	// Serialize the entity definition outside of any locks
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(def); err != nil {
		return fmt.Errorf("failed to encode entity definition: %w", err)
	}

	// Write to WAL
	return pe.WriteWALEntry(OpRegisterEntityType, def.Name, "", buf.Bytes())
}

// Insert adds a new entity and persists it
func (pe *Engine) Insert(store common.DatastoreEngine, entityType, entityID string, data map[string]interface{}) error {
	// Serialize the entity data outside of any locks
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(data); err != nil {
		return fmt.Errorf("failed to encode entity data: %w", err)
	}

	// Write to WAL
	return pe.WriteWALEntry(OpInsertEntity, entityType, entityID, buf.Bytes())
}

// Update updates an entity and persists the changes
func (pe *Engine) Update(store common.DatastoreEngine, entityID string, data map[string]interface{}) error {
	// Get the entity to determine its type
	entity, err := store.Get(entityID)
	if err != nil {
		return err
	}

	// Serialize the update data outside of any locks
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(data); err != nil {
		return fmt.Errorf("failed to encode entity data: %w", err)
	}

	// Write to WAL
	return pe.WriteWALEntry(OpUpdateEntity, entity.Type, entityID, buf.Bytes())
}

// Delete removes an entity and persists the deletion
func (pe *Engine) Delete(store common.DatastoreEngine, entityID string) error {
	// Get the entity to determine its type
	entity, err := store.Get(entityID)
	if err != nil {
		return err
	}

	// Write to WAL
	return pe.WriteWALEntry(OpDeleteEntity, entity.Type, entityID, nil)
}

// RunValueLogGC runs garbage collection on the value log
func (pe *Engine) RunValueLogGC(discardRatio float64) error {
	return pe.db.RunValueLogGC(discardRatio)
}

// getDatabaseSize returns the approximate size of the database in bytes
func (pe *Engine) getDatabaseSize() (int64, error) {
	lsm, vlog := pe.db.Size()
	// Return the combined size of LSM tree and value log
	return lsm + vlog, nil
}

// getFileStats returns counts of LSM and value log files
func (pe *Engine) getFileStats() (lsmFiles, valueLogFiles int, err error) {
	// Implementation to count .sst and .vlog files in the database directory
	lsmFiles = 0
	valueLogFiles = 0

	err = filepath.Walk(pe.path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			ext := filepath.Ext(path)
			if ext == ".sst" {
				lsmFiles++
			} else if ext == ".vlog" {
				valueLogFiles++
			}
		}
		return nil
	})

	return lsmFiles, valueLogFiles, err
}

// createBackup creates a backup of the database to the specified path
func (pe *Engine) createBackup(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create backup file: %w", err)
	}
	defer f.Close()

	_, err = pe.db.Backup(f, 0)
	if err != nil {
		return fmt.Errorf("failed to backup database: %w", err)
	}

	return nil
}

// RestoreFromBackup restores the database from a backup file
func (pe *Engine) RestoreFromBackup(store common.DatastoreEngine, backupPath string) error {
	// This is a complex operation that should be performed when the system is idle
	// Acquire a write lock to prevent any other operations
	pe.mu.Lock()
	defer pe.mu.Unlock()

	// Close the current database
	if err := pe.db.Close(); err != nil {
		return fmt.Errorf("failed to close database for restore: %w", err)
	}

	// Backup the existing data directory
	backupDir := pe.path + ".bak." + time.Now().Format("20060102150405")
	if err := os.Rename(pe.path, backupDir); err != nil {
		return fmt.Errorf("failed to backup existing data directory: %w", err)
	}

	// Create a new empty directory
	if err := os.MkdirAll(pe.path, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	// Open the backup file
	f, err := os.Open(backupPath)
	if err != nil {
		return fmt.Errorf("failed to open backup file: %w", err)
	}
	defer f.Close()

	// Create a new Badger database for the restore
	opts := badger.DefaultOptions(pe.path)
	db, err := badger.Open(opts)
	if err != nil {
		return fmt.Errorf("failed to open database for restore: %w", err)
	}

	// Load the backup
	if err := db.Load(f, 16); err != nil {
		db.Close()
		return fmt.Errorf("failed to load backup: %w", err)
	}

	// Close the temporary database
	if err := db.Close(); err != nil {
		return fmt.Errorf("failed to close database after restore: %w", err)
	}

	// Reopen the engine with the restored data
	pe.db, err = badger.Open(badger.DefaultOptions(pe.path))
	if err != nil {
		return fmt.Errorf("failed to reopen database after restore: %w", err)
	}

	// Load the restored data into the store - this is done outside normal operations
	if err := pe.LoadLatestSnapshot(store); err != nil {
		return fmt.Errorf("failed to load snapshot after restore: %w", err)
	}

	if err := pe.LoadWAL(store); err != nil {
		return fmt.Errorf("failed to load WAL after restore: %w", err)
	}

	return nil
}

// StreamBackup streams a backup of the database to the provided writer
func (pe *Engine) StreamBackup(w io.Writer) error {
	_, err := pe.db.Backup(w, 0)
	return err
}

// GetPersistenceProvider returns the persistence provider for the Manager
func (pe *Engine) GetPersistenceProvider() common.PersistenceProvider {
	return pe
}

// compareEntityDefinitions checks if two entity definitions are identical
func compareEntityDefinitions(def1, def2 common.EntityDefinition) bool {
	// Check if names match
	if def1.Name != def2.Name {
		return false
	}

	// Check if they have the same number of fields
	if len(def1.Fields) != len(def2.Fields) {
		return false
	}

	// Create a map of fields from def1 for easy lookup
	fields1 := make(map[string]common.FieldDefinition)
	for _, field := range def1.Fields {
		fields1[field.Name] = field
	}

	// Compare each field in def2 with def1
	for _, field2 := range def2.Fields {
		field1, exists := fields1[field2.Name]
		if !exists {
			// Field exists in def2 but not in def1
			return false
		}

		// Compare field properties
		if field1.Type != field2.Type ||
			field1.Indexed != field2.Indexed ||
			field1.Required != field2.Required {
			return false
		}
	}

	return true
}

// LoadCounters loads auto-increment counters from the database
func (pe *Engine) LoadCounters(store common.DatastoreEngine) error {
	// This method should be called after loading the snapshot and WAL
	// to ensure auto-increment counters are initialized correctly

	return pe.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte("counter:")

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			key := string(item.Key())
			parts := strings.SplitN(key, ":", 2)
			if len(parts) != 2 {
				continue
			}

			entityType := parts[1]

			err := item.Value(func(val []byte) error {
				counter := binary.LittleEndian.Uint64(val)

				// Call a method on the store to set the counter value
				// We need to add this method to our datastore engine
				return store.SetAutoIncrementCounter(entityType, counter)
			})

			if err != nil {
				return err
			}
		}

		return nil
	})
}

// SaveCounter saves an auto-increment counter to the database
func (pe *Engine) SaveCounter(entityType string, counter uint64) error {
	key := fmt.Sprintf("counter:%s", entityType)
	value := make([]byte, 8)
	binary.LittleEndian.PutUint64(value, counter)

	return pe.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(key), value)
	})
}
