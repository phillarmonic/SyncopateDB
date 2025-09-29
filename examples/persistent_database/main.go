package main

import (
	"fmt"
	"log"
	"time"

	"github.com/phillarmonic/syncopate-db/internal/common"
	"github.com/phillarmonic/syncopate-db/internal/datastore"
	"github.com/phillarmonic/syncopate-db/internal/persistence"
	"github.com/sirupsen/logrus"
)

func main() {
	// Setup logging
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)

	// Configure persistence
	persistenceConfig := persistence.Config{
		Path:             "./example_data",
		CacheSize:        5000,
		SyncWrites:       true,
		SnapshotInterval: 5 * time.Minute,
		Logger:           logger,
		UseCompression:   true,
		EnableAutoGC:     true,
		GCInterval:       2 * time.Minute,
	}

	// Create persistence manager
	persistenceManager, err := persistence.NewManager(persistenceConfig)
	if err != nil {
		log.Fatal("Failed to create persistence manager:", err)
	}
	defer persistenceManager.Close()

	// Create database with persistence
	db := datastore.NewDataStoreEngine(datastore.EngineConfig{
		Persistence:       persistenceManager.GetPersistenceProvider(),
		EnablePersistence: true,
	})
	defer db.Close()

	// Set engine in persistence manager
	persistenceManager.SetEngine(db)

	// Define schemas
	userSchema := common.EntityDefinition{
		Name:        "users",
		IDGenerator: common.IDTypeAutoIncrement,
		Fields: []common.FieldDefinition{
			{Name: "name", Type: "string", Required: true, Indexed: true},
			{Name: "email", Type: "string", Required: true, Unique: true},
			{Name: "age", Type: "integer", Nullable: true},
			{Name: "active", Type: "boolean", Required: true},
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
		log.Fatal("Failed to register user schema:", err)
	}

	if err := db.RegisterEntityType(postSchema); err != nil {
		log.Fatal("Failed to register post schema:", err)
	}

	// Insert sample data
	userData := map[string]interface{}{
		"name":   "Alice Johnson",
		"email":  "alice@example.com",
		"age":    28,
		"active": true,
	}

	if err := db.Insert("users", "", userData); err != nil {
		log.Fatal("Failed to insert user:", err)
	}

	postData := map[string]interface{}{
		"title":     "Getting Started with SyncopateDB",
		"content":   "SyncopateDB is a high-performance embedded database...",
		"author_id": "1", // References the user we just created
		"published": true,
	}

	if err := db.Insert("posts", "", postData); err != nil {
		log.Fatal("Failed to insert post:", err)
	}

	// Get database statistics
	stats := persistenceManager.GetStorageStats()
	fmt.Printf("Database Statistics:\n")
	fmt.Printf("- Entity Types: %d\n", stats["entity_types_count"])
	fmt.Printf("- Total Entities: %v\n", stats["entity_counts"])
	if dbSize, ok := stats["database_size_mb"]; ok {
		fmt.Printf("- Size: %.2f MB\n", dbSize)
	}

	// Force a snapshot
	if err := persistenceManager.ForceSnapshot(); err != nil {
		log.Printf("Warning: Failed to force snapshot: %v", err)
	} else {
		fmt.Println("Snapshot created successfully")
	}

	fmt.Println("Data has been persisted to ./example_data")
}
