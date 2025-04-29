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
  - [Updating Entity Types](#updating-entity-types)
  - [Listing Entity Types](#listing-entity-types)
  - [Creating Entities](#creating-entities)
  - [Retrieving Entities](#retrieving-entities)
  - [Updating Entities](#updating-entities)
  - [Deleting Entities](#deleting-entities)
  - [Advanced Querying](#advanced-querying)
- [ID Generation Strategies](#id-generation-strategies)
- [Configuration](#configuration)
- [Persistence](#persistence)
- [Building from Source](#building-from-source)

## Features

- **Schema Definition**: Define entity types with field definitions
- **Indexing**: Create indexes for fast data retrieval
- **Multiple ID Strategies**: Support for auto-increment, UUID, CUID generation
- **Advanced Querying**: Filter, sort, and paginate data with a flexible query API
- **Fuzzy Search**: Find data using fuzzy matching algorithms
- **Array Operations**: Query for values within arrays
- **Transaction Support**: Group operations for atomic changes
- **Persistence**: Store data on disk with WAL (Write-Ahead Logging) and snapshots
- **Schema Evolution**: Update entity type definitions with compatibility checks
- **RESTful API**: Easy-to-use HTTP API for all operations
- **Go Library**: Use SyncopateDB as an embedded database in your Go applications

## Installation

### Using pre-built binaries

Download the latest release from the GitHub Releases page.

### Building from source

```bash
git clone https://github.com/phillarmonic/syncopate-db.git
cd syncopate-db
go build ./cmd/main.go
```

## Getting Started

1. Start the server:

```bash
./main --port 8080 --data-dir ./data
```

2. The server accepts the following command-line flags:
   
   - `--port`: Port to listen on (default: 8080)
   - `--log-level`: Log level (debug, info, warn, error)
   - `--data-dir`: Directory for data storage (default: ./data)
   - `--cache-size`: Number of entities to cache in memory (default: 10000)
   - `--snapshot-interval`: Snapshot interval in seconds (default: 600)
   - `--sync-writes`: Sync writes to disk immediately (default: true)
   - `--debug`: Enable debug mode for easier debugging
   - `--color-logs`: Enable colorized log output

3. Visit `http://localhost:8080/` to see the welcome message and verify the server is running.

## API Reference

### Entity Types

Entity types define the structure of your data.

| Method | Endpoint                    | Description                   |
| ------ | --------------------------- | ----------------------------- |
| GET    | /api/v1/entity-types        | List all entity types         |
| POST   | /api/v1/entity-types        | Create a new entity type      |
| GET    | /api/v1/entity-types/{name} | Get a specific entity type    |
| PUT    | /api/v1/entity-types/{name} | Update a specific entity type |

### Entities

Entities are instances of entity types containing actual data.

| Method | Endpoint                     | Description                      |
| ------ | ---------------------------- | -------------------------------- |
| GET    | /api/v1/entities/{type}      | List entities of a specific type |
| POST   | /api/v1/entities/{type}      | Create a new entity              |
| GET    | /api/v1/entities/{type}/{id} | Get a specific entity            |
| PUT    | /api/v1/entities/{type}/{id} | Update a specific entity         |
| DELETE | /api/v1/entities/{type}/{id} | Delete a specific entity         |

### Querying

SyncopateDB supports advanced querying with filtering, sorting, and pagination.

| Method | Endpoint      | Description             |
| ------ | ------------- | ----------------------- |
| POST   | /api/v1/query | Execute a complex query |

## Examples

### Creating Entity Types

Create a "Product" entity type with auto-increment IDs:

```bash
curl -X POST http://localhost:8080/api/v1/entity-types \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Product",
    "idGenerator": "auto_increment",
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

Create a "Customer" entity type with UUID IDs:

```bash
curl -X POST http://localhost:8080/api/v1/entity-types \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Customer",
    "idGenerator": "uuid",
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

### Updating Entity Types

Update the "Product" entity type to add a new field:

```bash
curl -X PUT http://localhost:8080/api/v1/entity-types/Product \
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
      },
      {
        "name": "category",
        "type": "string",
        "indexed": true,
        "required": false
      }
    ]
  }'
```

Note: You cannot change the ID generator type after creation, and cannot make existing fields required if they weren't before.

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

Create a new product with auto-generated ID:

```bash
curl -X POST http://localhost:8080/api/v1/entities/Product \
  -H "Content-Type: application/json" \
  -d '{
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

Create a new customer with UUID generation:

### Retrieving Entities

List all products with pagination:

```bash
curl -X GET "http://localhost:8080/api/v1/entities/Product?limit=10&offset=0"
```

Get a specific product by ID:

```bash
curl -X GET http://localhost:8080/api/v1/entities/Product/1
```

List products with sorting:

```bash
curl -X GET "http://localhost:8080/api/v1/entities/Product?limit=10&offset=0&orderBy=price&orderDesc=true"
```

### Updating Entities

Update a product:

```bash
curl -X PUT http://localhost:8080/api/v1/entities/Product/1 \
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
curl -X DELETE http://localhost:8080/api/v1/entities/Product/1
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

Filter products by tags (array contains):

```bash
curl -X POST http://localhost:8080/api/v1/query \
  -H "Content-Type: application/json" \
  -d '{
    "entityType": "Product",
    "filters": [
      {
        "field": "tags",
        "operator": "array_contains",
        "value": "ergonomic"
      }
    ],
    "limit": 10,
    "offset": 0
  }'
```

Filter products containing all specified tags:

```bash
curl -X POST http://localhost:8080/api/v1/query \
  -H "Content-Type: application/json" \
  -d '{
    "entityType": "Product",
    "filters": [
      {
        "field": "tags",
        "operator": "array_contains_all",
        "value": ["electronics", "ergonomic"]
      }
    ],
    "limit": 10,
    "offset": 0
  }'
```

## ID Generation Strategies

SyncopateDB supports several ID generation strategies:

1. **auto_increment**: Sequential numeric IDs (default)
2. **uuid**: Universally Unique Identifiers (UUID v4)
3. **cuid**: Collision-resistant IDs optimized for horizontal scaling
4. **custom**: Client-provided IDs

Specify the ID generation strategy when creating an entity type:

```json
{
  "name": "Product",
  "idGenerator": "uuid",
  "fields": [...]
}
```

## Configuration

SyncopateDB can be configured through environment variables and command-line flags:

### Environment Variables

- `PORT`: Server port (default: 8080)
- `DEBUG`: Enable debug mode (default: false)
- `LOG_LEVEL`: Logging level (debug, info, warn, error)
- `ENABLE_WAL`: Enable Write-Ahead Logging (default: true)
- `ENABLE_ZSTD`: Enable ZSTD compression (default: false)
- `COLORIZED_LOGS`: Enable colorized logging (default: true)

### Command Line Flags

- `--port`: Server port
- `--log-level`: Logging level
- `--data-dir`: Directory for data storage
- `--cache-size`: Number of entities to cache in memory
- `--snapshot-interval`: Snapshot interval in seconds
- `--sync-writes`: Sync writes to disk immediately
- `--debug`: Enable debug mode
- `--color-logs`: Enable colorized logs

## Persistence

SyncopateDB uses a combination of Write-Ahead Logging (WAL) and periodic snapshots for data persistence. This provides durability while maintaining good performance.

The persistence is handled by the [Badger](https://github.com/dgraph-io/badger) key-value store, which provides excellent performance on SSDs.

Key persistence features:

- Write-Ahead Logging for durability
- Periodic snapshots for faster recovery
- Automatic garbage collection
- Compression support (optional)
- Automated backup capabilities

## Building from Source

Prerequisites:

- Go 1.23 or higher

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
   go build -o syncopatedb ./cmd/main.go
   ```

4. Run the server:
   
   ```bash
   ./syncopatedb
   ```

## License

MIT License