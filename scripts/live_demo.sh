#!/bin/bash

# ============================================
# Enterprise Risk Engine - Live Pipeline Demo
# ============================================
# This script generates random transactions and shows them
# flowing through the entire pipeline in real-time.
#
# Usage: ./scripts/live_demo.sh [count]
#   count: number of transactions to generate (default: 10)

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
WHITE='\033[1;37m'
NC='\033[0m'
BOLD='\033[1m'

# Configuration
API_URL="http://localhost:8080/api/v1"
TX_COUNT=${1:-10}
TOKEN=""
ACCOUNT_ID=""
USER_ID=""

# Transaction templates
MERCHANTS=("Starbucks" "Amazon" "Walmart" "Apple Store" "Gas Station" "Netflix" "Uber" "McDonald's" "Target" "Best Buy" "CryptoExchange" "Unknown Vendor" "Luxury Watches Inc" "Casino Online" "Wire Transfer Co")
CATEGORIES=("food" "retail" "electronics" "fuel" "entertainment" "transport" "crypto" "gambling" "money_transfer")
COUNTRIES=("US" "US" "US" "US" "GB" "DE" "FR" "JP" "RU" "CN" "NG" "KP" "IR" "BR" "IN")
CHANNELS=("online" "pos" "atm")
LOCATIONS=("New York" "Los Angeles" "London" "Berlin" "Tokyo" "Moscow" "Lagos" "Pyongyang" "Tehran" "São Paulo" "Mumbai" "Paris" "Sydney" "Toronto" "Miami")

# Risk profiles for amounts
LOW_RISK_AMOUNTS=(15 25 35 50 75 100 150)
MEDIUM_RISK_AMOUNTS=(500 1000 2500 5000)
HIGH_RISK_AMOUNTS=(10000 15000 25000 50000 100000)

print_header() {
    echo ""
    echo -e "${PURPLE}╔════════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${PURPLE}║${WHITE}  $1${PURPLE}$(printf '%*s' $((62 - ${#1})) '')║${NC}"
    echo -e "${PURPLE}╚════════════════════════════════════════════════════════════════╝${NC}"
    echo ""
}

print_subheader() {
    echo -e "${CYAN}━━━ $1 ━━━${NC}"
}

print_tx() {
    local status=$1
    local icon=$2
    local msg=$3
    echo -e "${icon} ${msg}"
}

# Get random element from array
random_element() {
    local arr=("$@")
    echo "${arr[$RANDOM % ${#arr[@]}]}"
}

# Get random amount based on risk profile
random_amount() {
    local profile=$((RANDOM % 100))
    if [ $profile -lt 60 ]; then
        # 60% low risk
        random_element "${LOW_RISK_AMOUNTS[@]}"
    elif [ $profile -lt 85 ]; then
        # 25% medium risk
        random_element "${MEDIUM_RISK_AMOUNTS[@]}"
    else
        # 15% high risk
        random_element "${HIGH_RISK_AMOUNTS[@]}"
    fi
}

# Setup: Login and create account
setup() {
    print_header "🔧 SETUP: Authenticating & Creating Account"
    
    # Try to login first
    echo -e "${CYAN}Attempting login...${NC}"
    RESPONSE=$(curl -s -X POST "$API_URL/auth/login" \
        -H "Content-Type: application/json" \
        -d '{"email":"demo@riskengine.com","password":"Demo123!@#"}' 2>/dev/null)
    
    TOKEN=$(echo "$RESPONSE" | jq -r '.token // empty' 2>/dev/null)
    
    if [ -z "$TOKEN" ]; then
        echo -e "${YELLOW}Creating new demo user...${NC}"
        RESPONSE=$(curl -s -X POST "$API_URL/auth/register" \
            -H "Content-Type: application/json" \
            -d '{"email":"demo@riskengine.com","password":"Demo123!@#","name":"Demo User","role":"admin"}')
        
        TOKEN=$(echo "$RESPONSE" | jq -r '.token // empty')
        USER_ID=$(echo "$RESPONSE" | jq -r '.user.id // empty')
        
        if [ -z "$TOKEN" ]; then
            echo -e "${RED}Failed to create user. Response: $RESPONSE${NC}"
            exit 1
        fi
        echo -e "${GREEN}✓ User created: $USER_ID${NC}"
    else
        echo -e "${GREEN}✓ Logged in successfully${NC}"
        # Decode user ID from token (base64 decode the payload)
        USER_ID=$(echo "$TOKEN" | cut -d'.' -f2 | base64 -d 2>/dev/null | jq -r '.user_id // empty')
    fi
    
    # Create account
    echo -e "${CYAN}Creating account...${NC}"
    RESPONSE=$(curl -s -X POST "$API_URL/accounts" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        -d "{\"user_id\":\"$USER_ID\",\"account_type\":\"premium\"}" 2>/dev/null)
    
    ACCOUNT_ID=$(echo "$RESPONSE" | jq -r '.id // empty')
    
    if [ -z "$ACCOUNT_ID" ]; then
        # Get existing account
        RESPONSE=$(curl -s "$API_URL/accounts?page_size=1" \
            -H "Authorization: Bearer $TOKEN")
        ACCOUNT_ID=$(echo "$RESPONSE" | jq -r '.accounts[0].id // empty')
    fi
    
    if [ -z "$ACCOUNT_ID" ]; then
        echo -e "${RED}Failed to get account${NC}"
        exit 1
    fi
    
    echo -e "${GREEN}✓ Account ID: ${CYAN}$ACCOUNT_ID${NC}"
    echo ""
}

