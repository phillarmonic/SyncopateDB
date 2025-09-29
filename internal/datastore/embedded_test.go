package datastore

import (
	"testing"
	"time"

	"github.com/phillarmonic/syncopate-db/internal/common"
	"github.com/phillarmonic/syncopate-db/internal/persistence"
	"github.com/sirupsen/logrus"
)

// TestBasicInMemoryUsage tests the basic in-memory database functionality
func TestBasicInMemoryUsage(t *testing.T) {
	// Create in-memory database
	db := NewDataStoreEngine()
	defer db.Close()

	// Define entity schema
	userSchema := common.EntityDefinition{
		Name:        "users",
		IDGenerator: common.IDTypeAutoIncrement,
		Fields: []common.FieldDefinition{
			{Name: "name", Type: "string", Required: true, Indexed: true},
			{Name: "email", Type: "string", Required: true, Unique: true},
			{Name: "age", Type: "integer", Nullable: true},
		},
	}

	// Register entity type
	err := db.RegisterEntityType(userSchema)
	if err != nil {
		t.Fatalf("Failed to register user schema: %v", err)
	}

	// Insert data
	userData := map[string]interface{}{
		"name":  "John Doe",
		"email": "john@example.com",
		"age":   30,
	}

	err = db.Insert("users", "", userData) // Empty ID for auto-generation
	if err != nil {
		t.Fatalf("Failed to insert user: %v", err)
	}

	// Get all users
	users, err := db.GetAllEntitiesOfType("users")
	if err != nil {
		t.Fatalf("Failed to get users: %v", err)
	}

	if len(users) != 1 {
		t.Errorf("Expected 1 user, got %d", len(users))
	}

	user := users[0]
	if user.Fields["name"] != "John Doe" {
		t.Errorf("Expected name 'John Doe', got %v", user.Fields["name"])
	}

	if user.Fields["email"] != "john@example.com" {
		t.Errorf("Expected email 'john@example.com', got %v", user.Fields["email"])
	}

	// Get user count
	count, err := db.GetEntityCount("users")
	if err != nil {
		t.Fatalf("Failed to get user count: %v", err)
	}

	if count != 1 {
		t.Errorf("Expected count 1, got %d", count)
	}
}

// TestPersistentDatabase tests the persistent database functionality
func TestPersistentDatabase(t *testing.T) {
	// Create temporary directory for test
	tempDir := t.TempDir()

	// Setup logging
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel) // Reduce noise in tests

	// Configure persistence
	persistenceConfig := persistence.Config{
		Path:             tempDir,
		CacheSize:        1000,
		SyncWrites:       true,
		SnapshotInterval: 1 * time.Minute,
		Logger:           logger,
		UseCompression:   false, // Disable for faster tests
		EnableAutoGC:     false, // Disable for tests
	}

	// Create persistence manager
	persistenceManager, err := persistence.NewManager(persistenceConfig)
	if err != nil {
		t.Fatalf("Failed to create persistence manager: %v", err)
	}
	defer persistenceManager.Close()

	// Create database with persistence
	db := NewDataStoreEngine(EngineConfig{
		Persistence:       persistenceManager.GetPersistenceProvider(),
		EnablePersistence: true,
	})
	defer db.Close()

	// Set engine in persistence manager
	persistenceManager.SetEngine(db)

	// Define schema
	userSchema := common.EntityDefinition{
		Name:        "users",
		IDGenerator: common.IDTypeAutoIncrement,
		Fields: []common.FieldDefinition{
			{Name: "name", Type: "string", Required: true, Indexed: true},
			{Name: "email", Type: "string", Required: true, Unique: true},
			{Name: "active", Type: "boolean", Required: true},
		},
	}

	// Register schema
	if err := db.RegisterEntityType(userSchema); err != nil {
		t.Fatalf("Failed to register user schema: %v", err)
	}

	// Insert sample data
	userData := map[string]interface{}{
		"name":   "Alice Johnson",
		"email":  "alice@example.com",
		"active": true,
	}

	if err := db.Insert("users", "", userData); err != nil {
		t.Fatalf("Failed to insert user: %v", err)
	}

	// Verify data was inserted
	users, err := db.GetAllEntitiesOfType("users")
	if err != nil {
		t.Fatalf("Failed to get users: %v", err)
	}

	if len(users) != 1 {
		t.Errorf("Expected 1 user, got %d", len(users))
	}

	// Force a snapshot
	if err := persistenceManager.ForceSnapshot(); err != nil {
		t.Errorf("Failed to force snapshot: %v", err)
	}

	// Get database statistics
	stats := persistenceManager.GetStorageStats()
	if stats["entity_types_count"] != 1 {
		t.Errorf("Expected 1 entity type, got %v", stats["entity_types_count"])
	}
}

