package persistence

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"github.com/dgraph-io/badger/v4"
	"github.com/phillarmonic/syncopate-db/internal/common"
	"strings"
)

func (pe *Engine) SaveDeletedIDs(entityType string, deletedIDs map[string]bool) error {
	// Create a key for the deleted IDs
	key := fmt.Sprintf("deleted_ids:%s", entityType)

	// Serialize the deleted IDs map
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(deletedIDs); err != nil {
		return fmt.Errorf("failed to encode deleted IDs: %w", err)
	}

	// Compress and store in the database
	return pe.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(key), pe.Compress(buf.Bytes()))
	})
}

// LoadDeletedIDs loads deleted IDs from the database into the data store
func (pe *Engine) LoadDeletedIDs(store common.DatastoreEngine) error {
	return pe.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte("deleted_ids:")

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			key := string(item.Key())

			// Extract entity type from key
			parts := strings.SplitN(key, ":", 2)
			if len(parts) != 2 {
				continue
			}

			entityType := parts[1]

			err := item.Value(func(val []byte) error {
				// Decompress the data
				data, err := pe.Decompress(val)
				if err != nil {
					return fmt.Errorf("failed to decompress deleted IDs: %w", err)
				}

				// Deserialize the deleted IDs map
				var deletedIDs map[string]bool
				if err := gob.NewDecoder(bytes.NewBuffer(data)).Decode(&deletedIDs); err != nil {
					return fmt.Errorf("failed to decode deleted IDs: %w", err)
				}

				// Load into the data store
				return store.LoadDeletedIDs(entityType, deletedIDs)
			})

			if err != nil {
				pe.logger.Warnf("Error loading deleted IDs for entity type %s: %v", entityType, err)
				// Continue loading other entity types despite errors
			}
		}

		return nil
	})
}
