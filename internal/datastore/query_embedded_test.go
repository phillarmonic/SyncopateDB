package datastore

import (
	"testing"

	"github.com/phillarmonic/syncopate-db/internal/common"
)

// TestFilterOperators tests all filter operators
func TestFilterOperators(t *testing.T) {
	// Create in-memory database
	db := NewDataStoreEngine()
	defer db.Close()

	queryService := NewQueryService(db)

	// Define schema
	schema := common.EntityDefinition{
		Name:        "filter_test_entities",
		IDGenerator: common.IDTypeAutoIncrement,
		Fields: []common.FieldDefinition{
			{Name: "name", Type: "string", Required: true, Indexed: true},
			{Name: "age", Type: "integer", Required: true, Indexed: true},
			{Name: "score", Type: "float", Required: true, Indexed: true},
			{Name: "active", Type: "boolean", Required: true, Indexed: true},
			{Name: "tags", Type: "array", Required: false},
		},
	}

	if err := db.RegisterEntityType(schema); err != nil {
		t.Fatalf("Failed to register schema: %v", err)
	}

	// Insert test data
	testData := []map[string]interface{}{
		{"name": "Alice", "age": 25, "score": 85.5, "active": true, "tags": []interface{}{"developer", "senior"}},
		{"name": "Bob", "age": 30, "score": 92.0, "active": false, "tags": []interface{}{"manager", "senior"}},
		{"name": "Charlie", "age": 22, "score": 78.5, "active": true, "tags": []interface{}{"developer", "junior"}},
		{"name": "Diana", "age": 35, "score": 88.0, "active": true, "tags": []interface{}{"designer", "senior"}},
	}

	for _, data := range testData {
		if err := db.Insert("filter_test_entities", "", data); err != nil {
			t.Fatalf("Failed to insert test data: %v", err)
		}
	}

	// Test FilterEq
	t.Run("FilterEq", func(t *testing.T) {
		response, err := queryService.ExecutePaginatedQuery(QueryOptions{
			EntityType: "filter_test_entities",
			Filters: []Filter{
				{Field: "name", Operator: FilterEq, Value: "Alice"},
			},
		})
		if err != nil {
			t.Fatalf("FilterEq query failed: %v", err)
		}
		if response.Count != 1 {
			t.Errorf("Expected 1 result, got %d", response.Count)
		}
	})

	// Test FilterNeq
	t.Run("FilterNeq", func(t *testing.T) {
		response, err := queryService.ExecutePaginatedQuery(QueryOptions{
			EntityType: "filter_test_entities",
			Filters: []Filter{
				{Field: "active", Operator: FilterNeq, Value: true},
			},
		})
		if err != nil {
			t.Fatalf("FilterNeq query failed: %v", err)
		}
		if response.Count != 1 {
			t.Errorf("Expected 1 result, got %d", response.Count)
		}
	})

	// Test FilterGt
	t.Run("FilterGt", func(t *testing.T) {
		response, err := queryService.ExecutePaginatedQuery(QueryOptions{
			EntityType: "filter_test_entities",
			Filters: []Filter{
				{Field: "age", Operator: FilterGt, Value: 25},
			},
		})
		if err != nil {
			t.Fatalf("FilterGt query failed: %v", err)
		}
		if response.Count != 2 {
			t.Errorf("Expected 2 results, got %d", response.Count)
		}
	})

	// Test FilterGte
	t.Run("FilterGte", func(t *testing.T) {
		response, err := queryService.ExecutePaginatedQuery(QueryOptions{
			EntityType: "filter_test_entities",
			Filters: []Filter{
				{Field: "age", Operator: FilterGte, Value: 25},
			},
		})
		if err != nil {
			t.Fatalf("FilterGte query failed: %v", err)
		}
		if response.Count != 3 {
			t.Errorf("Expected 3 results, got %d", response.Count)
		}
	})

	// Test FilterLt
	t.Run("FilterLt", func(t *testing.T) {
		response, err := queryService.ExecutePaginatedQuery(QueryOptions{
			EntityType: "filter_test_entities",
			Filters: []Filter{
				{Field: "score", Operator: FilterLt, Value: 80.0},
			},
		})
		if err != nil {
			t.Fatalf("FilterLt query failed: %v", err)
		}
		if response.Count != 1 {
			t.Errorf("Expected 1 result, got %d", response.Count)
		}
	})

	// Test FilterLte
	t.Run("FilterLte", func(t *testing.T) {
		response, err := queryService.ExecutePaginatedQuery(QueryOptions{
			EntityType: "filter_test_entities",
			Filters: []Filter{
				{Field: "score", Operator: FilterLte, Value: 85.5},
			},
		})
		if err != nil {
			t.Fatalf("FilterLte query failed: %v", err)
		}
		if response.Count != 2 {
			t.Errorf("Expected 2 results, got %d", response.Count)
		}
	})

	// Test FilterContains
	t.Run("FilterContains", func(t *testing.T) {
		response, err := queryService.ExecutePaginatedQuery(QueryOptions{
			EntityType: "filter_test_entities",
			Filters: []Filter{
				{Field: "name", Operator: FilterContains, Value: "li"},
			},
		})
		if err != nil {
			t.Fatalf("FilterContains query failed: %v", err)
		}
		if response.Count != 2 { // Alice and Charlie
			t.Errorf("Expected 2 results, got %d", response.Count)
		}
	})

	// Test FilterStartsWith
	t.Run("FilterStartsWith", func(t *testing.T) {
		response, err := queryService.ExecutePaginatedQuery(QueryOptions{
			EntityType: "filter_test_entities",
			Filters: []Filter{
				{Field: "name", Operator: FilterStartsWith, Value: "A"},
			},
		})
		if err != nil {
			t.Fatalf("FilterStartsWith query failed: %v", err)
		}
		if response.Count != 1 { // Alice
			t.Errorf("Expected 1 result, got %d", response.Count)
		}
	})

	// Test FilterEndsWith
	t.Run("FilterEndsWith", func(t *testing.T) {
		response, err := queryService.ExecutePaginatedQuery(QueryOptions{
			EntityType: "filter_test_entities",
			Filters: []Filter{
				{Field: "name", Operator: FilterEndsWith, Value: "e"},
			},
		})
		if err != nil {
			t.Fatalf("FilterEndsWith query failed: %v", err)
		}
		if response.Count != 2 { // Alice and Charlie
			t.Errorf("Expected 2 results, got %d", response.Count)
		}
	})

	// Test FilterIn
	t.Run("FilterIn", func(t *testing.T) {
		response, err := queryService.ExecutePaginatedQuery(QueryOptions{
			EntityType: "filter_test_entities",
			Filters: []Filter{
				{Field: "name", Operator: FilterIn, Value: []interface{}{"Alice", "Bob"}},
			},
		})
		if err != nil {
			t.Fatalf("FilterIn query failed: %v", err)
		}
		if response.Count != 2 {
			t.Errorf("Expected 2 results, got %d", response.Count)
		}
	})

	// Test FilterArrayContains
	t.Run("FilterArrayContains", func(t *testing.T) {
		response, err := queryService.ExecutePaginatedQuery(QueryOptions{
			EntityType: "filter_test_entities",
			Filters: []Filter{
				{Field: "tags", Operator: FilterArrayContains, Value: "senior"},
			},
		})
		if err != nil {
			t.Fatalf("FilterArrayContains query failed: %v", err)
		}
		if response.Count != 3 { // Alice, Bob, Diana
			t.Errorf("Expected 3 results, got %d", response.Count)
		}
	})

	// Test FilterArrayContainsAny
	t.Run("FilterArrayContainsAny", func(t *testing.T) {
		response, err := queryService.ExecutePaginatedQuery(QueryOptions{
			EntityType: "filter_test_entities",
			Filters: []Filter{
				{Field: "tags", Operator: FilterArrayContainsAny, Value: []interface{}{"manager", "designer"}},
			},
		})
		if err != nil {
			t.Fatalf("FilterArrayContainsAny query failed: %v", err)
		}
		if response.Count != 2 { // Bob, Diana
			t.Errorf("Expected 2 results, got %d", response.Count)
		}
	})

	// Test FilterArrayContainsAll
	t.Run("FilterArrayContainsAll", func(t *testing.T) {
		response, err := queryService.ExecutePaginatedQuery(QueryOptions{
			EntityType: "filter_test_entities",
			Filters: []Filter{
				{Field: "tags", Operator: FilterArrayContainsAll, Value: []interface{}{"developer", "senior"}},
			},
		})
		if err != nil {
			t.Fatalf("FilterArrayContainsAll query failed: %v", err)
		}
		if response.Count != 1 { // Alice
			t.Errorf("Expected 1 result, got %d", response.Count)
		}
	})
}