// TestAdvancedQuerying tests complex query functionality
func TestAdvancedQuerying(t *testing.T) {
	// Create in-memory database
	db := NewDataStoreEngine()
	defer db.Close()

	// Create query service
	queryService := NewQueryService(db)

	// Define schema
	userSchema := common.EntityDefinition{
		Name:        "users",
		IDGenerator: common.IDTypeAutoIncrement,
		Fields: []common.FieldDefinition{
			{Name: "name", Type: "string", Required: true, Indexed: true},
			{Name: "email", Type: "string", Required: true, Unique: true},
			{Name: "age", Type: "integer", Nullable: true, Indexed: true},
			{Name: "department", Type: "string", Required: true, Indexed: true},
			{Name: "tags", Type: "array", Required: false},
		},
	}

	// Register schema
	if err := db.RegisterEntityType(userSchema); err != nil {
		t.Fatalf("Failed to register user schema: %v", err)
	}

	// Insert sample users
	users := []map[string]interface{}{
		{"name": "Alice Johnson", "email": "alice@example.com", "age": 28, "department": "Engineering", "tags": []interface{}{"senior", "golang"}},
		{"name": "Bob Smith", "email": "bob@example.com", "age": 35, "department": "Marketing", "tags": []interface{}{"manager", "sales"}},
		{"name": "Charlie Brown", "email": "charlie@example.com", "age": 22, "department": "Engineering", "tags": []interface{}{"junior", "python"}},
		{"name": "Diana Prince", "email": "diana@example.com", "age": 30, "department": "Sales", "tags": []interface{}{"senior", "customer"}},
	}

	for _, userData := range users {
		if err := db.Insert("users", "", userData); err != nil {
			t.Fatalf("Failed to insert user: %v", err)
		}
	}

	// Test 1: Basic filtering with pagination
	queryOptions := QueryOptions{
		EntityType: "users",
		Filters: []Filter{
			{Field: "department", Operator: FilterEq, Value: "Engineering"},
		},
		OrderBy:   "age",
		OrderDesc: false,
		Limit:     10,
		Offset:    0,
	}

	response, err := queryService.ExecutePaginatedQuery(queryOptions)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if response.Count != 2 {
		t.Errorf("Expected 2 engineering users, got %d", response.Count)
	}

	// Test 2: Complex filtering with multiple conditions
	queryOptions = QueryOptions{
		EntityType: "users",
		Filters: []Filter{
			{Field: "age", Operator: FilterGte, Value: 25},
			{Field: "age", Operator: FilterLte, Value: 35},
		},
		OrderBy:   "age",
		OrderDesc: true,
	}

	response, err = queryService.ExecutePaginatedQuery(queryOptions)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if response.Count != 3 {
		t.Errorf("Expected 3 users aged 25-35, got %d", response.Count)
	}

	// Test 3: Array operations
	queryOptions = QueryOptions{
		EntityType: "users",
		Filters: []Filter{
			{Field: "tags", Operator: FilterArrayContains, Value: "senior"},
		},
	}

	response, err = queryService.ExecutePaginatedQuery(queryOptions)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if response.Count != 2 {
		t.Errorf("Expected 2 users with 'senior' tag, got %d", response.Count)
	}

	// Test 4: Count query
	totalUsers, err := queryService.ExecuteCountQuery(QueryOptions{
		EntityType: "users",
	})
	if err != nil {
		t.Fatalf("Count query failed: %v", err)
	}

	if totalUsers != 4 {
		t.Errorf("Expected 4 total users, got %d", totalUsers)
	}

	engineeringUsers, err := queryService.ExecuteCountQuery(QueryOptions{
		EntityType: "users",
		Filters: []Filter{
			{Field: "department", Operator: FilterEq, Value: "Engineering"},
		},
	})
	if err != nil {
		t.Fatalf("Count query failed: %v", err)
	}

	if engineeringUsers != 2 {
		t.Errorf("Expected 2 engineering users, got %d", engineeringUsers)
	}
}

