package persistence

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
	"github.com/phillarmonic/syncopate-db/internal/common"
	"github.com/phillarmonic/syncopate-db/internal/settings"
	"sort"
	"strings"
	"sync"
	"time"
)

// Operation types for WAL entries
const (
	OpRegisterEntityType = iota + 1
	OpInsertEntity
	OpUpdateEntity
	OpDeleteEntity
	OpUpdateEntityType
	OpTruncateEntityType
	OpTruncateDatabase
)

// WALEntry represents a write-ahead log entry
type WALEntry struct {
	Timestamp     int64
	SequenceNum   uint64
	TransactionID string // Added transaction ID
	Operation     int
	EntityType    string
	EntityID      string
	Data          []byte
	IsLastInTxn   bool // Flag indicating last operation in a transaction
}

// The Transaction represents a group of operations that should be applied atomically
type Transaction struct {
	ID      string
	Entries []WALEntry
	mu      sync.Mutex
}

// BeginTransaction starts a new transaction
func (pe *Engine) BeginTransaction() string {
	txnID := uuid.New().String() // You'll need to import "github.com/google/uuid"

	pe.txnMu.Lock()
	defer pe.txnMu.Unlock()

	pe.currentTxns[txnID] = &Transaction{
		ID:      txnID,
		Entries: []WALEntry{},
	}

	return txnID
}

// AbortTransaction discards a transaction
func (pe *Engine) AbortTransaction(txnID string) error {
	pe.txnMu.Lock()
	defer pe.txnMu.Unlock()

	if _, exists := pe.currentTxns[txnID]; !exists {
		return fmt.Errorf("transaction %s not found", txnID)
	}

	delete(pe.currentTxns, txnID)
	return nil
}

// CommitTransaction persists all operations in a transaction
func (pe *Engine) CommitTransaction(txnID string) error {
	pe.txnMu.Lock()
	txn, exists := pe.currentTxns[txnID]
	if !exists {
		pe.txnMu.Unlock()
		return fmt.Errorf("transaction %s not found", txnID)
	}

	// Make a copy of entries and remove transaction
	entries := make([]WALEntry, len(txn.Entries))
	copy(entries, txn.Entries)
	delete(pe.currentTxns, txnID)
	pe.txnMu.Unlock()

	// Mark last entry as last in transaction
	if len(entries) > 0 {
		entries[len(entries)-1].IsLastInTxn = true
	}

	// Write all entries to WAL
	for _, entry := range entries {
		// Get next sequence number
		pe.walSeqMutex.Lock()
		pe.walSequence++
		entry.SequenceNum = pe.walSequence
		pe.walSeqMutex.Unlock()

		// Serialize entry
		var buf bytes.Buffer
		if err := gob.NewEncoder(&buf).Encode(entry); err != nil {
			return fmt.Errorf("failed to encode WAL entry: %w", err)
		}

		// Create the key with sequence number for proper ordering
		key := fmt.Sprintf("wal:%020d:%s:%s", entry.SequenceNum, entry.EntityType, entry.EntityID)

		// Write to database
		if err := pe.db.Update(func(txn *badger.Txn) error {
			return txn.Set([]byte(key), buf.Bytes())
		}); err != nil {
			return fmt.Errorf("failed to write WAL entry: %w", err)
		}
	}

	return nil
}

// AddToTransaction adds an operation to a transaction
func (pe *Engine) AddToTransaction(txnID string, op int, entityType, entityID string, data []byte) error {
	pe.txnMu.Lock()
	defer pe.txnMu.Unlock()

	txn, exists := pe.currentTxns[txnID]
	if !exists {
		return fmt.Errorf("transaction %s not found", txnID)
	}

	// Add the entry to the transaction
	txn.mu.Lock()
	defer txn.mu.Unlock()

	txn.Entries = append(txn.Entries, WALEntry{
		Timestamp:     time.Now().UnixNano(),
		TransactionID: txnID,
		Operation:     op,
		EntityType:    entityType,
		EntityID:      entityID,
		Data:          pe.Compress(data),
	})

	return nil
}

