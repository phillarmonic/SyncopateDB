# SyncopateDB

<p align="center">
  <img src="https://github.com/user-attachments/assets/11dd16bf-f625-44cf-aa17-06d027544ce5" alt="syncopate" width="300">
</p>

SyncopateDB is a flexible, lightweight, optimized for SSD database with advanced query capabilities and low latency. It provides a REST API for data storage and retrieval with robust features including indexing, complex queries, and persistence.



## Key Features

SyncopateDB offers:

- Schema definition with strong typing and field validation
- Multiple ID generation strategies (auto-increment, UUID, CUID)
- Advanced querying with filtering, sorting, pagination, and fuzzy search
- Support for joins to link related data between entity types
- Data persistence with Write-Ahead Logging (WAL) and snapshots
- Schema evolution with compatibility checks
- Memory efficiency through strategic caching and optimizations

## Architecture

The system is built in Go (requires 1.23+) and uses [Badger](https://github.com/hypermodeinc/badger) as its storage backend (which is battle tested for hundreds of terabytes of data), and provides excellent performance on SSDs. The database exposes a RESTful API for all operations.

The core components include:

- A datastore engine that manages entity definitions and data
- A query service that handles filtering, sorting, and joins
- A persistence layer that ensures durability through WAL and snapshots
- A memory monitoring system to track resource usage

## Performance Characteristics

SyncopateDB is designed with the following in mind:

- Efficient indexing for fast queries
- Memory-efficient operations
- SSD optimization
- Data compression support (optional)
- Intelligent caching

## Use Cases

SyncopateDB  is the right choice for you if you require:

- Applications needing structured data with a schema
- Systems requiring flexible querying capabilities
- Projects where a full-featured SQL database might be overkill (or too slow)
- Applications where a REST API for data access is preferred
- Scenarios where relational-like queries (via joins) are needed but without the overhead of a traditional RDBMS

## ## Table of Contents

- [Key Features](#key-features)
- [Architecture](#architecture)
- [Performance Characteristics](#performance-characteristics)
- [Use Cases](#use-cases)
- [Features](#features)
- [Installation](#installation)
- [Getting Started](#getting-started)
- [API Reference](#api-reference)
  - [Entity Types](#entity-types)
  - [Entities](#entities)
  - [Querying](#querying)
  - [Joins](#joins)
- [Examples](#examples)
  - [Creating Entity Types](#creating-entity-types)
  - [Updating Entity Types](#updating-entity-types)
  - [Listing Entity Types](#listing-entity-types)
  - [Creating Entities](#creating-entities)
  - [Retrieving Entities](#retrieving-entities)
  - [Updating Entities](#updating-entities)
  - [Deleting Entities](#deleting-entities)
  - [Advanced Querying](#advanced-querying)
  - [Using Joins](#using-joins)
- [ID Generation Strategies](#id-generation-strategies)
- [Configuration](#configuration)
- [The verbose debug mode](#the-verbose-debug-mode)
- [Persistence](#persistence)
- [Using SyncopateDB with Docker](#using-syncopatedb-with-docker)
  - [Quick Start](#quick-start)
  - [Configuration Options](#configuration-options)
  - [Data Persistence](#data-persistence)
  - [Docker Compose Example](#docker-compose-example)
  - [Security Considerations](#security-considerations)
  - [Accessing the API](#accessing-the-api)
  - [Container Maintenance](#container-maintenance)
- [Docker](#using-syncopatedb-with-docker)
- [Building from Source](#building-from-source)
- [License](#license)

## Features

- **Schema Definition**: Define entity types with field definitions
- **Indexing**: Create indexes for fast data retrieval
- **Multiple ID Strategies**: Support for auto-increment, UUID, CUID generation
- **Advanced Querying**: Filter, sort, and paginate data with a flexible query API
- **Joins and Relations**: Link related data between entity types (with soft relationships)
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
   - `--debug`: Enable **verbose debug mode** for easier debugging
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

### Joins

SyncopateDB supports joining related data between entity types. In SyncopateDB, joins are non-destructive views made in memory to represent data. This means the source entities are never modified by the join operation.
Also, consider relationships are soft defined. Thus, there are no entity relationships defined in the database and there are no features such as cascade deleting.

| Method | Endpoint           | Description                |
| ------ | ------------------ | -------------------------- |
| POST   | /api/v1/query/join | Execute a query with joins |

## Examples

### Creating Entity Types

Pro tip: If you want to use auto_increment, you can omit it from the payload and it'll be automatically selected.

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

### Using Joins

SyncopateDB supports powerful join capabilities for querying related data across entity types. This is particularly useful for modeling relationships like one-to-many and many-to-many.

#### One-to-Many Relationship Example

Let's set up a blog with users, posts, and comments:

First, create the required entity types:

```bash
# Create users entity type
curl -X POST http://localhost:8080/api/v1/entity-types \
  -H "Content-Type: application/json" \
  -d '{
    "name": "users",
    "fields": [
      {"name": "name", "type": "string", "required": true},
      {"name": "email", "type": "string", "required": true, "indexed": true},
      {"name": "active", "type": "boolean", "indexed": true}
    ],
    "idGenerator": "auto_increment"
  }'

# Create posts entity type
curl -X POST http://localhost:8080/api/v1/entity-types \
  -H "Content-Type: application/json" \
  -d '{
    "name": "posts",
    "fields": [
      {"name": "title", "type": "string", "required": true},
      {"name": "content", "type": "string", "required": true},
      {"name": "author_id", "type": "integer", "required": true, "indexed": true},
      {"name": "category_id", "type": "integer", "indexed": true},
      {"name": "published", "type": "boolean", "indexed": true}
    ],
    "idGenerator": "auto_increment"
  }'

# Create comments entity type
curl -X POST http://localhost:8080/api/v1/entity-types \
  -H "Content-Type: application/json" \
  -d '{
    "name": "comments",
    "fields": [
      {"name": "content", "type": "string", "required": true},
      {"name": "post_id", "type": "integer", "required": true, "indexed": true},
      {"name": "user_id", "type": "integer", "required": true, "indexed": true}
    ],
    "idGenerator": "auto_increment"
  }'
```

#### Join Posts with Authors

To fetch posts with their author information:

```bash
curl -X POST http://localhost:8080/api/v1/query/join \
  -H "Content-Type: application/json" \
  -d '{
    "entityType": "posts",
    "joins": [
      {
        "entityType": "users",
        "localField": "author_id",
        "foreignField": "id",
        "as": "author",
        "type": "inner",
        "selectStrategy": "first"
      }
    ]
  }'
```

This query will return all posts with the author information embedded in each post.

#### Join Posts with Comments

To fetch posts along with all their comments:

```bash
curl -X POST http://localhost:8080/api/v1/query/join \
  -H "Content-Type: application/json" \
  -d '{
    "entityType": "posts",
    "joins": [
      {
        "entityType": "comments",
        "localField": "id",
        "foreignField": "post_id",
        "as": "comments",
        "type": "left",
        "selectStrategy": "all"
      }
    ]
  }'
```

#### Multiple Joins

You can combine multiple joins in a single query:

```bash
curl -X POST http://localhost:8080/api/v1/query/join \
  -H "Content-Type: application/json" \
  -d '{
    "entityType": "posts",
    "joins": [
      {
        "entityType": "users",
        "localField": "author_id",
        "foreignField": "id",
        "as": "author",
        "type": "inner",
        "selectStrategy": "first"
      },
      {
        "entityType": "comments",
        "localField": "id",
        "foreignField": "post_id",
        "as": "comments",
        "type": "left",
        "selectStrategy": "all"
      }
    ],
    "filters": [
      {
        "field": "published",
        "operator": "eq",
        "value": true
      }
    ],
    "limit": 10,
    "offset": 0
  }'
```

This query will return published posts with both author information and all comments.

#### Join Parameters

- **entityType**: The entity type to join with
- **localField**: Field in the main entity to join on
- **foreignField**: Field in the joined entity to match against
- **as**: Name to give the joined data in the result
- **type**: Join type (options: "inner" or "left")
  - "inner": Only returns main entities that have matching joined entities
  - "left": Returns all main entities, with joined data where available
- **selectStrategy**: How to handle multiple matching entities (options: "first" or "all")
  - "first": Select only the first matching entity
  - "all": Select all matching entities as an array
- **filters**: Optional filters to apply to the joined entities
- **includeFields**: Fields to include from the joined entities (empty = all)
- **excludeFields**: Fields to exclude from the joined entities

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
- `DEBUG`: Enable **verbose  debug mode** (default: false)
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
- `--debug`: Enable the **verbose debug mode**
- `--color-logs`: Enable colorized logs

## The verbose debug mode

This mode might show extra information useful for debugging edge cases. When having support for this software, you might be asked of a run with the verbose mode enabled.

## Persistence

SyncopateDB uses a combination of Write-Ahead Logging (WAL) and periodic snapshots for data persistence. This provides durability while maintaining good performance.

The persistence is handled by the [Badger](https://github.com/dgraph-io/badger) key-value store, which provides excellent performance on SSDs.

Key persistence features:

- Write-Ahead Logging for durability
- Periodic snapshots for faster recovery
- Automatic garbage collection
- Compression support (optional)
- Automated backup capabilities



# Using SyncopateDB with Docker

SyncopateDB is available as an official Docker image, making deployment quick and easy across different environments. The image is optimized for performance and security, with multi-architecture support for both amd64 and arm64 platforms.

## Quick Start

To get SyncopateDB up and running with default settings, simply run:

bash

```bash
docker run -d --name syncopatedb -p 8080:8080 -v syncopate-data:/data phillarmonic/syncopatedb
```

This will:

- Start SyncopateDB in detached mode
- Name the container "syncopatedb"
- Map port 8080 from the container to port 8080 on your host
- Create a Docker volume named "syncopate-data" for persistent storage

## Configuration Options

You can configure SyncopateDB using environment variables:

bash

```bash
docker run -d --name syncopatedb \
  -p 8080:8080 \
  -v syncopate-data:/data \
  -e PORT=8080 \
  -e DEBUG=false \
  -e LOG_LEVEL=info \
  -e ENABLE_WAL=true \
  -e ENABLE_ZSTD=true \
  -e COLORIZED_LOGS=false \
  phillarmonic/syncopatedb
```

### Available Environment Variables

- `PORT`: Server port (default: 8080)
- `DEBUG`: Enable verbose debug mode (default: false)
- `LOG_LEVEL`: Logging level (debug, info, warn, error)
- `ENABLE_WAL`: Enable Write-Ahead Logging (default: true)
- `ENABLE_ZSTD`: Enable ZSTD compression (default: true)
- `COLORIZED_LOGS`: Enable colorized logging (default: false)

### Command-line Arguments

You can also pass command-line arguments to the container:

bash

```bash
docker run -d --name syncopatedb \
  -p 8080:8080 \
  -v syncopate-data:/data \
  phillarmonic/syncopatedb \
  --port 8080 \
  --log-level info \
  --cache-size 20000 \
  --snapshot-interval 300
```

## Data Persistence

SyncopateDB stores all data in the `/data` directory inside the container. For persistence, mount this directory to a volume or host path:

### Using a Named Volume (Recommended)

bash

```bash
docker run -d --name syncopatedb \
  -p 8080:8080 \
  -v syncopate-data:/data \
  phillarmonic/syncopatedb
```

### Using a Host Directory

bash

```bash
docker run -d --name syncopatedb \
  -p 8080:8080 \
  -v /path/on/host:/data \
  phillarmonic/syncopatedb
```

## Docker Compose Example

For production deployments, a Docker Compose configuration is recommended:

yaml

```yaml
services:
  syncopatedb:
    image: phillarmonic/syncopatedb:latest
    container_name: syncopatedb
    restart: unless-stopped
    ports:
      - "8080:8080"
    volumes:
      - syncopate-data:/data
    environment:
      - PORT=8080
      - LOG_LEVEL=info
      - ENABLE_WAL=true
      - ENABLE_ZSTD=true
      - COLORIZED_LOGS=false
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:8080/health"]
      interval: 30s
      timeout: 5s
      retries: 3

volumes:
  syncopate-data:
```

Save this to a file named `docker-compose.yml` and run:

bash

```bash
docker-compose up -d
```

## Security Considerations

The SyncopateDB Docker image:

- Runs as a non-root user (`syncopate`)
- Has a minimal footprint using Alpine Linux
- Contains only the necessary dependencies

## Accessing the API

Once the container is running, you can access the SyncopateDB API at:

```
http://localhost:8080/
```

Verify that SyncopateDB is running by checking the health endpoint:

bash

```bash
curl http://localhost:8080/health
```

You should receive a response like:

json

```json
{"status":"ok"}
```

## Container Maintenance

### Viewing Logs

bash

```bash
docker logs syncopatedb
```

### Stopping the Container

bash

```bash
docker stop syncopatedb
```

### Upgrading to a New Version

bash

```bash
docker pull phillarmonic/syncopatedb:latest
docker stop syncopatedb
docker rm syncopatedb
docker run -d --name syncopatedb -p 8080:8080 -v syncopate-data:/data phillarmonic/syncopatedb
```

### Backing Up Data

The database data is stored in the mounted volume. To create a backup:

bash

```bash
# Stop the container before backing up
docker stop syncopatedb

# For named volumes
docker run --rm -v syncopate-data:/data -v $(pwd):/backup alpine tar -czvf /backup/syncopatedb-backup.tar.gz /data

# For host directories
tar -czvf syncopatedb-backup.tar.gz /path/on/host

# Restart the container
docker start syncopatedb
```



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