// TestQueryWithJoins tests join functionality
func TestQueryWithJoins(t *testing.T) {
	// Create in-memory database
	db := NewDataStoreEngine()
	defer db.Close()

	// Create query service
	queryService := NewQueryService(db)

	// Define schemas
	userSchema := common.EntityDefinition{
		Name:        "users",
		IDGenerator: common.IDTypeAutoIncrement,
		Fields: []common.FieldDefinition{
			{Name: "name", Type: "string", Required: true, Indexed: true},
			{Name: "email", Type: "string", Required: true, Unique: true},
		},
	}

	postSchema := common.EntityDefinition{
		Name:        "posts",
		IDGenerator: common.IDTypeUUID,
		Fields: []common.FieldDefinition{
			{Name: "title", Type: "string", Required: true, Indexed: true},
			{Name: "content", Type: "string", Required: true},
			{Name: "author_id", Type: "string", Required: true, Indexed: true},
			{Name: "published", Type: "boolean", Required: true},
		},
	}

	// Register schemas
	if err := db.RegisterEntityType(userSchema); err != nil {
		t.Fatalf("Failed to register user schema: %v", err)
	}

	if err := db.RegisterEntityType(postSchema); err != nil {
		t.Fatalf("Failed to register post schema: %v", err)
	}

	// Insert sample data
	userData := map[string]interface{}{
		"name":  "Alice Johnson",
		"email": "alice@example.com",
	}

	if err := db.Insert("users", "", userData); err != nil {
		t.Fatalf("Failed to insert user: %v", err)
	}

	postData := map[string]interface{}{
		"title":     "Getting Started with SyncopateDB",
		"content":   "SyncopateDB is a high-performance embedded database...",
		"author_id": "1", // References the user we just created
		"published": true,
	}

	if err := db.Insert("posts", "", postData); err != nil {
		t.Fatalf("Failed to insert post: %v", err)
	}

	// Query with joins
	queryOptions := QueryOptions{
		EntityType: "posts",
		Filters: []Filter{
			{Field: "published", Operator: FilterEq, Value: true},
		},
		Joins: []JoinOptions{
			{
				EntityType:   "users",
				LocalField:   "author_id",
				ForeignField: "id",
				JoinType:     JoinTypeLeft,
				ResultField:  "author",
			},
		},
		OrderBy: "_created_at",
		Limit:   10,
	}

	response, err := queryService.ExecutePaginatedQuery(queryOptions)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if response.Count != 1 {
		t.Errorf("Expected 1 published post, got %d", response.Count)
	}

	post := response.Data[0]
	authorInfo := post.Fields["author"]
	if authorInfo == nil {
		t.Error("Expected author information in joined result")
	}

	if author, ok := authorInfo.(map[string]interface{}); ok {
		if author["name"] != "Alice Johnson" {
			t.Errorf("Expected author name 'Alice Johnson', got %v", author["name"])
		}
	} else {
		t.Error("Author information should be a map")
	}
}