// TestSortingAndPagination tests sorting and pagination functionality
func TestSortingAndPagination(t *testing.T) {
	// Create in-memory database
	db := NewDataStoreEngine()
	defer db.Close()

	queryService := NewQueryService(db)

	// Define schema
	schema := common.EntityDefinition{
		Name:        "sort_test_entities",
		IDGenerator: common.IDTypeAutoIncrement,
		Fields: []common.FieldDefinition{
			{Name: "name", Type: "string", Required: true, Indexed: true},
			{Name: "age", Type: "integer", Required: true, Indexed: true},
			{Name: "score", Type: "float", Required: true, Indexed: true},
		},
	}

	if err := db.RegisterEntityType(schema); err != nil {
		t.Fatalf("Failed to register schema: %v", err)
	}

	// Insert test data
	testData := []map[string]interface{}{
		{"name": "Alice", "age": 25, "score": 85.5},
		{"name": "Bob", "age": 30, "score": 92.0},
		{"name": "Charlie", "age": 22, "score": 78.5},
		{"name": "Diana", "age": 35, "score": 88.0},
		{"name": "Eve", "age": 28, "score": 95.0},
	}

	for _, data := range testData {
		if err := db.Insert("sort_test_entities", "", data); err != nil {
			t.Fatalf("Failed to insert test data: %v", err)
		}
	}

	// Test sorting by age ascending
	t.Run("SortAgeAscending", func(t *testing.T) {
		response, err := queryService.ExecutePaginatedQuery(QueryOptions{
			EntityType: "sort_test_entities",
			OrderBy:    "age",
			OrderDesc:  false,
		})
		if err != nil {
			t.Fatalf("Sort query failed: %v", err)
		}

		if response.Count != 5 {
			t.Errorf("Expected 5 results, got %d", response.Count)
		}

		// Check order: Charlie(22), Alice(25), Eve(28), Bob(30), Diana(35)
		expectedAges := []int{22, 25, 28, 30, 35}
		for i, entity := range response.Data {
			if age := entity.Fields["age"]; age != expectedAges[i] {
				t.Errorf("Expected age %d at position %d, got %v", expectedAges[i], i, age)
			}
		}
	})

	// Test sorting by score descending
	t.Run("SortScoreDescending", func(t *testing.T) {
		response, err := queryService.ExecutePaginatedQuery(QueryOptions{
			EntityType: "sort_test_entities",
			OrderBy:    "score",
			OrderDesc:  true,
		})
		if err != nil {
			t.Fatalf("Sort query failed: %v", err)
		}

		// Check order: Eve(95.0), Bob(92.0), Diana(88.0), Alice(85.5), Charlie(78.5)
		expectedScores := []float64{95.0, 92.0, 88.0, 85.5, 78.5}
		for i, entity := range response.Data {
			if score := entity.Fields["score"]; score != expectedScores[i] {
				t.Errorf("Expected score %f at position %d, got %v", expectedScores[i], i, score)
			}
		}
	})

	// Test pagination
	t.Run("Pagination", func(t *testing.T) {
		// First page
		response1, err := queryService.ExecutePaginatedQuery(QueryOptions{
			EntityType: "sort_test_entities",
			OrderBy:    "age",
			OrderDesc:  false,
			Limit:      2,
			Offset:     0,
		})
		if err != nil {
			t.Fatalf("Pagination query failed: %v", err)
		}

		if response1.Count != 2 {
			t.Errorf("Expected 2 results on first page, got %d", response1.Count)
		}

		if response1.Total != 5 {
			t.Errorf("Expected total 5, got %d", response1.Total)
		}

		if !response1.HasMore {
			t.Error("Expected HasMore to be true")
		}

		// Second page
		response2, err := queryService.ExecutePaginatedQuery(QueryOptions{
			EntityType: "sort_test_entities",
			OrderBy:    "age",
			OrderDesc:  false,
			Limit:      2,
			Offset:     2,
		})
		if err != nil {
			t.Fatalf("Pagination query failed: %v", err)
		}

		if response2.Count != 2 {
			t.Errorf("Expected 2 results on second page, got %d", response2.Count)
		}

		if response2.Total != 5 {
			t.Errorf("Expected total 5, got %d", response2.Total)
		}

		if !response2.HasMore {
			t.Error("Expected HasMore to be true")
		}

		// Third page (last page)
		response3, err := queryService.ExecutePaginatedQuery(QueryOptions{
			EntityType: "sort_test_entities",
			OrderBy:    "age",
			OrderDesc:  false,
			Limit:      2,
			Offset:     4,
		})
		if err != nil {
			t.Fatalf("Pagination query failed: %v", err)
		}

		if response3.Count != 1 {
			t.Errorf("Expected 1 result on third page, got %d", response3.Count)
		}

		if response3.Total != 5 {
			t.Errorf("Expected total 5, got %d", response3.Total)
		}

		if response3.HasMore {
			t.Error("Expected HasMore to be false")
		}
	})
}

