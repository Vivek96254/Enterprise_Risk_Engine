#!/bin/bash

# ============================================
# Enterprise Risk Engine - Live Demo Script
# ============================================
# This script demonstrates the full capabilities of the system
# Run with: ./scripts/demo.sh

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# API Base URL
API_URL="http://localhost:8080/api/v1"
TOKEN=""

# Helper functions
print_header() {
    echo ""
    echo -e "${PURPLE}============================================${NC}"
    echo -e "${PURPLE}  $1${NC}"
    echo -e "${PURPLE}============================================${NC}"
    echo ""
}

print_step() {
    echo -e "${CYAN}▶ $1${NC}"
}

print_success() {
    echo -e "${GREEN}✓ $1${NC}"
}

print_warning() {
    echo -e "${YELLOW}⚠ $1${NC}"
}

print_error() {
    echo -e "${RED}✗ $1${NC}"
}

wait_for_key() {
    echo ""
    echo -e "${YELLOW}Press Enter to continue...${NC}"
    read -r
}

# Check if services are running
check_services() {
    print_header "Checking Services"
    
    print_step "Checking API server..."
    if curl -s http://localhost:8080/health > /dev/null 2>&1; then
        print_success "API server is running"
    else
        print_error "API server is not running. Start with: docker compose up -d"
        exit 1
    fi
    
    print_step "Checking Dashboard..."
    if curl -s http://localhost:3000 > /dev/null 2>&1; then
        print_success "Dashboard is running at http://localhost:3000"
    else
        print_warning "Dashboard may not be running"
    fi
}

# Login and get token
login() {
    print_header "Authentication"
    
    print_step "Logging in as admin@example.com..."
    
    RESPONSE=$(curl -s -X POST "$API_URL/auth/login" \
        -H "Content-Type: application/json" \
        -d '{"email":"admin@example.com","password":"admin123"}')
    
    TOKEN=$(echo "$RESPONSE" | jq -r '.token // empty')
    
    if [ -z "$TOKEN" ]; then
        print_error "Login failed. Response: $RESPONSE"
        print_step "Creating admin user..."
        
        # Try to register
        curl -s -X POST "$API_URL/auth/register" \
            -H "Content-Type: application/json" \
            -d '{"email":"admin@example.com","password":"admin123","name":"Admin User","role":"admin"}' > /dev/null
        
        # Login again
        RESPONSE=$(curl -s -X POST "$API_URL/auth/login" \
            -H "Content-Type: application/json" \
            -d '{"email":"admin@example.com","password":"admin123"}')
        
        TOKEN=$(echo "$RESPONSE" | jq -r '.token // empty')
    fi
    
    if [ -n "$TOKEN" ]; then
        print_success "Logged in successfully"
        echo -e "   Token: ${TOKEN:0:50}..."
    else
        print_error "Could not authenticate"
        exit 1
    fi
}

# Create test account
create_account() {
    print_header "Creating Test Account"
    
    print_step "Creating a new user and account..."
    
    # Create user
    USER_RESPONSE=$(curl -s -X POST "$API_URL/auth/register" \
        -H "Content-Type: application/json" \
        -d "{\"email\":\"demo-user-$(date +%s)@test.com\",\"password\":\"DemoPass123!\",\"name\":\"Demo User\",\"role\":\"user\"}")
    
    USER_ID=$(echo "$USER_RESPONSE" | jq -r '.user.id // empty')
    
    if [ -z "$USER_ID" ]; then
        print_warning "Could not create user, using existing account"
        # Get existing account
        ACCOUNTS=$(curl -s "$API_URL/accounts?page_size=1" -H "Authorization: Bearer $TOKEN")
        ACCOUNT_ID=$(echo "$ACCOUNTS" | jq -r '.accounts[0].id // empty')
    else
        print_success "User created: $USER_ID"
        
        # Create account for user
        ACCOUNT_RESPONSE=$(curl -s -X POST "$API_URL/accounts" \
            -H "Authorization: Bearer $TOKEN" \
            -H "Content-Type: application/json" \
            -d "{\"user_id\":\"$USER_ID\",\"account_type\":\"premium\"}")
        
        ACCOUNT_ID=$(echo "$ACCOUNT_RESPONSE" | jq -r '.id // empty')
        print_success "Account created: $ACCOUNT_ID"
    fi
    
    export DEMO_ACCOUNT_ID="$ACCOUNT_ID"
    echo -e "   Account ID: ${CYAN}$ACCOUNT_ID${NC}"
}

