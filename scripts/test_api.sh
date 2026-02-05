#!/bin/bash

# Enterprise Risk Engine - API Test Script
# This script tests the main API endpoints

BASE_URL="${BASE_URL:-http://localhost:8080}"
TOKEN=""

echo "🧪 Enterprise Risk Engine API Test"
echo "=================================="
echo "Base URL: $BASE_URL"
echo ""

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Helper function
test_endpoint() {
    local method=$1
    local endpoint=$2
    local data=$3
    local description=$4
    
    echo -n "Testing: $description... "
    
    if [ -z "$data" ]; then
        response=$(curl -s -w "\n%{http_code}" -X $method "$BASE_URL$endpoint" \
            -H "Content-Type: application/json" \
            -H "Authorization: Bearer $TOKEN")
    else
        response=$(curl -s -w "\n%{http_code}" -X $method "$BASE_URL$endpoint" \
            -H "Content-Type: application/json" \
            -H "Authorization: Bearer $TOKEN" \
            -d "$data")
    fi
    
    http_code=$(echo "$response" | tail -n1)
    body=$(echo "$response" | sed '$d')
    
    if [[ $http_code -ge 200 && $http_code -lt 300 ]]; then
        echo -e "${GREEN}✓ PASS${NC} (HTTP $http_code)"
        return 0
    else
        echo -e "${RED}✗ FAIL${NC} (HTTP $http_code)"
        echo "Response: $body"
        return 1
    fi
}

# Test 1: Health Check
echo ""
echo "📋 1. Health Check"
test_endpoint "GET" "/health" "" "Health endpoint"

# Test 2: Register User
echo ""
echo "📋 2. User Registration"
REGISTER_RESPONSE=$(curl -s -X POST "$BASE_URL/api/v1/auth/register" \
    -H "Content-Type: application/json" \
    -d '{
        "email": "test@example.com",
        "password": "SecurePass123",
        "role": "admin"
    }')

if echo "$REGISTER_RESPONSE" | grep -q "token"; then
    echo -e "Register: ${GREEN}✓ PASS${NC}"
    TOKEN=$(echo "$REGISTER_RESPONSE" | grep -o '"token":"[^"]*"' | cut -d'"' -f4)
    USER_ID=$(echo "$REGISTER_RESPONSE" | grep -o '"id":"[^"]*"' | cut -d'"' -f4)
    echo "Token obtained: ${TOKEN:0:20}..."
else
    echo -e "Register: ${YELLOW}⚠ User may already exist${NC}"
fi

# Test 3: Login
echo ""
echo "📋 3. User Login"
LOGIN_RESPONSE=$(curl -s -X POST "$BASE_URL/api/v1/auth/login" \
    -H "Content-Type: application/json" \
    -d '{
        "email": "test@example.com",
        "password": "SecurePass123"
    }')

if echo "$LOGIN_RESPONSE" | grep -q "token"; then
    echo -e "Login: ${GREEN}✓ PASS${NC}"
    TOKEN=$(echo "$LOGIN_RESPONSE" | grep -o '"token":"[^"]*"' | cut -d'"' -f4)
    USER_ID=$(echo "$LOGIN_RESPONSE" | grep -o '"id":"[^"]*"' | cut -d'"' -f4)
    echo "Token refreshed: ${TOKEN:0:20}..."
else
    echo -e "Login: ${RED}✗ FAIL${NC}"
    echo "Response: $LOGIN_RESPONSE"
    exit 1
fi