// TestIDGenerationStrategies tests different ID generation strategies
func TestIDGenerationStrategies(t *testing.T) {
	// Create in-memory database
	db := NewDataStoreEngine()
	defer db.Close()

	// Test auto_increment
	autoIncrementSchema := common.EntityDefinition{
		Name:        "auto_increment_entities",
		IDGenerator: common.IDTypeAutoIncrement,
		Fields: []common.FieldDefinition{
			{Name: "name", Type: "string", Required: true},
		},
	}

	if err := db.RegisterEntityType(autoIncrementSchema); err != nil {
		t.Fatalf("Failed to register auto_increment schema: %v", err)
	}

	// Insert entities and check IDs
	for i := 1; i <= 3; i++ {
		data := map[string]interface{}{"name": "Entity " + string(rune(i+'0'))}
		if err := db.Insert("auto_increment_entities", "", data); err != nil {
			t.Fatalf("Failed to insert auto_increment entity: %v", err)
		}
	}

	entities, err := db.GetAllEntitiesOfType("auto_increment_entities")
	if err != nil {
		t.Fatalf("Failed to get auto_increment entities: %v", err)
	}

	if len(entities) != 3 {
		t.Errorf("Expected 3 entities, got %d", len(entities))
	}

	// Check that IDs are sequential
	expectedIDs := []string{"1", "2", "3"}
	for i, entity := range entities {
		if entity.ID != expectedIDs[i] {
			t.Errorf("Expected ID %s, got %s", expectedIDs[i], entity.ID)
		}
	}

	// Test UUID
	uuidSchema := common.EntityDefinition{
		Name:        "uuid_entities",
		IDGenerator: common.IDTypeUUID,
		Fields: []common.FieldDefinition{
			{Name: "name", Type: "string", Required: true},
		},
	}

	if err := db.RegisterEntityType(uuidSchema); err != nil {
		t.Fatalf("Failed to register UUID schema: %v", err)
	}

	data := map[string]interface{}{"name": "UUID Entity"}
	if err := db.Insert("uuid_entities", "", data); err != nil {
		t.Fatalf("Failed to insert UUID entity: %v", err)
	}

	uuidEntities, err := db.GetAllEntitiesOfType("uuid_entities")
	if err != nil {
		t.Fatalf("Failed to get UUID entities: %v", err)
	}

	if len(uuidEntities) != 1 {
		t.Errorf("Expected 1 UUID entity, got %d", len(uuidEntities))
	}

	// Check that ID looks like a UUID (36 characters with dashes)
	uuidID := uuidEntities[0].ID
	if len(uuidID) != 36 {
		t.Errorf("Expected UUID length 36, got %d", len(uuidID))
	}

	// Test CUID
	cuidSchema := common.EntityDefinition{
		Name:        "cuid_entities",
		IDGenerator: common.IDTypeCUID,
		Fields: []common.FieldDefinition{
			{Name: "name", Type: "string", Required: true},
		},
	}

	if err := db.RegisterEntityType(cuidSchema); err != nil {
		t.Fatalf("Failed to register CUID schema: %v", err)
	}

	data = map[string]interface{}{"name": "CUID Entity"}
	if err := db.Insert("cuid_entities", "", data); err != nil {
		t.Fatalf("Failed to insert CUID entity: %v", err)
	}

	cuidEntities, err := db.GetAllEntitiesOfType("cuid_entities")
	if err != nil {
		t.Fatalf("Failed to get CUID entities: %v", err)
	}

	if len(cuidEntities) != 1 {
		t.Errorf("Expected 1 CUID entity, got %d", len(cuidEntities))
	}

	// Check that ID starts with 'c' (CUID format)
	cuidID := cuidEntities[0].ID
	if cuidID[0] != 'c' {
		t.Errorf("Expected CUID to start with 'c', got %s", cuidID)
	}

	// Test custom
	customSchema := common.EntityDefinition{
		Name:        "custom_entities",
		IDGenerator: common.IDTypeCustom,
		Fields: []common.FieldDefinition{
			{Name: "name", Type: "string", Required: true},
		},
	}

	if err := db.RegisterEntityType(customSchema); err != nil {
		t.Fatalf("Failed to register custom schema: %v", err)
	}

	data = map[string]interface{}{"name": "Custom Entity"}
	customID := "custom-123"
	if err := db.Insert("custom_entities", customID, data); err != nil {
		t.Fatalf("Failed to insert custom entity: %v", err)
	}

	customEntities, err := db.GetAllEntitiesOfType("custom_entities")
	if err != nil {
		t.Fatalf("Failed to get custom entities: %v", err)
	}

	if len(customEntities) != 1 {
		t.Errorf("Expected 1 custom entity, got %d", len(customEntities))
	}

	if customEntities[0].ID != customID {
		t.Errorf("Expected custom ID %s, got %s", customID, customEntities[0].ID)
	}
}

