package common

// PersistenceProvider defines the interface for storage backends
type PersistenceProvider interface {
	// Core persistence operations
	RegisterEntityType(store DatastoreEngine, def EntityDefinition) error
	Insert(store DatastoreEngine, entityType, entityID string, data map[string]interface{}) error
	Update(store DatastoreEngine, entityID string, data map[string]interface{}) error
	Delete(store DatastoreEngine, entityID string) error

	// Snapshot and recovery operations
	TakeSnapshot(store DatastoreEngine) error
	LoadLatestSnapshot(store DatastoreEngine) error
	LoadWAL(store DatastoreEngine) error

	// Lifecycle management
	Close() error
}

// DatastoreEngine defines the interface for the datastore engine
// This allows the persistence layer to interact with the datastore
// without creating a circular dependency
type DatastoreEngine interface {
	RegisterEntityType(def EntityDefinition) error
	GetEntityDefinition(entityType string) (EntityDefinition, error)
	ListEntityTypes() []string
	Insert(entityType, id string, data map[string]interface{}) error
	Update(id string, data map[string]interface{}) error
	Delete(id string) error
	Get(id string) (Entity, error)
	GetEntityCount(entityType string) (int, error)
	GetAllEntitiesOfType(entityType string) ([]Entity, error)
}

// Entity represents a concrete instance with data
type Entity struct {
	ID     string                 `json:"id"`
	Type   string                 `json:"type"`
	Fields map[string]interface{} `json:"fields"`
}

// FieldDefinition defines a field's name and type
type FieldDefinition struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Indexed  bool   `json:"indexed"`
	Required bool   `json:"required"`
}

// EntityDefinition defines an entity's structure with fields
type EntityDefinition struct {
	Name   string            `json:"name"`
	Fields []FieldDefinition `json:"fields"`
}