# Generate and submit a single transaction
submit_transaction() {
    local tx_num=$1
    local amount=$(random_amount)
    local merchant=$(random_element "${MERCHANTS[@]}")
    local category=$(random_element "${CATEGORIES[@]}")
    local country=$(random_element "${COUNTRIES[@]}")
    local channel=$(random_element "${CHANNELS[@]}")
    local location=$(random_element "${LOCATIONS[@]}")
    local idem_key="live-demo-$(date +%s%N)-$tx_num"
    
    # Determine expected risk based on transaction
    local expected_risk="LOW"
    local risk_color=$GREEN
    if [ $amount -ge 10000 ]; then
        expected_risk="HIGH"
        risk_color=$RED
    elif [ $amount -ge 1000 ]; then
        expected_risk="MEDIUM"
        risk_color=$YELLOW
    fi
    
    # High risk countries
    if [[ "$country" == "KP" || "$country" == "IR" || "$country" == "RU" || "$country" == "NG" ]]; then
        expected_risk="HIGH"
        risk_color=$RED
    fi
    
    # High risk categories
    if [[ "$category" == "crypto" || "$category" == "gambling" || "$category" == "money_transfer" ]]; then
        if [ $amount -ge 5000 ]; then
            expected_risk="HIGH"
            risk_color=$RED
        fi
    fi
    
    # Print transaction details
    echo -e "${WHITE}┌────────────────────────────────────────────────────────────────────────────┐${NC}"
    echo -e "${WHITE}│${NC} ${BOLD}Transaction #$tx_num${NC}                              ${WHITE}│${NC}"
    echo -e "${WHITE}├────────────────────────────────────────────────────────────────────────────┤${NC}"
    printf "${WHITE}│${NC}  💰 Amount:   ${CYAN}\$%-10s${NC}                              ${WHITE}│${NC}\n" "$amount"
    printf "${WHITE}│${NC}  🏪 Merchant: ${CYAN}%-20s${NC}                                ${WHITE}│${NC}\n" "$merchant"
    printf "${WHITE}│${NC}  📁 Category: ${CYAN}%-15s${NC}                                ${WHITE}│${NC}\n" "$category"
    printf "${WHITE}│${NC}  🌍 Country:  ${CYAN}%-5s${NC}  📍 Location: ${CYAN}%-15s${NC} ${WHITE}│${NC}\n" "$country" "$location"
    printf "${WHITE}│${NC}  📱 Channel:  ${CYAN}%-10s${NC}                                ${WHITE}│${NC}\n" "$channel"
    echo -e "${WHITE}│${NC}  🎯 Expected: ${risk_color}${expected_risk}${NC}              ${WHITE}│${NC}"
    echo -e "${WHITE}└────────────────────────────────────────────────────────────────────────────┘${NC}"
    
    # Submit transaction
    echo -ne "   ${YELLOW}⏳ Submitting...${NC}"
    
    local start_time=$(date +%s%3N)
    
    RESPONSE=$(curl -s -X POST "$API_URL/transactions" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        -d "{
            \"account_id\": \"$ACCOUNT_ID\",
            \"amount\": $amount,
            \"currency\": \"USD\",
            \"merchant\": \"$merchant\",
            \"merchant_category\": \"$category\",
            \"country\": \"$country\",
            \"channel\": \"$channel\",
            \"location\": \"$location\",
            \"idempotency_key\": \"$idem_key\"
        }")
    
    local tx_id=$(echo "$RESPONSE" | jq -r '.transaction_id // empty')
    
    if [ -z "$tx_id" ]; then
        echo -e "\r   ${RED}✗ Failed: $(echo $RESPONSE | jq -r '.error // "unknown error"')${NC}          "
        return
    fi
    
    echo -e "\r   ${GREEN}✓ Submitted: ${CYAN}${tx_id:0:8}...${NC}                    "
    
    # Wait for processing
    echo -ne "   ${YELLOW}⏳ Waiting for scoring...${NC}"
    sleep 1.5
    
    local end_time=$(date +%s%3N)
    local total_time=$((end_time - start_time))
    
    # Get transaction status to check if processed
    local tx_response=$(curl -s "$API_URL/accounts/$ACCOUNT_ID/transactions?page_size=1" \
        -H "Authorization: Bearer $TOKEN" 2>/dev/null)
    
    local tx_status=$(echo "$tx_response" | jq -r ".transactions[] | select(.id == \"$tx_id\") | .status" 2>/dev/null)
    
    if [ "$tx_status" == "processed" ] || [ "$tx_status" == "flagged" ]; then
        local status_color=$GREEN
        local status_icon="🟢"
        if [ "$tx_status" == "flagged" ]; then
            status_color=$RED
            status_icon="🔴"
        fi
        
        echo -e "\r   ${GREEN}✓ Processed${NC}                                      "
        echo -e "   ${status_icon} ${BOLD}Status: ${status_color}${tx_status^^}${NC}"
        echo -e "   ⏱️  Total pipeline time: ${CYAN}${total_time}ms${NC}"
    else
        echo -e "\r   ${GREEN}✓ Submitted (processing async)${NC}                    "
        echo -e "   ⏱️  Submission time: ${CYAN}${total_time}ms${NC}"
    fi
    
    echo ""
}

