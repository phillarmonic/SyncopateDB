package persistence

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"github.com/phillarmonic/syncopate-db/internal/settings"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/klauspost/compress/zstd"
	"github.com/phillarmonic/syncopate-db/internal/common"
	"github.com/sirupsen/logrus"
)

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
	useCompression   bool       // Flag to indicate if compression is enabled
	walSequence      uint64     // Last used WAL sequence number
	walSeqMutex      sync.Mutex // Mutex specifically for sequence number
	currentTxns      map[string]*Transaction
	txnMu            sync.Mutex
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
	UseCompression   bool
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
		UseCompression:   settings.Config.EnableZSTD, // Get from settings
	}
}

// NewPersistenceEngine creates a new persistence engine
func NewPersistenceEngine(config Config) (*Engine, error) {
	// Create a data directory if it doesn't exist
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
	}

	db, err := badger.Open(badgerOpts)

	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Initialize zstd compressor and decompressor if enabled in settings
	var compressor *zstd.Encoder
	var decompressor *zstd.Decoder

	// Check if ZSTD compression is enabled in settings
	useCompression := settings.Config.EnableZSTD

	if useCompression {
		compressor, err = zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
		if err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to create compressor: %w", err)
		}

		decompressor, err = zstd.NewReader(nil)
		if err != nil {
			db.Close()
			if compressor != nil {
				compressor.Close()
			}
			return nil, fmt.Errorf("failed to create decompressor: %w", err)
		}
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
		useCompression:   useCompression,
		walSequence:      0, // Initialize sequence counter
		currentTxns:      make(map[string]*Transaction),
	}

	// Start a snapshot routine
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
	if compressor != nil {
		compressor.Close()
	}
	if decompressor != nil {
		decompressor.Close()
	}
	return db.Close()
}

// Compress compresses data using zstd if enabled, otherwise returns the original data
func (pe *Engine) Compress(data []byte) []byte {
	if !pe.useCompression || pe.compressor == nil {
		return data // Return uncompressed data if compression is disabled
	}
	return pe.compressor.EncodeAll(data, nil)
}

