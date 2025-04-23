#!/usr/bin/env bash
set -e

# SyncopateDB Demo Script
# This script demonstrates creating an entity type, inserting entities, and querying
# Usage: ./syncopate_demo.sh [host] [port]

# Default values
HOST=${1:-localhost}
PORT=${2:-8080}
BASE_URL="http://${HOST}:${PORT}/api/v1"

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Helper function for HTTP requests
function make_request() {
  local method=$1
  local endpoint=$2
  local data=$3
  local curl_cmd="curl -s -w \"\n%{http_code}\" -X ${method} ${BASE_URL}${endpoint}"

  if [ -n "$data" ]; then
    curl_cmd="${curl_cmd} -H 'Content-Type: application/json' -d '${data}'"
  fi

  echo "🔄 ${method} ${endpoint}" >&2
  if [ -n "$data" ]; then
    echo "📤 ${data}" | grep -v password >&2
  fi

  # Execute the command and capture output with status code
  local response=$(eval ${curl_cmd})

  # Split response and status code
  local status_code=$(echo "$response" | tail -n1)
  local body=$(echo "$response" | sed '$d')

  # Print response in blue
  echo -e "📥 ${BLUE}${body}${NC}" >&2

  # Check if request was successful
  if [[ $status_code -ge 400 ]]; then
    echo -e "${RED}Error: Request failed with status code ${status_code}${NC}" >&2
    echo -e "${RED}Request: ${method} ${endpoint}${NC}" >&2
    echo -e "${RED}Response: ${body}${NC}" >&2
    echo -e "${RED}Script aborted due to API error.${NC}" >&2
    exit 1
  fi

  echo "${body}"
}
# Check if server is running
echo -e "${YELLOW}Checking if SyncopateDB server is running...${NC}"
SERVER_STATUS=$(curl -s -o /dev/null -w "%{http_code}" "http://${HOST}:${PORT}/health")

if [ "$SERVER_STATUS" != "200" ]; then
  echo -e "${RED}Error: SyncopateDB server is not running at ${HOST}:${PORT}${NC}"
  echo "Please start the server and try again."
  exit 1
fi

echo -e "${GREEN}✓ Server is running!${NC}"
echo

# Step 1: Create a Product entity type with nullable fields
echo -e "${YELLOW}Step 1: Creating a Product entity type with nullable fields${NC}"
ENTITY_TYPE_DEFINITION='{
  "name": "Product",
  "fields": [
    {
      "name": "name",
      "type": "string",
      "indexed": true,
      "required": true
    },
    {
      "name": "price",
      "type": "float",
      "indexed": true,
      "required": true,
      "nullable": false
    },
    {
      "name": "category",
      "type": "string",
      "indexed": true,
      "required": false,
      "nullable": true
    },
    {
      "name": "inStock",
      "type": "boolean",
      "indexed": true,
      "required": false,
      "nullable": true
    },
    {
      "name": "description",
      "type": "text",
      "indexed": false,
      "required": false,
      "nullable": true
    },
    {
      "name": "createdAt",
      "type": "datetime",
      "indexed": true,
      "required": false,
      "nullable": true
    },
    {
      "name": "tags",
      "type": "json",
      "indexed": false,
      "required": false,
      "nullable": true
    }
  ]
}'
CREATE_ENTITY_RESPONSE=$(make_request "POST" "/entity-types" "$ENTITY_TYPE_DEFINITION")
echo

# Step 2: Insert some product entities
echo -e "${YELLOW}Step 2: Inserting product entities${NC}"

# Product 1
PRODUCT1='{
  "fields": {
    "name": "Laptop",
    "price": 999.99,
    "category": "Electronics",
    "inStock": true,
    "description": "A powerful laptop for developers",
    "createdAt": "2025-04-20T10:00:00Z"
  }
}'

PRODUCT1_RESPONSE=$(make_request "POST" "/entities/Product" "$PRODUCT1")
PRODUCT1_ID=$(echo $PRODUCT1_RESPONSE | grep -o '"id":"[^"]*"' | cut -d'"' -f4)
echo "Created Product with ID: $PRODUCT1_ID"
echo

# Product 2
PRODUCT2='{
  "fields": {
    "name": "Coffee Maker",
    "price": 49.99,
    "category": "Kitchen",
    "inStock": true,
    "description": "Makes delicious coffee every morning",
    "createdAt": "2025-04-21T11:30:00Z"
  }
}'

PRODUCT2_RESPONSE=$(make_request "POST" "/entities/Product" "$PRODUCT2")
PRODUCT2_ID=$(echo $PRODUCT2_RESPONSE | grep -o '"id":"[^"]*"' | cut -d'"' -f4)
echo "Created Product with ID: $PRODUCT2_ID"
echo

# Product 3
PRODUCT3='{
  "fields": {
    "name": "Headphones",
    "price": 79.99,
    "category": "Electronics",
    "inStock": false,
    "description": "Noise-cancelling headphones for immersive sound",
    "createdAt": "2025-04-22T09:15:00Z"
  }
}'

