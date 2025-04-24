#!/usr/bin/env bash
set -e

# SyncopateDB Complete Demo Script
# This script creates Product and Order entity types and populates them with data
# Usage: ./syncopate_complete_demo.sh [host] [port]

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

###########################################
# PART 1: PRODUCT ENTITY TYPE AND DATA
###########################################

# Step 1: Create a Product entity type
echo -e "${YELLOW}Step 1: Creating a Product entity type${NC}"
PRODUCT_TYPE_DEFINITION='{
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

CREATE_PRODUCT_TYPE_RESPONSE=$(make_request "POST" "/entity-types" "$PRODUCT_TYPE_DEFINITION")
echo

# Step 2: Insert product entities
echo -e "${YELLOW}Step 2: Inserting product entities${NC}"

# Product 1
PRODUCT1='{
  "fields": {
    "name": "Laptop",
    "price": 999.99,
    "category": "Electronics",
    "inStock": true,
    "description": "A powerful laptop for developers",
    "createdAt": "2025-04-20T10:00:00Z",
    "tags": ["laptop", "computer", "work"]
  }
}'

PRODUCT1_RESPONSE=$(make_request "POST" "/entities/Product" "$PRODUCT1")
PRODUCT1_ID=$(echo $PRODUCT1_RESPONSE | grep -o '"id":[0-9]*' | cut -d':' -f2)
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
    "createdAt": "2025-04-21T11:30:00Z",
    "tags": ["coffee", "kitchen", "appliance"]
  }
}'

PRODUCT2_RESPONSE=$(make_request "POST" "/entities/Product" "$PRODUCT2")
PRODUCT2_ID=$(echo $PRODUCT2_RESPONSE | grep -o '"id":[0-9]*' | cut -d':' -f2)
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
    "createdAt": "2025-04-22T09:15:00Z",
    "tags": ["audio", "music", "entertainment"]
  }
}'

PRODUCT3_RESPONSE=$(make_request "POST" "/entities/Product" "$PRODUCT3")
PRODUCT3_ID=$(echo $PRODUCT3_RESPONSE | grep -o '"id":[0-9]*' | cut -d':' -f2)
echo "Created Product with ID: $PRODUCT3_ID"
echo

# Product 4
PRODUCT4='{
  "fields": {
    "name": "Wireless Mouse",
    "price": 29.99,
    "category": "Electronics",
    "inStock": true,
    "description": "Ergonomic wireless mouse with long battery life",
    "createdAt": "2025-04-23T14:45:00Z",
    "tags": ["computer", "accessory", "ergonomic"]
  }
}'

PRODUCT4_RESPONSE=$(make_request "POST" "/entities/Product" "$PRODUCT4")
PRODUCT4_ID=$(echo $PRODUCT4_RESPONSE | grep -o '"id":[0-9]*' | cut -d':' -f2)
echo "Created Product with ID: $PRODUCT4_ID"
echo

# Step A3: Query all products to verify insertion
echo -e "${YELLOW}Step 3: Verifying all products were created${NC}"
ALL_PRODUCTS=$(make_request "GET" "/entities/Product" "")
echo

###########################################
# PART 2: ORDER ENTITY TYPE AND DATA
###########################################

# Step 4: Create an Order entity type
echo -e "${YELLOW}Step 4: Creating an Order entity type${NC}"
ORDER_TYPE_DEFINITION='{
  "name": "Order",
  "fields": [
    {
      "name": "customerId",
      "type": "string",
      "indexed": true,
      "required": true
    },
    {
      "name": "products",
      "type": "json",
      "indexed": false,
      "required": true
    },
    {
      "name": "totalAmount",
      "type": "float",
      "indexed": true,
      "required": true
    },
    {
      "name": "status",
      "type": "string",
      "indexed": true,
      "required": true
    },
    {
      "name": "orderDate",
      "type": "datetime",
      "indexed": true,
      "required": true
    }
  ]
}'

CREATE_ORDER_TYPE_RESPONSE=$(make_request "POST" "/entity-types" "$ORDER_TYPE_DEFINITION")
echo

# Step 5: Insert order entities
echo -e "${YELLOW}Step 5: Inserting order entities${NC}"

# Order 1: A customer buying a laptop
ORDER1='{
  "fields": {
    "customerId": "cust_001",
    "products": [
      {
        "productId": '${PRODUCT1_ID}',
        "name": "Laptop",
        "quantity": 1,
        "unitPrice": 999.99
      }
    ],
    "totalAmount": 999.99,
    "status": "Shipped",
    "orderDate": "2025-04-23T14:30:00Z"
  }
}'

ORDER1_RESPONSE=$(make_request "POST" "/entities/Order" "$ORDER1")
ORDER1_ID=$(echo $ORDER1_RESPONSE | grep -o '"id":[0-9]*' | cut -d':' -f2)
echo "Created Order with ID: $ORDER1_ID"
echo

# Order 2: A customer buying coffee maker and headphones
ORDER2='{
  "fields": {
    "customerId": "cust_002",
    "products": [
      {
        "productId": '${PRODUCT2_ID}',
        "name": "Coffee Maker",
        "quantity": 1,
        "unitPrice": 49.99
      },
      {
        "productId": '${PRODUCT3_ID}',
        "name": "Headphones",
        "quantity": 2,
        "unitPrice": 79.99
      }
    ],
    "totalAmount": 209.97,
    "status": "Processing",
    "orderDate": "2025-04-24T09:15:00Z"
  }
}'