// Decompress decompresses data using zstd if compression is enabled
func (pe *Engine) Decompress(data []byte) ([]byte, error) {
	if !pe.useCompression || pe.decompressor == nil {
		return data, nil // Return the data as-is if compression is disabled
	}
	return pe.decompressor.DecodeAll(data, nil)
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

	// Write the snapshot to the database
	err := pe.db.Update(func(txn *badger.Txn) error {
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

	if err != nil {
		return err
	}

	// Prune old WAL entries now that we have a new snapshot
	if err := pe.PruneWALBeforeTimestamp(timestamp); err != nil {
		pe.logger.Warnf("Failed to prune WAL entries: %v", err)
		// Continue even if pruning fails - this is not fatal
	}

	return nil
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

				// Clean up any duplicate internal fields
				cleanInternalFields(&def)

				// Mark internal fields
				for j := range def.Fields {
					if strings.HasPrefix(def.Fields[j].Name, "_") {
						def.Fields[j].Internal = true
					}
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
	// Check if WAL is disabled in settings
	if !settings.Config.EnableWAL {
		// Even without WAL, we still need to register the definition
		// in the database for future use
		var buf bytes.Buffer
		if err := gob.NewEncoder(&buf).Encode(def); err != nil {
			return fmt.Errorf("failed to encode entity definition: %w", err)
		}

		// Create a direct entry for the entity definition
		key := fmt.Sprintf("entitydef:%s", def.Name)
		return pe.db.Update(func(txn *badger.Txn) error {
			return txn.Set([]byte(key), pe.Compress(buf.Bytes()))
		})
	}

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

	// If WAL is disabled, write directly to the database
	if !settings.Config.EnableWAL {
		key := fmt.Sprintf("entity:%s:%s", entityType, entityID)
		return pe.db.Update(func(txn *badger.Txn) error {
			return txn.Set([]byte(key), pe.Compress(buf.Bytes()))
		})
	}

	// Write to WAL
	return pe.WriteWALEntry(OpInsertEntity, entityType, entityID, buf.Bytes())
}

// Update updates an entity and persists the changes
func (pe *Engine) Update(store common.DatastoreEngine, entityType string, entityID string, data map[string]interface{}) error {
	// Serialize the update data outside of any locks
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(data); err != nil {
		return fmt.Errorf("failed to encode entity data: %w", err)
	}

	// If WAL is disabled, update directly in the database
	if !settings.Config.EnableWAL {
		key := fmt.Sprintf("entity:%s:%s", entityType, entityID)
		return pe.db.Update(func(txn *badger.Txn) error {
			return txn.Set([]byte(key), pe.Compress(buf.Bytes()))
		})
	}

	// Write to WAL
	return pe.WriteWALEntry(OpUpdateEntity, entityType, entityID, buf.Bytes())
}

// Delete removes an entity and persists the deletion
func (pe *Engine) Delete(store common.DatastoreEngine, entityID string, entityType string) error {
	if !settings.Config.EnableWAL {
		// Key becomes, e.g., "entity:product:product:123" (using entityType and composite entityID)
		key := fmt.Sprintf("entity:%s:%s", entityType, entityID)
		// This key ("entity:product:product:123") likely doesn't match the stored key ("entity:product:123").
		// So, txn.Delete() might silently fail to delete the actual Badger entry.
		return pe.db.Update(func(txn *badger.Txn) error {
			return txn.Delete([]byte(key))
		})
	}
	// For WAL, entry.EntityID becomes "product:123", WAL key becomes "wal:...:product:product:123"
	return pe.WriteWALEntry(OpDeleteEntity, entityType, entityID, nil)
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

// PruneWALBeforeTimestamp removes WAL entries older than the given timestamp
func (pe *Engine) PruneWALBeforeTimestamp(timestamp int64) error {
	return pe.db.Update(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte("wal:")

		it := txn.NewIterator(opts)
		defer it.Close()

		keysToDelete := [][]byte{}

		// Collect all WAL keys before the timestamp
		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			key := item.Key()

			// Parse the timestamp from the key (format: "wal:timestamp:entityType:entityID")
			keyParts := strings.SplitN(string(key), ":", 4)
			if len(keyParts) < 2 {
				continue // Skip malformed keys
			}

			entryTimestamp, err := strconv.ParseInt(keyParts[1], 10, 64)
			if err != nil {
				pe.logger.Warnf("Invalid timestamp in WAL key %s: %v", string(key), err)
				continue
			}

			if entryTimestamp <= timestamp {
				keysToDelete = append(keysToDelete, append([]byte{}, key...))
			}
		}

		// Delete collected keys
		for _, key := range keysToDelete {
			if err := txn.Delete(key); err != nil {
				return fmt.Errorf("failed to delete old WAL entry: %w", err)
			}
		}

		pe.logger.Infof("Pruned %d WAL entries older than %s", len(keysToDelete),
			time.Unix(0, timestamp).Format(time.RFC3339))
		return nil
	})
}

// applyOperationWithErrorHandling applies a WAL operation with improved error handling
func (pe *Engine) applyOperationWithErrorHandling(store common.DatastoreEngine, op int, entityType, entityID string, data []byte) error {
	switch op {
	case OpRegisterEntityType:
		var def common.EntityDefinition
		if err := gob.NewDecoder(bytes.NewBuffer(data)).Decode(&def); err != nil {
			return fmt.Errorf("failed to decode entity definition: %w", err)
		}

		// Check if the entity type already exists
		existingDef, err := store.GetEntityDefinition(def.Name)
		if err == nil {
			// Entity type already exists, compare the definitions
			if compareEntityDefinitions(existingDef, def) {
				// Definitions are identical, skip registration
				pe.logger.Debugf("Entity type %s already exists with identical definition, skipping", def.Name)
				return nil
			} else {
				// Definitions are different, log a warning
				pe.logger.Warnf("Entity type %s already exists with different definition, using existing definition", def.Name)
				return nil
			}
		}

		// Clean up any duplicate internal fields
		ensureNoDuplicateInternalFields(&def)

		// Mark internal fields
		for i := range def.Fields {
			if strings.HasPrefix(def.Fields[i].Name, "_") {
				def.Fields[i].Internal = true
			}
		}

		return store.RegisterEntityType(def)

	case OpInsertEntity:
		var fields map[string]interface{}
		if err := gob.NewDecoder(bytes.NewBuffer(data)).Decode(&fields); err != nil {
			return fmt.Errorf("failed to decode entity fields: %w", err)
		}

		// Check if entity already exists before inserting
		_, err := store.Get(entityID)
		if err == nil {
			// Entity already exists, skip insertion
			pe.logger.Debugf("Entity '%s' already exists, skipping insertion", entityID)
			return nil
		}

		return store.Insert(entityType, entityID, fields)

	case OpUpdateEntity:
		var fields map[string]interface{}
		if err := gob.NewDecoder(bytes.NewBuffer(data)).Decode(&fields); err != nil {
			return fmt.Errorf("failed to decode update fields: %w", err)
		}

		// Check if entity exists before updating
		entity, err := store.Get(entityID)
		if err != nil {
			// Entity doesn't exist, skip update
			pe.logger.Warnf("Entity '%s' doesn't exist, skipping update", entityID)
			return nil
		}

		// Pass the entity type from the entity we retrieved
		return store.Update(entity.Type, entityID, fields)

	case OpDeleteEntity:
		// Check if entity exists before deleting
		_, err := store.Get(entityID)
		if err != nil {
			// Entity doesn't exist, skip deletion
			pe.logger.Warnf("Entity '%s' doesn't exist, skipping deletion", entityID)
			return nil
		}

		return store.Delete(entityType, entityID)

	case OpUpdateEntityType:
		var def common.EntityDefinition
		if err := gob.NewDecoder(bytes.NewBuffer(data)).Decode(&def); err != nil {
			return fmt.Errorf("failed to decode entity definition: %w", err)
		}

		// For WAL replays, we need to check if the entity type exists
		_, err := store.GetEntityDefinition(def.Name)
		if err != nil {
			// Entity type doesn't exist, register it instead

			// Clean up any duplicate internal fields before registering
			ensureNoDuplicateInternalFields(&def)

			// Mark internal fields
			for i := range def.Fields {
				if strings.HasPrefix(def.Fields[i].Name, "_") {
					def.Fields[i].Internal = true
				}
			}

			return store.RegisterEntityType(def)
		}

		// Clean up any duplicate internal fields before updating
		ensureNoDuplicateInternalFields(&def)

		// Mark internal fields
		for i := range def.Fields {
			if strings.HasPrefix(def.Fields[i].Name, "_") {
				def.Fields[i].Internal = true
			}
		}

		// Update the entity type definition
		return store.UpdateEntityType(def)

	case OpTruncateEntityType:
		// For WAL replay, truncate the specified entity type
		return store.TruncateEntityType(entityType)

	case OpTruncateDatabase:
		// For WAL replay, truncate the entire database
		return store.TruncateDatabase()
	default:
		return fmt.Errorf("unknown operation: %d", op)
	}
}

// cleanInternalFields removes duplicate internal fields from entity definitions
// while preserving all data fields
func cleanInternalFields(def *common.EntityDefinition) {
	// Check for duplicate internal fields and remove them
	createdAtCount := 0
	updatedAtCount := 0

	// First count how many of each internal field we have
	for _, field := range def.Fields {
		if field.Name == "_created_at" {
			createdAtCount++
		}
		if field.Name == "_updated_at" {
			updatedAtCount++
		}
	}

	// If duplicates exist, rebuild the fields slice without them
	if createdAtCount > 1 || updatedAtCount > 1 {
		newFields := make([]common.FieldDefinition, 0, len(def.Fields))
		createdAtAdded := false
		updatedAtAdded := false

		for _, field := range def.Fields {
			// For internal fields, only add the first instance
			if field.Name == "_created_at" {
				if !createdAtAdded {
					newFields = append(newFields, field)
					createdAtAdded = true
				}
				continue
			}
			if field.Name == "_updated_at" {
				if !updatedAtAdded {
					newFields = append(newFields, field)
					updatedAtAdded = true
				}
				continue
			}

			// Add all other fields (keeping all data fields intact)
			newFields = append(newFields, field)
		}

		// Replace the fields with the cleaned version
		def.Fields = newFields
	}
}

func (pe *Engine) Write(store common.DatastoreEngine, data []byte) error {
	// Convert the byte data to a string to analyze it
	operation := string(data)

	// Check if this is a truncate operation
	if len(operation) > 19 && operation[:19] == "TRUNCATE_ENTITY_TYPE" {
		// Extract the entity type
		entityType := operation[20:] // Skip the "TRUNCATE_ENTITY_TYPE:" prefix

		// Check if WAL is disabled in settings
		if !settings.Config.EnableWAL {
			// Direct truncate without WAL
			// We'll use a prefix pattern for entity types
			prefix := fmt.Sprintf("entity:%s:", entityType)

			return pe.db.Update(func(txn *badger.Txn) error {
				opts := badger.DefaultIteratorOptions
				opts.Prefix = []byte(prefix)

				it := txn.NewIterator(opts)
				defer it.Close()

				// Collect all keys to delete
				var keysToDelete [][]byte
				for it.Rewind(); it.Valid(); it.Next() {
					key := it.Item().Key()
					keysToDelete = append(keysToDelete, append([]byte{}, key...))
				}

				// Delete all collected keys
				for _, key := range keysToDelete {
					if err := txn.Delete(key); err != nil {
						return fmt.Errorf("failed to delete entity: %w", err)
					}
				}

				return nil
			})
		}

		// With WAL enabled, write a special truncate operation to the WAL
		return pe.WriteWALEntry(OpTruncateEntityType, entityType, "", nil)
	} else if operation == "TRUNCATE_DATABASE" {
		// Check if WAL is disabled in settings
		if !settings.Config.EnableWAL {
			// Direct truncate without WAL - much more efficient to do a range scan and delete
			return pe.db.Update(func(txn *badger.Txn) error {
				opts := badger.DefaultIteratorOptions
				opts.Prefix = []byte("entity:") // All entity keys start with this prefix

				it := txn.NewIterator(opts)
				defer it.Close()

				// Collect all keys to delete
				var keysToDelete [][]byte
				for it.Rewind(); it.Valid(); it.Next() {
					key := it.Item().Key()
					keysToDelete = append(keysToDelete, append([]byte{}, key...))
				}

				// Delete all collected keys
				for _, key := range keysToDelete {
					if err := txn.Delete(key); err != nil {
						return fmt.Errorf("failed to delete entity: %w", err)
					}
				}

				return nil
			})
		}

		// With WAL enabled, write a special truncate operation to the WAL
		return pe.WriteWALEntry(OpTruncateDatabase, "", "", nil)
	}

	// For other custom operations, store as-is with a generic key
	key := fmt.Sprintf("custom:%d", time.Now().UnixNano())
	return pe.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(key), data)
	})
}
