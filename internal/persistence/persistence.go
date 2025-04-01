package persistence

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/klauspost/compress/zstd"
	"github.com/phillarmonic/syncopate-db/internal/datastore"
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

// PersistenceEngine implements disk persistence for the datastore
type Engine struct {
	db           *badger.DB
	path         string
	compressor   *zstd.Encoder
	decompressor *zstd.Decoder
	entityCache  *LRUCache
	logger       *logrus.Logger
	mu           sync.RWMutex
	syncWAL      bool
}

// Config holds configuration for the persistence engine
type Config struct {
	Path             string
	CacheSize        int
	SyncWrites       bool
	SnapshotInterval time.Duration
	Logger           *logrus.Logger
}

// DefaultConfig returns a default configuration
func DefaultConfig() Config {
	return Config{
		Path:             "./data",
		CacheSize:        10000,
		SyncWrites:       true,
		SnapshotInterval: 10 * time.Minute,
		Logger:           logrus.New(),
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
		return nil, fmt.Errorf("failed to create decompressor: %w", err)
	}

	engine := &Engine{
		db:           db,
		path:         config.Path,
		compressor:   compressor,
		decompressor: decompressor,
		entityCache:  NewLRUCache(config.CacheSize),
		logger:       config.Logger,
		syncWAL:      config.SyncWrites,
	}

	// Start snapshot routine
	if config.SnapshotInterval > 0 {
		go engine.snapshotRoutine(config.SnapshotInterval)
	}

	return engine, nil
}

// Close closes the persistence engine
func (pe *Engine) Close() error {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	pe.compressor.Close()
	pe.decompressor.Close()
	return pe.db.Close()
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
	pe.mu.Lock()
	defer pe.mu.Unlock()

	// Create WAL entry
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

	// Write to database
	key := fmt.Sprintf("wal:%d:%s:%s", entry.Timestamp, entityType, entityID)
	return pe.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(key), buf.Bytes())
	})
}

// LoadWAL loads all WAL entries and applies them to the in-memory store
func (pe *Engine) LoadWAL(store *datastore.Engine) error {
	pe.mu.RLock()
	defer pe.mu.RUnlock()

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
func (pe *Engine) applyOperation(store *datastore.Engine, op int, entityType, entityID string, data []byte) error {
	switch op {
	case OpRegisterEntityType:
		var def datastore.EntityDefinition
		if err := gob.NewDecoder(bytes.NewBuffer(data)).Decode(&def); err != nil {
			return err
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
func (pe *Engine) TakeSnapshot(store *datastore.Engine) error {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	// Get the current timestamp
	timestamp := time.Now().UnixNano()
	snapshotKey := fmt.Sprintf("snapshot:%d", timestamp)

	// Serialize and compress the datastore
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)

	// First, write entity definitions
	entityTypes := store.ListEntityTypes()
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
func (pe *Engine) LoadLatestSnapshot(store *datastore.Engine) error {
	pe.mu.RLock()
	defer pe.mu.RUnlock()

	var snapshotKey string

	// Find the latest snapshot key
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

	// Load the snapshot
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
				var def datastore.EntityDefinition
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
					var entity datastore.Entity
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

// snapshotRoutine periodically creates snapshots
func (pe *Engine) snapshotRoutine(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		// We need a reference to the datastore to snapshot it
		// This will need to be provided when we integrate this with the main engine
	}
}

// RegisterEntityType registers a new entity type and persists the definition
func (pe *Engine) RegisterEntityType(store *datastore.Engine, def datastore.EntityDefinition) error {
	// First register in memory
	if err := store.RegisterEntityType(def); err != nil {
		return err
	}

	// Then write to WAL
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(def); err != nil {
		return fmt.Errorf("failed to encode entity definition: %w", err)
	}

	return pe.WriteWALEntry(OpRegisterEntityType, def.Name, "", buf.Bytes())
}

// Insert adds a new entity and persists it
func (pe *Engine) Insert(store *datastore.Engine, entityType, entityID string, data map[string]interface{}) error {
	// First insert in memory
	if err := store.Insert(entityType, entityID, data); err != nil {
		return err
	}

	// Then write to WAL
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(data); err != nil {
		return fmt.Errorf("failed to encode entity data: %w", err)
	}

	return pe.WriteWALEntry(OpInsertEntity, entityType, entityID, buf.Bytes())
}

// Update updates an entity and persists the changes
func (pe *Engine) Update(store *datastore.Engine, entityID string, data map[string]interface{}) error {
	// Get the entity to determine its type
	entity, err := store.Get(entityID)
	if err != nil {
		return err
	}

	// Update in memory
	if err := store.Update(entityID, data); err != nil {
		return err
	}

	// Write to WAL
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(data); err != nil {
		return fmt.Errorf("failed to encode entity data: %w", err)
	}

	return pe.WriteWALEntry(OpUpdateEntity, entity.Type, entityID, buf.Bytes())
}

// Delete removes an entity and persists the deletion
func (pe *Engine) Delete(store *datastore.Engine, entityID string) error {
	// Get the entity to determine its type
	entity, err := store.Get(entityID)
	if err != nil {
		return err
	}

	// Delete from memory
	if err := store.Delete(entityID); err != nil {
		return err
	}

	// Write to WAL
	return pe.WriteWALEntry(OpDeleteEntity, entity.Type, entityID, nil)
}