// TestJoinTypes tests different join types
func TestJoinTypes(t *testing.T) {
	// Create in-memory database
	db := NewDataStoreEngine()
	defer db.Close()

	queryService := NewQueryService(db)

	// Define schemas
	userSchema := common.EntityDefinition{
		Name:        "join_users",
		IDGenerator: common.IDTypeAutoIncrement,
		Fields: []common.FieldDefinition{
			{Name: "name", Type: "string", Required: true, Indexed: true},
			{Name: "department", Type: "string", Required: true, Indexed: true},
		},
	}

	postSchema := common.EntityDefinition{
		Name:        "join_posts",
		IDGenerator: common.IDTypeAutoIncrement,
		Fields: []common.FieldDefinition{
			{Name: "title", Type: "string", Required: true, Indexed: true},
			{Name: "author_id", Type: "string", Required: true, Indexed: true},
		},
	}

	// Register schemas
	if err := db.RegisterEntityType(userSchema); err != nil {
		t.Fatalf("Failed to register user schema: %v", err)
	}

	if err := db.RegisterEntityType(postSchema); err != nil {
		t.Fatalf("Failed to register post schema: %v", err)
	}

	// Insert users
	users := []map[string]interface{}{
		{"name": "Alice", "department": "Engineering"},
		{"name": "Bob", "department": "Marketing"},
		{"name": "Charlie", "department": "Engineering"},
	}

	for _, userData := range users {
		if err := db.Insert("join_users", "", userData); err != nil {
			t.Fatalf("Failed to insert user: %v", err)
		}
	}

	// Insert posts (only Alice and Bob have posts)
	posts := []map[string]interface{}{
		{"title": "Alice's First Post", "author_id": "1"},
		{"title": "Alice's Second Post", "author_id": "1"},
		{"title": "Bob's Post", "author_id": "2"},
	}

	for _, postData := range posts {
		if err := db.Insert("join_posts", "", postData); err != nil {
			t.Fatalf("Failed to insert post: %v", err)
		}
	}

	// Test Inner Join
	t.Run("InnerJoin", func(t *testing.T) {
		response, err := queryService.ExecutePaginatedQuery(QueryOptions{
			EntityType: "join_users",
			Joins: []JoinOptions{
				{
					EntityType:   "join_posts",
					LocalField:   "id",
					ForeignField: "author_id",
					JoinType:     JoinTypeInner,
					ResultField:  "posts",
				},
			},
		})
		if err != nil {
			t.Fatalf("Inner join query failed: %v", err)
		}

		// Inner join should only return users who have posts (Alice and Bob)
		if response.Count != 2 {
			t.Errorf("Expected 2 results for inner join, got %d", response.Count)
		}

		// Verify that all returned users have posts
		for _, user := range response.Data {
			if _, hasPosts := user.Fields["posts"]; !hasPosts {
				t.Error("Inner join result should have posts field")
			}
		}
	})

	// Test Left Join
	t.Run("LeftJoin", func(t *testing.T) {
		response, err := queryService.ExecutePaginatedQuery(QueryOptions{
			EntityType: "join_users",
			Joins: []JoinOptions{
				{
					EntityType:   "join_posts",
					LocalField:   "id",
					ForeignField: "author_id",
					JoinType:     JoinTypeLeft,
					ResultField:  "posts",
				},
			},
		})
		if err != nil {
			t.Fatalf("Left join query failed: %v", err)
		}

		// Left join should return all users (Alice, Bob, and Charlie)
		if response.Count != 3 {
			t.Errorf("Expected 3 results for left join, got %d", response.Count)
		}

		// Count users with and without posts
		usersWithPosts := 0
		usersWithoutPosts := 0

		for _, user := range response.Data {
			if posts, hasPosts := user.Fields["posts"]; hasPosts && posts != nil {
				usersWithPosts++
			} else {
				usersWithoutPosts++
			}
		}

		if usersWithPosts != 2 {
			t.Errorf("Expected 2 users with posts, got %d", usersWithPosts)
		}

		if usersWithoutPosts != 1 {
			t.Errorf("Expected 1 user without posts, got %d", usersWithoutPosts)
		}
	})
}

