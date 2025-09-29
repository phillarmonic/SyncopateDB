package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/phillarmonic/syncopate-db/internal/common"
	"github.com/phillarmonic/syncopate-db/internal/datastore"
	"github.com/phillarmonic/syncopate-db/internal/persistence"
	"github.com/sirupsen/logrus"
)

// setupTestServer creates a test server with the full SyncopateDB stack
func setupTestServer(t *testing.T) (*httptest.Server, func()) {
	// Create temporary directory for persistence
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
		UseCompression:   false,
		EnableAutoGC:     false,
	}

	// Create persistence manager
	persistenceManager, err := persistence.NewManager(persistenceConfig)
	if err != nil {
		t.Fatalf("Failed to create persistence manager: %v", err)
	}

	// Create database with persistence
	db := datastore.NewDataStoreEngine(datastore.EngineConfig{
		Persistence:       persistenceManager.GetPersistenceProvider(),
		EnablePersistence: true,
	})

	// Set engine in persistence manager
	persistenceManager.SetEngine(db)

	// Create query service
	queryService := datastore.NewQueryService(db)

	// Create API server
	serverConfig := ServerConfig{
		Port:         8080,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
		LogLevel:     logrus.ErrorLevel,
		RateLimit:    1000,
		RateWindow:   time.Minute,
		DebugMode:    true,
	}

	apiServer := NewServer(db, queryService, serverConfig)

	// Create test server using the handler from the API server
	server := httptest.NewServer(apiServer.Handler())

	// Return cleanup function
	cleanup := func() {
		server.Close()
		db.Close()
		persistenceManager.Close()
	}

	return server, cleanup
}

// createEntityRequest wraps entity data in the correct API format
func createEntityRequest(fields map[string]interface{}, id ...string) map[string]interface{} {
	request := map[string]interface{}{
		"fields": fields,
	}
	if len(id) > 0 && id[0] != "" {
		request["id"] = id[0]
	}
	return request
}

// makeRequest is a helper function to make HTTP requests to the test server
func makeRequest(t *testing.T, server *httptest.Server, method, path string, body interface{}) (*http.Response, []byte) {
	var reqBody []byte
	var err error

	if body != nil {
		reqBody, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("Failed to marshal request body: %v", err)
		}
	}

	req, err := http.NewRequest(method, server.URL+path, bytes.NewBuffer(reqBody))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	respBody := make([]byte, 0)
	buf := make([]byte, 1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			respBody = append(respBody, buf[:n]...)
		}
		if err != nil {
			break
		}
	}

	return resp, respBody
}

// TestAPIBasicInMemoryUsage tests basic in-memory usage through API
func TestAPIBasicInMemoryUsage(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

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

	// Register entity type via API
	resp, body := makeRequest(t, server, "POST", "/api/v1/entity-types", userSchema)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Failed to register schema: %d - %s", resp.StatusCode, string(body))
	}

	// Insert data via API
	userData := createEntityRequest(map[string]interface{}{
		"name":  "John Doe",
		"email": "john@example.com",
		"age":   30,
	})

	resp, body = makeRequest(t, server, "POST", "/api/v1/entities/users", userData)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Failed to insert user: %d - %s", resp.StatusCode, string(body))
	}

	// Parse response to get created entity
	var createdEntity map[string]interface{}
	if err := json.Unmarshal(body, &createdEntity); err != nil {
		t.Fatalf("Failed to parse created entity: %v", err)
	}

	// Verify entity has ID
	if _, hasID := createdEntity["id"]; !hasID {
		t.Error("Created entity should have an ID")
	}

	// Query data via API
	resp, body = makeRequest(t, server, "GET", "/api/v1/entities/users", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Failed to get users: %d - %s", resp.StatusCode, string(body))
	}

	// Parse response (API returns paginated response)
	var usersResponse struct {
		Total      int                      `json:"total"`
		Count      int                      `json:"count"`
		Data       []map[string]interface{} `json:"data"`
		EntityType string                   `json:"entityType"`
	}
	if err := json.Unmarshal(body, &usersResponse); err != nil {
		t.Fatalf("Failed to parse users response: %v", err)
	}

	if usersResponse.Count != 1 {
		t.Errorf("Expected 1 user, got %d", usersResponse.Count)
	}

	if len(usersResponse.Data) != 1 {
		t.Errorf("Expected 1 user in data array, got %d", len(usersResponse.Data))
	}

	user := usersResponse.Data[0]
	userFields := user["fields"].(map[string]interface{})

	if userFields["name"] != "John Doe" {
		t.Errorf("Expected name 'John Doe', got %v", userFields["name"])
	}

	if userFields["email"] != "john@example.com" {
		t.Errorf("Expected email 'john@example.com', got %v", userFields["email"])
	}
}

