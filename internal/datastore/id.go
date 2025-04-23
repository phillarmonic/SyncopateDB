package datastore

import (
	"crypto/rand"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/phillarmonic/syncopate-db/internal/common"
)

// AutoIncrementGenerator generates auto-incrementing IDs
type AutoIncrementGenerator struct {
	counters map[string]*uint64
	mu       sync.RWMutex
}

// NewAutoIncrementGenerator creates a new auto-increment ID generator
func NewAutoIncrementGenerator() *AutoIncrementGenerator {
	return &AutoIncrementGenerator{
		counters: make(map[string]*uint64),
	}
}

// GenerateID generates a new auto-incrementing ID
func (g *AutoIncrementGenerator) GenerateID(entityType string) (string, error) {
	g.mu.RLock()
	counter, exists := g.counters[entityType]
	g.mu.RUnlock()

	if !exists {
		g.mu.Lock()
		// Double-check to handle race conditions
		counter, exists = g.counters[entityType]
		if !exists {
			var initialValue uint64 = 0
			counter = &initialValue
			g.counters[entityType] = counter
		}
		g.mu.Unlock()
	}

	// Atomically increment the counter
	newID := atomic.AddUint64(counter, 1)
	return strconv.FormatUint(newID, 10), nil
}

// ValidateID validates if an ID is a valid auto-increment ID
func (g *AutoIncrementGenerator) ValidateID(id string) bool {
	_, err := strconv.ParseUint(id, 10, 64)
	return err == nil
}

// Type returns the type of ID generator
func (g *AutoIncrementGenerator) Type() common.IDGenerationType {
	return common.IDTypeAutoIncrement
}

// UUIDGenerator generates UUID v4 IDs
type UUIDGenerator struct{}

// NewUUIDGenerator creates a new UUID generator
func NewUUIDGenerator() *UUIDGenerator {
	return &UUIDGenerator{}
}

// GenerateID generates a new UUID v4
func (g *UUIDGenerator) GenerateID(entityType string) (string, error) {
	uuid := make([]byte, 16)
	_, err := rand.Read(uuid)
	if err != nil {
		return "", fmt.Errorf("%w: %v", common.ErrIDGenerationFailed, err)
	}

	// Set version (4) and variant bits
	uuid[6] = (uuid[6] & 0x0f) | 0x40 // Version 4
	uuid[8] = (uuid[8] & 0x3f) | 0x80 // Variant 1

	return fmt.Sprintf("%x-%x-%x-%x-%x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16]), nil
}

// uuidRegex is a regular expression to validate UUID format
var uuidRegex = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// ValidateID validates if an ID is a valid UUID v4
func (g *UUIDGenerator) ValidateID(id string) bool {
	return uuidRegex.MatchString(strings.ToLower(id))
}

// Type returns the type of ID generator
func (g *UUIDGenerator) Type() common.IDGenerationType {
	return common.IDTypeUUID
}

// CUIDGenerator generates CUID format IDs
type CUIDGenerator struct {
	counter uint32
	mu      sync.Mutex
}

// NewCUIDGenerator creates a new CUID generator
func NewCUIDGenerator() *CUIDGenerator {
	return &CUIDGenerator{
		counter: 0,
	}
}

// GenerateID generates a new CUID
// CUIDs are collision-resistant IDs optimized for horizontal scaling and performance
func (g *CUIDGenerator) GenerateID(entityType string) (string, error) {
	// Get a timestamp component (base 36)
	timestamp := strconv.FormatInt(time.Now().UnixNano()/1000000, 36)

	// Get a counter component (base 36)
	g.mu.Lock()
	g.counter++
	count := g.counter
	g.mu.Unlock()
	counter := strconv.FormatUint(uint64(count), 36)

	// Get a random component
	random := make([]byte, 8)
	_, err := rand.Read(random)
	if err != nil {
		return "", fmt.Errorf("%w: %v", common.ErrIDGenerationFailed, err)
	}

	randomPart := fmt.Sprintf("%x", random)

	// Create a fingerprint for the host/process
	fingerprint := make([]byte, 3)
	_, err = rand.Read(fingerprint)
	if err != nil {
		return "", fmt.Errorf("%w: %v", common.ErrIDGenerationFailed, err)
	}

	fingerprintPart := fmt.Sprintf("%x", fingerprint)

	// Combine all parts with a prefix
	return fmt.Sprintf("c%s%s%s%s", timestamp, counter, randomPart, fingerprintPart), nil
}

// cuidRegex is a regular expression to validate CUID format
var cuidRegex = regexp.MustCompile(`^c[a-z0-9]{24,}$`)

// ValidateID validates if an ID is a valid CUID
func (g *CUIDGenerator) ValidateID(id string) bool {
	return cuidRegex.MatchString(id)
}

// Type returns the type of ID generator
func (g *CUIDGenerator) Type() common.IDGenerationType {
	return common.IDTypeCUID
}

// CustomIDGenerator is used when clients provide their own IDs
type CustomIDGenerator struct{}

// NewCustomIDGenerator creates a new custom ID generator
func NewCustomIDGenerator() *CustomIDGenerator {
	return &CustomIDGenerator{}
}

// GenerateID for custom generator returns an error since clients should provide IDs
func (g *CustomIDGenerator) GenerateID(entityType string) (string, error) {
	return "", fmt.Errorf("custom ID generator requires client to provide ID")
}

// ValidateID for custom generator accepts any non-empty string
func (g *CustomIDGenerator) ValidateID(id string) bool {
	return id != ""
}

// Type returns the type of ID generator
func (g *CustomIDGenerator) Type() common.IDGenerationType {
	return common.IDTypeCustom
}

// IDGeneratorManager manages ID generators for different entity types
type IDGeneratorManager struct {
	autoIncrement    *AutoIncrementGenerator
	uuid             *UUIDGenerator
	cuid             *CUIDGenerator
	custom           *CustomIDGenerator
	entityGenerators map[string]common.IDGenerationType
	mu               sync.RWMutex
}

// NewIDGeneratorManager creates a new ID generator manager
func NewIDGeneratorManager() *IDGeneratorManager {
	return &IDGeneratorManager{
		autoIncrement:    NewAutoIncrementGenerator(),
		uuid:             NewUUIDGenerator(),
		cuid:             NewCUIDGenerator(),
		custom:           NewCustomIDGenerator(),
		entityGenerators: make(map[string]common.IDGenerationType),
	}
}

// RegisterEntityType registers an entity type with its ID generation strategy
func (m *IDGeneratorManager) RegisterEntityType(entityType string, generatorType common.IDGenerationType) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entityGenerators[entityType] = generatorType
}