// TestJoinSelectStrategies tests different join select strategies
func TestJoinSelectStrategies(t *testing.T) {
	// Create in-memory database
	db := NewDataStoreEngine()
	defer db.Close()

	queryService := NewQueryService(db)

	// Define schemas
	authorSchema := common.EntityDefinition{
		Name:        "join_authors",
		IDGenerator: common.IDTypeAutoIncrement,
		Fields: []common.FieldDefinition{
			{Name: "name", Type: "string", Required: true, Indexed: true},
		},
	}

	bookSchema := common.EntityDefinition{
		Name:        "join_books",
		IDGenerator: common.IDTypeAutoIncrement,
		Fields: []common.FieldDefinition{
			{Name: "title", Type: "string", Required: true, Indexed: true},
			{Name: "author_id", Type: "string", Required: true, Indexed: true},
		},
	}

	// Register schemas
	if err := db.RegisterEntityType(authorSchema); err != nil {
		t.Fatalf("Failed to register author schema: %v", err)
	}

	if err := db.RegisterEntityType(bookSchema); err != nil {
		t.Fatalf("Failed to register book schema: %v", err)
	}

	// Insert author
	authorData := map[string]interface{}{"name": "John Author"}
	if err := db.Insert("join_authors", "", authorData); err != nil {
		t.Fatalf("Failed to insert author: %v", err)
	}

	// Insert multiple books by the same author
	books := []map[string]interface{}{
		{"title": "Book One", "author_id": "1"},
		{"title": "Book Two", "author_id": "1"},
		{"title": "Book Three", "author_id": "1"},
	}

	for _, bookData := range books {
		if err := db.Insert("join_books", "", bookData); err != nil {
			t.Fatalf("Failed to insert book: %v", err)
		}
	}

	// Test "first" select strategy
	t.Run("FirstSelectStrategy", func(t *testing.T) {
		response, err := queryService.ExecutePaginatedQuery(QueryOptions{
			EntityType: "join_authors",
			Joins: []JoinOptions{
				{
					EntityType:     "join_books",
					LocalField:     "id",
					ForeignField:   "author_id",
					JoinType:       JoinTypeLeft,
					ResultField:    "book",
					SelectStrategy: "first",
					Filters: []Filter{
						{Field: "title", Operator: "eq", Value: "Book One"},
					},
				},
			},
		})
		if err != nil {
			t.Fatalf("First select strategy query failed: %v", err)
		}

		if response.Count != 1 {
			t.Errorf("Expected 1 result, got %d", response.Count)
		}

		author := response.Data[0]
		book := author.Fields["book"]

		// Should be a single book (map), not an array
		if bookMap, ok := book.(map[string]interface{}); ok {
			if bookMap["title"] != "Book One" {
				t.Errorf("Expected first book title 'Book One', got %v", bookMap["title"])
			}
		} else {
			t.Errorf("Expected book to be a map, got %T", book)
		}
	})

	// Test "all" select strategy
	t.Run("AllSelectStrategy", func(t *testing.T) {
		response, err := queryService.ExecutePaginatedQuery(QueryOptions{
			EntityType: "join_authors",
			Joins: []JoinOptions{
				{
					EntityType:     "join_books",
					LocalField:     "id",
					ForeignField:   "author_id",
					JoinType:       JoinTypeLeft,
					ResultField:    "books",
					SelectStrategy: "all",
				},
			},
		})
		if err != nil {
			t.Fatalf("All select strategy query failed: %v", err)
		}

		if response.Count != 1 {
			t.Errorf("Expected 1 result, got %d", response.Count)
		}

		author := response.Data[0]
		books := author.Fields["books"]

		// Should be an array of books
		if bookArray, ok := books.([]map[string]interface{}); ok {
			if len(bookArray) != 3 {
				t.Errorf("Expected 3 books, got %d", len(bookArray))
			}

			// Check that all books are present
			titles := make(map[string]bool)
			for _, book := range bookArray {
				if title, ok := book["title"].(string); ok {
					titles[title] = true
				}
			}

			expectedTitles := []string{"Book One", "Book Two", "Book Three"}
			for _, expectedTitle := range expectedTitles {
				if !titles[expectedTitle] {
					t.Errorf("Expected to find book with title '%s'", expectedTitle)
				}
			}
		} else {
			t.Errorf("Expected books to be an array, got %T", books)
		}
	})
}