// TestAPIAdvancedQuerying tests advanced querying through API
func TestAPIAdvancedQuerying(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

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
	resp, body := makeRequest(t, server, "POST", "/api/v1/entity-types", userSchema)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Failed to register schema: %d - %s", resp.StatusCode, string(body))
	}

	// Insert sample users
	users := []map[string]interface{}{
		{"name": "Alice Johnson", "email": "alice@example.com", "age": 28, "department": "Engineering", "tags": []interface{}{"senior", "golang"}},
		{"name": "Bob Smith", "email": "bob@example.com", "age": 35, "department": "Marketing", "tags": []interface{}{"manager", "sales"}},
		{"name": "Charlie Brown", "email": "charlie@example.com", "age": 22, "department": "Engineering", "tags": []interface{}{"junior", "python"}},
		{"name": "Diana Prince", "email": "diana@example.com", "age": 30, "department": "Sales", "tags": []interface{}{"senior", "customer"}},
	}

	for _, userData := range users {
		userRequest := createEntityRequest(userData)
		resp, body := makeRequest(t, server, "POST", "/api/v1/entities/users", userRequest)
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("Failed to insert user: %d - %s", resp.StatusCode, string(body))
		}
	}

	// Test 1: Basic filtering via POST query endpoint
	filterRequest := map[string]interface{}{
		"entityType": "users",
		"filters": []map[string]interface{}{
			{"field": "department", "operator": "eq", "value": "Engineering"},
		},
	}

	resp, body = makeRequest(t, server, "POST", "/api/v1/query", filterRequest)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Failed to query users: %d - %s", resp.StatusCode, string(body))
	}

	var engineeringResponse struct {
		Data []map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(body, &engineeringResponse); err != nil {
		t.Fatalf("Failed to parse users: %v", err)
	}

	if len(engineeringResponse.Data) != 2 {
		t.Errorf("Expected 2 engineering users, got %d", len(engineeringResponse.Data))
	}

	// Test 2: Complex query via POST /query endpoint
	queryRequest := map[string]interface{}{
		"entityType": "users",
		"filters": []map[string]interface{}{
			{"field": "age", "operator": "gte", "value": 25},
			{"field": "age", "operator": "lte", "value": 35},
		},
		"orderBy":   "age",
		"orderDesc": true,
		"limit":     10,
		"offset":    0,
	}

	resp, body = makeRequest(t, server, "POST", "/api/v1/query", queryRequest)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Failed to execute query: %d - %s", resp.StatusCode, string(body))
	}

	var queryResponse map[string]interface{}
	if err := json.Unmarshal(body, &queryResponse); err != nil {
		t.Fatalf("Failed to parse query response: %v", err)
	}

	data, ok := queryResponse["data"].([]interface{})
	if !ok {
		t.Fatal("Query response should have data array")
	}

	if len(data) != 3 {
		t.Errorf("Expected 3 users aged 25-35, got %d", len(data))
	}

	// Test 3: Array operations
	arrayQueryRequest := map[string]interface{}{
		"entityType": "users",
		"filters": []map[string]interface{}{
			{"field": "tags", "operator": "array_contains", "value": "senior"},
		},
	}

	resp, body = makeRequest(t, server, "POST", "/api/v1/query", arrayQueryRequest)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Failed to execute array query: %d - %s", resp.StatusCode, string(body))
	}

	if err := json.Unmarshal(body, &queryResponse); err != nil {
		t.Fatalf("Failed to parse array query response: %v", err)
	}

	data, ok = queryResponse["data"].([]interface{})
	if !ok {
		t.Fatal("Array query response should have data array")
	}

	if len(data) != 2 {
		t.Errorf("Expected 2 users with 'senior' tag, got %d", len(data))
	}
}