# Show final statistics
show_stats() {
    print_header "📊 FINAL STATISTICS"
    
    echo -e "${CYAN}Fetching analytics...${NC}"
    echo ""
    
    # Risk distribution
    print_subheader "Risk Distribution (Last 24h)"
    local dist=$(curl -s "$API_URL/risk/distribution?days=1" \
        -H "Authorization: Bearer $TOKEN")
    
    local low=$(echo "$dist" | jq -r '.levels.low // 0')
    local medium=$(echo "$dist" | jq -r '.levels.medium // 0')
    local high=$(echo "$dist" | jq -r '.levels.high // 0')
    local critical=$(echo "$dist" | jq -r '.levels.critical // 0')
    local total=$(echo "$dist" | jq -r '.total // 0')
    
    echo -e "   ${GREEN}🟢 Low:      $low${NC}"
    echo -e "   ${YELLOW}🟡 Medium:   $medium${NC}"
    echo -e "   ${RED}🔴 High:     $high${NC}"
    echo -e "   ${RED}⛔ Critical: $critical${NC}"
    echo -e "   ━━━━━━━━━━━━━━━━━"
    echo -e "   ${WHITE}📈 Total:    $total${NC}"
    echo ""
    
    # Top rules
    print_subheader "Top Triggered Rules"
    local rules=$(curl -s "$API_URL/risk/rules/top?days=1&limit=5" \
        -H "Authorization: Bearer $TOKEN")
    
    echo "$rules" | jq -r '.rules[] | "   \(.rule_id): \(.count) times"' 2>/dev/null || echo "   No rules triggered yet"
    echo ""
    
    # Flagged transactions
    print_subheader "Flagged Transactions"
    local flagged=$(curl -s "$API_URL/transactions/flagged?page_size=5" \
        -H "Authorization: Bearer $TOKEN")
    
    local flagged_count=$(echo "$flagged" | jq -r '.pagination.total // 0')
    echo -e "   ${RED}🚨 Total flagged: $flagged_count${NC}"
    
    if [ "$flagged_count" -gt 0 ]; then
        echo ""
        echo "$flagged" | jq -r '.transactions[] | "   • \(.id | .[0:8])... | $\(.amount) | \(.merchant) | \(.country)"' 2>/dev/null | head -5
    fi
}

# Main
main() {
    echo ""
    echo -e "${PURPLE}"
    cat << 'EOF'
    ╔═══════════════════════════════════════════════════════════════════╗
    ║                                                                   ║
    ║   🛡️  ENTERPRISE RISK ENGINE - HYBRID PIPELINE DEMO  🛡️           ║
    ║                                                                   ║
    ║   FAST PATH (Scoring):                                            ║
    ║   API → Redis Stream → Worker → Score (~30ms) → DB                ║
    ║                                                                   ║
    ║   CDC PATH (Analytics):                                           ║
    ║   DB → Debezium → Kafka → Analytics Pipeline (audit/ML)           ║
    ║                                                                   ║
    ╚═══════════════════════════════════════════════════════════════════╝
EOF
    echo -e "${NC}"
    echo ""
    echo -e "   ${WHITE}Generating ${CYAN}$TX_COUNT${WHITE} random transactions...${NC}"
    echo ""
    
    # Check API health
    if ! curl -s http://localhost:8080/health > /dev/null 2>&1; then
        echo -e "${RED}Error: API server is not running!${NC}"
        echo -e "Start with: ${CYAN}docker compose up -d${NC}"
        exit 1
    fi
    
    setup
    
    print_header "🚀 GENERATING TRANSACTIONS"
    
    for i in $(seq 1 $TX_COUNT); do
        submit_transaction $i
        
        # Random delay between transactions (0.5-2 seconds)
        if [ $i -lt $TX_COUNT ]; then
            sleep 0.$((RANDOM % 15 + 5))
        fi
    done
    
    show_stats
    
    print_header "✅ DEMO COMPLETE"
    echo -e "   ${WHITE}Dashboard:${NC}  ${CYAN}http://localhost:3000${NC}"
    echo -e "   ${WHITE}Kafka UI:${NC}   ${CYAN}http://localhost:8090${NC}"
    echo -e "   ${WHITE}API Health:${NC} ${CYAN}http://localhost:8080/health${NC}"
    echo ""
    echo -e "   ${YELLOW}Tip: Run with more transactions:${NC} ${CYAN}./scripts/live_demo.sh 50${NC}"
    echo ""
}

main "$@"
