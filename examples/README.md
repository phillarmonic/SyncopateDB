# SyncopateDB Examples

This directory contains examples demonstrating how to use SyncopateDB as an embedded library in your Go applications.

## Examples Overview

### 1. Basic In-Memory Database (`basic_in_memory/`)

Demonstrates the simplest usage of SyncopateDB:
- Creating an in-memory database
- Defining entity schemas
- Inserting and retrieving data
- Using auto-increment IDs

**Run:**
```bash
cd basic_in_memory
go run main.go
```

### 2. Persistent Database (`persistent_database/`)

Shows how to use SyncopateDB with persistence:
- Configuring persistence with Badger storage
- Setting up automatic snapshots and garbage collection
- Using different ID generation strategies (auto-increment and UUID)
- Getting database statistics

**Run:**
```bash
cd persistent_database
go run main.go
```

### 3. Advanced Querying (`advanced_querying/`)

Demonstrates complex query capabilities:
- Filtering with multiple conditions
- Sorting and pagination
- Array operations
- Joins between entity types
- Count queries

**Run:**
```bash
cd advanced_querying
go run main.go
```

### 4. Complete Application (`complete_application/`)

A comprehensive example showing:
- Full application setup with persistence
- Schema definition and data insertion
- Complex queries with joins
- Database statistics and monitoring
- Backup functionality

**Run:**
```bash
cd complete_application
go run main.go
```

## Key Features Demonstrated

### Entity Definition
```go
userSchema := common.EntityDefinition{
    Name:        "users",
    IDGenerator: common.IDTypeAutoIncrement,
    Fields: []common.FieldDefinition{
        {Name: "name", Type: "string", Required: true, Indexed: true},
        {Name: "email", Type: "string", Required: true, Unique: true},
        {Name: "age", Type: "integer", Nullable: true},
    },
}
```

### Persistence Configuration
```go
persistenceConfig := persistence.Config{
    Path:             "./data",
    CacheSize:        5000,
    SyncWrites:       true,
    SnapshotInterval: 5 * time.Minute,
    Logger:           logger,
    UseCompression:   true,
    EnableAutoGC:     true,
    GCInterval:       2 * time.Minute,
}
```

### Advanced Queries
```go
queryOptions := datastore.QueryOptions{
    EntityType: "posts",
    Filters: []datastore.Filter{
        {Field: "published", Operator: datastore.FilterEq, Value: true},
        {Field: "age", Operator: datastore.FilterGte, Value: 18},
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
    OrderBy:   "age",
    OrderDesc: false,
    Limit:     10,
    Offset:    0,
}
```

## Supported Data Types

- `string`: Text data
- `integer`: Whole numbers
- `float`: Decimal numbers
- `boolean`: True/false values
- `datetime`: Time values (stored as time.Time)
- `array`: Arrays of values
- `object`: JSON objects (map[string]interface{})

## ID Generation Strategies

- `auto_increment`: Sequential numeric IDs (1, 2, 3, ...)
- `uuid`: UUID v4 strings
- `cuid`: Collision-resistant unique identifiers
- `custom`: Client-provided IDs

## Filter Operations

- `eq`, `neq`: Equality and inequality
- `gt`, `gte`, `lt`, `lte`: Numeric comparisons
- `contains`, `startswith`, `endswith`: String operations
- `in`: Check if value is in array
- `array_contains`: Check if array contains value
- `fuzzy`: Fuzzy string matching

## Join Types

- `inner`: Only return records with matches in both tables
- `left`: Return all records from left table, with matches from right table

## Prerequisites

Make sure you have Go 1.23+ installed and the SyncopateDB module available in your Go workspace.

## Running the Examples

Each example is self-contained and can be run independently. Navigate to the specific example directory and run:

```bash
go run main.go
```

Some examples create data directories or files. You can clean these up after running the examples if desired.