// TestAPIJoinOperations tests join operations through API
func TestAPIJoinOperations(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

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
	resp, body := makeRequest(t, server, "POST", "/api/v1/entity-types", userSchema)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Failed to register user schema: %d - %s", resp.StatusCode, string(body))
	}

	resp, body = makeRequest(t, server, "POST", "/api/v1/entity-types", postSchema)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Failed to register post schema: %d - %s", resp.StatusCode, string(body))
	}

	// Insert sample data
	userData := createEntityRequest(map[string]interface{}{
		"name":  "Alice Johnson",
		"email": "alice@example.com",
	})

	resp, body = makeRequest(t, server, "POST", "/api/v1/entities/users", userData)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Failed to insert user: %d - %s", resp.StatusCode, string(body))
	}

	postData := createEntityRequest(map[string]interface{}{
		"title":     "Getting Started with SyncopateDB",
		"content":   "SyncopateDB is a high-performance embedded database...",
		"author_id": "1", // References the user we just created
		"published": true,
	})

	resp, body = makeRequest(t, server, "POST", "/api/v1/entities/posts", postData)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Failed to insert post: %d - %s", resp.StatusCode, string(body))
	}

	// Query with joins
	joinQueryRequest := map[string]interface{}{
		"entityType": "posts",
		"filters": []map[string]interface{}{
			{"field": "published", "operator": "eq", "value": true},
		},
		"joins": []map[string]interface{}{
			{
				"entityType":   "users",
				"localField":   "author_id",
				"foreignField": "id",
				"joinType":     "left",
				"resultField":  "author",
			},
		},
		"orderBy": "_created_at",
		"limit":   10,
	}

	resp, body = makeRequest(t, server, "POST", "/api/v1/query", joinQueryRequest)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Failed to execute join query: %d - %s", resp.StatusCode, string(body))
	}

	var queryResponse map[string]interface{}
	if err := json.Unmarshal(body, &queryResponse); err != nil {
		t.Fatalf("Failed to parse join query response: %v", err)
	}

	data, ok := queryResponse["data"].([]interface{})
	if !ok {
		t.Fatal("Join query response should have data array")
	}

	if len(data) != 1 {
		t.Errorf("Expected 1 published post, got %d", len(data))
	}

	post := data[0].(map[string]interface{})

	// The API returns entities with fields nested under "fields"
	postFields := post["fields"].(map[string]interface{})
	authorInfo := postFields["author"]
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

