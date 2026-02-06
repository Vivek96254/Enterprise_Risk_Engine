# 🏦 Enterprise Transaction Risk Analytics & Decision Engine

A production-grade, scalable transaction risk analytics system built with Go, PostgreSQL, and Redis. This system ingests real-time and batch transactions, computes risk scores using a configurable rule engine, and serves analytics through a RESTful API.

![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat&logo=go)
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

This system uses a **True Hybrid Architecture** that separates concerns for optimal performance:

- **Redis Streams (Fast Path)**: Handles real-time transaction scoring with sub-100ms latency
- **Kafka CDC (Analytics Path)**: Captures all database changes asynchronously for analytics, audit trails, and ML training

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
│  │  • JWT Authentication    • Rate Limiting (100/min/IP)                 │  │
│  │  • CORS                  • Request ID Generation                      │  │
│  │  • Structured Logging    • Error Handling                             │  │
│  └───────────────────────────────────┬───────────────────────────────────┘  │
│                                      │                                       │
│  ┌───────────────────────────────────┼───────────────────────────────────┐  │
│  │                    TRANSACTION INGESTION MODULE                        │  │
│  │  • Input Validation      • Idempotency Check (deduplication)          │  │
│  │  • Account Verification  • Batch Processing (up to 1000 txns)        │  │
│  │  • Metadata Enrichment   • Audit Log Creation                         │  │
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
│  │  • Accounts/Users          │    │  • Analytics Cache                 │  │
│  │  • Rules Configuration     │    │  • Rate Limiting State             │  │
│  └────────────┬───────────────┘    └─────────────────┬──────────────────┘  │
│               │                                      │                      │
│               │ CDC (Debezium)                       │                      │
│               │ (Optional - for analytics)           │                      │
│               ▼                                      ▼                      │
│  ┌────────────────────────────┐    ┌────────────────────────────────────┐  │
│  │         Kafka              │    │     SCORING WORKERS (Fast Path)    │  │
│  │  • CDC Events Topic        │    │  ┌──────────┐ ┌──────────┐         │  │
│  │  • Audit Trail             │    │  │ Worker 1 │ │ Worker N │         │  │
│  │  • Event Replay            │    │  │ • Rules  │ │ • Rules  │         │  │
│  │  • ML Training Data        │    │  │ • ML     │ │ • ML     │         │  │
│  └────────────┬───────────────┘    │  │ • Score  │ │ • Score  │         │  │
│               │                    │  │ • A/B    │ │ • A/B    │         │  │
│               ▼                    │  └──────────┘ └──────────┘         │  │
│  ┌────────────────────────────┐    │  Processing: ~30-150ms per txn     │  │
│  │   ANALYTICS PIPELINE       │    └────────────────────────────────────┘  │
│  │   (Kafka Consumer)         │                     │                      │
│  │  • Real-time Metrics       │                     │                      │
│  │  • Audit Logging           │    ┌────────────────┴───────────────────┐  │
│  │  • ML Training Data        │    │           Redis Cache              │  │
│  │  • Data Lake Sync          │    │  • Risk Score Cache (24h TTL)      │  │
│  │  • Event Replay            │    │  • Account Profiles (5m TTL)       │  │
│  └────────────────────────────┘    │  • Daily Summaries (varies)        │  │
│                                    └────────────────────────────────────┘  │
│                                                                             │
│                            ENTERPRISE RISK ENGINE                           │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Complete Transaction Flow (Step-by-Step)

#### 1. Transaction Ingestion (API Layer)

```
Client Request
    │
    ▼
[API Gateway]
    │ • Rate Limiting (100 req/min per IP)
    │ • JWT Authentication
    │ • Request ID Generation
    │
    ▼
[Ingestion Module]
    │ • Validate request payload
    │ • Check idempotency key (prevent duplicates)
    │ • Verify account exists and is active
    │ • Create transaction record in PostgreSQL
    │ • Generate audit log entry
    │
    ├─────────────────────────────────────┐
    │                                     │
    ▼                                     ▼
[PostgreSQL]                      [Redis Streams]
    │ • Transaction saved                │ • Event published to stream
    │   (status: "pending")              │ • Consumer group: "scoring-workers"
    │                                     │ • Message ID returned
    │                                     │
    │                                     │
    └─────────────────────────────────────┘
                    │
                    │ (Async - non-blocking)
                    ▼
            [Scoring Worker]
```

**Key Points:**
- Transaction is **immediately persisted** to PostgreSQL (source of truth)
- Event is **asynchronously published** to Redis Streams
- API returns **HTTP 201** with transaction ID (scoring happens async)
- Idempotency keys prevent duplicate processing

#### 2. Fast-Path Scoring (Redis Streams → Workers)