# Test 4: Create Account
echo ""
echo "📋 4. Create Account"
ACCOUNT_RESPONSE=$(curl -s -X POST "$BASE_URL/api/v1/accounts" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $TOKEN" \
    -d "{
        \"user_id\": \"$USER_ID\",
        \"account_type\": \"standard\"
    }")

if echo "$ACCOUNT_RESPONSE" | grep -q "id"; then
    echo -e "Create Account: ${GREEN}✓ PASS${NC}"
    ACCOUNT_ID=$(echo "$ACCOUNT_RESPONSE" | grep -o '"id":"[^"]*"' | cut -d'"' -f4)
    echo "Account ID: $ACCOUNT_ID"
else
    echo -e "Create Account: ${RED}✗ FAIL${NC}"
    echo "Response: $ACCOUNT_RESPONSE"
fi

# Test 5: Ingest Transaction
echo ""
echo "📋 5. Ingest Transaction"
if [ -n "$ACCOUNT_ID" ]; then
    TX_RESPONSE=$(curl -s -X POST "$BASE_URL/api/v1/transactions" \
        -H "Content-Type: application/json" \
        -H "Authorization: Bearer $TOKEN" \
        -d "{
            \"account_id\": \"$ACCOUNT_ID\",
            \"amount\": 1500.00,
            \"currency\": \"USD\",
            \"merchant\": \"Amazon\",
            \"merchant_category\": \"retail\",
            \"location\": \"New York, NY\",
            \"country\": \"US\",
            \"channel\": \"online\",
            \"idempotency_key\": \"test-tx-$(date +%s)\"
        }")
    
    if echo "$TX_RESPONSE" | grep -q "transaction_id"; then
        echo -e "Ingest Transaction: ${GREEN}✓ PASS${NC}"
        TX_ID=$(echo "$TX_RESPONSE" | grep -o '"transaction_id":"[^"]*"' | cut -d'"' -f4)
        echo "Transaction ID: $TX_ID"
    else
        echo -e "Ingest Transaction: ${RED}✗ FAIL${NC}"
        echo "Response: $TX_RESPONSE"
    fi
else
    echo -e "Ingest Transaction: ${YELLOW}⚠ SKIPPED (no account)${NC}"
fi

# Test 6: Get Risk Summary
echo ""
echo "📋 6. Get Risk Summary"
test_endpoint "GET" "/api/v1/risk/summary" "" "Risk summary"

# Test 7: Get Flagged Transactions
echo ""
echo "📋 7. Get Flagged Transactions"
test_endpoint "GET" "/api/v1/transactions/flagged" "" "Flagged transactions"

# Test 8: Get Risk Distribution
echo ""
echo "📋 8. Get Risk Distribution"
test_endpoint "GET" "/api/v1/risk/distribution?days=7" "" "Risk distribution"

# Test 9: Get System Metrics
echo ""
echo "📋 9. Get System Metrics"
test_endpoint "GET" "/api/v1/metrics/system" "" "System metrics"

# Test 10: Batch Transaction Ingestion
echo ""
echo "📋 10. Batch Transaction Ingestion"
if [ -n "$ACCOUNT_ID" ]; then
    BATCH_RESPONSE=$(curl -s -X POST "$BASE_URL/api/v1/transactions/batch" \
        -H "Content-Type: application/json" \
        -H "Authorization: Bearer $TOKEN" \
        -d "{
            \"transactions\": [
                {
                    \"account_id\": \"$ACCOUNT_ID\",
                    \"amount\": 100.00,
                    \"currency\": \"USD\",
                    \"merchant\": \"Starbucks\",
                    \"location\": \"Seattle, WA\",
                    \"country\": \"US\",
                    \"channel\": \"pos\",
                    \"idempotency_key\": \"batch-tx-1-$(date +%s)\"
                },
                {
                    \"account_id\": \"$ACCOUNT_ID\",
                    \"amount\": 250.00,
                    \"currency\": \"USD\",
                    \"merchant\": \"Target\",
                    \"location\": \"Los Angeles, CA\",
                    \"country\": \"US\",
                    \"channel\": \"pos\",
                    \"idempotency_key\": \"batch-tx-2-$(date +%s)\"
                }
            ]
        }")
    
    if echo "$BATCH_RESPONSE" | grep -q "successful"; then
        echo -e "Batch Ingestion: ${GREEN}✓ PASS${NC}"
        SUCCESSFUL=$(echo "$BATCH_RESPONSE" | grep -o '"successful":[0-9]*' | cut -d':' -f2)
        echo "Successful: $SUCCESSFUL transactions"
    else
        echo -e "Batch Ingestion: ${RED}✗ FAIL${NC}"
        echo "Response: $BATCH_RESPONSE"
    fi
else
    echo -e "Batch Ingestion: ${YELLOW}⚠ SKIPPED (no account)${NC}"
fi

echo ""
echo "=================================="
echo "🏁 Test Complete!"
echo ""
echo "Note: Make sure the worker is running to process transactions and generate risk scores."
