package common

import "errors"

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

// PersistenceWithCounters extends PersistenceProvider with counter operations
// for auto-increment ID generators
type PersistenceWithCounters interface {
	PersistenceProvider

	// Counter operations
	LoadCounters(store DatastoreEngine) error
	SaveCounter(entityType string, counter uint64) error
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

	// New methods for ID generation
	SetAutoIncrementCounter(entityType string, counter uint64) error
	GetAutoIncrementCounter(entityType string) (uint64, error)
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
	Nullable bool   `json:"nullable,omitempty"`
}

// EntityDefinition defines an entity's structure with fields
type EntityDefinition struct {
	Name        string            `json:"name"`
	Fields      []FieldDefinition `json:"fields"`
	IDGenerator IDGenerationType  `json:"idGenerator"`
}

// IDGenerationType defines the type of ID generation strategy
type IDGenerationType string

// ID generation types
const (
	IDTypeAutoIncrement IDGenerationType = "auto_increment"
	IDTypeUUID          IDGenerationType = "uuid"
	IDTypeCUID          IDGenerationType = "cuid"
	IDTypeCustom        IDGenerationType = "custom" // Client provides the ID
)

// IDGenerator defines the interface for ID generation
type IDGenerator interface {
	// GenerateID generates a new unique ID for an entity type
	GenerateID(entityType string) (string, error)

	// ValidateID validates if an ID is valid for this generator
	ValidateID(id string) bool

	// Type returns the type of ID generator
	Type() IDGenerationType
}

// ErrInvalidID is returned when an ID doesn't match the expected format
var ErrInvalidID = errors.New("invalid ID format")

// ErrIDGenerationFailed is returned when ID generation fails
var ErrIDGenerationFailed = errors.New("failed to generate ID")