```
Redis Stream Message
    │
    ▼
[Worker Consumes Event]
    │ • Consumer group ensures no duplicate processing
    │ • Batch processing (100 messages at a time)
    │
    ▼
[Fetch Transaction from DB]
    │ • Load full transaction details
    │ • Get account information
    │
    ▼
[Feature Computation]
    │ • Historical transaction analysis (7d, 30d windows)
    │ • Velocity metrics (tx/hour, tx/day)
    │ • Location patterns (distance, impossible travel)
    │ • Peer group comparison
    │ • Sequence detection (probe patterns)
    │ • Behavioral anomaly scores
    │
    ▼
[Hybrid Scoring]
    │
    ├─────────────────────────────────────────┐
    │                                         │
    ▼                                         ▼
[Rule Engine]                          [ML/Behavioral]
    │ • Evaluate 15+ rules                    │ • Behavioral z-score analysis
    │ • Calculate rule_score (0-100)          │ • ML score (if available)
    │ • Track triggered rules                 │ • Anomaly detection
    │                                         │
    └─────────────────────────────────────────┘
                    │
                    ▼
            [Final Score Calculation]
                │
                │ Final = (0.50 × RuleScore) +
                │        (0.35 × BehavioralScore) +
                │        (0.15 × MLScore)
                │
                ▼
        [Risk Level Determination]
            │ • Low: 0-24
            │ • Medium: 25-49
            │ • High: 50-69
            │ • Critical: 70-100
            │
            ▼
    [Transaction Status Update]
        │ • Processed (low/medium)
        │ • Flagged (high)
        │ • Blocked (critical)
        │
        ▼
[Save Risk Score to DB]
    │ • Store all score components
    │ • Store features (JSONB)
    │ • Store triggered rules
    │ • Store anomalies detected
    │
    ▼
[Update Cache]
    │ • Cache risk score (24h TTL)
    │ • Update account profile cache
    │
    ▼
[Acknowledge Message]
    │ • Mark message as processed
    │ • Remove from pending list
```

**Processing Time:**
- **Fast Path** (low risk): ~30-50ms
- **Full Path** (high risk): ~150-300ms
- **Average**: ~45ms (p50), ~145ms (p95)

#### 3. CDC Path (Kafka - Optional, for Analytics)

```
PostgreSQL Change
    │
    ▼
[Debezium CDC Connector]
    │ • Captures INSERT/UPDATE/DELETE
    │ • Converts to Kafka events
    │ • Topic: risk-engine.public.transactions
    │
    ▼
[Kafka Topic]
    │ • Persistent storage
    │ • Event replay capability
    │ • Multiple consumers supported
    │
    ▼
[Analytics Pipeline Consumer]
    │ • Real-time metrics aggregation
    │ • Audit trail logging
    │ • ML training data collection
    │ • Data lake synchronization
    │ • NO SCORING (observes only)
```

**Why Two Paths?**

| Path | Purpose | Latency | Scoring | Use Case |
|------|---------|---------|---------|----------|
| **Redis Streams** | Real-time scoring | ~30-150ms | ✅ Yes | Transaction decision making |
| **Kafka CDC** | Analytics & audit | ~100-500ms | ❌ No | Compliance, ML training, analytics |

**Key Design Decision:**
- **Scoring happens ONCE** via Redis Streams (fast path)
- **Kafka observes** all changes for analytics (no duplicate scoring)
- This prevents double-scoring while enabling comprehensive audit trails

### Fast-Path Optimization

For low-risk transactions, the system can use an optimized fast path:

```
Transaction Arrives
    │
    ▼
[Quick Risk Assessment]
    │ • Amount < $500?
    │ • Known device/location?
    │ • Normal business hours?
    │ • Low velocity?
    │
    ├──────────────┬──────────────┐
    │              │              │
    ▼              ▼              ▼
LOW RISK      MEDIUM RISK    HIGH RISK
    │              │              │
    ▼              ▼              ▼
[FAST PATH]   [FULL PATH]   [FULL PATH]
• Minimal      • All rules   • All rules
  rules        • Behavioral  • Behavioral
• Inline       • ML scoring • ML scoring
• <100ms       • ~150ms     • ~300ms
• Async        • Async      • Async
  persist        persist      persist
```

**Fast Path Criteria:**
- Rule score < 20
- Behavioral score < 15
- No critical anomalies detected
- Known account patterns

### Hybrid Scoring Model Details

The system combines three scoring signals:

1. **Rule Engine (50% weight)**
   - 15+ configurable rules
   - Classic fraud patterns (velocity, amount, location)
   - Modern patterns (sequence detection, peer group, impossible travel)
   - Score range: 0-100

2. **Behavioral Analysis (35% weight)**
   - Z-score based anomaly detection
   - Spending pattern deviation
   - Velocity anomalies
   - Temporal pattern analysis
   - Score range: 0-100

3. **ML Scorer (15% weight)**
   - Pluggable ML model interface
   - Current: Lightweight ensemble model
   - Future: External service (SageMaker, Vertex AI)
   - Score range: 0-100 (nullable if ML unavailable)

**Final Score Formula:**
```python
if ml_score is not None:
    final_score = (0.50 × rule_score) + (0.35 × behavioral_score) + (0.15 × ml_score)
else:
    # Redistribute ML weight: 60% to rules, 40% to behavioral
    final_score = (0.59 × rule_score) + (0.41 × behavioral_score)

final_score = min(final_score, 100)  # Cap at 100
```

**Stored Score Breakdown:**
```json
{
  "score": 42.5,              // Final composite score
  "rule_score": 35.0,          // From rule engine
  "behavioral_score": 55.0,    // From behavioral analysis
  "ml_score": 48.0,           // From ML model (nullable)
  "risk_level": "medium",      // Determined from final score
  "rules_triggered": ["RULE_VELOCITY_BURST", "RULE_SPIKE_ANOMALY"],
  "anomalies_detected": ["SPENDING_SPIKE", "PEER_GROUP_DEVIATION"],
  "scoring_path": "full",      // "fast" or "full"
  "model_version": "v2.0.0-hybrid"
}
```

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

