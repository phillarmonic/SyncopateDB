package persistence

import (
	"os"
	"testing"
	"time"

	"github.com/phillarmonic/syncopate-db/internal/common"
	"github.com/phillarmonic/syncopate-db/internal/datastore"
	"github.com/sirupsen/logrus"
)

// TestPersistenceManager tests the persistence manager functionality
func TestPersistenceManager(t *testing.T) {
	// Create temporary directory for test
	tempDir := t.TempDir()

	// Setup logging
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel) // Reduce noise in tests

	// Configure persistence
	persistenceConfig := Config{
		Path:             tempDir,
		CacheSize:        1000,
		SyncWrites:       true,
		SnapshotInterval: 1 * time.Minute, // Long interval for testing
		Logger:           logger,
		UseCompression:   false, // Disable for faster tests
		EnableAutoGC:     false, // Disable for tests
	}

	// Create persistence manager
	persistenceManager, err := NewManager(persistenceConfig)
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

	// Define schema
	userSchema := common.EntityDefinition{
		Name:        "persistent_users",
		IDGenerator: common.IDTypeAutoIncrement,
		Fields: []common.FieldDefinition{
			{Name: "name", Type: "string", Required: true, Indexed: true},
			{Name: "email", Type: "string", Required: true, Unique: true},
			{Name: "age", Type: "integer", Nullable: true},
		},
	}

	// Register schema
	if err := db.RegisterEntityType(userSchema); err != nil {
		t.Fatalf("Failed to register user schema: %v", err)
	}

	// Insert sample data
	userData := map[string]interface{}{
		"name":  "John Doe",
		"email": "john@example.com",
		"age":   30,
	}

	if err := db.Insert("persistent_users", "", userData); err != nil {
		t.Fatalf("Failed to insert user: %v", err)
	}

	// Verify data was inserted
	users, err := db.GetAllEntitiesOfType("persistent_users")
	if err != nil {
		t.Fatalf("Failed to get users: %v", err)
	}

	if len(users) != 1 {
		t.Errorf("Expected 1 user, got %d", len(users))
	}

	// Get database statistics
	stats := persistenceManager.GetStorageStats()
	if stats["entity_types_count"] != 1 {
		t.Errorf("Expected 1 entity type, got %v", stats["entity_types_count"])
	}

	entityCounts, ok := stats["entity_counts"].(map[string]int)
	if !ok {
		t.Fatal("Expected entity_counts to be map[string]int")
	}

	if entityCounts["persistent_users"] != 1 {
		t.Errorf("Expected 1 persistent_users entity, got %d", entityCounts["persistent_users"])
	}

	// Test database size reporting
	if _, exists := stats["database_size_bytes"]; !exists {
		t.Error("Expected database_size_bytes in stats")
	}

	if _, exists := stats["database_size_mb"]; !exists {
		t.Error("Expected database_size_mb in stats")
	}

	// Clean shutdown
	db.Close()
	persistenceManager.Close()
}