# Demonstrate normal transactions
demo_normal_transactions() {
    print_header "Demo: Normal Transactions (Low Risk)"
    
    echo "These transactions should be APPROVED with low risk scores:"
    echo ""
    
    # Transaction 1: Small retail purchase
    print_step "Creating small retail purchase ($50 at Starbucks, US)..."
    RESPONSE=$(curl -s -X POST "$API_URL/transactions" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        -d "{
            \"account_id\": \"$DEMO_ACCOUNT_ID\",
            \"amount\": 50,
            \"currency\": \"USD\",
            \"merchant\": \"Starbucks\",
            \"merchant_category\": \"food\",
            \"country\": \"US\",
            \"channel\": \"pos\",
            \"location\": \"New York\",
            \"idempotency_key\": \"demo-$(date +%s)-1\"
        }")
    TX_ID=$(echo "$RESPONSE" | jq -r '.transaction_id')
    print_success "Transaction: $TX_ID"
    sleep 2
    
    # Transaction 2: Online shopping
    print_step "Creating online purchase ($150 at Amazon, US)..."
    RESPONSE=$(curl -s -X POST "$API_URL/transactions" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        -d "{
            \"account_id\": \"$DEMO_ACCOUNT_ID\",
            \"amount\": 150,
            \"currency\": \"USD\",
            \"merchant\": \"Amazon\",
            \"merchant_category\": \"retail\",
            \"country\": \"US\",
            \"channel\": \"online\",
            \"location\": \"Seattle\",
            \"idempotency_key\": \"demo-$(date +%s)-2\"
        }")
    TX_ID=$(echo "$RESPONSE" | jq -r '.transaction_id')
    print_success "Transaction: $TX_ID"
    sleep 2
    
    echo ""
    print_success "Normal transactions created - check dashboard for low risk scores"
}

# Demonstrate suspicious transactions
demo_suspicious_transactions() {
    print_header "Demo: Suspicious Transactions (High Risk)"
    
    echo "These transactions should be FLAGGED with high risk scores:"
    echo ""
    
    # Transaction 1: Large amount
    print_step "Creating large transaction ($25,000 - triggers CRITICAL_AMOUNT rule)..."
    RESPONSE=$(curl -s -X POST "$API_URL/transactions" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        -d "{
            \"account_id\": \"$DEMO_ACCOUNT_ID\",
            \"amount\": 25000,
            \"currency\": \"USD\",
            \"merchant\": \"Luxury Watches Inc\",
            \"merchant_category\": \"retail\",
            \"country\": \"US\",
            \"channel\": \"online\",
            \"location\": \"Miami\",
            \"idempotency_key\": \"demo-$(date +%s)-3\"
        }")
    TX_ID=$(echo "$RESPONSE" | jq -r '.transaction_id')
    print_success "Transaction: $TX_ID"
    sleep 2
    
    # Transaction 2: Sanctioned country
    print_step "Creating transaction from SANCTIONED country (North Korea)..."
    RESPONSE=$(curl -s -X POST "$API_URL/transactions" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        -d "{
            \"account_id\": \"$DEMO_ACCOUNT_ID\",
            \"amount\": 5000,
            \"currency\": \"USD\",
            \"merchant\": \"Unknown Vendor\",
            \"merchant_category\": \"money_transfer\",
            \"country\": \"KP\",
            \"channel\": \"wire\",
            \"location\": \"Pyongyang\",
            \"idempotency_key\": \"demo-$(date +%s)-4\"
        }")
    TX_ID=$(echo "$RESPONSE" | jq -r '.transaction_id')
    print_success "Transaction: $TX_ID"
    sleep 2
    
    # Transaction 3: High-risk category
    print_step "Creating crypto transaction ($15,000 - high risk category)..."
    RESPONSE=$(curl -s -X POST "$API_URL/transactions" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        -d "{
            \"account_id\": \"$DEMO_ACCOUNT_ID\",
            \"amount\": 15000,
            \"currency\": \"USD\",
            \"merchant\": \"CryptoExchange\",
            \"merchant_category\": \"crypto\",
            \"country\": \"RU\",
            \"channel\": \"online\",
            \"location\": \"Moscow\",
            \"idempotency_key\": \"demo-$(date +%s)-5\"
        }")
    TX_ID=$(echo "$RESPONSE" | jq -r '.transaction_id')
    print_success "Transaction: $TX_ID"
    sleep 2
    
    echo ""
    print_success "Suspicious transactions created - check dashboard for HIGH risk scores"
}