### Feature Computation Process

Before scoring, the system computes 30+ risk features from historical data:

```
Transaction Arrives
    │
    ▼
[Fetch Historical Data]
    │ • Last 7 days transactions
    │ • Last 30 days transactions
    │ • Last 1 hour transactions
    │ • Last 24 hours transactions
    │
    ▼
[Compute Spending Patterns]
    │ • Rolling average (7d, 30d)
    │ • Standard deviation (30d)
    │ • Z-score: (amount - mean) / stddev
    │ • Amount deviation from baseline
    │
    ▼
[Compute Velocity Metrics]
    │ • Transactions per hour (last 1h)
    │ • Transactions per day (last 24h)
    │ • Velocity z-score (vs historical)
    │ • Time since last transaction
    │
    ▼
[Compute Location Patterns]
    │ • Unique locations (last 7d)
    │ • Location change count
    │ • Distance from last transaction (km)
    │ • Impossible travel detection (speed > 900 km/h)
    │ • Is new location? (not seen in 7d)
    │ • High-risk country check
    │
    ▼
[Compute Merchant Patterns]
    │ • Is new merchant? (not seen in 7d)
    │ • Merchant risk score (historical)
    │ • Merchant category analysis
    │
    ▼
[Compute Temporal Patterns]
    │ • Is unusual hour? (vs user's pattern)
    │ • Day of week anomaly
    │ • Time since last transaction (hours)
    │
    ▼
[Compute Sequence Patterns]
    │ • Recent small transactions count (last 10 min)
    │ • Follows probe pattern? (small → large)
    │ • Shared beneficiary count (mule detection)
    │
    ▼
[Compute Peer Group Metrics]
    │ • Peer group average spend
    │ • Deviation from peer group (z-score)
    │ • Similar accounts comparison
    │
    ▼
[Compute Behavioral Anomalies]
    │ • Composite behavioral anomaly score
    │ • Anomaly ratio (flagged / total)
    │ • Channel switch count
    │ • Device change detection
    │
    ▼
[Feature Set Complete]
    │ • 30+ features computed
    │ • Ready for scoring
```

**Example Feature Values:**
```json
{
  "rolling_avg_spend_7d": 450.00,
  "rolling_avg_spend_30d": 520.00,
  "rolling_std_dev_30d": 150.00,
  "spending_z_score": 2.5,
  "transaction_velocity_1h": 3,
  "transaction_velocity_24h": 15,
  "velocity_z_score": 1.8,
  "unique_locations_7d": 2,
  "location_change_count": 1,
  "is_new_location": true,
  "distance_from_last_tx_km": 250.5,
  "is_new_merchant": false,
  "merchant_risk_score": 12.5,
  "time_since_last_tx_hours": 4.5,
  "is_unusual_hour": false,
  "recent_small_tx_count": 0,
  "follows_probe_pattern": false,
  "peer_group_avg_spend": 480.00,
  "peer_group_deviation": 0.5,
  "behavioral_anomaly_score": 25.0
}
```

### Hybrid Scoring Architecture 🧠

The system uses a modern **hybrid scoring model** combining multiple signal sources:

```
┌─────────────────────────────────────────────────────────────────┐
│                    HYBRID RISK SCORING                          │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│   Transaction + Features                                        │
│       │                                                         │
│       ├─────────────────────────────────────────────────────┐  │
│       │                                                     │  │
│       ▼                                                     ▼  │
│   ┌──────────────────┐                          ┌──────────────────┐
│   │   RULE ENGINE     │                          │  A/B TEST CHECK  │
│   │   (50% weight)    │                          │  (if active)      │
│   │                   │                          └────────┬─────────┘
│   │ • 15+ rules       │                                   │
│   │ • Priority order  │                                   │
│   │ • Score impact    │                                   │
│   │ • Rule score:     │                                   │
│   │   0-100           │                                   │
│   └─────────┬─────────┘                                   │
│             │                                             │
│             └──────────────────┬──────────────────────────┘
│                                │
│                                ▼
│   ┌─────────────────────────────────────────────────────────┐  │
│   │              BEHAVIORAL ANALYSIS (35% weight)           │  │
│   │  • Z-score based anomaly detection                       │  │
│   │  • Spending pattern deviation                            │  │
│   │  • Velocity anomalies                                    │  │
│   │  • Temporal pattern analysis                             │  │
│   │  • Behavioral score: 0-100                                │  │
│   └─────────────────────────────────────────────────────────┘  │
│                                │                                 │
│                                ▼                                 │
│   ┌─────────────────────────────────────────────────────────┐  │
│   │              ML SCORER (15% weight, optional)           │  │
│   │  • Pluggable ML model interface                         │  │
│   │  • Current: Lightweight ensemble                        │  │
│   │  • Future: External service (SageMaker, Vertex AI)      │  │
│   │  • ML score: 0-100 (nullable)                           │  │
│   └─────────────────────────────────────────────────────────┘  │
│                                │                                 │
│                                ▼                                 │
│   ┌─────────────────────────────────────────────────────────┐  │
│   │              FINAL SCORE CALCULATION                     │  │
│   │                                                         │  │
│   │  if ml_score is not None:                               │  │
│   │    final = (0.50 × rule_score) +                        │  │
│   │            (0.35 × behavioral_score) +                  │  │
│   │            (0.15 × ml_score)                            │  │
│   │  else:                                                  │  │
│   │    final = (0.59 × rule_score) +                        │  │
│   │            (0.41 × behavioral_score)                     │  │
│   │                                                         │  │
│   │  final = min(final, 100)  // Cap at 100                │  │
│   └─────────────────────────────────────────────────────────┘  │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

**Score Breakdown (stored with each transaction):**
```json
{
  "score": 42.5,              // Final composite score (0-100)
  "rule_score": 35.0,          // From rule engine (0-100)
  "behavioral_score": 55.0,    // From behavioral analysis (0-100)
  "ml_score": 48.0,           // From ML model (0-100, nullable)
  "risk_level": "medium",      // low/medium/high/critical
  "rules_triggered": ["RULE_VELOCITY_BURST", "RULE_SPIKE_ANOMALY"],
  "anomalies_detected": ["SPENDING_SPIKE", "PEER_GROUP_DEVIATION"],
  "scoring_path": "full",      // "fast" or "full"
  "model_version": "v2.0.0-hybrid",
  "features": {
    "rolling_avg_spend_30d": 520.0,
    "spending_z_score": 2.5,
    "velocity_z_score": 1.8,
    // ... 30+ features
  }
}
```

**ML Integration (Pluggable):**
- **Current**: Lightweight behavioral z-score ensemble (built-in)
- **Future**: External ML service (SageMaker, Vertex AI, custom)
- **Interface**: `MLScorerInterface` for easy swapping
- **Fallback**: If ML unavailable, weight redistributed to rules (60%) and behavioral (40%)

### A/B Testing Flow

The system supports A/B testing of scoring rules to measure impact:

```
1. Create Experiment
   POST /api/v1/experiments
   {
     "name": "Stricter Velocity Rules",
     "control_rules": ["RULE_VELOCITY_BURST"],
     "test_rules": ["RULE_VELOCITY_BURST", "RULE_RAPID_SMALL_TRANSACTIONS"],
     "traffic_split": 0.2  // 20% to test group
   }

2. Start Experiment
   POST /api/v1/experiments/{id}/start
   │
   ▼
3. Transaction Scoring (with A/B assignment)
   │
   ├─► Account ID → Consistent Hashing → Group Assignment
   │   • Same account always in same group
   │   • Traffic split: 80% control, 20% test
   │
   ├─► Control Group
   │   • Uses control_rules
   │   • Standard scoring
   │
   └─► Test Group
       • Uses test_rules
       • Experimental scoring
       • Results tracked separately
   │
   ▼
4. Results Aggregation
   │ • Control group metrics
   │ • Test group metrics
   │ • Statistical significance calculation
   │
   ▼
5. Analysis
   GET /api/v1/experiments/{id}/results
   GET /api/v1/experiments/{id}/significance
   │
   └─► Decision: Keep test rules? Stop experiment?
```

**A/B Testing Features:**
- **Consistent Assignment**: Same account always in same group (via consistent hashing)
- **Traffic Splitting**: Configurable split (e.g., 10%, 20%, 50%)
- **Statistical Significance**: P-value, confidence intervals
- **Real-time Tracking**: Results updated as transactions flow
- **Comparison Metrics**: Score differences, flag rate differences

### Backtesting Flow

Replay historical transactions with new rule sets:

```
1. Submit Backtest Request
   POST /api/v1/backtest/run
   {
     "account_id": "...",
     "start_date": "2026-01-01T00:00:00Z",
     "end_date": "2026-02-01T00:00:00Z",
     "sample_size": 100
   }
   │
   ▼
2. Fetch Historical Transactions
   │ • Query transactions in date range
   │ • Sample if sample_size specified
   │ • Order by created_at
   │
   ▼
3. Re-score Each Transaction
   │ For each transaction:
   │   • Load transaction details
   │   • Compute features (using historical context)
   │   • Apply current rule set
   │   • Calculate new score
   │   • Compare with original score
   │
   ▼
4. Aggregate Results
   │ • Total transactions processed
   │ • Average score
   │ • Risk distribution
   │ • Top triggered rules
   │ • Comparison with live scores:
   │   - Matching scores count
   │   - Different scores count
   │   - Upgraded risk count
   │   - Downgraded risk count
   │
   ▼
5. Return Results
   {
     "total_transactions": 100,
     "processed_count": 98,
     "average_score": 22.5,
     "risk_distribution": {...},
     "comparison_with_live": {...}
   }
```

**Backtesting Use Cases:**
- Test new rules before deployment
- Measure impact of rule changes
- Validate rule effectiveness
- Compare scoring models

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

- **Language**: Go 1.24+ (compatible with Go 1.21+)
- **Web Framework**: Gin
- **Database**: PostgreSQL 15+ (with table partitioning)
- **Message Queue**: Redis Streams (fast path), Kafka (optional, for CDC analytics)
- **Cache**: Redis
- **Auth**: JWT (HS256)
- **Container**: Docker & Docker Compose
- **CDC**: Debezium (optional, for Kafka CDC pipeline)

## 🚀 Quick Start

### Prerequisites

- **Go 1.24+** (or Go 1.21+ for compatibility)
- **Docker & Docker Compose** (for infrastructure)
- **PostgreSQL 15+** (or use Docker)
- **Redis 7+** (or use Docker)
- **Make** (optional, for convenience commands)

### Option 1: Docker Compose (Recommended for First-Time Setup)

This is the easiest way to get everything running:

```bash
# 1. Clone the repository
git clone https://github.com/yourusername/enterprise-risk-engine.git
cd enterprise-risk-engine