// TestAPIIDGenerationStrategies tests different ID generation strategies through API
func TestAPIIDGenerationStrategies(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// Test auto_increment
	autoIncrementSchema := common.EntityDefinition{
		Name:        "auto_increment_entities",
		IDGenerator: common.IDTypeAutoIncrement,
		Fields: []common.FieldDefinition{
			{Name: "name", Type: "string", Required: true},
		},
	}

	resp, body := makeRequest(t, server, "POST", "/api/v1/entity-types", autoIncrementSchema)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Failed to register auto_increment schema: %d - %s", resp.StatusCode, string(body))
	}

	// Insert entities and check IDs
	for i := 1; i <= 3; i++ {
		data := createEntityRequest(map[string]interface{}{"name": fmt.Sprintf("Entity %d", i)})
		resp, body := makeRequest(t, server, "POST", "/api/v1/entities/auto_increment_entities", data)
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("Failed to insert auto_increment entity: %d - %s", resp.StatusCode, string(body))
		}

		var entity map[string]interface{}
		if err := json.Unmarshal(body, &entity); err != nil {
			t.Fatalf("Failed to parse entity: %v", err)
		}

		actualID := entity["id"]
		// Auto-increment IDs are returned as float64 in JSON
		expectedID := float64(i)
		if actualID != expectedID {
			t.Errorf("Expected ID %v, got %v (type: %T)", expectedID, actualID, actualID)
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

	resp, body = makeRequest(t, server, "POST", "/api/v1/entity-types", uuidSchema)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Failed to register UUID schema: %d - %s", resp.StatusCode, string(body))
	}

	data := createEntityRequest(map[string]interface{}{"name": "UUID Entity"})
	resp, body = makeRequest(t, server, "POST", "/api/v1/entities/uuid_entities", data)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Failed to insert UUID entity: %d - %s", resp.StatusCode, string(body))
	}

	var uuidEntity map[string]interface{}
	if err := json.Unmarshal(body, &uuidEntity); err != nil {
		t.Fatalf("Failed to parse UUID entity: %v", err)
	}

	// Check that ID looks like a UUID (36 characters with dashes)
	uuidIDInterface, ok := uuidEntity["id"]
	if !ok {
		t.Fatal("UUID entity should have an ID")
	}
	uuidID, ok := uuidIDInterface.(string)
	if !ok {
		t.Fatalf("UUID ID should be a string, got %T", uuidIDInterface)
	}
	if len(uuidID) != 36 {
		t.Errorf("Expected UUID length 36, got %d for UUID: %s", len(uuidID), uuidID)
	}

	// Test custom ID
	customSchema := common.EntityDefinition{
		Name:        "custom_entities",
		IDGenerator: common.IDTypeCustom,
		Fields: []common.FieldDefinition{
			{Name: "name", Type: "string", Required: true},
		},
	}

	resp, body = makeRequest(t, server, "POST", "/api/v1/entity-types", customSchema)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Failed to register custom schema: %d - %s", resp.StatusCode, string(body))
	}

	customID := "custom-123"
	customData := createEntityRequest(map[string]interface{}{"name": "Custom Entity"}, customID)
	resp, body = makeRequest(t, server, "POST", "/api/v1/entities/custom_entities", customData)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Failed to insert custom entity: %d - %s", resp.StatusCode, string(body))
	}

	var customEntity map[string]interface{}
	if err := json.Unmarshal(body, &customEntity); err != nil {
		t.Fatalf("Failed to parse custom entity: %v", err)
	}

	if customEntity["id"] != customID {
		t.Errorf("Expected custom ID %s, got %v", customID, customEntity["id"])
	}
}