// TestDataTypes tests different data types
func TestDataTypes(t *testing.T) {
	// Create in-memory database
	db := NewDataStoreEngine()
	defer db.Close()

	// Define schema with various data types
	schema := common.EntityDefinition{
		Name:        "test_entities",
		IDGenerator: common.IDTypeAutoIncrement,
		Fields: []common.FieldDefinition{
			{Name: "string_field", Type: "string", Required: true},
			{Name: "integer_field", Type: "integer", Required: true},
			{Name: "float_field", Type: "float", Required: true},
			{Name: "boolean_field", Type: "boolean", Required: true},
			{Name: "datetime_field", Type: "datetime", Required: true},
			{Name: "array_field", Type: "array", Required: false},
			{Name: "object_field", Type: "object", Required: false},
		},
	}

	if err := db.RegisterEntityType(schema); err != nil {
		t.Fatalf("Failed to register schema: %v", err)
	}

	// Insert entity with various data types
	now := time.Now()
	data := map[string]interface{}{
		"string_field":   "test string",
		"integer_field":  42,
		"float_field":    3.14,
		"boolean_field":  true,
		"datetime_field": now,
		"array_field":    []interface{}{"item1", "item2", 123},
		"object_field":   map[string]interface{}{"key1": "value1", "key2": 456},
	}

	if err := db.Insert("test_entities", "", data); err != nil {
		t.Fatalf("Failed to insert entity: %v", err)
	}

	// Retrieve and verify data
	entities, err := db.GetAllEntitiesOfType("test_entities")
	if err != nil {
		t.Fatalf("Failed to get entities: %v", err)
	}

	if len(entities) != 1 {
		t.Errorf("Expected 1 entity, got %d", len(entities))
	}

	entity := entities[0]

	// Verify each field type
	if entity.Fields["string_field"] != "test string" {
		t.Errorf("String field mismatch: expected 'test string', got %v", entity.Fields["string_field"])
	}

	if entity.Fields["integer_field"] != 42 {
		t.Errorf("Integer field mismatch: expected 42, got %v", entity.Fields["integer_field"])
	}

	if entity.Fields["float_field"] != 3.14 {
		t.Errorf("Float field mismatch: expected 3.14, got %v", entity.Fields["float_field"])
	}

	if entity.Fields["boolean_field"] != true {
		t.Errorf("Boolean field mismatch: expected true, got %v", entity.Fields["boolean_field"])
	}

	// Verify array field
	if arrayField, ok := entity.Fields["array_field"].([]interface{}); ok {
		if len(arrayField) != 3 {
			t.Errorf("Array field length mismatch: expected 3, got %d", len(arrayField))
		}
		if arrayField[0] != "item1" || arrayField[1] != "item2" || arrayField[2] != 123 {
			t.Errorf("Array field content mismatch: got %v", arrayField)
		}
	} else {
		t.Error("Array field should be []interface{}")
	}

	// Verify object field
	if objectField, ok := entity.Fields["object_field"].(map[string]interface{}); ok {
		if objectField["key1"] != "value1" || objectField["key2"] != 456 {
			t.Errorf("Object field content mismatch: got %v", objectField)
		}
	} else {
		t.Error("Object field should be map[string]interface{}")
	}
}

// TestThreadSafety tests concurrent access to the database
func TestThreadSafety(t *testing.T) {
	// Create in-memory database
	db := NewDataStoreEngine()
	defer db.Close()

	// Define schema
	schema := common.EntityDefinition{
		Name:        "concurrent_entities",
		IDGenerator: common.IDTypeAutoIncrement,
		Fields: []common.FieldDefinition{
			{Name: "name", Type: "string", Required: true},
			{Name: "value", Type: "integer", Required: true},
		},
	}

	if err := db.RegisterEntityType(schema); err != nil {
		t.Fatalf("Failed to register schema: %v", err)
	}

	// Number of concurrent goroutines
	numGoroutines := 10
	numInserts := 10

	// Channel to collect errors
	errChan := make(chan error, numGoroutines*numInserts)
	done := make(chan bool, numGoroutines)

	// Launch concurrent goroutines
	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			defer func() { done <- true }()

			for j := 0; j < numInserts; j++ {
				data := map[string]interface{}{
					"name":  "Entity",
					"value": goroutineID*numInserts + j,
				}

				if err := db.Insert("concurrent_entities", "", data); err != nil {
					errChan <- err
					return
				}
			}
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Check for errors
	close(errChan)
	for err := range errChan {
		t.Errorf("Concurrent insert error: %v", err)
	}

	// Verify all entities were inserted
	entities, err := db.GetAllEntitiesOfType("concurrent_entities")
	if err != nil {
		t.Fatalf("Failed to get entities: %v", err)
	}

	expectedCount := numGoroutines * numInserts
	if len(entities) != expectedCount {
		t.Errorf("Expected %d entities, got %d", expectedCount, len(entities))
	}

	// Verify all entities have unique IDs
	idMap := make(map[string]bool)
	for _, entity := range entities {
		if idMap[entity.ID] {
			t.Errorf("Duplicate ID found: %s", entity.ID)
		}
		idMap[entity.ID] = true
	}
}

