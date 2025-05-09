package common

import (
	"errors"
	"strconv"
)

// PersistenceProvider defines the interface for storage backends
type PersistenceProvider interface {
	// Core persistence operations
	RegisterEntityType(store DatastoreEngine, def EntityDefinition) error
	Insert(store DatastoreEngine, entityType, entityID string, data map[string]interface{}) error
	Update(store DatastoreEngine, entityType string, entityID string, data map[string]interface{}) error
	Delete(store DatastoreEngine, entityID string, entityType string) error

	UpdateEntityType(store DatastoreEngine, def EntityDefinition) error

	TruncateEntityType(store DatastoreEngine, entityType string) error
	TruncateDatabase(store DatastoreEngine) error

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
	Update(entityType string, id string, data map[string]interface{}) error // Updated signature
	Delete(entityType string, id string) error
	Get(id string) (Entity, error)
	GetEntityCount(entityType string) (int, error)
	GetAllEntitiesOfType(entityType string) ([]Entity, error)

	UpdateEntityType(updatedDef EntityDefinition) error
	SetAutoIncrementCounter(entityType string, counter uint64) error
	GetAutoIncrementCounter(entityType string) (uint64, error)
	MarkIDAsDeleted(entityType string, id string) error
	GetDeletedIDs(entityType string) (map[string]bool, error)
	LoadDeletedIDs(entityType string, deletedIDs map[string]bool) error
	TruncateEntityType(entityType string) error
	TruncateDatabase() error
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
	Internal bool   `json:"internal,omitempty"`
	Unique   bool   `json:"unique,omitempty"`
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

// EntityRepresentation represents an entity with appropriate ID type for API responses
type EntityRepresentation struct {
	ID     interface{}            `json:"id"` // Can be either string or int
	Type   string                 `json:"type"`
	Fields map[string]interface{} `json:"fields"`
}

// ConvertToRepresentation converts an Entity to an EntityRepresentation
// where ID is converted to the appropriate type based on the entity type's ID generator
func ConvertToRepresentation(entity Entity, idGeneratorType IDGenerationType) EntityRepresentation {
	representation := EntityRepresentation{
		Type:   entity.Type,
		Fields: entity.Fields,
	}

	// Convert ID to int for auto_increment entities
	if idGeneratorType == IDTypeAutoIncrement {
		// Try to convert ID to int
		if id, err := strconv.Atoi(entity.ID); err == nil {
			representation.ID = id
		} else {
			// Fallback to string if conversion fails
			representation.ID = entity.ID
		}
	} else {
		// For other ID types, keep as string
		representation.ID = entity.ID
	}

	return representation
}

// PersistenceWithDeletedIDs extends PersistenceProvider with deleted ID operations
// for auto-increment ID generators
type PersistenceWithDeletedIDs interface {
	PersistenceProvider

	// DeletedIDs operations
	LoadDeletedIDs(store DatastoreEngine) error
	SaveDeletedIDs(entityType string, deletedIDs map[string]bool) error
}