ORDER2_RESPONSE=$(make_request "POST" "/entities/Order" "$ORDER2")
ORDER2_ID=$(echo $ORDER2_RESPONSE | grep -o '"id":[0-9]*' | cut -d':' -f2)
echo "Created Order with ID: $ORDER2_ID"
echo

# Order 3: A customer buying multiple products
ORDER3='{
  "fields": {
    "customerId": "cust_003",
    "products": [
      {
        "productId": '${PRODUCT1_ID}',
        "name": "Laptop",
        "quantity": 1,
        "unitPrice": 999.99
      },
      {
        "productId": '${PRODUCT2_ID}',
        "name": "Coffee Maker",
        "quantity": 1,
        "unitPrice": 49.99
      },
      {
        "productId": '${PRODUCT3_ID}',
        "name": "Headphones",
        "quantity": 1,
        "unitPrice": 79.99
      }
    ],
    "totalAmount": 1129.97,
    "status": "Pending",
    "orderDate": "2025-04-24T10:45:00Z"
  }
}'

ORDER3_RESPONSE=$(make_request "POST" "/entities/Order" "$ORDER3")
ORDER3_ID=$(echo $ORDER3_RESPONSE | grep -o '"id":[0-9]*' | cut -d':' -f2)
echo "Created Order with ID: $ORDER3_ID"
echo

# Order 4: A customer buying wireless mouse
ORDER4='{
  "fields": {
    "customerId": "cust_004",
    "products": [
      {
        "productId": '${PRODUCT4_ID}',
        "name": "Wireless Mouse",
        "quantity": 2,
        "unitPrice": 29.99
      }
    ],
    "totalAmount": 59.98,
    "status": "Delivered",
    "orderDate": "2025-04-22T16:20:00Z"
  }
}'

ORDER4_RESPONSE=$(make_request "POST" "/entities/Order" "$ORDER4")
ORDER4_ID=$(echo $ORDER4_RESPONSE | grep -o '"id":[0-9]*' | cut -d':' -f2)
echo "Created Order with ID: $ORDER4_ID"
echo

# Step 6: Query all orders to verify insertion
echo -e "${YELLOW}Step 6: Verifying all orders were created${NC}"
ALL_ORDERS=$(make_request "GET" "/entities/Order" "")
echo

###########################################
# PART 3: DATA OPERATIONS AND QUERIES
###########################################

# Step 7: Perform some interesting queries

# 7.1: Find all electronics products
echo -e "${YELLOW}Step 7.1: Finding all electronics products${NC}"
ELECTRONICS_QUERY='{
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

ELECTRONICS_PRODUCTS=$(make_request "POST" "/query" "$ELECTRONICS_QUERY")
echo

# 7.2: Find all orders with a value over $200
echo -e "${YELLOW}Step 7.2: Finding orders over $200${NC}"
HIGH_VALUE_QUERY='{
  "entityType": "Order",
  "filters": [
    {
      "field": "totalAmount",
      "operator": "gt",
      "value": 200
    }
  ],
  "orderBy": "totalAmount",
  "orderDesc": true
}'

HIGH_VALUE_ORDERS=$(make_request "POST" "/query" "$HIGH_VALUE_QUERY")
echo

# 7.3: Find all orders in "Processing" status
echo -e "${YELLOW}Step 7.3: Finding orders in 'Processing' status${NC}"
PROCESSING_QUERY='{
  "entityType": "Order",
  "filters": [
    {
      "field": "status",
      "operator": "eq",
      "value": "Processing"
    }
  ]
}'

PROCESSING_ORDERS=$(make_request "POST" "/query" "$PROCESSING_QUERY")
echo

# Step 8: Update operations

# 8.1: Update the price of the laptop
echo -e "${YELLOW}Step 8.1: Updating the laptop price${NC}"
LAPTOP_UPDATE='{
  "fields": {
    "price": 899.99,
    "description": "A powerful laptop for developers - Now on sale!"
  }
}'

LAPTOP_UPDATE_RESPONSE=$(make_request "PUT" "/entities/Product/${PRODUCT1_ID}" "$LAPTOP_UPDATE")
echo

# 8.2: Update an order status
echo -e "${YELLOW}Step 8.2: Updating order status${NC}"
ORDER_UPDATE='{
  "fields": {
    "status": "Delivered"
  }
}'

ORDER_UPDATE_RESPONSE=$(make_request "PUT" "/entities/Order/${ORDER1_ID}" "$ORDER_UPDATE")
echo

# 8.3: Verify updates
echo -e "${YELLOW}Step 8.3: Verifying updates${NC}"
UPDATED_LAPTOP=$(make_request "GET" "/entities/Product/${PRODUCT1_ID}" "")
echo
UPDATED_ORDER=$(make_request "GET" "/entities/Order/${ORDER1_ID}" "")
echo

# Summary
echo -e "${GREEN}Demo completed successfully!${NC}"
echo -e "The script:"
echo -e "1. Created Product and Order entity types"
echo -e "2. Added 4 products (Laptop, Coffee Maker, Headphones, and Wireless Mouse)"
echo -e "3. Added 4 orders with various products"
echo -e "4. Performed queries on products by category"
echo -e "5. Performed queries on orders by value and status"
echo -e "6. Updated product details and order status"
echo
echo -e "You can now interact with these entities using the SyncopateDB API:"
echo -e "Products: ${BLUE}http://${HOST}:${PORT}/api/v1/entities/Product${NC}"
echo -e "Orders: ${BLUE}http://${HOST}:${PORT}/api/v1/entities/Order${NC}"