// TestUniqueConstraints tests unique field constraints
func TestUniqueConstraints(t *testing.T) {
	// Create in-memory database
	db := NewDataStoreEngine()
	defer db.Close()

	// Define schema with unique field
	schema := common.EntityDefinition{
		Name:        "unique_entities",
		IDGenerator: common.IDTypeAutoIncrement,
		Fields: []common.FieldDefinition{
			{Name: "name", Type: "string", Required: true},
			{Name: "email", Type: "string", Required: true, Unique: true},
		},
	}

	if err := db.RegisterEntityType(schema); err != nil {
		t.Fatalf("Failed to register schema: %v", err)
	}

	// Insert first entity
	data1 := map[string]interface{}{
		"name":  "John Doe",
		"email": "john@example.com",
	}

	if err := db.Insert("unique_entities", "", data1); err != nil {
		t.Fatalf("Failed to insert first entity: %v", err)
	}

	// Try to insert entity with duplicate email (should fail)
	data2 := map[string]interface{}{
		"name":  "Jane Doe",
		"email": "john@example.com", // Same email
	}

	err := db.Insert("unique_entities", "", data2)
	if err == nil {
		t.Error("Expected error for duplicate email, but insert succeeded")
	}

	// Insert entity with different email (should succeed)
	data3 := map[string]interface{}{
		"name":  "Jane Doe",
		"email": "jane@example.com", // Different email
	}

	if err := db.Insert("unique_entities", "", data3); err != nil {
		t.Fatalf("Failed to insert entity with unique email: %v", err)
	}

	// Verify only 2 entities exist
	entities, err := db.GetAllEntitiesOfType("unique_entities")
	if err != nil {
		t.Fatalf("Failed to get entities: %v", err)
	}

	if len(entities) != 2 {
		t.Errorf("Expected 2 entities, got %d", len(entities))
	}
}

// TestSchemaEvolution tests updating entity type definitions
func TestSchemaEvolution(t *testing.T) {
	// Create in-memory database
	db := NewDataStoreEngine()
	defer db.Close()

	// Define initial schema
	initialSchema := common.EntityDefinition{
		Name:        "evolving_entities",
		IDGenerator: common.IDTypeAutoIncrement,
		Fields: []common.FieldDefinition{
			{Name: "name", Type: "string", Required: true},
			{Name: "email", Type: "string", Required: true},
		},
	}

	if err := db.RegisterEntityType(initialSchema); err != nil {
		t.Fatalf("Failed to register initial schema: %v", err)
	}

	// Insert entity with initial schema
	data := map[string]interface{}{
		"name":  "John Doe",
		"email": "john@example.com",
	}

	if err := db.Insert("evolving_entities", "", data); err != nil {
		t.Fatalf("Failed to insert entity: %v", err)
	}

	// Update schema to add new field
	updatedSchema := common.EntityDefinition{
		Name:        "evolving_entities",
		IDGenerator: common.IDTypeAutoIncrement,
		Fields: []common.FieldDefinition{
			{Name: "name", Type: "string", Required: true},
			{Name: "email", Type: "string", Required: true},
			{Name: "phone", Type: "string", Required: false}, // New field
		},
	}

	if err := db.UpdateEntityType(updatedSchema); err != nil {
		t.Fatalf("Failed to update schema: %v", err)
	}

	// Insert entity with new schema
	newData := map[string]interface{}{
		"name":  "Jane Doe",
		"email": "jane@example.com",
		"phone": "123-456-7890",
	}

	if err := db.Insert("evolving_entities", "", newData); err != nil {
		t.Fatalf("Failed to insert entity with updated schema: %v", err)
	}

	// Verify both entities exist
	entities, err := db.GetAllEntitiesOfType("evolving_entities")
	if err != nil {
		t.Fatalf("Failed to get entities: %v", err)
	}

	if len(entities) != 2 {
		t.Errorf("Expected 2 entities, got %d", len(entities))
	}

	// Verify first entity doesn't have phone field
	firstEntity := entities[0]
	if _, hasPhone := firstEntity.Fields["phone"]; hasPhone {
		t.Error("First entity should not have phone field")
	}

	// Verify second entity has phone field
	secondEntity := entities[1]
	if phone, hasPhone := secondEntity.Fields["phone"]; !hasPhone || phone != "123-456-7890" {
		t.Errorf("Second entity should have phone field with value '123-456-7890', got %v", phone)
	}
}