// TestFuzzySearch tests fuzzy search functionality
func TestFuzzySearch(t *testing.T) {
	// Create in-memory database
	db := NewDataStoreEngine()
	defer db.Close()

	queryService := NewQueryService(db)

	// Define schema
	schema := common.EntityDefinition{
		Name:        "fuzzy_test_entities",
		IDGenerator: common.IDTypeAutoIncrement,
		Fields: []common.FieldDefinition{
			{Name: "name", Type: "string", Required: true, Indexed: true},
			{Name: "description", Type: "string", Required: true},
		},
	}

	if err := db.RegisterEntityType(schema); err != nil {
		t.Fatalf("Failed to register schema: %v", err)
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
		if err := db.Insert("fuzzy_test_entities", "", data); err != nil {
			t.Fatalf("Failed to insert test data: %v", err)
		}
	}

	// Test fuzzy search with default options
	t.Run("FuzzySearchDefault", func(t *testing.T) {
		response, err := queryService.ExecutePaginatedQuery(QueryOptions{
			EntityType: "fuzzy_test_entities",
			Filters: []Filter{
				{Field: "name", Operator: FilterFuzzy, Value: "Syncopate"},
			},
		})
		if err != nil {
			t.Fatalf("Fuzzy search query failed: %v", err)
		}

		// Should find SyncopateDB, Syncopate, and possibly Syncope
		if response.Count < 2 {
			t.Errorf("Expected at least 2 fuzzy matches, got %d", response.Count)
		}

		// Verify that exact match is included
		foundExactMatch := false
		for _, entity := range response.Data {
			if entity.Fields["name"] == "Syncopate" {
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
		response, err := queryService.ExecutePaginatedQuery(QueryOptions{
			EntityType: "fuzzy_test_entities",
			Filters: []Filter{
				{Field: "name", Operator: FilterFuzzy, Value: "Syncopate"},
			},
			FuzzyOpts: &FuzzySearchOptions{
				Threshold:   0.8, // Higher threshold for stricter matching
				MaxDistance: 2,   // Maximum edit distance
			},
		})
		if err != nil {
			t.Fatalf("Fuzzy search with custom options failed: %v", err)
		}

		// With stricter options, should find fewer matches
		if response.Count == 0 {
			t.Error("Expected at least 1 match with custom fuzzy options")
		}

		// Should still include exact match
		foundExactMatch := false
		for _, entity := range response.Data {
			if entity.Fields["name"] == "Syncopate" {
				foundExactMatch = true
				break
			}
		}

		if !foundExactMatch {
			t.Error("Expected to find exact match 'Syncopate' in fuzzy search results")
		}
	})
}
