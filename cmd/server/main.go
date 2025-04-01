package main

import (
	"flag"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/phillarmonic/syncopate-db/internal/api"
	"github.com/phillarmonic/syncopate-db/internal/datastore"
)

func main() {
	// Parse command-line flags
	port := flag.Int("port", 8080, "Port to listen on")
	logLevel := flag.String("log-level", "info", "Log level (debug, info, warn, error)")
	flag.Parse()

	// Set up logging
	logger := logrus.New()
	level, err := logrus.ParseLevel(*logLevel)
	if err != nil {
		level = logrus.InfoLevel
	}
	logger.SetLevel(level)
	logger.SetFormatter(&logrus.JSONFormatter{})

	// Initialize the data store
	logger.Info("Initializing data store...")
	engine := datastore.NewDataStoreEngine()
	queryService := datastore.NewQueryService(engine)

	// Register example entity types
	setupExampleEntityTypes(engine, logger)

	// Configure and start the server
	serverConfig := api.ServerConfig{
		Port:         *port,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
		LogLevel:     level,
	}

	server := api.NewServer(engine, queryService, serverConfig)
	logger.Fatal(server.Start())
}

// setupExampleEntityTypes registers some example entity types
func setupExampleEntityTypes(engine *datastore.Engine, logger *logrus.Logger) {
	// User entity type
	userDef := datastore.EntityDefinition{
		Name: "user",
		Fields: []datastore.FieldDefinition{
			{Name: "name", Type: datastore.TypeString, Indexed: true, Required: true},
			{Name: "email", Type: datastore.TypeString, Indexed: true, Required: true},
			{Name: "age", Type: datastore.TypeInteger, Indexed: true},
			{Name: "active", Type: datastore.TypeBoolean, Indexed: true},
			{Name: "created", Type: datastore.TypeDateTime, Indexed: true},
			{Name: "profile", Type: datastore.TypeJSON},
			{Name: "bio", Type: datastore.TypeText},
		},
	}

	if err := engine.RegisterEntityType(userDef); err != nil {
		logger.Warnf("Error registering user entity type: %v", err)
	} else {
		logger.Info("Registered user entity type")
	}

	// Product entity type
	productDef := datastore.EntityDefinition{
		Name: "product",
		Fields: []datastore.FieldDefinition{
			{Name: "name", Type: datastore.TypeString, Indexed: true, Required: true},
			{Name: "price", Type: datastore.TypeFloat, Indexed: true, Required: true},
			{Name: "description", Type: datastore.TypeText},
			{Name: "inStock", Type: datastore.TypeBoolean, Indexed: true},
			{Name: "releaseDate", Type: datastore.TypeDate, Indexed: true},
			{Name: "categories", Type: datastore.TypeJSON},
		},
	}

	if err := engine.RegisterEntityType(productDef); err != nil {
		logger.Warnf("Error registering product entity type: %v", err)
	} else {
		logger.Info("Registered product entity type")
	}

	// Task entity type
	taskDef := datastore.EntityDefinition{
		Name: "task",
		Fields: []datastore.FieldDefinition{
			{Name: "title", Type: datastore.TypeString, Indexed: true, Required: true},
			{Name: "description", Type: datastore.TypeText},
			{Name: "dueDate", Type: datastore.TypeDate, Indexed: true},
			{Name: "status", Type: datastore.TypeString, Indexed: true, Required: true},
			{Name: "priority", Type: datastore.TypeInteger, Indexed: true},
			{Name: "assignedTo", Type: datastore.TypeString, Indexed: true},
			{Name: "tags", Type: datastore.TypeJSON},
		},
	}

	if err := engine.RegisterEntityType(taskDef); err != nil {
		logger.Warnf("Error registering task entity type: %v", err)
	} else {
		logger.Info("Registered task entity type")
	}

	// Insert some example data
	insertExampleData(engine, logger)
}

// insertExampleData creates some sample records
func insertExampleData(engine *datastore.Engine, logger *logrus.Logger) {
	// Example user
	userData := map[string]interface{}{
		"name":    "John Doe",
		"email":   "john@example.com",
		"age":     30,
		"active":  true,
		"created": time.Now(),
		"profile": map[string]interface{}{
			"address": "123 Main St",
			"phone":   "555-1234",
		},
		"bio": "Software engineer with 5 years of experience",
	}

	if err := engine.Insert("user", "user-1", userData); err != nil {
		logger.Warnf("Error inserting example user: %v", err)
	} else {
		logger.Info("Inserted example user data")
	}

	// Example product
	productData := map[string]interface{}{
		"name":        "Laptop Pro",
		"price":       1299.99,
		"description": "High-performance laptop with 16GB RAM and SSD",
		"inStock":     true,
		"releaseDate": time.Now().AddDate(0, -3, 0),
		"categories":  []string{"electronics", "computers"},
	}

	if err := engine.Insert("product", "product-1", productData); err != nil {
		logger.Warnf("Error inserting example product: %v", err)
	} else {
		logger.Info("Inserted example product data")
	}

	// Example task
	taskData := map[string]interface{}{
		"title":       "Complete project proposal",
		"description": "Draft and submit the proposal for the new client project",
		"dueDate":     time.Now().AddDate(0, 0, 7),
		"status":      "in_progress",
		"priority":    1,
		"assignedTo":  "user-1",
		"tags":        []string{"client", "proposal", "urgent"},
	}

	if err := engine.Insert("task", "task-1", taskData); err != nil {
		logger.Warnf("Error inserting example task: %v", err)
	} else {
		logger.Info("Inserted example task data")
	}
}