# 2. Start all services (PostgreSQL, Redis, API, Workers, Dashboard)
docker-compose up -d

# 3. Wait for services to be ready (about 10-15 seconds)
docker-compose ps

# 4. Run database migrations
make migrate-docker
# OR manually:
# docker exec -i risk-engine-postgres psql -U postgres -d risk_engine < db/migrations/001_initial_schema.sql
# docker exec -i risk-engine-postgres psql -U postgres -d risk_engine < db/migrations/002_create_partitions.sql
# docker exec -i risk-engine-postgres psql -U postgres -d risk_engine < db/migrations/003_seed_rules.sql

# 5. Verify services are running
curl http://localhost:8080/health
# Should return: {"status":"healthy","timestamp":"..."}

# 6. Access the dashboard
# Open http://localhost:3000 in your browser
```

**Services Available:**
- **API Server**: http://localhost:8080
- **Dashboard**: http://localhost:3000
- **PostgreSQL**: localhost:5437 (mapped from container port 5432)
- **Redis**: localhost:6382 (mapped from container port 6379)

### Option 2: Local Development (For Active Development)

For active development with hot-reload capabilities:

```bash
# 1. Start only infrastructure (PostgreSQL + Redis)
docker-compose up -d postgres redis

# 2. Set up environment variables
cp configs/env.example .env
# Edit .env if needed (defaults work for local Docker setup)

# 3. Run database migrations
export DATABASE_URL="postgres://postgres:postgres@localhost:5437/risk_engine?sslmode=disable"
psql $DATABASE_URL -f db/migrations/001_initial_schema.sql
psql $DATABASE_URL -f db/migrations/002_create_partitions.sql
psql $DATABASE_URL -f db/migrations/003_seed_rules.sql

# 4. Start API server (Terminal 1)
go run ./cmd/api-server
# API will be available at http://localhost:8080

# 5. Start worker (Terminal 2)
go run ./cmd/worker
# Worker will start processing transactions from Redis Streams

# 6. (Optional) Start dashboard (Terminal 3)
make dashboard
# Dashboard will be available at http://localhost:3000
```

### Option 3: Using Make Commands

The project includes a comprehensive Makefile:

```bash
# Start development environment (PostgreSQL + Redis only)
make dev

# Build binaries
make build

# Run API server
make run-api

# Run worker
make run-worker

# Run tests
make test

# Run API test script
make test-api

# View all available commands
make help
```

### First Transaction Flow Example

Let's walk through what happens when you submit your first transaction:

```bash
# 1. Register a user
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "email": "test@example.com",
    "password": "SecurePass123",
    "role": "admin"
  }'

# Response: {"token":"eyJhbGci...","expires_in":86400,"user":{...}}

# 2. Create an account
curl -X POST http://localhost:8080/api/v1/accounts \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "<user_id_from_step_1>",
    "account_type": "standard"
  }'

# Response: {"id":"<account_id>","user_id":"...","risk_profile":"low",...}

# 3. Submit a transaction
curl -X POST http://localhost:8080/api/v1/transactions \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "account_id": "<account_id_from_step_2>",
    "amount": 1500.00,
    "currency": "USD",
    "merchant": "Amazon",
    "merchant_category": "retail",
    "location": "New York, NY",
    "country": "US",
    "channel": "online",
    "idempotency_key": "tx-001-$(date +%s)"
  }'

# Response: {
#   "transaction_id": "550e8400-...",
#   "status": "pending",
#   "idempotency_key": "tx-001-...",
#   "created_at": "2026-02-03T10:30:00Z"
# }

# 4. Check transaction status (after ~50-150ms)
curl http://localhost:8080/api/v1/transactions/<transaction_id> \
  -H "Authorization: Bearer <token>"

# Response: {
#   "id": "...",
#   "status": "processed",  // or "flagged" or "blocked"
#   "amount": 1500.00,
#   ...
# }

# 5. Get risk score details
curl http://localhost:8080/api/v1/risk/account/<account_id> \
  -H "Authorization: Bearer <token>"

# Response: {
#   "account_id": "...",
#   "current_risk_level": "low",
#   "avg_transaction_amount": 1500.00,
#   "transaction_count_30d": 1,
#   "flagged_count_30d": 0,
#   ...
# }
```

**What Happened Behind the Scenes:**

1. **API Layer** (0-5ms):
   - Rate limiting check passed
   - JWT token validated
   - Request logged with correlation ID

2. **Ingestion** (5-20ms):
   - Transaction validated
   - Idempotency key checked (no duplicates)
   - Account verified (exists and active)
   - Transaction saved to PostgreSQL with status "pending"
   - Event published to Redis Streams
   - API returns HTTP 201

3. **Scoring** (20-170ms, async):
   - Worker consumes event from Redis Stream
   - Fetches transaction and account details
   - Computes 30+ risk features
   - Applies 15+ scoring rules
   - Calculates behavioral score
   - (Optional) Gets ML score
   - Computes final hybrid score
   - Updates transaction status (processed/flagged/blocked)
   - Saves risk score to database
   - Caches result in Redis

4. **Result**:
   - Transaction status updated
   - Risk score stored with full breakdown
   - Account risk profile updated if needed
   - Analytics metrics updated

### Dashboard

The system includes a sleek, minimalistic dashboard for real-time monitoring:

```bash
# With Docker (recommended) - Dashboard available at http://localhost:3000
docker-compose up -d