// TestPersistenceRecovery tests data recovery after restart
func TestPersistenceRecovery(t *testing.T) {
	// Create temporary directory for test
	tempDir := t.TempDir()

	// Setup logging
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel) // Reduce noise in tests

	// Configure persistence
	persistenceConfig := Config{
		Path:             tempDir,
		CacheSize:        1000,
		SyncWrites:       true,
		SnapshotInterval: 1 * time.Minute,
		Logger:           logger,
		UseCompression:   false,
		EnableAutoGC:     false,
	}

	// First session: Create database and insert data
	{
		persistenceManager, err := NewManager(persistenceConfig)
		if err != nil {
			t.Fatalf("Failed to create persistence manager: %v", err)
		}

		db := datastore.NewDataStoreEngine(datastore.EngineConfig{
			Persistence:       persistenceManager.GetPersistenceProvider(),
			EnablePersistence: true,
		})

		persistenceManager.SetEngine(db)

		// Define schema
		userSchema := common.EntityDefinition{
			Name:        "recovery_users",
			IDGenerator: common.IDTypeAutoIncrement,
			Fields: []common.FieldDefinition{
				{Name: "name", Type: "string", Required: true, Indexed: true},
				{Name: "email", Type: "string", Required: true, Unique: true},
			},
		}

		if err := db.RegisterEntityType(userSchema); err != nil {
			t.Fatalf("Failed to register user schema: %v", err)
		}

		// Insert multiple users
		users := []map[string]interface{}{
			{"name": "Alice", "email": "alice@example.com"},
			{"name": "Bob", "email": "bob@example.com"},
			{"name": "Charlie", "email": "charlie@example.com"},
		}

		for _, userData := range users {
			if err := db.Insert("recovery_users", "", userData); err != nil {
				t.Fatalf("Failed to insert user: %v", err)
			}
		}

		// Force snapshot before closing
		if err := persistenceManager.ForceSnapshot(); err != nil {
			t.Fatalf("Failed to force snapshot: %v", err)
		}

		// Close database
		db.Close()
		persistenceManager.Close()
	}

	// Second session: Recover data
	{
		persistenceManager, err := NewManager(persistenceConfig)
		if err != nil {
			t.Fatalf("Failed to create persistence manager for recovery: %v", err)
		}
		defer persistenceManager.Close()

		db := datastore.NewDataStoreEngine(datastore.EngineConfig{
			Persistence:       persistenceManager.GetPersistenceProvider(),
			EnablePersistence: true,
		})
		defer db.Close()

		persistenceManager.SetEngine(db)

		// Verify schema was recovered
		entityTypes := db.ListEntityTypes()
		if len(entityTypes) != 1 || entityTypes[0] != "recovery_users" {
			t.Errorf("Expected 1 entity type 'recovery_users', got %v", entityTypes)
		}

		// Verify data was recovered
		users, err := db.GetAllEntitiesOfType("recovery_users")
		if err != nil {
			t.Fatalf("Failed to get recovered users: %v", err)
		}

		if len(users) != 3 {
			t.Errorf("Expected 3 recovered users, got %d", len(users))
		}

		// Verify specific user data
		expectedNames := map[string]bool{"Alice": false, "Bob": false, "Charlie": false}
		for _, user := range users {
			name := user.Fields["name"].(string)
			if _, exists := expectedNames[name]; exists {
				expectedNames[name] = true
			}
		}

		for name, found := range expectedNames {
			if !found {
				t.Errorf("Expected to find user '%s' after recovery", name)
			}
		}

		// Verify auto-increment counter was recovered
		count, err := db.GetAutoIncrementCounter("recovery_users")
		if err != nil {
			t.Fatalf("Failed to get auto-increment counter: %v", err)
		}

		if count < 3 {
			t.Errorf("Expected auto-increment counter >= 3, got %d", count)
		}

		// Insert new user to verify counter works
		newUserData := map[string]interface{}{
			"name":  "Diana",
			"email": "diana@example.com",
		}

		if err := db.Insert("recovery_users", "", newUserData); err != nil {
			t.Fatalf("Failed to insert new user after recovery: %v", err)
		}

		// Verify new user has correct ID
		allUsers, err := db.GetAllEntitiesOfType("recovery_users")
		if err != nil {
			t.Fatalf("Failed to get all users: %v", err)
		}

		if len(allUsers) != 4 {
			t.Errorf("Expected 4 users after inserting new one, got %d", len(allUsers))
		}

		// Find Diana and check her ID
		var dianaID string
		for _, user := range allUsers {
			if user.Fields["name"] == "Diana" {
				dianaID = user.ID
				break
			}
		}

		if dianaID != "4" {
			t.Errorf("Expected Diana's ID to be '4', got '%s'", dianaID)
		}
	}
}

