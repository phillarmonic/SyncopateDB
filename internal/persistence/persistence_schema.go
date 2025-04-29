package persistence

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"github.com/dgraph-io/badger/v4"
	"github.com/phillarmonic/syncopate-db/internal/common"
	"github.com/phillarmonic/syncopate-db/internal/settings"
)

// UpdateEntityType persists an updated entity type definition
func (pe *Engine) UpdateEntityType(store common.DatastoreEngine, def common.EntityDefinition) error {
	// Check if WAL is disabled in settings
	if !settings.Config.EnableWAL {
		// Even without WAL, we still need to update the definition
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
	return pe.WriteWALEntry(OpUpdateEntityType, def.Name, "", buf.Bytes())
}