# Or serve locally with Python (API must be running on :8080)
make dashboard
```

**Dashboard Features:**
- 📊 **Real-time System Metrics**: TPS, latency, error rate, queue depth
- 📈 **Risk Distribution**: Visual breakdown of risk levels
- 🚩 **Flagged Transactions**: View high-risk transactions with rule details
- 🧪 **A/B Testing**: Create and manage scoring experiments
- 🔍 **Transaction Search**: Search by account, date range, or risk level
- 📉 **Trend Analysis**: Historical risk trends and patterns
- 🌙 **Dark Theme**: Beautiful UI with smooth animations

**Access:**
- URL: http://localhost:3000
- Default login: `admin@example.com` / `admin123` (if seeded)

### Kafka CDC Setup (Optional - for Analytics Pipeline)

To enable the full hybrid architecture with Kafka CDC:

```bash
# Start all services including Kafka ecosystem
make kafka-up

# Wait for services to be ready (~30 seconds)
docker-compose ps

# Set up Debezium CDC connector
make debezium-setup

# Access Kafka UI
make kafka-ui
# Opens http://localhost:8090

# View Kafka worker logs
make kafka-logs
```

**Kafka Services:**
- **Kafka Broker**: localhost:9095
- **Kafka UI**: http://localhost:8090
- **Debezium Connect**: http://localhost:8083

**Note**: Kafka is optional. The system works perfectly with just Redis Streams for scoring. Kafka adds:
- Complete audit trail
- Event replay capability
- ML training data collection
- Data lake synchronization

### Stop All Services

```bash
# Stop all services
docker-compose down

# Stop and remove volumes (clean slate)
docker-compose down -v

# Stop only Kafka services
make kafka-down
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

## 🔄 Complete Request/Response Cycle

### Example: Transaction Ingestion with Timing

```
Time    Component              Action                                    Duration
─────────────────────────────────────────────────────────────────────────────
0ms     Client                 HTTP POST /api/v1/transactions
                               
5ms     API Gateway            • Rate limit check                         2ms
                               • JWT validation
                               • Request ID generation
                               
10ms    Ingestion Module       • Validate payload                        5ms
                               • Check idempotency key
                               • Verify account exists
                               
20ms    PostgreSQL             • INSERT transaction (status: pending)    8ms
                               • Transaction ID returned
                               
25ms    Redis Streams          • Publish event to stream                 3ms
                               • Message ID returned
                               
30ms    API Response           • HTTP 201 Created                        2ms
                               • Transaction ID in response
                               • Status: "pending"
                               
        [ASYNC PROCESSING STARTS]
                               
35ms    Worker                 • Consume event from Redis Stream         5ms
                               • Message acknowledged in group
                               
45ms    PostgreSQL             • SELECT transaction details               8ms
                               • SELECT account details
                               • SELECT recent transactions (for features)
                               
60ms    Feature Computation    • Compute 30+ features                    25ms
                               • Historical analysis
                               • Pattern detection
                               
90ms    Rule Engine            • Evaluate 15+ rules                      15ms
                               • Calculate rule_score: 35.0
                               
110ms   Behavioral Analysis    • Z-score calculations                   20ms
                               • Anomaly detection
                               • Calculate behavioral_score: 55.0
                               
125ms   ML Scorer              • ML model inference (if enabled)         15ms
                               • Calculate ml_score: 48.0
                               
140ms   Final Score            • Weighted combination                    5ms
                               • final_score = 42.5
                               • risk_level = "medium"
                               
150ms   PostgreSQL             • UPDATE transaction status               8ms
                               • INSERT risk_score record
                               • UPDATE account risk profile (if needed)
                               
160ms   Redis Cache            • Cache risk score (24h TTL)              3ms
                               • Update account profile cache
                               
165ms   Processing Complete    • Transaction fully scored
                               • Status: "processed"
                               • Available via API
```

**Total Time:**
- **API Response**: ~30ms (transaction created, scoring queued)
- **Scoring Complete**: ~165ms (full processing done)
- **User Experience**: Immediate response, scoring happens async

### Request Flow Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                    COMPLETE REQUEST FLOW                          │
└─────────────────────────────────────────────────────────────────┘

Client Request
    │
    ▼