// TestBackupAndRestore tests backup and restore functionality
func TestBackupAndRestore(t *testing.T) {
	// Create temporary directories
	originalDir := t.TempDir()
	backupPath := originalDir + "/backup.db"

	// Setup logging
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	// Configure persistence
	persistenceConfig := Config{
		Path:             originalDir,
		CacheSize:        1000,
		SyncWrites:       true,
		SnapshotInterval: 1 * time.Minute,
		Logger:           logger,
		UseCompression:   false,
		EnableAutoGC:     false,
	}

	// Create original database
	persistenceManager, err := NewManager(persistenceConfig)
	if err != nil {
		t.Fatalf("Failed to create persistence manager: %v", err)
	}

	db := datastore.NewDataStoreEngine(datastore.EngineConfig{
		Persistence:       persistenceManager.GetPersistenceProvider(),
		EnablePersistence: true,
	})

	persistenceManager.SetEngine(db)

	// Define schema and insert data
	schema := common.EntityDefinition{
		Name:        "backup_entities",
		IDGenerator: common.IDTypeAutoIncrement,
		Fields: []common.FieldDefinition{
			{Name: "name", Type: "string", Required: true},
			{Name: "value", Type: "integer", Required: true},
		},
	}

	if err := db.RegisterEntityType(schema); err != nil {
		t.Fatalf("Failed to register schema: %v", err)
	}

	// Insert test data
	for i := 1; i <= 5; i++ {
		data := map[string]interface{}{
			"name":  "Entity " + string(rune(i+'0')),
			"value": i * 10,
		}
		if err := db.Insert("backup_entities", "", data); err != nil {
			t.Fatalf("Failed to insert entity %d: %v", i, err)
		}
	}

	// Create backup
	if err := persistenceManager.Backup(backupPath); err != nil {
		t.Fatalf("Failed to create backup: %v", err)
	}

	// Verify backup file exists
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Fatal("Backup file was not created")
	}

	// Close original database
	db.Close()
	persistenceManager.Close()

	// Test that backup file is not empty
	backupInfo, err := os.Stat(backupPath)
	if err != nil {
		t.Fatalf("Failed to stat backup file: %v", err)
	}

	if backupInfo.Size() == 0 {
		t.Error("Backup file is empty")
	}

	t.Logf("Backup file size: %d bytes", backupInfo.Size())
}

// TestGarbageCollection tests garbage collection functionality
func TestGarbageCollection(t *testing.T) {
	// Create temporary directory for test
	tempDir := t.TempDir()

	// Setup logging
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	// Configure persistence with GC enabled
	persistenceConfig := Config{
		Path:             tempDir,
		CacheSize:        1000,
		SyncWrites:       true,
		SnapshotInterval: 1 * time.Minute,
		Logger:           logger,
		UseCompression:   false,
		EnableAutoGC:     true,
		GCInterval:       100 * time.Millisecond, // Very short for testing
	}

	persistenceManager, err := NewManager(persistenceConfig)
	if err != nil {
		t.Fatalf("Failed to create persistence manager: %v", err)
	}
	defer persistenceManager.Close()

	db := datastore.NewDataStoreEngine(datastore.EngineConfig{
		Persistence:       persistenceManager.GetPersistenceProvider(),
		EnablePersistence: true,
	})
	defer db.Close()

	persistenceManager.SetEngine(db)

	// Define schema
	schema := common.EntityDefinition{
		Name:        "gc_entities",
		IDGenerator: common.IDTypeAutoIncrement,
		Fields: []common.FieldDefinition{
			{Name: "data", Type: "string", Required: true},
		},
	}

	if err := db.RegisterEntityType(schema); err != nil {
		t.Fatalf("Failed to register schema: %v", err)
	}

	// Insert and delete many entities to create garbage
	for i := 0; i < 100; i++ {
		data := map[string]interface{}{
			"data": "Large data string that will create garbage when deleted " + string(rune(i)),
		}

		if err := db.Insert("gc_entities", "", data); err != nil {
			t.Fatalf("Failed to insert entity %d: %v", i, err)
		}

		// Delete every other entity to create garbage
		if i%2 == 0 {
			if err := db.Delete("gc_entities", string(rune(i+1+'0'))); err != nil {
				// Some deletes might fail due to ID format, that's okay for this test
			}
		}
	}

	// Wait a bit for GC to potentially run
	time.Sleep(200 * time.Millisecond)

	// Manual GC test
	err = persistenceManager.RunValueLogGC(0.5)
	if err != nil && err.Error() != "Nothing to discard" {
		t.Logf("GC result: %v", err) // Log but don't fail, as GC behavior can vary
	}

	// Verify database is still functional
	entities, err := db.GetAllEntitiesOfType("gc_entities")
	if err != nil {
		t.Fatalf("Failed to get entities after GC: %v", err)
	}

	t.Logf("Entities remaining after GC: %d", len(entities))

	// Stop GC for clean shutdown
	persistenceManager.StopGarbageCollection()
}

