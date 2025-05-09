package persistence

import (
	"fmt"
	"github.com/dgraph-io/badger/v4"
	"github.com/phillarmonic/syncopate-db/internal/common"
	"github.com/phillarmonic/syncopate-db/internal/settings"
)

// TruncateEntityType records a truncate operation for an entity type in the WAL
func (pe *Engine) TruncateEntityType(store common.DatastoreEngine, entityType string) error {
	// Check if WAL is disabled in settings
	if !settings.Config.EnableWAL {
		// Direct truncate without WAL
		// We'll use a special key pattern for entity types
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
}

// TruncateDatabase records a truncate operation for the entire database in the WAL
func (pe *Engine) TruncateDatabase(store common.DatastoreEngine) error {
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