┌─────────────────────────────────────────────────────────────────┐
│ 1. API GATEWAY (0-5ms)                                          │
│    • Rate Limiting: 100 req/min per IP                          │
│    • JWT Authentication                                         │
│    • CORS Handling                                              │
│    • Request ID Generation                                      │
└─────────────────────────────────────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────────────────────────────────────┐
│ 2. INGESTION MODULE (5-25ms)                                    │
│    • Payload Validation                                         │
│    • Idempotency Check (prevent duplicates)                     │
│    • Account Verification                                       │
│    • Transaction Creation (PostgreSQL)                           │
│    • Event Publishing (Redis Streams)                           │
│    • Audit Log Creation                                         │
└─────────────────────────────────────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────────────────────────────────────┐
│ 3. API RESPONSE (25-30ms)                                       │
│    • HTTP 201 Created                                           │
│    • Transaction ID returned                                    │
│    • Status: "pending"                                          │
│    • Client can proceed (scoring is async)                      │
└─────────────────────────────────────────────────────────────────┘

[ASYNC PATH - Non-blocking]

    │
    ▼
┌─────────────────────────────────────────────────────────────────┐
│ 4. WORKER CONSUMPTION (30-40ms)                                 │
│    • Consumer group ensures no duplicate processing             │
│    • Batch processing (100 messages at a time)                 │
│    • Message claimed from Redis Stream                         │
└─────────────────────────────────────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────────────────────────────────────┐
│ 5. DATA FETCHING (40-60ms)                                      │
│    • Load transaction from PostgreSQL                          │
│    • Load account details                                       │
│    • Load historical transactions (7d, 30d windows)           │
│    • Load peer group data (if available)                       │
└─────────────────────────────────────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────────────────────────────────────┐
│ 6. FEATURE COMPUTATION (60-90ms)                                 │
│    • 30+ features computed                                      │
│    • Spending patterns, velocity, location, etc.               │
│    • Anomaly detection                                          │
│    • Peer group comparison                                     │
└─────────────────────────────────────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────────────────────────────────────┐
│ 7. SCORING (90-140ms)                                           │
│    ├─► Rule Engine (90-110ms)                                   │
│    │   • Evaluate 15+ rules                                    │
│    │   • Calculate rule_score                                   │
│    │                                                             │
│    ├─► Behavioral Analysis (110-130ms)                          │
│    │   • Z-score calculations                                   │
│    │   • Calculate behavioral_score                             │
│    │                                                             │
│    └─► ML Scorer (130-140ms, optional)                          │
│        • ML model inference                                      │
│        • Calculate ml_score                                      │
└─────────────────────────────────────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────────────────────────────────────┐
│ 8. FINAL SCORE CALCULATION (140-150ms)                          │
│    • Weighted combination:                                      │
│      final = 0.50×rule + 0.35×behavioral + 0.15×ml              │
│    • Risk level determination                                   │
│    • Transaction status update (processed/flagged/blocked)      │
└─────────────────────────────────────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────────────────────────────────────┐
│ 9. PERSISTENCE (150-165ms)                                      │
│    • Update transaction status in PostgreSQL                     │
│    • Save risk_score record                                      │
│    • Update account risk profile (if escalated)                 │
│    • Cache results in Redis                                     │
│    • Acknowledge message (remove from pending)                  │
└─────────────────────────────────────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────────────────────────────────────┐
│ 10. COMPLETE (165ms)                                            │
│     • Transaction fully processed                                │
│     • Risk score available via API                                   │
│     • Analytics updated                                          │
└─────────────────────────────────────────────────────────────────┘
```

## 🐛 Troubleshooting

### Common Issues and Solutions

#### Issue: Transactions stuck in "pending" status

**Symptoms:**
- Transactions created but never scored
- Status remains "pending" indefinitely

**Diagnosis:**
```bash
# Check if workers are running
docker-compose ps worker

# Check worker logs
docker-compose logs worker

# Check Redis Stream depth
redis-cli XLEN transactions

# Check pending messages
redis-cli XPENDING transactions scoring-workers
```

**Solutions:**
1. **Workers not running**: Start workers with `docker-compose up -d worker`
2. **Redis connection issue**: Check `REDIS_URL` environment variable
3. **Database connection issue**: Check `DATABASE_URL` and connection pool settings
4. **Consumer group not created**: Workers should auto-create, but you can manually create:
   ```bash
   redis-cli XGROUP CREATE transactions scoring-workers 0
   ```

#### Issue: High latency (>500ms)

**Symptoms:**
- Transactions taking too long to score
- API responses slow

**Diagnosis:**
```bash
# Check system metrics
curl http://localhost:8080/api/v1/metrics/system \
  -H "Authorization: Bearer <token>"

# Check database connections
# Look for: db_connections_active, db_connections_idle

# Check queue depth
# Look for: queue_depth (should be < 100)
```

**Solutions:**
1. **Database bottleneck**: Increase connection pool size or add read replicas
2. **Queue backlog**: Scale up workers: `docker-compose --profile scale up -d`
3. **Slow queries**: Check database indexes, use EXPLAIN ANALYZE
4. **Network latency**: Check Redis/PostgreSQL network connectivity

#### Issue: Rate limiting errors (429)

**Symptoms:**
- API returns `429 Too Many Requests`
- `Retry-After` header present

**Solutions:**
1. **Wait**: Rate limit is 100 requests/minute per IP
2. **Distribute load**: Use multiple IPs or implement client-side rate limiting
3. **Adjust limit**: Modify rate limiter configuration in code (not recommended for production)

#### Issue: Duplicate transactions

**Symptoms:**
- Same transaction processed multiple times
- Duplicate risk scores

**Solutions:**
1. **Use idempotency keys**: Always provide unique `idempotency_key` per transaction
2. **Check idempotency**: System should return existing transaction if key matches
3. **Verify uniqueness**: Ensure idempotency keys are truly unique

#### Issue: Workers not processing messages

**Symptoms:**
- Messages accumulating in Redis Stream
- No worker activity in logs

**Diagnosis:**
```bash
# Check consumer group status
redis-cli XINFO GROUPS transactions