PRODUCT3_RESPONSE=$(make_request "POST" "/entities/Product" "$PRODUCT3")
PRODUCT3_ID=$(echo $PRODUCT3_RESPONSE | grep -o '"id":"[^"]*"' | cut -d'"' -f4)
echo "Created Product with ID: $PRODUCT3_ID"
echo

# Step 3: Query for all products
echo -e "${YELLOW}Step 3: Querying all products${NC}"
ALL_PRODUCTS=$(make_request "GET" "/entities/Product" "")
echo

# Step 4: Query with filtering
echo -e "${YELLOW}Step 4: Querying electronics products${NC}"
QUERY='{
  "entityType": "Product",
  "filters": [
    {
      "field": "category",
      "operator": "eq",
      "value": "Electronics"
    }
  ],
  "orderBy": "price",
  "orderDesc": true
}'

FILTERED_PRODUCTS=$(make_request "POST" "/query" "$QUERY")
echo

# Step 5: Get a specific product
echo -e "${YELLOW}Step 5: Getting a specific product${NC}"
SPECIFIC_PRODUCT=$(make_request "GET" "/entities/Product/${PRODUCT1_ID}" "")
echo

# Step 6: Update a product
echo -e "${YELLOW}Step 6: Updating a product${NC}"
UPDATE='{
  "fields": {
    "price": 899.99,
    "description": "A powerful laptop for developers - Now on sale!"
  }
}'

UPDATE_RESPONSE=$(make_request "PUT" "/entities/Product/${PRODUCT1_ID}" "$UPDATE")
echo

# Verify the update
echo -e "${YELLOW}Verifying the update:${NC}"
UPDATED_PRODUCT=$(make_request "GET" "/entities/Product/${PRODUCT1_ID}" "")
echo

# Step 7: Get entity type definition
echo -e "${YELLOW}Step 7: Getting entity type definition${NC}"
ENTITY_TYPE_INFO=$(make_request "GET" "/entity-types/Product" "")
echo

# Step 8: Create a product with null fields
echo -e "${YELLOW}Step 8: Creating a product with null fields${NC}"
PRODUCT_WITH_NULLS='{
  "fields": {
    "name": "Wireless Mouse",
    "price": 29.99,
    "category": "Electronics",
    "inStock": true,
    "description": null,
    "createdAt": null,
    "tags": null
  }
}'

PRODUCT_NULL_RESPONSE=$(make_request "POST" "/entities/Product" "$PRODUCT_WITH_NULLS")
PRODUCT_NULL_ID=$(echo $PRODUCT_NULL_RESPONSE | grep -o '"id":"[^"]*"' | cut -d'"' -f4)
echo "Created Product with ID: $PRODUCT_NULL_ID"
echo

# Display the product with null fields
echo -e "${YELLOW}Verifying the product with null fields:${NC}"
NULL_FIELDS_PRODUCT=$(make_request "GET" "/entities/Product/${PRODUCT_NULL_ID}" "")
echo

# Step 9: Update a field to null
echo -e "${YELLOW}Step 9: Updating a product to have a null field${NC}"
NULL_UPDATE='{
  "fields": {
    "category": null,
    "description": "Updated with a null category"
  }
}'

NULL_UPDATE_RESPONSE=$(make_request "PUT" "/entities/Product/${PRODUCT1_ID}" "$NULL_UPDATE")
echo

# Verify the null update
echo -e "${YELLOW}Verifying the update with null field:${NC}"
NULL_UPDATED_PRODUCT=$(make_request "GET" "/entities/Product/${PRODUCT1_ID}" "")
echo

# Step 10: Query for products with null fields
echo -e "${YELLOW}Step 10: Querying products with null category${NC}"
NULL_QUERY='{
  "entityType": "Product",
  "filters": [
    {
      "field": "category",
      "operator": "eq",
      "value": null
    }
  ]
}'

NULL_FILTERED_PRODUCTS=$(make_request "POST" "/query" "$NULL_QUERY")
echo

# Summary
echo -e "${GREEN}Demo completed successfully!${NC}"
echo -e "The script:"
echo -e "1. Created a Product entity type with nullable fields"
echo -e "2. Inserted 3 product entities with different properties"
echo -e "3. Queried all products"
echo -e "4. Queried products filtered by category"
echo -e "5. Retrieved a specific product by ID"
echo -e "6. Updated a product's price and description"
echo -e "7. Retrieved the Product entity type definition"
echo -e "8. Created a product with explicit null fields"
echo -e "9. Updated a product to have a null category field"
echo -e "10. Queried products with null category fields"
echo
echo -e "You can now interact with these entities using the SyncopateDB API."
echo -e "Access your data at: ${BLUE}http://${HOST}:${PORT}/api/v1/entities/Product${NC}"