// TestAPIDataTypes tests different data types through API
func TestAPIDataTypes(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

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

	resp, body := makeRequest(t, server, "POST", "/api/v1/entity-types", schema)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Failed to register schema: %d - %s", resp.StatusCode, string(body))
	}

	// Insert entity with various data types
	now := time.Now()
	data := map[string]interface{}{
		"string_field":   "test string",
		"integer_field":  42,
		"float_field":    3.14,
		"boolean_field":  true,
		"datetime_field": now.Format(time.RFC3339),
		"array_field":    []interface{}{"item1", "item2", 123},
		"object_field":   map[string]interface{}{"key1": "value1", "key2": 456},
	}

	entityData := createEntityRequest(data)
	resp, body = makeRequest(t, server, "POST", "/api/v1/entities/test_entities", entityData)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Failed to insert entity: %d - %s", resp.StatusCode, string(body))
	}

	// Retrieve and verify data
	resp, body = makeRequest(t, server, "GET", "/api/v1/entities/test_entities", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Failed to get entities: %d - %s", resp.StatusCode, string(body))
	}

	var entitiesResponse struct {
		Data []map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(body, &entitiesResponse); err != nil {
		t.Fatalf("Failed to parse entities: %v", err)
	}

	if len(entitiesResponse.Data) != 1 {
		t.Errorf("Expected 1 entity, got %d", len(entitiesResponse.Data))
	}

	entity := entitiesResponse.Data[0]
	entityFields := entity["fields"].(map[string]interface{})

	// Verify each field type
	if entityFields["string_field"] != "test string" {
		t.Errorf("String field mismatch: expected 'test string', got %v", entityFields["string_field"])
	}

	// JSON unmarshaling converts numbers to float64
	if entityFields["integer_field"].(float64) != 42 {
		t.Errorf("Integer field mismatch: expected 42, got %v", entityFields["integer_field"])
	}

	if entityFields["float_field"].(float64) != 3.14 {
		t.Errorf("Float field mismatch: expected 3.14, got %v", entityFields["float_field"])
	}

	if entityFields["boolean_field"] != true {
		t.Errorf("Boolean field mismatch: expected true, got %v", entityFields["boolean_field"])
	}

	// Verify array field
	if arrayField, ok := entityFields["array_field"].([]interface{}); ok {
		if len(arrayField) != 3 {
			t.Errorf("Array field length mismatch: expected 3, got %d", len(arrayField))
		}
		if arrayField[0] != "item1" || arrayField[1] != "item2" || arrayField[2].(float64) != 123 {
			t.Errorf("Array field content mismatch: got %v", arrayField)
		}
	} else {
		t.Error("Array field should be []interface{}")
	}

	// Verify object field
	if objectField, ok := entityFields["object_field"].(map[string]interface{}); ok {
		if objectField["key1"] != "value1" || objectField["key2"].(float64) != 456 {
			t.Errorf("Object field content mismatch: got %v", objectField)
		}
	} else {
		t.Error("Object field should be map[string]interface{}")
	}
}