// GetGenerator returns the appropriate ID generator for an entity type
func (m *IDGeneratorManager) GetGenerator(entityType string) (common.IDGenerator, error) {
	m.mu.RLock()
	generatorType, exists := m.entityGenerators[entityType]
	m.mu.RUnlock()

	if !exists {
		// Default to custom if not registered
		return m.custom, nil
	}

	switch generatorType {
	case common.IDTypeAutoIncrement:
		return m.autoIncrement, nil
	case common.IDTypeUUID:
		return m.uuid, nil
	case common.IDTypeCUID:
		return m.cuid, nil
	case common.IDTypeCustom:
		return m.custom, nil
	default:
		return nil, fmt.Errorf("unknown ID generator type: %s", generatorType)
	}
}

// ValidateID validates an ID against its expected format
func (m *IDGeneratorManager) ValidateID(entityType string, id string) (bool, error) {
	generator, err := m.GetGenerator(entityType)
	if err != nil {
		return false, err
	}
	return generator.ValidateID(id), nil
}

// GenerateID generates a new ID for an entity type
func (m *IDGeneratorManager) GenerateID(entityType string) (string, error) {
	generator, err := m.GetGenerator(entityType)
	if err != nil {
		return "", err
	}
	return generator.GenerateID(entityType)
}

func (dse *Engine) SetAutoIncrementCounter(entityType string, counter uint64) error {
	generator, err := dse.idGeneratorMgr.GetGenerator(entityType)
	if err != nil {
		return err
	}

	// Check if this is an auto-increment generator
	if generator.Type() != common.IDTypeAutoIncrement {
		return nil // Not an error, just no-op for non-auto-increment types
	}

	// Type assertion to access the auto-increment-specific method
	autoGen, ok := generator.(*AutoIncrementGenerator)
	if !ok {
		return fmt.Errorf("expected AutoIncrementGenerator, got %T", generator)
	}

	// Set the counter - we need to add this method to AutoIncrementGenerator
	return autoGen.SetCounter(entityType, counter)
}

// SetCounter sets the counter value for an entity type
func (g *AutoIncrementGenerator) SetCounter(entityType string, value uint64) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	counter, exists := g.counters[entityType]
	if !exists {
		var newCounter uint64 = value
		g.counters[entityType] = &newCounter
		return nil
	}

	// Only set if the new value is higher than the current one
	// This ensures we never reuse IDs
	currentValue := atomic.LoadUint64(counter)
	if value > currentValue {
		atomic.StoreUint64(counter, value)
	}

	return nil
}

// GetAutoIncrementCounter gets the current auto-increment counter for an entity type
func (dse *Engine) GetAutoIncrementCounter(entityType string) (uint64, error) {
	generator, err := dse.idGeneratorMgr.GetGenerator(entityType)
	if err != nil {
		return 0, err
	}

	// Check if this is an auto-increment generator
	if generator.Type() != common.IDTypeAutoIncrement {
		return 0, fmt.Errorf("entity type %s does not use auto-increment", entityType)
	}

	// Type assertion to access the auto-increment-specific method
	autoGen, ok := generator.(*AutoIncrementGenerator)
	if !ok {
		return 0, fmt.Errorf("expected AutoIncrementGenerator, got %T", generator)
	}

	// Get the counter value
	return autoGen.GetCounter(entityType), nil
}

// GetCounter gets the current counter value for an entity type
func (g *AutoIncrementGenerator) GetCounter(entityType string) uint64 {
	g.mu.RLock()
	defer g.mu.RUnlock()

	counter, exists := g.counters[entityType]
	if !exists {
		return 0
	}

	return atomic.LoadUint64(counter)
}

// GetIDGeneratorType returns the ID generator type for an entity type
func (dse *Engine) GetIDGeneratorType(entityType string) (common.IDGenerationType, error) {
	dse.mu.RLock()
	defer dse.mu.RUnlock()

	def, exists := dse.definitions[entityType]
	if !exists {
		return "", fmt.Errorf("entity type %s not registered", entityType)
	}

	return def.IDGenerator, nil
}