# Demonstrate velocity attack
demo_velocity_attack() {
    print_header "Demo: Velocity Attack Pattern"
    
    echo "Creating rapid transactions to trigger VELOCITY_BURST rule:"
    echo ""
    
    for i in {1..5}; do
        print_step "Rapid transaction $i/5 ($500)..."
        curl -s -X POST "$API_URL/transactions" \
            -H "Authorization: Bearer $TOKEN" \
            -H "Content-Type: application/json" \
            -d "{
                \"account_id\": \"$DEMO_ACCOUNT_ID\",
                \"amount\": 500,
                \"currency\": \"USD\",
                \"merchant\": \"QuickMart $i\",
                \"merchant_category\": \"retail\",
                \"country\": \"US\",
                \"channel\": \"online\",
                \"location\": \"Chicago\",
                \"idempotency_key\": \"demo-velocity-$(date +%s)-$i\"
            }" > /dev/null
        sleep 0.5
    done
    
    print_success "Velocity attack simulated - check for VELOCITY_BURST flags"
}

# Show analytics
show_analytics() {
    print_header "Analytics & Risk Summary"
    
    print_step "Fetching risk distribution..."
    DIST=$(curl -s "$API_URL/risk/distribution?days=1" -H "Authorization: Bearer $TOKEN")
    echo "$DIST" | jq '.'
    
    echo ""
    print_step "Fetching top triggered rules..."
    RULES=$(curl -s "$API_URL/risk/rules/top?days=1&limit=5" -H "Authorization: Bearer $TOKEN")
    echo "$RULES" | jq '.'
    
    echo ""
    print_step "Fetching flagged transactions..."
    FLAGGED=$(curl -s "$API_URL/transactions/flagged?page_size=5" -H "Authorization: Bearer $TOKEN")
    echo "$FLAGGED" | jq '.transactions | length' | xargs -I {} echo "   Total flagged: {} transactions"
}

# Main demo flow
main() {
    clear
    echo ""
    echo -e "${PURPLE}"
    echo "  ╔═══════════════════════════════════════════════════════════╗"
    echo "  ║                                                           ║"
    echo "  ║   🛡️  ENTERPRISE RISK ENGINE - LIVE DEMONSTRATION  🛡️    ║"
    echo "  ║                                                           ║"
    echo "  ║   Real-time Transaction Fraud Detection System            ║"
    echo "  ║                                                           ║"
    echo "  ╚═══════════════════════════════════════════════════════════╝"
    echo -e "${NC}"
    echo ""
    echo "This demo will showcase:"
    echo "  1. Real-time transaction ingestion"
    echo "  2. Hybrid risk scoring (Rules + ML + Behavioral)"
    echo "  3. Fraud pattern detection"
    echo "  4. Live dashboard monitoring"
    echo ""
    echo -e "${YELLOW}Open the dashboard in your browser: http://localhost:3000${NC}"
    echo ""
    
    wait_for_key
    
    check_services
    wait_for_key
    
    login
    wait_for_key
    
    create_account
    wait_for_key
    
    demo_normal_transactions
    wait_for_key
    
    demo_suspicious_transactions
    wait_for_key
    
    demo_velocity_attack
    wait_for_key
    
    show_analytics
    
    print_header "Demo Complete!"
    echo ""
    echo "Key URLs:"
    echo -e "  Dashboard:    ${CYAN}http://localhost:3000${NC}"
    echo -e "  API Docs:     ${CYAN}http://localhost:8080/health${NC}"
    echo -e "  Kafka UI:     ${CYAN}http://localhost:8090${NC} (if Kafka enabled)"
    echo ""
    echo "To run with Kafka/CDC:"
    echo -e "  ${GREEN}docker compose --profile kafka up -d${NC}"
    echo ""
    echo "Thank you for watching!"
    echo ""
}

# Run main
main "$@"