// TestAPIFuzzySearch tests fuzzy search functionality through API
func TestAPIFuzzySearch(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// Define schema
	schema := common.EntityDefinition{
		Name:        "fuzzy_test_entities",
		IDGenerator: common.IDTypeAutoIncrement,
		Fields: []common.FieldDefinition{
			{Name: "name", Type: "string", Required: true, Indexed: true},
			{Name: "description", Type: "string", Required: true},
		},
	}

	resp, body := makeRequest(t, server, "POST", "/api/v1/entity-types", schema)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Failed to register schema: %d - %s", resp.StatusCode, string(body))
	}

	// Insert test data with similar names
	testData := []map[string]interface{}{
		{"name": "SyncopateDB", "description": "A high-performance database"},
		{"name": "Syncopate", "description": "Database system"},
		{"name": "Syncope", "description": "Medical term"},
		{"name": "Synchronize", "description": "To coordinate"},
		{"name": "PostgreSQL", "description": "Another database"},
	}

	for _, data := range testData {
		entityData := createEntityRequest(data)
		resp, body := makeRequest(t, server, "POST", "/api/v1/entities/fuzzy_test_entities", entityData)
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("Failed to insert test data: %d - %s", resp.StatusCode, string(body))
		}
	}

	// Test fuzzy search with default options
	t.Run("FuzzySearchDefault", func(t *testing.T) {
		fuzzyQueryRequest := map[string]interface{}{
			"entityType": "fuzzy_test_entities",
			"filters": []map[string]interface{}{
				{"field": "name", "operator": "fuzzy", "value": "Syncopate"},
			},
		}

		resp, body := makeRequest(t, server, "POST", "/api/v1/query", fuzzyQueryRequest)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Fuzzy search query failed: %d - %s", resp.StatusCode, string(body))
		}

		var queryResponse map[string]interface{}
		if err := json.Unmarshal(body, &queryResponse); err != nil {
			t.Fatalf("Failed to parse fuzzy query response: %v", err)
		}

		data, ok := queryResponse["data"].([]interface{})
		if !ok {
			t.Fatal("Fuzzy query response should have data array")
		}

		// Should find SyncopateDB, Syncopate, and possibly Syncope
		if len(data) < 2 {
			t.Errorf("Expected at least 2 fuzzy matches, got %d", len(data))
		}

		// Verify that exact match is included
		foundExactMatch := false
		for _, item := range data {
			entity := item.(map[string]interface{})
			entityFields := entity["fields"].(map[string]interface{})
			if entityFields["name"] == "Syncopate" {
				foundExactMatch = true
				break
			}
		}

		if !foundExactMatch {
			t.Error("Expected to find exact match 'Syncopate' in fuzzy search results")
		}
	})

	// Test fuzzy search with custom options
	t.Run("FuzzySearchCustomOptions", func(t *testing.T) {
		fuzzyQueryRequest := map[string]interface{}{
			"entityType": "fuzzy_test_entities",
			"filters": []map[string]interface{}{
				{"field": "name", "operator": "fuzzy", "value": "Syncopate"},
			},
			"fuzzyOpts": map[string]interface{}{
				"threshold":   0.8, // Higher threshold for stricter matching
				"maxDistance": 2,   // Maximum edit distance
			},
		}

		resp, body := makeRequest(t, server, "POST", "/api/v1/query", fuzzyQueryRequest)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Fuzzy search with custom options failed: %d - %s", resp.StatusCode, string(body))
		}

		var queryResponse map[string]interface{}
		if err := json.Unmarshal(body, &queryResponse); err != nil {
			t.Fatalf("Failed to parse fuzzy query response: %v", err)
		}

		data, ok := queryResponse["data"].([]interface{})
		if !ok {
			t.Fatal("Fuzzy query response should have data array")
		}

		// With stricter options, should find fewer matches
		if len(data) == 0 {
			t.Error("Expected at least 1 match with custom fuzzy options")
		}

		// Should still include exact match
		foundExactMatch := false
		for _, item := range data {
			entity := item.(map[string]interface{})
			entityFields := entity["fields"].(map[string]interface{})
			if entityFields["name"] == "Syncopate" {
				foundExactMatch = true
				break
			}
		}

		if !foundExactMatch {
			t.Error("Expected to find exact match 'Syncopate' in fuzzy search results")
		}
	})
}