// WriteWALEntry writes an operation to the write-ahead log
func (pe *Engine) WriteWALEntry(op int, entityType, entityID string, data []byte) error {
	// Check if WAL is disabled in settings
	if !settings.Config.EnableWAL {
		return nil // Skip WAL if disabled
	}

	// Get next sequence number with proper locking
	pe.walSeqMutex.Lock()
	pe.walSequence++
	sequenceNum := pe.walSequence
	pe.walSeqMutex.Unlock()

	// Create WAL entry outside the lock
	entry := WALEntry{
		Timestamp:   time.Now().UnixNano(),
		SequenceNum: sequenceNum,
		Operation:   op,
		EntityType:  entityType,
		EntityID:    entityID,
		Data:        pe.Compress(data),
	}

	// Serialize entry
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(entry); err != nil {
		return fmt.Errorf("failed to encode WAL entry: %w", err)
	}

	// Create the key with a sequence number for proper ordering
	key := fmt.Sprintf("wal:%020d:%s:%s", sequenceNum, entityType, entityID)

	// No need to lock for this DB operation - Badger handles its own thread safety
	return pe.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(key), buf.Bytes())
	})
}

// LoadWAL loads all WAL entries and applies them to the in-memory store
// This should only be called during initialization before the server starts
// LoadWAL loads all WAL entries and applies them to the in-memory store
func (pe *Engine) LoadWAL(store common.DatastoreEngine) error {
	errorCount := 0
	skipCount := 0

	type walEntryWithKey struct {
		key   string
		entry WALEntry
	}

	// First collect and sort all entries
	entries := []walEntryWithKey{}

	err := pe.db.View(func(txn *badger.Txn) error {
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
					pe.logger.Warnf("Failed to decode WAL entry: %v, skipping", err)
					skipCount++
					return nil
				}

				entries = append(entries, walEntryWithKey{
					key:   string(item.Key()),
					entry: entry,
				})
				return nil
			})

			if err != nil {
				pe.logger.Warnf("Error processing WAL entry: %v, skipping", err)
				skipCount++
			}
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("error during WAL iteration: %w", err)
	}

	// Sort entries by sequence number
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].entry.SequenceNum < entries[j].entry.SequenceNum
	})

	// Group entries by transaction
	txnGroups := make(map[string][]walEntryWithKey)
	nonTxnEntries := []walEntryWithKey{}

	for _, e := range entries {
		if e.entry.TransactionID != "" {
			txnGroups[e.entry.TransactionID] = append(txnGroups[e.entry.TransactionID], e)
		} else {
			nonTxnEntries = append(nonTxnEntries, e)
		}
	}

	// First apply non-transaction entries
	for _, e := range nonTxnEntries {
		entry := e.entry

		// Decompress data
		data, err := pe.Decompress(entry.Data)
		if err != nil {
			pe.logger.Warnf("Failed to decompress WAL data: %v, skipping entry", err)
			skipCount++
			continue
		}

		// Apply operation to the store with error handling
		if err := pe.applyOperationWithErrorHandling(store, entry.Operation, entry.EntityType, entry.EntityID, data); err != nil {
			pe.logger.Warnf("Error applying WAL operation: %v, skipping", err)
			errorCount++
		}
	}

	// Now apply transaction groups
	for txnID, txnEntries := range txnGroups {
		// Check if transaction is complete (has an entry with IsLastInTxn=true)
		isComplete := false
		for _, e := range txnEntries {
			if e.entry.IsLastInTxn {
				isComplete = true
				break
			}
		}

		if !isComplete {
			pe.logger.Warnf("Incomplete transaction %s found in WAL, skipping all %d operations",
				txnID, len(txnEntries))
			continue
		}

		// Apply all entries in the transaction
		txnErrorCount := 0
		for _, e := range txnEntries {
			entry := e.entry

			// Decompress data
			data, err := pe.Decompress(entry.Data)
			if err != nil {
				pe.logger.Warnf("Failed to decompress WAL data in txn %s: %v, skipping entry",
					txnID, err)
				txnErrorCount++
				continue
			}

			// Apply operation to the store with error handling
			if err := pe.applyOperationWithErrorHandling(store, entry.Operation, entry.EntityType, entry.EntityID, data); err != nil {
				pe.logger.Warnf("Error applying WAL operation in txn %s: %v", txnID, err)
				txnErrorCount++
			}
		}

		if txnErrorCount > 0 {
			pe.logger.Warnf("Transaction %s completed with %d errors out of %d operations",
				txnID, txnErrorCount, len(txnEntries))
			errorCount += txnErrorCount
		}
	}

	if errorCount > 0 || skipCount > 0 {
		pe.logger.Warnf("WAL recovery completed with %d errors and %d skipped entries",
			errorCount, skipCount)
	} else {
		pe.logger.Info("WAL recovery completed successfully")
	}

	return nil
}

