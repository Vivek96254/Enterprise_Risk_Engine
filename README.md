# 🏦 Enterprise Transaction Risk Analytics & Decision Engine

A production-grade, scalable transaction risk analytics system built with Go, PostgreSQL, and Redis. This system ingests real-time and batch transactions, computes risk scores using a configurable rule engine, and serves analytics through a RESTful API.

![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)
![PostgreSQL](https://img.shields.io/badge/PostgreSQL-15+-336791?style=flat&logo=postgresql)
![Redis](https://img.shields.io/badge/Redis-7+-DC382D?style=flat&logo=redis)
![License](https://img.shields.io/badge/License-MIT-green.svg)

## 📋 Table of Contents

- [Problem Statement](#-problem-statement)
- [Architecture](#-architecture)
- [Features](#-features)
- [Tech Stack](#-tech-stack)
- [Quick Start](#-quick-start)
- [API Reference](#-api-reference)
- [Database Schema](#-database-schema)
- [Scaling Strategy](#-scaling-strategy)
- [Deployment](#-deployment)
- [Configuration](#-configuration)

## 🎯 Problem Statement

Financial institutions need to process millions of transactions daily while detecting fraudulent or risky activities in real-time. This system provides:

- **Real-time Risk Assessment**: Score transactions as they occur
- **Configurable Rule Engine**: Flexible rules that can be updated without code changes
- **Historical Analytics**: Insights into transaction patterns and risk trends
- **Horizontal Scalability**: Handle traffic spikes during peak hours
- **Audit Trail**: Complete transaction and decision history for compliance

## 🏗 Architecture

### Hybrid Pipeline Architecture

This system uses a **True Hybrid Architecture** where:
- **Redis Streams** handle fast, real-time scoring (~30ms)
- **Kafka CDC** captures all database changes for analytics, audit, and ML training

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              CLIENT LAYER                                    │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐    │
│  │  Dashboard   │  │  Mobile Apps │  │ Batch Upload │  │  External    │    │
│  │  (React)     │  │              │  │   (CSV)      │  │  Systems     │    │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘    │
└─────────┼─────────────────┼─────────────────┼─────────────────┼─────────────┘
          └─────────────────┴────────┬────────┴─────────────────┘
                                     │
┌────────────────────────────────────┼────────────────────────────────────────┐
│                                    ▼                                         │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │                         API GATEWAY + AUTH                             │  │
│  │  • JWT Authentication    • Rate Limiting    • CORS    • Logging       │  │
│  └───────────────────────────────────┬───────────────────────────────────┘  │
│                                      │                                       │
│  ┌───────────────────────────────────┼───────────────────────────────────┐  │
│  │                         INGESTION MODULE                               │  │
│  │  • Validation        • Idempotency         • Batch Processing         │  │
│  └───────────────────────────────────┬───────────────────────────────────┘  │
│                                      │                                       │
│                       ┌──────────────┴──────────────┐                       │
│                       │                             │                       │
│                       ▼                             ▼                       │
│  ┌────────────────────────────┐    ┌────────────────────────────────────┐  │
│  │       PostgreSQL           │    │        Redis Streams               │  │
│  │  • Transactions (Part.)    │    │  • Event Queue (Fast Path)         │  │
│  │  • Risk Scores             │    │  • Consumer Groups                 │  │
│  │  • Audit Logs              │    │  • Dead Letter Queue               │  │
│  │  • Geo Locations           │    │  • Analytics Cache                 │  │
│  └────────────┬───────────────┘    └─────────────────┬──────────────────┘  │
│               │                                      │                      │
│               │ CDC (Debezium)                       │                      │
│               ▼                                      ▼                      │
│  ┌────────────────────────────┐    ┌────────────────────────────────────┐  │
│  │         Kafka              │    │     SCORING WORKERS (Fast Path)    │  │
│  │  • CDC Events Topic        │    │  ┌──────────┐ ┌──────────┐         │  │
│  │  • Audit Trail             │    │  │ Worker 1 │ │ Worker N │  ~30ms  │  │
│  │  • Event Replay            │    │  │ • Rules  │ │ • Rules  │         │  │
│  └────────────┬───────────────┘    │  │ • ML     │ │ • ML     │         │  │
│               │                    │  │ • Score  │ │ • Score  │         │  │
│               ▼                    │  └──────────┘ └──────────┘         │  │
│  ┌────────────────────────────┐    └────────────────────────────────────┘  │
│  │   ANALYTICS PIPELINE       │                     │                      │
│  │   (Kafka Consumer)         │                     │                      │
│  │  • Real-time Metrics       │                     │                      │
│  │  • Audit Logging           │    ┌────────────────┴───────────────────┐  │
│  │  • ML Training Data        │    │           Redis Cache              │  │
│  │  • Data Lake Sync          │    │  • Risk Score Cache                │  │
│  │  • Event Replay            │    │  • Account Profiles                │  │
│  └────────────────────────────┘    │  • Rate Limiting                   │  │
│                                    └────────────────────────────────────┘  │
│                                                                             │
│                            ENTERPRISE RISK ENGINE                           │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Data Flow

```
FAST PATH (Real-time Scoring):
  API Request → Validation → Redis Stream → Worker → Score → DB → Response
  └─────────────────────────── ~30-50ms ───────────────────────────┘

CDC PATH (Analytics & Audit):
  DB Change → Debezium → Kafka → Analytics Pipeline → Metrics/Audit/ML
  └─────────────────── Async, no duplicate scoring ─────────────────┘
```

### Why Hybrid?

| Aspect | Redis Streams | Kafka CDC |
|--------|---------------|-----------|
| **Purpose** | Real-time scoring | Analytics & Audit |
| **Latency** | ~30ms | ~100-500ms |
| **Scoring** | ✅ Yes | ❌ No (observes only) |
| **Replay** | Limited | ✅ Full event replay |
| **Audit** | Basic | ✅ Complete trail |
| **ML Training** | ❌ | ✅ Training data |

## ✨ Features

### Core Capabilities

| Feature | Description |
|---------|-------------|
| **Real-time Ingestion** | Process transactions via REST API (~150-300ms p95 end-to-end) |
| **Fast-Path Scoring** | Sub-100ms for low-risk transactions (async persistence) |
| **Batch Processing** | Upload up to 1000 transactions per batch |
| **Hybrid Scoring** | Rule Engine + Behavioral Analysis + ML Score (pluggable) |
| **Async Scoring** | Redis Streams for reliable event processing |
| **Rule Engine** | JSON-configurable rules with modern fraud patterns |
| **Risk Analytics** | Daily summaries, account profiles, trend analysis |
| **Audit Trail** | Complete transaction and decision history |
| **Rate Limiting** | Token bucket algorithm, 100 req/min per IP |
| **Backtesting** | Replay historical transactions with new rule sets |
| **A/B Testing** | Experiment with different rule sets, statistical significance |
| **Load Testing** | k6 scripts for smoke, load, stress, and spike testing |

### Risk Scoring Rules

#### Classic Rules
| Rule ID | Description | Score Impact |
|---------|-------------|--------------|
| `RULE_CRITICAL_AMOUNT` | Transaction > $10,000 | +40 (Critical) |
| `RULE_SPIKE_ANOMALY` | Amount > 3σ from average | +30 (High) |
| `RULE_HIGH_RISK_COUNTRY` | Transaction from high-risk country | +35 (High) |
| `RULE_VELOCITY_BURST` | > 10 transactions/hour | +20 (Medium) |
| `RULE_NEW_LOCATION_HIGH_AMOUNT` | New location + > $1,000 | +25 (Medium) |
| `RULE_LOCATION_HOPPING` | > 3 location changes | +15 (Medium) |
| `RULE_NIGHT_TRANSACTION` | Transaction 12am-5am | +10 (Low) |

#### Modern Fraud Pattern Rules 🔥
| Rule ID | Description | Score Impact |
|---------|-------------|--------------|
| `RULE_SEQUENCE_EXFIL_PATTERN` | Small probe txn → large txn within 5 min | +35 (High) |
| `RULE_PEER_GROUP_ANOMALY` | User deviates 3σ from similar accounts | +25 (Medium) |
| `RULE_SHARED_BENEFICIARY_NETWORK` | Multiple accounts sending to same target | +30 (High) |
| `RULE_RAPID_DEVICE_SWITCH` | New device + high amount transaction | +25 (Medium) |
| `RULE_GEO_IMPOSSIBLE_TRAVEL` | Location change faster than flight speed | +40 (Critical) |
| `RULE_RAPID_CHANNEL_SWITCH` | Switching online→POS→ATM rapidly | +15 (Medium) |
| `RULE_BEHAVIORAL_ANOMALY` | Composite behavioral score > threshold | +20 (Medium) |

### Risk Levels

| Level | Score Range | Action |
|-------|-------------|--------|
| Low | 0-24 | Processed |
| Medium | 25-49 | Processed |
| High | 50-69 | Flagged |
| Critical | 70-100 | Blocked |

### Hybrid Scoring Architecture 🧠

The system uses a modern **hybrid scoring model** combining multiple signal sources:

```
┌─────────────────────────────────────────────────────────────────┐
│                    HYBRID RISK SCORING                          │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│   Transaction                                                   │
│       │                                                         │
│       ▼                                                         │
│   ┌─────────────────────────────────────────────────────────┐  │
│   │              FEATURE COMPUTATION                         │  │
│   │  • Spending patterns (rolling avg, std dev)             │  │
│   │  • Velocity metrics (tx/hour, z-scores)                 │  │
│   │  • Location analysis (distance, impossible travel)      │  │
│   │  • Peer group comparison                                │  │
│   │  • Sequence detection (probe patterns)                  │  │
│   └─────────────────────────────────────────────────────────┘  │
│       │                                                         │
│       ├──────────────┬──────────────┬──────────────┐           │
│       ▼              ▼              ▼              │           │
│   ┌────────┐    ┌────────┐    ┌────────┐          │           │
│   │ RULE   │    │BEHAVIOR│    │   ML   │          │           │
│   │ ENGINE │    │ANALYSIS│    │ SCORER │          │           │
│   │  50%   │    │  35%   │    │  15%   │          │           │
│   └───┬────┘    └───┬────┘    └───┬────┘          │           │
│       │             │             │               │           │
│       └─────────────┴─────────────┘               │           │
│                     │                             │           │
│                     ▼                             │           │
│   ┌─────────────────────────────────────────────────────────┐  │
│   │              FINAL SCORE = Σ(weight × score)            │  │
│   │                                                         │  │
│   │  Final = (0.50 × RuleScore) +                          │  │
│   │          (0.35 × BehavioralScore) +                    │  │
│   │          (0.15 × MLScore)                              │  │
│   └─────────────────────────────────────────────────────────┘  │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

**Score Breakdown (stored with each transaction):**
```json
{
  "score": 42.5,
  "rule_score": 35.0,
  "behavioral_score": 55.0,
  "ml_score": 48.0,
  "anomalies_detected": ["SPENDING_SPIKE", "PEER_GROUP_DEVIATION"],
  "scoring_path": "full"
}
```

**ML Integration (Pluggable):**
- Current: Lightweight behavioral z-score ensemble
- Future: External ML service (SageMaker, Vertex AI, custom)
- Interface: `MLScorerInterface` for easy swapping

### Fast-Path Scoring ⚡

For low-risk transactions, the system supports fast-path scoring:

```
┌─────────────────────────────────────────────────────────────────┐
│                     FAST-PATH OPTIMIZATION                      │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│   Transaction arrives                                           │
│         │                                                       │
│         ▼                                                       │
│   ┌─────────────────┐                                          │
│   │ Quick Risk Check│  (in-memory, ~10ms)                      │
│   │ • Amount < $500 │                                          │
│   │ • Known device  │                                          │
│   │ • Normal hours  │                                          │
│   └────────┬────────┘                                          │
│            │                                                    │
│    ┌───────┴───────┐                                           │
│    │               │                                           │
│    ▼               ▼                                           │
│ LOW RISK        HIGH RISK                                      │
│    │               │                                           │
│    ▼               ▼                                           │
│ ┌──────────┐  ┌──────────────────┐                            │
│ │FAST PATH │  │   FULL PIPELINE  │                            │
│ │• Score   │  │• All rules       │                            │
│ │  inline  │  │• ML scoring      │                            │
│ │• Return  │  │• Behavioral      │                            │
│ │  <100ms  │  │• ~150-300ms      │                            │
│ │• Async   │  │                  │                            │
│ │  persist │  │                  │                            │
│ └──────────┘  └──────────────────┘                            │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

## 🛠 Tech Stack

- **Language**: Go 1.21+
- **Web Framework**: Gin
- **Database**: PostgreSQL 15+ (with table partitioning)
- **Message Queue**: Redis Streams
- **Cache**: Redis
- **Auth**: JWT (HS256)
- **Container**: Docker & Docker Compose

## 🚀 Quick Start

### Prerequisites

- Go 1.21+
- Docker & Docker Compose
- Make (optional)

### Local Development

1. **Clone the repository**
```bash
git clone https://github.com/yourusername/enterprise-risk-engine.git
cd enterprise-risk-engine
```

2. **Start infrastructure**
```bash
docker-compose up -d postgres redis
```

3. **Run database migrations**
```bash
psql $DATABASE_URL -f db/migrations/001_initial_schema.sql
psql $DATABASE_URL -f db/migrations/002_create_partitions.sql
psql $DATABASE_URL -f db/migrations/003_seed_rules.sql
```

4. **Start the API server**
```bash
cp configs/env.example .env
go run ./cmd/api-server
```

5. **Start the worker (in another terminal)**
```bash
go run ./cmd/worker
```

### Using Docker Compose (Recommended)

```bash
# Start all services (API, Workers, Dashboard)
docker-compose up -d

# View logs
docker-compose logs -f

# Scale workers
docker-compose --profile scale up -d
```

### Dashboard

The system includes a sleek, minimalistic dashboard for real-time monitoring:

```bash
# With Docker (recommended) - Dashboard available at http://localhost:3000
docker-compose up -d

# Or serve locally with Python (API must be running on :8080)
make dashboard
```

**Dashboard Features:**
- 📊 Real-time system metrics (TPS, latency, error rate)
- 📈 Risk distribution visualization
- 🚩 Flagged transactions view with rule details
- 🧪 A/B Testing experiment management
- 🔍 Transaction search by account
- 🌙 Beautiful dark theme with smooth animations

![Dashboard Preview](docs/dashboard-preview.png)

# Stop all services
docker-compose down
```

## 📡 API Reference

### Authentication

#### Register User
```bash
POST /api/v1/auth/register
Content-Type: application/json

{
  "email": "user@example.com",
  "password": "SecurePass123",
  "role": "user"
}
```

#### Login
```bash
POST /api/v1/auth/login
Content-Type: application/json

{
  "email": "user@example.com",
  "password": "SecurePass123"
}
```

**Response:**
```json
{
  "token": "eyJhbGciOiJIUzI1NiIs...",
  "expires_in": 86400,
  "user": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "email": "user@example.com",
    "role": "user"
  }
}
```

### Transactions

#### Ingest Transaction
```bash
POST /api/v1/transactions
Authorization: Bearer <token>
Content-Type: application/json

{
  "account_id": "550e8400-e29b-41d4-a716-446655440000",
  "amount": 1500.00,
  "currency": "USD",
  "merchant": "Amazon",
  "merchant_category": "retail",
  "location": "New York, NY",
  "country": "US",
  "channel": "online",
  "idempotency_key": "tx-unique-key-123"
}
```

#### Batch Ingest
```bash
POST /api/v1/transactions/batch
Authorization: Bearer <token>
Content-Type: application/json

{
  "transactions": [
    { "account_id": "...", "amount": 100, ... },
    { "account_id": "...", "amount": 200, ... }
  ]
}
```

#### Get Flagged Transactions
```bash
GET /api/v1/transactions/flagged?page=1&page_size=20
Authorization: Bearer <token>
```

### Risk Analytics

#### Get Risk Summary
```bash
GET /api/v1/risk/summary?date=2026-02-03
Authorization: Bearer <token>
```

**Response:**
```json
{
  "date": "2026-02-03",
  "total_transactions": 15420,
  "total_amount": 2543210.50,
  "flagged_count": 234,
  "blocked_count": 45,
  "avg_risk_score": 18.5,
  "high_risk_count": 156,
  "critical_risk_count": 45,
  "top_rules_triggered": [
    {"rule_id": "RULE_VELOCITY_BURST", "count": 89},
    {"rule_id": "RULE_SPIKE_ANOMALY", "count": 67}
  ]
}
```

#### Get Account Risk Profile
```bash
GET /api/v1/risk/account/{account_id}
Authorization: Bearer <token>
```

### System Metrics
```bash
GET /api/v1/metrics/system
Authorization: Bearer <token>  # Requires admin/analyst role
```

**Response:**
```json
{
  "timestamp": "2026-02-03T10:30:00Z",
  "transactions_per_sec": 125.5,
  "avg_processing_time_ms": 45.2,
  "queue_depth": 150,
  "active_workers": 5,
  "db_connections_active": 12,
  "db_connections_idle": 13,
  "error_rate": 0.002
}
```

### Backtesting (Event Replay)
```bash
POST /api/v1/backtest/run
Authorization: Bearer <token>  # Requires admin/analyst role
Content-Type: application/json

{
  "account_id": "550e8400-e29b-41d4-a716-446655440000",
  "start_date": "2026-01-01T00:00:00Z",
  "end_date": "2026-02-01T00:00:00Z",
  "sample_size": 100
}
```

**Response:**
```json
{
  "total_transactions": 100,
  "processed_count": 98,
  "failed_count": 2,
  "average_score": 22.5,
  "risk_distribution": {
    "low": 65,
    "medium": 20,
    "high": 10,
    "critical": 3
  },
  "top_triggered_rules": [
    {"rule_id": "RULE_VELOCITY_BURST", "count": 25},
    {"rule_id": "RULE_NEW_LOCATION_HIGH_AMOUNT", "count": 18}
  ],
  "processing_time_ms": 1250,
  "comparison_with_live": {
    "matching_scores": 85,
    "different_scores": 13,
    "avg_score_difference": 2.3,
    "upgraded_risk": 8,
    "downgraded_risk": 5
  }
}
```

### A/B Testing (Experiments)

#### Create Experiment
```bash
POST /api/v1/experiments
Authorization: Bearer <token>  # Requires admin role
Content-Type: application/json

{
  "name": "New Velocity Rules Test",
  "description": "Testing stricter velocity rules",
  "control_rules": ["RULE_VELOCITY_BURST", "RULE_SPIKE_ANOMALY"],
  "test_rules": ["RULE_VELOCITY_BURST", "RULE_SPIKE_ANOMALY", "RULE_RAPID_SMALL_TRANSACTIONS"],
  "traffic_split": 0.2
}
```

#### Start Experiment
```bash
POST /api/v1/experiments/{id}/start
Authorization: Bearer <token>
```

#### Get Experiment Results
```bash
GET /api/v1/experiments/{id}/results
Authorization: Bearer <token>
```

**Response:**
```json
{
  "experiment_id": "abc123",
  "control": {
    "total_transactions": 800,
    "total_amount": 125000.50,
    "avg_risk_score": 18.5,
    "risk_distribution": {"low": 650, "medium": 100, "high": 40, "critical": 10},
    "flagged_count": 40,
    "blocked_count": 10,
    "rules_triggered": {"RULE_VELOCITY_BURST": 45, "RULE_SPIKE_ANOMALY": 30}
  },
  "test": {
    "total_transactions": 200,
    "total_amount": 31250.25,
    "avg_risk_score": 24.2,
    "risk_distribution": {"low": 140, "medium": 35, "high": 18, "critical": 7},
    "flagged_count": 18,
    "blocked_count": 7,
    "rules_triggered": {"RULE_VELOCITY_BURST": 15, "RULE_RAPID_SMALL_TRANSACTIONS": 22}
  },
  "start_time": "2026-02-01T00:00:00Z",
  "last_updated": "2026-02-03T10:30:00Z"
}
```

#### Get Statistical Significance
```bash
GET /api/v1/experiments/{id}/significance
Authorization: Bearer <token>
```

**Response:**
```json
{
  "is_significant": true,
  "confidence_level": 0.95,
  "p_value": 0.023,
  "score_difference": 5.7,
  "score_difference_pct": 30.8,
  "flag_rate_difference": 0.025,
  "sample_size_control": 800,
  "sample_size_test": 200,
  "recommendation": "Test group shows 30.8% higher risk scores. Consider if this aligns with your goals."
}
```

#### Stop/Pause/Delete Experiment
```bash
POST /api/v1/experiments/{id}/stop
POST /api/v1/experiments/{id}/pause
DELETE /api/v1/experiments/{id}
```

## 🧪 Load Testing

Run load tests using k6:

```bash
# Install k6
brew install k6  # macOS
# or: sudo apt install k6  # Ubuntu

# Run smoke test (quick validation)
k6 run scripts/load_test.js

# Run with custom VUs and duration
k6 run --vus 50 --duration 5m scripts/load_test.js

# Run against production
k6 run --env BASE_URL=https://your-api.onrender.com scripts/load_test.js
```

**Test Scenarios:**
| Scenario | VUs | Duration | Purpose |
|----------|-----|----------|---------|
| smoke | 5 | 30s | Quick validation |
| load | 50 | 5m | Normal load testing |
| stress | 100-200 | 10m | Find breaking point |
| spike | 10→200→10 | 2m | Sudden traffic spike |

## 📊 Database Schema

### Entity Relationship Diagram

```
┌──────────────┐       ┌──────────────┐       ┌──────────────────────┐
│    users     │       │   accounts   │       │    transactions      │
├──────────────┤       ├──────────────┤       │   (partitioned)      │
│ id (PK)      │──┐    │ id (PK)      │──┐    ├──────────────────────┤
│ email        │  │    │ user_id (FK) │◄─┘    │ id (PK)              │
│ password_hash│  └───►│ account_type │       │ account_id (FK)      │◄─┐
│ role         │       │ risk_profile │       │ amount               │  │
│ created_at   │       │ status       │       │ currency             │  │
│ updated_at   │       │ created_at   │       │ merchant             │  │
│ deleted_at   │       │ updated_at   │       │ location             │  │
└──────────────┘       └──────────────┘       │ country              │  │
                                              │ channel              │  │
                                              │ status               │  │
                                              │ idempotency_key      │  │
                                              │ created_at           │  │
                                              │ processed_at         │  │
                                              └──────────────────────┘  │
                                                                        │
┌──────────────────────┐       ┌──────────────────────┐                │
│    risk_scores       │       │    audit_logs        │                │
├──────────────────────┤       ├──────────────────────┤                │
│ id (PK)              │       │ id (PK)              │                │
│ transaction_id (FK)  │───────│ event_type           │                │
│ score                │       │ entity_id            │                │
│ risk_level           │       │ entity_type          │                │
│ rules_triggered[]    │       │ user_id (FK)         │                │
│ features (JSONB)     │       │ action               │                │
│ model_version        │       │ payload (JSONB)      │                │
│ processing_time_ms   │       │ ip_address           │                │
│ created_at           │       │ request_id           │                │
└──────────────────────┘       │ created_at           │                │
                               └──────────────────────┘                │
                                                                       │
┌──────────────────────┐                                               │
│      rules           │                                               │
├──────────────────────┤                                               │
│ id (PK)              │                                               │
│ name                 │                                               │
│ description          │                                               │
│ condition (JSONB)    │                                               │
│ score_impact         │                                               │
│ risk_level           │                                               │
│ priority             │                                               │
│ enabled              │                                               │
└──────────────────────┘                                               │
```

### Table Partitioning

Transactions are partitioned by month for optimal query performance:

```sql
transactions_2026_01  -- January 2026
transactions_2026_02  -- February 2026
...
transactions_2026_12  -- December 2026
```

## 📈 Scaling Strategy

### Horizontal Scaling

| Component | Scaling Method | Notes |
|-----------|---------------|-------|
| API Servers | Add replicas behind load balancer | Stateless, scale independently |
| Workers | Add more worker instances | Consumer groups ensure no duplicate processing |
| PostgreSQL | Read replicas for analytics | Write to primary, read from replicas |
| Redis | Redis Cluster for high availability | Streams support consumer groups natively |

### Performance Optimizations

1. **Database**
   - Monthly partitioning on transactions table
   - Composite indexes on (account_id, created_at)
   - Batch inserts for high-throughput ingestion
   - Connection pooling with pgx

2. **Queue Processing**
   - Consumer groups for parallel processing
   - Batch acknowledgment
   - Dead letter queue for failed messages
   - Automatic retry with exponential backoff

3. **Caching**
   - Risk scores cached for 24 hours
   - Account profiles cached for 5 minutes
   - Daily summaries cached (longer for historical data)

### Load Handling

```
Normal Load:    1 API + 1 Worker  →  ~100 TPS
Medium Load:    2 API + 3 Workers →  ~500 TPS
High Load:      4 API + 10 Workers → ~2000 TPS
```

## 🎯 Design Tradeoffs

### Why Redis Streams over Kafka?

| Factor | Redis Streams | Kafka |
|--------|--------------|-------|
| **Cost** | Free tier friendly (Upstash) | Requires managed service ($100+/mo) |
| **Operational Complexity** | Single binary, minimal config | ZooKeeper/KRaft, partitions, topics |
| **Throughput** | ~10K TPS (sufficient for this scale) | ~100K+ TPS |
| **Consumer Groups** | Native support | Native support |
| **Message Retention** | Memory-bound, configurable | Disk-based, unlimited |

**Decision**: Redis Streams provides Kafka-like semantics (consumer groups, acknowledgments, replay) at zero infrastructure cost. For free-tier deployment, this is the pragmatic choice. Migration to Kafka is straightforward when scale demands it.

### Why Modular Monolith First?

```
Monolith Benefits:
├── Single deployment unit (simpler CI/CD)
├── Shared database transactions
├── No network latency between modules
├── Easier debugging and tracing
└── Faster development iteration

Microservices Later:
├── When team grows beyond 5-7 engineers
├── When modules need independent scaling
├── When different tech stacks are needed
└── When deployment independence is critical
```

**Decision**: Start with clear module boundaries (`internal/auth`, `internal/scoring`, etc.) but deploy as one unit. This avoids premature distributed systems complexity while maintaining the option to split later.

### Why Partitioning over Sharding?

| Approach | Partitioning | Sharding |
|----------|-------------|----------|
| **Complexity** | Native PostgreSQL feature | Requires application logic or Citus |
| **Query Routing** | Automatic partition pruning | Manual shard key routing |
| **Transactions** | Full ACID within DB | Distributed transactions needed |
| **Maintenance** | `DETACH PARTITION` for archival | Complex rebalancing |

**Decision**: Time-based partitioning (monthly) handles our access patterns perfectly—most queries filter by `created_at`. Sharding would be necessary only if single-partition write throughput becomes a bottleneck (unlikely below 50K TPS).

## 🛡️ Failure Scenarios & Recovery

### Worker Crash Mid-Batch

```
Scenario: Worker processes 50 of 100 messages, then crashes
┌─────────────────────────────────────────────────────────────┐
│ 1. Messages remain in "pending" state (not acknowledged)   │
│ 2. Redis tracks pending messages per consumer              │
│ 3. Other workers claim abandoned messages after 30s        │
│ 4. XCLAIM moves ownership to healthy worker                │
│ 5. Processing resumes from where it left off               │
└─────────────────────────────────────────────────────────────┘
```

**Protection**: Consumer groups + pending entry list (PEL) ensure at-least-once delivery. Idempotency keys prevent duplicate scoring.

### Duplicate Events

```
Scenario: Network glitch causes same transaction to be published twice
┌─────────────────────────────────────────────────────────────┐
│ 1. First event arrives → Transaction created with          │
│    idempotency_key = "tx-abc-123"                          │
│ 2. Second event arrives → INSERT fails (unique constraint) │
│ 3. API returns existing transaction (HTTP 200, not 201)    │
│ 4. No duplicate processing occurs                          │
└─────────────────────────────────────────────────────────────┘
```

**Protection**: `idempotency_key` column with unique constraint + `ON CONFLICT DO NOTHING` for batch inserts.

### Database Outage

```
Scenario: PostgreSQL becomes unavailable for 2 minutes
┌─────────────────────────────────────────────────────────────┐
│ API Server:                                                 │
│ ├── Connection pool detects failure                        │
│ ├── Returns HTTP 503 (Service Unavailable)                 │
│ └── Health check fails → Load balancer removes instance    │
│                                                             │
│ Worker:                                                     │
│ ├── DB operations fail with timeout                        │
│ ├── Messages not acknowledged (remain pending)             │
│ ├── Exponential backoff on retries                         │
│ └── After max retries → Dead letter queue                  │
│                                                             │
│ Recovery:                                                   │
│ ├── DB comes back online                                   │
│ ├── Connection pool reconnects automatically               │
│ ├── Pending messages reprocessed                           │
│ └── DLQ messages can be replayed manually                  │
└─────────────────────────────────────────────────────────────┘
```

**Protection**: Connection pooling with health checks, message persistence in Redis, dead letter queue for failed messages.

### Redis Outage

```
Scenario: Redis becomes unavailable
┌─────────────────────────────────────────────────────────────┐
│ Impact:                                                     │
│ ├── New transactions saved to DB but not queued            │
│ ├── Scoring delayed (not lost)                             │
│ ├── Cache misses → Direct DB queries (slower)              │
│ └── Rate limiting falls back to permissive mode            │
│                                                             │
│ Recovery:                                                   │
│ ├── Unscored transactions detected via status='pending'    │
│ ├── Batch job can requeue pending transactions             │
│ └── Cache rebuilds on-demand                               │
└─────────────────────────────────────────────────────────────┘
```

**Protection**: Transactions are persisted to PostgreSQL first, then queued. Redis is used for acceleration, not as source of truth.

## 🔐 Security Considerations

### Authentication & Authorization

| Layer | Implementation | Details |
|-------|---------------|---------|
| **Password Storage** | bcrypt (cost factor 12) | Resistant to rainbow tables, GPU attacks |
| **Token Format** | JWT (HS256) | Stateless, includes user ID and role |
| **Token Expiration** | 24 hours | Configurable via `JWT_EXPIRATION` |
| **Role-Based Access** | admin, analyst, user | Middleware enforces per-endpoint |

### JWT Security Best Practices

```go
// Current implementation
Token: {
  "user_id": "uuid",
  "email": "user@example.com",
  "role": "user",
  "exp": 1706954400,  // 24h expiration
  "iat": 1706868000   // Issued at
}

// Rotation strategy (recommended for production):
// 1. Short-lived access tokens (15 min)
// 2. Long-lived refresh tokens (7 days)
// 3. Refresh token rotation on use
// 4. Token blacklist for logout/revocation
```

### Rate Limiting

```
Current: Token bucket algorithm
├── 100 requests per minute per IP
├── Automatic cleanup of stale entries
├── Returns 429 with Retry-After header
└── Protects against:
    ├── Brute force login attempts
    ├── API abuse / scraping
    └── DoS attacks (basic)

Production Enhancements:
├── Per-user rate limits (not just IP)
├── Tiered limits by role/plan
├── Distributed rate limiting (Redis)
└── Adaptive limits based on behavior
```

### Data Protection

| Concern | Mitigation |
|---------|-----------|
| **SQL Injection** | Parameterized queries via pgx (prepared statements) |
| **XSS** | JSON API only, no HTML rendering |
| **CSRF** | Stateless JWT auth, no cookies |
| **Data at Rest** | PostgreSQL encryption (Render managed) |
| **Data in Transit** | TLS enforced (Render provides HTTPS) |
| **PII Handling** | Minimal PII stored, audit logs for access |

### Audit Trail Immutability

```sql
-- Audit logs table design
CREATE TABLE audit_logs (
    id UUID PRIMARY KEY,
    event_type VARCHAR(50) NOT NULL,
    entity_id UUID,
    entity_type VARCHAR(50),
    user_id UUID,
    action VARCHAR(50) NOT NULL,
    payload JSONB,              -- Full event details
    ip_address INET,            -- Client IP
    request_id VARCHAR(100),    -- Correlation ID
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
    -- No updated_at, no deleted_at → Immutable
);

-- Protection:
-- 1. No UPDATE/DELETE permissions for application user
-- 2. Append-only table design
-- 3. Consider write-once storage (S3 Glacier) for compliance
-- 4. Cryptographic hashing for tamper detection (future)
```

### Security Checklist for Production

- [ ] Rotate `JWT_SECRET` (use 256-bit random key)
- [ ] Enable PostgreSQL SSL (`sslmode=require`)
- [ ] Set up WAF rules (Cloudflare/AWS WAF)
- [ ] Implement IP allowlisting for admin endpoints
- [ ] Add request signing for inter-service calls
- [ ] Set up security headers (HSTS, CSP, etc.)
- [ ] Regular dependency vulnerability scanning
- [ ] Penetration testing before launch

## ☁️ Deployment

### Render.com (Free Tier)

1. **Create services on Render:**
   - Web Service (API Server)
   - Background Worker (Scoring Engine)
   - PostgreSQL (Managed)
   - Redis (Use Upstash for free tier)

2. **Configure environment variables:**
```
DATABASE_URL=<render-postgres-url>
REDIS_URL=<upstash-redis-url>
JWT_SECRET=<generate-secure-key>
ENVIRONMENT=production
```

3. **Deploy using render.yaml:**
```bash
render blueprint apply
```

### Docker Production

```bash
# Build images
docker build -t risk-engine-api -f Dockerfile.api .
docker build -t risk-engine-worker -f Dockerfile.worker .

# Push to registry
docker push your-registry/risk-engine-api:latest
docker push your-registry/risk-engine-worker:latest
```

## ⚙️ Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | 8080 | API server port |
| `ENVIRONMENT` | development | Environment (development/production) |
| `DATABASE_URL` | - | PostgreSQL connection string |
| `REDIS_URL` | - | Redis connection string |
| `JWT_SECRET` | - | Secret for JWT signing |
| `JWT_EXPIRATION` | 24h | Token expiration duration |
| `WORKER_CONCURRENCY` | 5 | Number of worker goroutines |
| `WORKER_BATCH_SIZE` | 100 | Messages per batch |

## 📡 Observability

The system is designed for enterprise-grade observability and monitoring:

### Structured Logging

All logs are JSON-formatted with correlation IDs for distributed tracing:

```json
{
  "level": "info",
  "time": 1706954400,
  "request_id": "req-abc123",
  "transaction_id": "tx-def456",
  "method": "POST",
  "path": "/api/v1/transactions",
  "status": 201,
  "latency_ms": 145,
  "final_score": 42.5,
  "rule_score": 35.0,
  "behavioral_score": 55.0,
  "scoring_path": "full",
  "rules_triggered": ["RULE_VELOCITY_BURST"],
  "anomalies_detected": ["SPENDING_SPIKE"]
}
```

### Key Metrics Exposed

| Metric | Type | Description |
|--------|------|-------------|
| `transactions_per_sec` | Gauge | Current ingestion rate |
| `avg_processing_time_ms` | Histogram | Scoring latency distribution |
| `queue_depth` | Gauge | Pending messages in Redis Stream |
| `error_rate` | Gauge | Failed transactions / total |
| `db_connections_active` | Gauge | PostgreSQL pool utilization |
| `scoring_path_distribution` | Counter | Fast vs full path breakdown |
| `rules_triggered_total` | Counter | Rule trigger frequency |
| `anomalies_detected_total` | Counter | Anomaly type frequency |

### Alerting Thresholds

```yaml
alerts:
  - name: HighLatency
    condition: avg_processing_time_ms > 500 for 5m
    severity: warning
    
  - name: QueueBacklog
    condition: queue_depth > 1000 for 2m
    severity: critical
    
  - name: HighErrorRate
    condition: error_rate > 0.05 for 5m
    severity: critical
    
  - name: ScoringDrift
    condition: avg_risk_score change > 20% in 1h
    severity: warning
```

### OpenTelemetry Ready

The system is designed for distributed tracing integration:

```go
// Trace context propagation
ctx = otel.GetTextMapPropagator().Extract(ctx, carrier)
span := tracer.Start(ctx, "ScoreTransaction")
defer span.End()

// Span attributes
span.SetAttributes(
    attribute.String("transaction.id", txID),
    attribute.Float64("score.final", finalScore),
    attribute.String("scoring.path", scoringPath),
)
```

**Integration Points:**
- Jaeger / Zipkin for distributed tracing
- Prometheus for metrics scraping
- Grafana for dashboards
- PagerDuty / OpsGenie for alerting

### Health Endpoints

| Endpoint | Purpose |
|----------|---------|
| `GET /health` | Load balancer health check |
| `GET /health/ready` | Kubernetes readiness probe |
| `GET /health/live` | Kubernetes liveness probe |
| `GET /api/v1/metrics/system` | Detailed system metrics (auth required) |

## 🔮 Future AWS Migration Path

| Current | AWS Equivalent |
|---------|---------------|
| Redis Streams | Amazon MSK (Kafka) |
| Render PostgreSQL | Amazon RDS |
| Docker on Render | Amazon ECS/EKS |
| Basic Metrics | Amazon CloudWatch + Prometheus |
| Manual Scaling | AWS Auto Scaling |
| Behavioral Scoring | Amazon SageMaker |

## 📄 License

MIT License - see [LICENSE](LICENSE) for details.

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch
3. Commit your changes
4. Push to the branch
5. Open a Pull Request

---

Built with ❤️ for enterprise-grade risk management