// TestAPIPersistenceAndRecovery tests persistence and recovery through API
func TestAPIPersistenceAndRecovery(t *testing.T) {
	// This test will create a server, insert data, shut it down, and start a new one
	// to verify data persistence

	tempDir := t.TempDir()

	// First session: Create server and insert data
	{
		// Setup logging
		logger := logrus.New()
		logger.SetLevel(logrus.ErrorLevel)

		// Configure persistence
		persistenceConfig := persistence.Config{
			Path:             tempDir,
			CacheSize:        1000,
			SyncWrites:       true,
			SnapshotInterval: 1 * time.Minute,
			Logger:           logger,
			UseCompression:   false,
			EnableAutoGC:     false,
		}

		// Create persistence manager
		persistenceManager, err := persistence.NewManager(persistenceConfig)
		if err != nil {
			t.Fatalf("Failed to create persistence manager: %v", err)
		}

		// Create database with persistence
		db := datastore.NewDataStoreEngine(datastore.EngineConfig{
			Persistence:       persistenceManager.GetPersistenceProvider(),
			EnablePersistence: true,
		})

		persistenceManager.SetEngine(db)
		queryService := datastore.NewQueryService(db)

		// Create API server
		serverConfig := ServerConfig{
			Port:         8080,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 15 * time.Second,
			IdleTimeout:  60 * time.Second,
			LogLevel:     logrus.ErrorLevel,
			RateLimit:    1000,
			RateWindow:   time.Minute,
			DebugMode:    true,
		}

		apiServer := NewServer(db, queryService, serverConfig)
		server := httptest.NewServer(apiServer.Handler())

		// Define schema
		userSchema := common.EntityDefinition{
			Name:        "persistent_users",
			IDGenerator: common.IDTypeAutoIncrement,
			Fields: []common.FieldDefinition{
				{Name: "name", Type: "string", Required: true, Indexed: true},
				{Name: "email", Type: "string", Required: true, Unique: true},
			},
		}

		resp, body := makeRequest(t, server, "POST", "/api/v1/entity-types", userSchema)
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("Failed to register schema: %d - %s", resp.StatusCode, string(body))
		}

		// Insert users
		users := []map[string]interface{}{
			{"name": "Alice", "email": "alice@example.com"},
			{"name": "Bob", "email": "bob@example.com"},
			{"name": "Charlie", "email": "charlie@example.com"},
		}

		for _, userData := range users {
			userRequest := createEntityRequest(userData)
			resp, body := makeRequest(t, server, "POST", "/api/v1/entities/persistent_users", userRequest)
			if resp.StatusCode != http.StatusCreated {
				t.Fatalf("Failed to insert user: %d - %s", resp.StatusCode, string(body))
			}

			// Just verify the insert was successful
			var entity map[string]interface{}
			if err := json.Unmarshal(body, &entity); err != nil {
				t.Fatalf("Failed to parse entity: %v", err)
			}
		}

		// Force snapshot before closing (using persistence manager directly)
		if err := persistenceManager.ForceSnapshot(); err != nil {
			t.Logf("Snapshot failed: %v", err)
		}

		// Close server
		server.Close()
		db.Close()
		persistenceManager.Close()
	}

	// Second session: Recover data
	{
		// Setup logging
		logger := logrus.New()
		logger.SetLevel(logrus.ErrorLevel)

		// Configure persistence with same path
		persistenceConfig := persistence.Config{
			Path:             tempDir,
			CacheSize:        1000,
			SyncWrites:       true,
			SnapshotInterval: 1 * time.Minute,
			Logger:           logger,
			UseCompression:   false,
			EnableAutoGC:     false,
		}

		// Create persistence manager
		persistenceManager, err := persistence.NewManager(persistenceConfig)
		if err != nil {
			t.Fatalf("Failed to create persistence manager for recovery: %v", err)
		}

		// Create database with persistence
		db := datastore.NewDataStoreEngine(datastore.EngineConfig{
			Persistence:       persistenceManager.GetPersistenceProvider(),
			EnablePersistence: true,
		})

		persistenceManager.SetEngine(db)
		queryService := datastore.NewQueryService(db)

		// Create API server
		serverConfig := ServerConfig{
			Port:         8080,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 15 * time.Second,
			IdleTimeout:  60 * time.Second,
			LogLevel:     logrus.ErrorLevel,
			RateLimit:    1000,
			RateWindow:   time.Minute,
			DebugMode:    true,
		}

		apiServer := NewServer(db, queryService, serverConfig)
		server := httptest.NewServer(apiServer.Handler())

		defer func() {
			server.Close()
			db.Close()
			persistenceManager.Close()
		}()

		// Verify schema was recovered by trying to get entities
		resp, body := makeRequest(t, server, "GET", "/api/v1/entities/persistent_users", nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Failed to get recovered users: %d - %s", resp.StatusCode, string(body))
		}

		var usersResponse struct {
			Data []map[string]interface{} `json:"data"`
		}
		if err := json.Unmarshal(body, &usersResponse); err != nil {
			t.Fatalf("Failed to parse recovered users: %v", err)
		}

		if len(usersResponse.Data) != 3 {
			t.Errorf("Expected 3 recovered users, got %d", len(usersResponse.Data))
		}

		// Verify specific user data
		expectedNames := map[string]bool{"Alice": false, "Bob": false, "Charlie": false}
		for _, user := range usersResponse.Data {
			userFields := user["fields"].(map[string]interface{})
			name := userFields["name"].(string)
			if _, exists := expectedNames[name]; exists {
				expectedNames[name] = true
			}
		}

		for name, found := range expectedNames {
			if !found {
				t.Errorf("Expected to find user '%s' after recovery", name)
			}
		}

		// Insert new user to verify system still works
		newUserData := map[string]interface{}{
			"name":  "Diana",
			"email": "diana@example.com",
		}

		newUserRequest := createEntityRequest(newUserData)
		resp, body = makeRequest(t, server, "POST", "/api/v1/entities/persistent_users", newUserRequest)
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("Failed to insert new user after recovery: %d - %s", resp.StatusCode, string(body))
		}

		// Verify new user has correct ID
		var newEntity map[string]interface{}
		if err := json.Unmarshal(body, &newEntity); err != nil {
			t.Fatalf("Failed to parse new entity: %v", err)
		}

		// Auto-increment IDs are returned as float64 in JSON
		if newEntity["id"] != float64(4) {
			t.Errorf("Expected new user ID to be 4, got %v (type: %T)", newEntity["id"], newEntity["id"])
		}
	}
}