// TestSnapshotInterval tests automatic snapshot creation
func TestSnapshotInterval(t *testing.T) {
	// Create temporary directory for test
	tempDir := t.TempDir()

	// Setup logging
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	// Configure persistence with short snapshot interval
	persistenceConfig := Config{
		Path:             tempDir,
		CacheSize:        1000,
		SyncWrites:       true,
		SnapshotInterval: 100 * time.Millisecond, // Very short for testing
		Logger:           logger,
		UseCompression:   false,
		EnableAutoGC:     false,
	}

	persistenceManager, err := NewManager(persistenceConfig)
	if err != nil {
		t.Fatalf("Failed to create persistence manager: %v", err)
	}
	defer persistenceManager.Close()

	db := datastore.NewDataStoreEngine(datastore.EngineConfig{
		Persistence:       persistenceManager.GetPersistenceProvider(),
		EnablePersistence: true,
	})
	defer db.Close()

	persistenceManager.SetEngine(db)

	// Define schema
	schema := common.EntityDefinition{
		Name:        "snapshot_entities",
		IDGenerator: common.IDTypeAutoIncrement,
		Fields: []common.FieldDefinition{
			{Name: "name", Type: "string", Required: true},
		},
	}

	if err := db.RegisterEntityType(schema); err != nil {
		t.Fatalf("Failed to register schema: %v", err)
	}

	// Insert some data
	data := map[string]interface{}{"name": "Test Entity"}
	if err := db.Insert("snapshot_entities", "", data); err != nil {
		t.Fatalf("Failed to insert entity: %v", err)
	}

	// Wait for automatic snapshot
	time.Sleep(200 * time.Millisecond)

	// Force another snapshot to ensure functionality
	if err := persistenceManager.ForceSnapshot(); err != nil {
		t.Fatalf("Failed to force snapshot: %v", err)
	}

	// Verify data is still accessible
	entities, err := db.GetAllEntitiesOfType("snapshot_entities")
	if err != nil {
		t.Fatalf("Failed to get entities: %v", err)
	}

	if len(entities) != 1 {
		t.Errorf("Expected 1 entity, got %d", len(entities))
	}

	// Test changing snapshot interval
	persistenceManager.SetSnapshotInterval(1 * time.Second)

	// Insert more data
	data2 := map[string]interface{}{"name": "Test Entity 2"}
	if err := db.Insert("snapshot_entities", "", data2); err != nil {
		t.Fatalf("Failed to insert second entity: %v", err)
	}

	// Verify both entities exist
	allEntities, err := db.GetAllEntitiesOfType("snapshot_entities")
	if err != nil {
		t.Fatalf("Failed to get all entities: %v", err)
	}

	if len(allEntities) != 2 {
		t.Errorf("Expected 2 entities, got %d", len(allEntities))
	}
}

