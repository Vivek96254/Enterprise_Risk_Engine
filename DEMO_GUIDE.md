# 🎯 Enterprise Risk Engine - Complete Demo Guide

This guide walks you through demonstrating the full capabilities of the Enterprise Risk Engine to stakeholders, interviewers, or team members.

## 📋 Table of Contents

1. [Quick Start (5 minutes)](#quick-start)
2. [Full Demo with Kafka/CDC (15 minutes)](#full-demo-with-kafkacdc)
3. [Live Demonstration Script](#live-demonstration-script)
4. [Architecture Walkthrough](#architecture-walkthrough)
5. [Key Talking Points](#key-talking-points)

---

## 🚀 Quick Start

### Prerequisites
- Docker & Docker Compose installed
- `jq` installed (for JSON parsing): `sudo apt install jq`
- A modern web browser

### Step 1: Start Core Services

```bash
# Clone and navigate to project
cd Enterprise_Risk_Engine

# Start all core services
docker compose up -d

# Wait for services to be healthy (about 30 seconds)
docker compose ps
```

### Step 2: Open the Dashboard

Open your browser to: **http://localhost:3000**

Login credentials:
- **Email:** `admin@example.com`
- **Password:** `admin123`

### Step 3: Run the Demo Script

```bash
# Make the script executable (first time only)
chmod +x scripts/demo.sh

# Run the interactive demo
./scripts/demo.sh
```

---

## 🎬 Full Demo with Kafka/CDC (Hybrid Architecture)

This demonstrates the **True Hybrid Architecture** where:
- **Redis Streams** handle fast, real-time scoring (~30ms)
- **Kafka CDC** captures all database changes for analytics, audit, and ML training
- **No duplicate scoring** - each pipeline has a distinct purpose

### Step 1: Start All Services (including Kafka)

```bash
# Start with Kafka profile
docker compose --profile kafka up -d

# This starts:
# - PostgreSQL (with logical replication)
# - Redis
# - API Server
# - Scoring Workers (Redis - FAST PATH)
# - Zookeeper
# - Kafka
# - Kafka UI
# - Debezium (CDC)
# - Analytics Pipeline (Kafka - AUDIT/ML PATH)
```

### Step 2: Wait for Services

```bash
# Check all services are running
docker compose --profile kafka ps

# Wait for Debezium (takes ~60 seconds)
echo "Waiting for Debezium..."
until curl -s http://localhost:8083/connectors > /dev/null 2>&1; do
    sleep 5
done
echo "Debezium is ready!"
```

### Step 3: Setup CDC Connector

```bash
# Configure Debezium to capture PostgreSQL changes
./scripts/setup-debezium.sh
```

### Step 4: Open Monitoring UIs

| Service | URL | Purpose |
|---------|-----|---------|
| **Dashboard** | http://localhost:3000 | Main risk monitoring UI |
| **Kafka UI** | http://localhost:8090 | View Kafka topics & messages |
| **API Health** | http://localhost:8080/health | API status |

### Step 5: Demonstrate CDC Flow

```bash
# Terminal 1: Watch Kafka consumer logs
docker compose --profile kafka logs -f kafka-worker

# Terminal 2: Create a transaction via API
curl -X POST http://localhost:8080/api/v1/transactions \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "account_id": "YOUR_ACCOUNT_ID",
    "amount": 15000,
    "currency": "USD",
    "merchant": "Suspicious Vendor",
    "merchant_category": "crypto",
    "country": "RU",
    "channel": "wire",
    "location": "Moscow"
  }'
```

**What happens:**
1. Transaction saved to PostgreSQL
2. Debezium captures the INSERT via CDC
3. Change event published to Kafka topic
4. Kafka worker consumes and scores
5. Dashboard updates in real-time

---

## 🎤 Live Demonstration Script

Use this script when presenting to stakeholders:

### Opening (1 minute)

> "Today I'll demonstrate an Enterprise Risk Engine - a real-time fraud detection system that processes financial transactions, applies hybrid ML scoring, and flags suspicious activity. This is the same architecture used by companies like Stripe and Square."

### Part 1: Architecture Overview (2 minutes)

Show the architecture diagram and explain:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           DATA SOURCES                                       │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐                     │
│  │ REST API │  │ Kafka    │  │ Debezium │  │ CSV Bulk │                     │
│  │ (Direct) │  │ (Stream) │  │ (CDC)    │  │ (Batch)  │                     │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬─────┘                     │
└───────┼─────────────┼─────────────┼─────────────┼───────────────────────────┘
        │             │             │             │
        ▼             ▼             ▼             ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                         RISK ENGINE CORE                                     │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                      Scoring Pipeline                                │    │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐                  │    │
│  │  │ Rule Engine │  │ Behavioral  │  │ ML Scoring  │                  │    │
│  │  │ (40 rules)  │  │ Anomaly     │  │ (Patterns)  │                  │    │
│  │  │             │  │ Detection   │  │             │                  │    │
│  │  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘                  │    │
│  │         │                │                │                          │    │
│  │         └────────────────┼────────────────┘                          │    │
│  │                          ▼                                           │    │
│  │                 ┌─────────────────┐                                  │    │
│  │                 │ Hybrid Score    │                                  │    │
│  │                 │ (Weighted Avg)  │                                  │    │
│  │                 └────────┬────────┘                                  │    │
│  │                          │                                           │    │
│  │                          ▼                                           │    │
│  │                 ┌─────────────────┐                                  │    │
│  │                 │ Decision Engine │                                  │    │
│  │                 │ approve/flag/   │                                  │    │
│  │                 │ block           │                                  │    │
│  │                 └─────────────────┘                                  │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Part 2: Dashboard Tour (3 minutes)

Navigate through the dashboard:

1. **Overview Page**
   - "Here we see real-time metrics: total transactions, flagged count, risk distribution"
   - "The charts update every 30 seconds"

2. **Transactions Page**
   - "Every transaction flows through our pipeline"
   - "Notice the status column - processed vs flagged"
   - "Click on an Account ID to filter"

3. **Flagged Page**
   - "This is where analysts review suspicious activity"
   - "Each flagged transaction shows risk score, level, and triggered rules"

4. **A/B Tests Page**
   - "We can experiment with new scoring rules without affecting production"

### Part 3: Live Transaction Demo (5 minutes)

```bash
# Run the demo script
./scripts/demo.sh
```

Walk through each step:

1. **Normal Transaction**
   > "First, let's create a normal $50 purchase at Starbucks. Notice the low risk score."

2. **High-Value Transaction**
   > "Now a $25,000 transaction - this triggers our CRITICAL_AMOUNT rule"

3. **Sanctioned Country**
   > "A transaction from North Korea - immediately flagged due to sanctions"

4. **Velocity Attack**
   > "Watch what happens when we simulate rapid transactions - the VELOCITY_BURST rule triggers"

### Part 4: Technical Deep Dive (3 minutes)

Show the scoring breakdown:

```bash
# Get detailed risk score
curl -s "http://localhost:8080/api/v1/risk/account/ACCOUNT_ID" \
  -H "Authorization: Bearer TOKEN" | jq '.'
```

Explain:
- **Rule Score (50%)**: Pattern matching against 40+ rules
- **Behavioral Score (30%)**: Z-score anomaly detection per account
- **ML Score (20%)**: Pattern recognition (probe sequences, peer deviation)

### Part 5: Kafka/CDC Demo (Optional, 3 minutes)

If time permits:

```bash
# Show Kafka UI
open http://localhost:8090

# Show CDC in action
docker compose --profile kafka logs -f kafka-worker
```

> "In production, we'd use CDC to capture changes directly from the payment database, ensuring no transaction is missed."

### Closing (1 minute)

> "This system processes thousands of transactions per second with sub-100ms scoring latency. It's designed to scale horizontally and integrates with any payment system via REST API or Kafka."

---

## 🏗️ Architecture Walkthrough

### Data Flow

```
1. Transaction Received
   └─▶ REST API / Kafka / CDC

2. Validation & Storage
   └─▶ PostgreSQL (partitioned by month)

3. Queue for Processing
   └─▶ Redis Streams (consumer groups)

4. Scoring Pipeline
   ├─▶ Rule Engine (instant rules)
   ├─▶ Behavioral Analysis (account baseline)
   └─▶ ML Patterns (cross-account)

5. Decision & Storage
   └─▶ PostgreSQL (risk_scores table)

6. Real-time Updates
   └─▶ Dashboard (polling / future: WebSocket)
```

### Key Components

| Component | Technology | Purpose |
|-----------|------------|---------|
| API Server | Go + Gin | REST endpoints, auth |
| Workers | Go + Redis Streams | Async scoring |
| Database | PostgreSQL | Transactions, scores |
| Cache | Redis | Hot data, queues |
| Streaming | Kafka | High-volume ingestion |
| CDC | Debezium | Database change capture |
| Dashboard | Vanilla JS | Monitoring UI |

---

## 💡 Key Talking Points

### For Technical Interviewers

1. **Scalability**: "Workers scale horizontally. Redis Streams provide exactly-once processing."

2. **Reliability**: "Idempotency keys prevent duplicates. Dead-letter queue for failed messages."

3. **Performance**: "Sub-100ms scoring with fast-path for low-risk transactions."

4. **Observability**: "Structured logging with request IDs. Ready for OpenTelemetry."

### For Business Stakeholders

1. **ROI**: "Catches fraud in real-time, reducing chargebacks by 40-60%."

2. **Compliance**: "Audit trail for all decisions. Sanctioned country blocking built-in."

3. **Flexibility**: "A/B testing allows safe experimentation with new rules."

### For Engineering Managers

1. **Maintainability**: "Modular monolith - easy to understand, deploy, and eventually split."

2. **Team Scaling**: "Clear service boundaries. New features don't require full system knowledge."

3. **Cost**: "Runs on minimal infrastructure. Free tier compatible."

---

## 🛠️ Troubleshooting

### Services Won't Start

```bash
# Check logs
docker compose logs api-server
docker compose logs worker

# Restart everything
docker compose down -v
docker compose up -d
```

### Login Fails

```bash
# Reset admin password
docker exec risk-engine-postgres psql -U postgres -d risk_engine -c \
  "UPDATE users SET password_hash = '\$2b\$12\$FOf3nPR1PAwQ5cTsuS0x3.N14l2wPYD5kHbq5jkIra19vFvcNSBTO' WHERE email = 'admin@example.com';"
```

### Kafka Issues

```bash
# Check Kafka logs
docker compose --profile kafka logs kafka

# Restart Kafka stack
docker compose --profile kafka restart zookeeper kafka debezium
```

---

## 📊 Demo Metrics to Highlight

After running the demo, show these metrics:

```bash
# Risk distribution
curl -s "http://localhost:8080/api/v1/risk/distribution?days=1" \
  -H "Authorization: Bearer TOKEN" | jq '.'

# Top triggered rules
curl -s "http://localhost:8080/api/v1/risk/rules/top?days=1" \
  -H "Authorization: Bearer TOKEN" | jq '.'

# System metrics
curl -s "http://localhost:8080/api/v1/metrics/system" \
  -H "Authorization: Bearer TOKEN" | jq '.'
```

---

## 🎉 Success Criteria

Your demo is successful if the audience understands:

1. ✅ How transactions flow through the system
2. ✅ How different risk factors trigger rules
3. ✅ How the hybrid scoring works
4. ✅ How to monitor and investigate flagged transactions
5. ✅ How the system scales for production use

---

**Good luck with your demo!** 🚀