// TestAPIErrorHandling tests error handling through API
func TestAPIErrorHandling(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// Test 1: Try to insert into non-existent entity type
	userData := map[string]interface{}{
		"name":  "John Doe",
		"email": "john@example.com",
	}

	userRequest := createEntityRequest(userData)
	resp, body := makeRequest(t, server, "POST", "/api/v1/entities/nonexistent", userRequest)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected 400 for non-existent entity type, got %d", resp.StatusCode)
	}

	// Test 2: Register schema and try to insert invalid data
	userSchema := common.EntityDefinition{
		Name:        "users",
		IDGenerator: common.IDTypeAutoIncrement,
		Fields: []common.FieldDefinition{
			{Name: "name", Type: "string", Required: true},
			{Name: "email", Type: "string", Required: true, Unique: true},
			{Name: "age", Type: "integer", Required: true},
		},
	}

	resp, body = makeRequest(t, server, "POST", "/api/v1/entity-types", userSchema)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Failed to register schema: %d - %s", resp.StatusCode, string(body))
	}

	// Try to insert data missing required field
	invalidData := map[string]interface{}{
		"name": "John Doe",
		// Missing email and age
	}

	invalidRequest := createEntityRequest(invalidData)
	resp, body = makeRequest(t, server, "POST", "/api/v1/entities/users", invalidRequest)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected 400 for missing required fields, got %d", resp.StatusCode)
	}

	// Test 3: Try to insert duplicate unique field
	validData := map[string]interface{}{
		"name":  "John Doe",
		"email": "john@example.com",
		"age":   30,
	}

	validRequest := createEntityRequest(validData)
	resp, body = makeRequest(t, server, "POST", "/api/v1/entities/users", validRequest)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Failed to insert valid user: %d - %s", resp.StatusCode, string(body))
	}

	// Try to insert another user with same email
	duplicateData := map[string]interface{}{
		"name":  "Jane Doe",
		"email": "john@example.com", // Same email
		"age":   25,
	}

	duplicateRequest := createEntityRequest(duplicateData)
	resp, body = makeRequest(t, server, "POST", "/api/v1/entities/users", duplicateRequest)
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("Expected 409 for duplicate unique field, got %d", resp.StatusCode)
	}

	// Test 4: Try to get non-existent entity
	resp, body = makeRequest(t, server, "GET", "/api/v1/entities/users/999", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected 404 for non-existent entity, got %d", resp.StatusCode)
	}
}
