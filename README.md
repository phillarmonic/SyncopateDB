# SyncopateDB

SyncopateDB is a flexible, lightweight data store with advanced query capabilities. It provides a REST API for data storage and retrieval with robust features including indexing, complex queries, and persistence.

## Table of Contents

- [Features](#features)
- [Installation](#installation)
- [Getting Started](#getting-started)
- [API Reference](#api-reference)
  - [Entity Types](#entity-types)
  - [Entities](#entities)
  - [Querying](#querying)
- [Examples](#examples)
  - [Creating Entity Types](#creating-entity-types)
  - [Listing Entity Types](#listing-entity-types)
  - [Creating Entities](#creating-entities)
  - [Retrieving Entities](#retrieving-entities)
  - [Updating Entities](#updating-entities)
  - [Deleting Entities](#deleting-entities)
  - [Advanced Querying](#advanced-querying)
- [Configuration](#configuration)
- [Persistence](#persistence)
- [Building from Source](#building-from-source)

## Features

- **Schema Definition**: Define entity types with field definitions
- **Indexing**: Create indexes for fast data retrieval
- **Advanced Querying**: Filter, sort, and paginate data with a flexible query API
- **Fuzzy Search**: Find data using fuzzy matching algorithms
- **Persistence**: Store data on disk with WAL (Write-Ahead Logging) and snapshots
- **RESTful API**: Easy-to-use HTTP API for all operations
- **Go Library**: Use SyncopateDB as an embedded database in your Go applications

## Installation

### Using pre-built binaries

Download the latest release from [GitHub Releases](https://github.com/phillarmonic/syncopate-db/releases).

### Building from source

```bash
git clone https://github.com/phillarmonic/syncopate-db.git
cd syncopate-db
go build ./cmd/server
```

## Getting Started

1. Start the server:

```bash
./server --port 8080 --data-dir ./data
```

2. The server accepts the following command-line flags:
   - `--port`: Port to listen on (default: 8080)
   - `--log-level`: Log level (debug, info, warn, error)
   - `--data-dir`: Directory for data storage (default: ./data)
   - `--memory-only`: Run in memory-only mode without persistence
   - `--cache-size`: Number of entities to cache in memory (default: 10000)
   - `--snapshot-interval`: Snapshot interval in seconds (default: 600)
   - `--sync-writes`: Sync writes to disk immediately (default: true)

3. Visit `http://localhost:8080/` to see the welcome message and verify the server is running.

## API Reference

### Entity Types

Entity types define the structure of your data.

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | /api/v1/entity-types | List all entity types |
| POST | /api/v1/entity-types | Create a new entity type |
| GET | /api/v1/entity-types/{name} | Get a specific entity type |

### Entities

Entities are instances of entity types containing actual data.

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | /api/v1/entities/{type} | List entities of a specific type |
| POST | /api/v1/entities/{type} | Create a new entity |
| GET | /api/v1/entities/{type}/{id} | Get a specific entity |
| PUT | /api/v1/entities/{type}/{id} | Update a specific entity |
| DELETE | /api/v1/entities/{type}/{id} | Delete a specific entity |

### Querying

SyncopateDB supports advanced querying with filtering, sorting, and pagination.

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | /api/v1/query | Execute a complex query |

## Examples

### Creating Entity Types

Create a "Product" entity type:

```bash
curl -X POST http://localhost:8080/api/v1/entity-types \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Product",
    "fields": [
      {
        "name": "name",
        "type": "string",
        "indexed": true,
        "required": true
      },
      {
        "name": "description",
        "type": "text",
        "indexed": false,
        "required": false
      },
      {
        "name": "price",
        "type": "float",
        "indexed": true,
        "required": true
      },
      {
        "name": "inStock",
        "type": "boolean",
        "indexed": true,
        "required": true
      },
      {
        "name": "tags",
        "type": "json",
        "indexed": false,
        "required": false
      },
      {
        "name": "createdAt",
        "type": "datetime",
        "indexed": true,
        "required": true
      }
    ]
  }'
```

Create a "Customer" entity type:

```bash
curl -X POST http://localhost:8080/api/v1/entity-types \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Customer",
    "fields": [
      {
        "name": "firstName",
        "type": "string",
        "indexed": true,
        "required": true
      },
      {
        "name": "lastName",
        "type": "string",
        "indexed": true,
        "required": true
      },
      {
        "name": "email",
        "type": "string",
        "indexed": true,
        "required": true
      },
      {
        "name": "address",
        "type": "json",
        "indexed": false,
        "required": false
      },
      {
        "name": "joinedAt",
        "type": "datetime",
        "indexed": true,
        "required": true
      }
    ]
  }'
```

### Listing Entity Types

List all entity types:

```bash
curl -X GET http://localhost:8080/api/v1/entity-types
```

Get details of a specific entity type:

```bash
curl -X GET http://localhost:8080/api/v1/entity-types/Product
```

### Creating Entities

Create a new product:

```bash
curl -X POST http://localhost:8080/api/v1/entities/Product \
  -H "Content-Type: application/json" \
  -d '{
    "id": "prod-001",
    "fields": {
      "name": "Ergonomic Keyboard",
      "description": "A comfortable keyboard for long typing sessions",
      "price": 59.99,
      "inStock": true,
      "tags": ["electronics", "office", "ergonomic"],
      "createdAt": "2025-03-28T10:30:00Z"
    }
  }'
```

Create a new customer:

```bash
curl -X POST http://localhost:8080/api/v1/entities/Customer \
  -H "Content-Type: application/json" \
  -d '{
    "id": "cust-001",
    "fields": {
      "firstName": "Jane",
      "lastName": "Smith",
      "email": "jane.smith@example.com",
      "address": {
        "street": "123 Main St",
        "city": "Springfield",
        "state": "IL",
        "zip": "62704"
      },
      "joinedAt": "2025-03-15T09:45:00Z"
    }
  }'
```

### Retrieving Entities

List all products with pagination:

```bash
curl -X GET "http://localhost:8080/api/v1/entities/Product?limit=10&offset=0"
```

Get a specific product by ID:

```bash
curl -X GET http://localhost:8080/api/v1/entities/Product/prod-001
```

### Updating Entities

Update a product:

```bash
curl -X PUT http://localhost:8080/api/v1/entities/Product/prod-001 \
  -H "Content-Type: application/json" \
  -d '{
    "fields": {
      "price": 49.99,
      "inStock": false
    }
  }'
```

### Deleting Entities

Delete a product:

```bash
curl -X DELETE http://localhost:8080/api/v1/entities/Product/prod-001
```

### Advanced Querying

Filter products by price range and sort by name:

```bash
curl -X POST http://localhost:8080/api/v1/query \
  -H "Content-Type: application/json" \
  -d '{
    "entityType": "Product",
    "filters": [
      {
        "field": "price",
        "operator": "gte",
        "value": 20.0
      },
      {
        "field": "price",
        "operator": "lte",
        "value": 100.0
      },
      {
        "field": "inStock",
        "operator": "eq",
        "value": true
      }
    ],
    "orderBy": "name",
    "orderDesc": false,
    "limit": 10,
    "offset": 0
  }'
```

Find customers with fuzzy name matching:

```bash
curl -X POST http://localhost:8080/api/v1/query \
  -H "Content-Type: application/json" \
  -d '{
    "entityType": "Customer",
    "filters": [
      {
        "field": "lastName",
        "operator": "fuzzy",
        "value": "Smth"
      }
    ],
    "fuzzyOpts": {
      "threshold": 0.7,
      "maxDistance": 2
    },
    "limit": 10,
    "offset": 0
  }'
```

Filter products by tags (using JSON field):

```bash
curl -X POST http://localhost:8080/api/v1/query \
  -H "Content-Type: application/json" \
  -d '{
    "entityType": "Product",
    "filters": [
      {
        "field": "tags",
        "operator": "contains",
        "value": "ergonomic"
      }
    ],
    "limit": 10,
    "offset": 0
  }'
```

## Configuration

SyncopateDB can be configured through command-line flags when starting the server:

```bash
./server --port 8080 --log-level=info --data-dir=./data --cache-size=20000 --snapshot-interval=300 --sync-writes=true
```

## Persistence

SyncopateDB uses a combination of Write-Ahead Logging (WAL) and periodic snapshots for data persistence. This provides durability while maintaining good performance.

The persistence is handled by the [Badger](https://github.com/dgraph-io/badger) key-value store, which provides excellent performance and reliability.

For development or testing where persistence isn't needed, you can use the `--memory-only` flag:

```bash
./server --memory-only
```

## Building from Source

Prerequisites:
- Go 1.24 or higher

Steps:
1. Clone the repository:
   ```bash
   git clone https://github.com/phillarmonic/syncopate-db.git
   ```

2. Navigate to the project directory:
   ```bash
   cd syncopate-db
   ```

3. Build the server:
   ```bash
   go build -o server ./cmd/server
   ```

4. Run the server:
   ```bash
   ./server
   ```

## License

[MIT License](LICENSE)

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.