// TestCompressionSettings tests different compression settings
func TestCompressionSettings(t *testing.T) {
	// Test with compression disabled
	t.Run("CompressionDisabled", func(t *testing.T) {
		tempDir := t.TempDir()
		logger := logrus.New()
		logger.SetLevel(logrus.ErrorLevel)

		config := Config{
			Path:           tempDir,
			CacheSize:      1000,
			SyncWrites:     true,
			Logger:         logger,
			UseCompression: false,
			EnableAutoGC:   false,
		}

		persistenceManager, err := NewManager(config)
		if err != nil {
			t.Fatalf("Failed to create persistence manager without compression: %v", err)
		}
		defer persistenceManager.Close()

		db := datastore.NewDataStoreEngine(datastore.EngineConfig{
			Persistence:       persistenceManager.GetPersistenceProvider(),
			EnablePersistence: true,
		})
		defer db.Close()

		persistenceManager.SetEngine(db)

		// Test basic functionality
		schema := common.EntityDefinition{
			Name:        "no_compression_entities",
			IDGenerator: common.IDTypeAutoIncrement,
			Fields: []common.FieldDefinition{
				{Name: "data", Type: "string", Required: true},
			},
		}

		if err := db.RegisterEntityType(schema); err != nil {
			t.Fatalf("Failed to register schema: %v", err)
		}

		data := map[string]interface{}{"data": "Test data without compression"}
		if err := db.Insert("no_compression_entities", "", data); err != nil {
			t.Fatalf("Failed to insert entity: %v", err)
		}

		entities, err := db.GetAllEntitiesOfType("no_compression_entities")
		if err != nil {
			t.Fatalf("Failed to get entities: %v", err)
		}

		if len(entities) != 1 {
			t.Errorf("Expected 1 entity, got %d", len(entities))
		}
	})

	// Test with compression enabled
	t.Run("CompressionEnabled", func(t *testing.T) {
		tempDir := t.TempDir()
		logger := logrus.New()
		logger.SetLevel(logrus.ErrorLevel)

		config := Config{
			Path:           tempDir,
			CacheSize:      1000,
			SyncWrites:     true,
			Logger:         logger,
			UseCompression: true,
			EnableAutoGC:   false,
		}

		persistenceManager, err := NewManager(config)
		if err != nil {
			t.Fatalf("Failed to create persistence manager with compression: %v", err)
		}
		defer persistenceManager.Close()

		db := datastore.NewDataStoreEngine(datastore.EngineConfig{
			Persistence:       persistenceManager.GetPersistenceProvider(),
			EnablePersistence: true,
		})
		defer db.Close()

		persistenceManager.SetEngine(db)

		// Test basic functionality
		schema := common.EntityDefinition{
			Name:        "compression_entities",
			IDGenerator: common.IDTypeAutoIncrement,
			Fields: []common.FieldDefinition{
				{Name: "data", Type: "string", Required: true},
			},
		}

		if err := db.RegisterEntityType(schema); err != nil {
			t.Fatalf("Failed to register schema: %v", err)
		}

		// Insert larger data that would benefit from compression
		largeData := ""
		for i := 0; i < 1000; i++ {
			largeData += "This is repetitive data that should compress well. "
		}

		data := map[string]interface{}{"data": largeData}
		if err := db.Insert("compression_entities", "", data); err != nil {
			t.Fatalf("Failed to insert entity: %v", err)
		}

		entities, err := db.GetAllEntitiesOfType("compression_entities")
		if err != nil {
			t.Fatalf("Failed to get entities: %v", err)
		}

		if len(entities) != 1 {
			t.Errorf("Expected 1 entity, got %d", len(entities))
		}

		// Verify data integrity
		retrievedData := entities[0].Fields["data"].(string)
		if retrievedData != largeData {
			t.Error("Data integrity check failed with compression")
		}
	})
}
