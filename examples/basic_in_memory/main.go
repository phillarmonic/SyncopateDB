package main

import (
	"fmt"
	"log"

	"github.com/phillarmonic/syncopate-db/internal/common"
	"github.com/phillarmonic/syncopate-db/internal/datastore"
)

func main() {
	// Create in-memory database
	db := datastore.NewDataStoreEngine()
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
		log.Fatal("Failed to register user schema:", err)
	}

	// Insert data
	userData := map[string]interface{}{
		"name":  "John Doe",
		"email": "john@example.com",
		"age":   30,
	}

	err = db.Insert("users", "", userData) // Empty ID for auto-generation
	if err != nil {
		log.Fatal("Failed to insert user:", err)
	}

	// Insert another user
	userData2 := map[string]interface{}{
		"name":  "Jane Smith",
		"email": "jane@example.com",
		"age":   25,
	}

	err = db.Insert("users", "", userData2)
	if err != nil {
		log.Fatal("Failed to insert second user:", err)
	}

	// Get all users
	users, err := db.GetAllEntitiesOfType("users")
	if err != nil {
		log.Fatal("Failed to get users:", err)
	}

	fmt.Printf("Found %d users:\n", len(users))
	for _, user := range users {
		fmt.Printf("- ID: %s, Name: %s, Email: %s, Age: %v\n",
			user.ID,
			user.Fields["name"],
			user.Fields["email"],
			user.Fields["age"])
	}

	// Get user count
	count, err := db.GetEntityCount("users")
	if err != nil {
		log.Fatal("Failed to get user count:", err)
	}
	fmt.Printf("Total users: %d\n", count)
}