// applyOperation applies a WAL operation to the datastore
// This is only called during initialization
func (pe *Engine) applyOperation(store common.DatastoreEngine, op int, entityType, entityID string, data []byte) error {
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

		// Mark internal fields
		for i := range def.Fields {
			if strings.HasPrefix(def.Fields[i].Name, "_") {
				def.Fields[i].Internal = true
			}
		}

		return store.RegisterEntityType(def)

	case OpUpdateEntityType:
		var def common.EntityDefinition
		if err := gob.NewDecoder(bytes.NewBuffer(data)).Decode(&def); err != nil {
			return fmt.Errorf("failed to decode entity definition: %w", err)
		}

		// For WAL replays, we need to check if the entity type exists
		_, err := store.GetEntityDefinition(def.Name)
		if err != nil {
			// Entity type doesn't exist, register it instead
			return store.RegisterEntityType(def)
		}

		// Mark internal fields
		for i := range def.Fields {
			if strings.HasPrefix(def.Fields[i].Name, "_") {
				def.Fields[i].Internal = true
			}
		}

		// Update the entity type definition
		return store.UpdateEntityType(def)

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
		// We already have entityType as a parameter to this function
		return store.Update(entityType, entityID, fields)

	case OpDeleteEntity:
		return store.Delete(entityType, entityID)

	default:
		return fmt.Errorf("unknown operation: %d", op)
	}
}

func ensureNoDuplicateInternalFields(def *common.EntityDefinition) {
	// Check for duplicate internal fields and remove them
	createdAtIndices := []int{}
	updatedAtIndices := []int{}

	// Find all instances of internal fields
	for i, field := range def.Fields {
		if field.Name == "_created_at" {
			createdAtIndices = append(createdAtIndices, i)
		}
		if field.Name == "_updated_at" {
			updatedAtIndices = append(updatedAtIndices, i)
		}
	}

	// Keep only the first instance of each internal field if duplicates are found
	// Process in reverse order to avoid index shifting issues
	if len(createdAtIndices) > 1 {
		// Sort in descending order to remove from end first
		sort.Sort(sort.Reverse(sort.IntSlice(createdAtIndices)))
		// Keep the first one (which will be the last in this reversed slice)
		for i := 0; i < len(createdAtIndices)-1; i++ {
			// Remove duplicate field - adjust the slice to exclude this index
			def.Fields = append(def.Fields[:createdAtIndices[i]], def.Fields[createdAtIndices[i]+1:]...)
		}
	}

	if len(updatedAtIndices) > 1 {
		// Sort in descending order to remove from end first
		sort.Sort(sort.Reverse(sort.IntSlice(updatedAtIndices)))
		// Keep the first one (which will be the last in this reversed slice)
		for i := 0; i < len(updatedAtIndices)-1; i++ {
			// Remove duplicate field - adjust the slice to exclude this index
			def.Fields = append(def.Fields[:updatedAtIndices[i]], def.Fields[updatedAtIndices[i]+1:]...)
		}
	}
}