# Check pending messages
redis-cli XPENDING transactions scoring-workers

# Check worker logs
docker-compose logs -f worker
```

**Solutions:**
1. **Restart workers**: `docker-compose restart worker`
2. **Check consumer group**: Ensure group name matches in config
3. **Check Redis connectivity**: Verify `REDIS_URL` is correct
4. **Claim pending messages**: Workers should auto-claim, but you can manually:
   ```bash
   # Workers will claim messages after 30s idle time
   ```

#### Issue: Database connection errors

**Symptoms:**
- `connection refused` errors
- `too many connections` errors

**Solutions:**
1. **Check PostgreSQL is running**: `docker-compose ps postgres`
2. **Check connection string**: Verify `DATABASE_URL` format
3. **Reduce connection pool**: Lower `DB_MAX_OPEN_CONNS` if hitting limits
4. **Check PostgreSQL max connections**: Default is 100, may need to increase

#### Issue: Cache misses

**Symptoms:**
- Slow API responses for risk scores
- Direct database queries instead of cache

**Solutions:**
1. **Check Redis connectivity**: Verify `REDIS_URL`
2. **Check cache TTL**: Risk scores cached for 24h, profiles for 5m
3. **Monitor cache hit rate**: Should be > 70% for risk scores
4. **Warm cache**: Pre-load frequently accessed data

### Debugging Commands

```bash
# View all service logs
docker-compose logs -f

# View specific service logs
docker-compose logs -f api-server
docker-compose logs -f worker

# Check service health
curl http://localhost:8080/health

# Check system metrics
curl http://localhost:8080/api/v1/metrics/system \
  -H "Authorization: Bearer <token>"

# Check Redis Stream info
redis-cli XINFO STREAM transactions

# Check database connections
docker exec risk-engine-postgres psql -U postgres -d risk_engine \
  -c "SELECT count(*) FROM pg_stat_activity;"

# Check recent transactions
docker exec risk-engine-postgres psql -U postgres -d risk_engine \
  -c "SELECT id, status, created_at FROM transactions ORDER BY created_at DESC LIMIT 10;"

# Check risk scores
docker exec risk-engine-postgres psql -U postgres -d risk_engine \
  -c "SELECT transaction_id, score, risk_level, created_at FROM risk_scores ORDER BY created_at DESC LIMIT 10;"
```

### Performance Tuning

**For Higher Throughput:**
1. Increase worker concurrency: `WORKER_CONCURRENCY=10`
2. Increase batch size: `WORKER_BATCH_SIZE=200`
3. Scale workers: `docker-compose --profile scale up -d`
4. Optimize database queries (add indexes)
5. Use connection pooling effectively

**For Lower Latency:**
1. Enable fast-path scoring (automatic for low-risk)
2. Increase Redis cache hit rate
3. Use read replicas for analytics queries
4. Optimize feature computation (cache historical data)
5. Reduce database round trips (batch queries)

## 🔮 Future AWS Migration Path

| Current | AWS Equivalent | Benefits |
|---------|---------------|----------|
| Redis Streams | Amazon MSK (Kafka) | Higher throughput, better durability |
| Render PostgreSQL | Amazon RDS | Multi-AZ, automated backups, read replicas |
| Docker on Render | Amazon ECS/EKS | Auto-scaling, better orchestration |
| Basic Metrics | Amazon CloudWatch + Prometheus | Comprehensive observability |
| Manual Scaling | AWS Auto Scaling | Automatic scaling based on metrics |
| Behavioral Scoring | Amazon SageMaker | Advanced ML models, training pipelines |
| Rate Limiting (in-memory) | AWS API Gateway | Distributed rate limiting, WAF integration |

**Migration Strategy:**
1. **Phase 1**: Move to RDS (database migration)
2. **Phase 2**: Replace Redis Streams with MSK (message queue migration)
3. **Phase 3**: Deploy to ECS/EKS (container orchestration)
4. **Phase 4**: Integrate CloudWatch/Prometheus (observability)
5. **Phase 5**: Connect to SageMaker (ML integration)

## 📚 Additional Resources

- **Architecture Details**: See [docs/architecture.md](docs/architecture.md)
- **Data Model**: See [docs/data-model.md](docs/data-model.md)
- **Scaling Strategy**: See [docs/scaling-strategy.md](docs/scaling-strategy.md)
- **API Examples**: See [scripts/test_api.sh](scripts/test_api.sh)
- **Load Testing**: See [scripts/load_test.js](scripts/load_test.js)

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

**Development Guidelines:**
- Follow Go best practices and conventions
- Add tests for new features
- Update documentation
- Ensure backward compatibility
- Run `make test` before submitting PR

## 📄 License

MIT License - see [LICENSE](LICENSE) for details.

---

**Built with ❤️ for enterprise-grade risk management**

*For questions, issues, or contributions, please open an issue on GitHub.*
