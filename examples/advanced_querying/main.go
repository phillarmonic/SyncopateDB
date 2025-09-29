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

	// Create query service
	queryService := datastore.NewQueryService(db)

	// Define schemas
	userSchema := common.EntityDefinition{
		Name:        "users",
		IDGenerator: common.IDTypeAutoIncrement,
		Fields: []common.FieldDefinition{
			{Name: "name", Type: "string", Required: true, Indexed: true},
			{Name: "email", Type: "string", Required: true, Unique: true},
			{Name: "age", Type: "integer", Nullable: true, Indexed: true},
			{Name: "department", Type: "string", Required: true, Indexed: true},
		},
	}

	postSchema := common.EntityDefinition{
		Name:        "posts",
		IDGenerator: common.IDTypeUUID,
		Fields: []common.FieldDefinition{
			{Name: "title", Type: "string", Required: true, Indexed: true},
			{Name: "content", Type: "string", Required: true},
			{Name: "author_id", Type: "string", Required: true, Indexed: true},
			{Name: "tags", Type: "array", Required: false},
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

	// Insert sample users
	users := []map[string]interface{}{
		{"name": "Alice Johnson", "email": "alice@example.com", "age": 28, "department": "Engineering"},
		{"name": "Bob Smith", "email": "bob@example.com", "age": 35, "department": "Marketing"},
		{"name": "Charlie Brown", "email": "charlie@example.com", "age": 22, "department": "Engineering"},
		{"name": "Diana Prince", "email": "diana@example.com", "age": 30, "department": "Sales"},
	}

	for _, userData := range users {
		if err := db.Insert("users", "", userData); err != nil {
			log.Fatal("Failed to insert user:", err)
		}
	}

	// Insert sample posts
	posts := []map[string]interface{}{
		{
			"title":     "Introduction to SyncopateDB",
			"content":   "This is a comprehensive guide...",
			"author_id": "1",
			"published": true,
			"tags":      []interface{}{"database", "tutorial", "syncopate"},
		},
		{
			"title":     "Advanced Query Techniques",
			"content":   "Learn how to write complex queries...",
			"author_id": "1",
			"published": true,
			"tags":      []interface{}{"database", "advanced", "queries"},
		},
		{
			"title":     "Marketing Strategies",
			"content":   "Effective marketing approaches...",
			"author_id": "2",
			"published": false,
			"tags":      []interface{}{"marketing", "strategy"},
		},
	}

	for _, postData := range posts {
		if err := db.Insert("posts", "", postData); err != nil {
			log.Fatal("Failed to insert post:", err)
		}
	}

	// Example 1: Basic filtering with pagination
	fmt.Println("=== Example 1: Users in Engineering Department ===")
	queryOptions := datastore.QueryOptions{
		EntityType: "users",
		Filters: []datastore.Filter{
			{Field: "department", Operator: datastore.FilterEq, Value: "Engineering"},
		},
		OrderBy:   "age",
		OrderDesc: false,
		Limit:     10,
		Offset:    0,
	}

	response, err := queryService.ExecutePaginatedQuery(queryOptions)
	if err != nil {
		log.Fatal("Query failed:", err)
	}

	fmt.Printf("Found %d engineering users (total: %d):\n", response.Count, response.Total)
	for _, user := range response.Data {
		fmt.Printf("- %s (age: %v)\n", user.Fields["name"], user.Fields["age"])
	}

	// Example 2: Complex filtering with multiple conditions
	fmt.Println("\n=== Example 2: Users aged 25-35 ===")
	queryOptions = datastore.QueryOptions{
		EntityType: "users",
		Filters: []datastore.Filter{
			{Field: "age", Operator: datastore.FilterGte, Value: 25},
			{Field: "age", Operator: datastore.FilterLte, Value: 35},
		},
		OrderBy:   "age",
		OrderDesc: true,
	}

	response, err = queryService.ExecutePaginatedQuery(queryOptions)
	if err != nil {
		log.Fatal("Query failed:", err)
	}

	fmt.Printf("Found %d users aged 25-35:\n", response.Count)
	for _, user := range response.Data {
		fmt.Printf("- %s (age: %v, dept: %s)\n",
			user.Fields["name"], user.Fields["age"], user.Fields["department"])
	}

	// Example 3: Array operations
	fmt.Println("\n=== Example 3: Posts with 'database' tag ===")
	queryOptions = datastore.QueryOptions{
		EntityType: "posts",
		Filters: []datastore.Filter{
			{Field: "tags", Operator: datastore.FilterArrayContains, Value: "database"},
		},
	}

	response, err = queryService.ExecutePaginatedQuery(queryOptions)
	if err != nil {
		log.Fatal("Query failed:", err)
	}

	fmt.Printf("Found %d posts with 'database' tag:\n", response.Count)
	for _, post := range response.Data {
		fmt.Printf("- %s (tags: %v)\n", post.Fields["title"], post.Fields["tags"])
	}

	// Example 4: Query with joins
	fmt.Println("\n=== Example 4: Posts with Author Information ===")
	queryOptions = datastore.QueryOptions{
		EntityType: "posts",
		Filters: []datastore.Filter{
			{Field: "published", Operator: datastore.FilterEq, Value: true},
		},
		Joins: []datastore.JoinOptions{
			{
				EntityType:   "users",
				LocalField:   "author_id",
				ForeignField: "id",
				JoinType:     datastore.JoinTypeLeft,
				ResultField:  "author",
			},
		},
		OrderBy: "_created_at",
		Limit:   10,
	}

	response, err = queryService.ExecutePaginatedQuery(queryOptions)
	if err != nil {
		log.Fatal("Query failed:", err)
	}

	fmt.Printf("Found %d published posts:\n", response.Count)
	for _, post := range response.Data {
		authorInfo := post.Fields["author"]
		if author, ok := authorInfo.(map[string]interface{}); ok {
			fmt.Printf("- '%s' by %s\n", post.Fields["title"], author["name"])
		} else {
			fmt.Printf("- '%s' (no author info)\n", post.Fields["title"])
		}
	}

	// Example 5: Count query
	fmt.Println("\n=== Example 5: Count Queries ===")
	totalUsers, err := queryService.ExecuteCountQuery(datastore.QueryOptions{
		EntityType: "users",
	})
	if err != nil {
		log.Fatal("Count query failed:", err)
	}

	engineeringUsers, err := queryService.ExecuteCountQuery(datastore.QueryOptions{
		EntityType: "users",
		Filters: []datastore.Filter{
			{Field: "department", Operator: datastore.FilterEq, Value: "Engineering"},
		},
	})
	if err != nil {
		log.Fatal("Count query failed:", err)
	}

	fmt.Printf("Total users: %d\n", totalUsers)
	fmt.Printf("Engineering users: %d\n", engineeringUsers)
}
