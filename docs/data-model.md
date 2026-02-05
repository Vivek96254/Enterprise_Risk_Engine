# Data Model Documentation

## Overview

This document describes the database schema, relationships, and data flow for the Enterprise Risk Engine.

## Entity Descriptions

### Users

Represents system users who can access the API.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | UUID | PK, DEFAULT uuid_generate_v4() | Unique identifier |
| email | VARCHAR(255) | UNIQUE, NOT NULL | User email address |
| password_hash | VARCHAR(255) | NOT NULL | Bcrypt hashed password |
| role | VARCHAR(50) | NOT NULL, CHECK | User role (admin, analyst, user) |
| created_at | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Creation timestamp |
| updated_at | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Last update timestamp |
| deleted_at | TIMESTAMPTZ | NULL | Soft delete timestamp |

**Indexes:**
- `idx_users_email` - Email lookup (WHERE deleted_at IS NULL)
- `idx_users_created_at` - Time-based queries

### Accounts

Represents financial accounts that can have transactions.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | UUID | PK, DEFAULT uuid_generate_v4() | Unique identifier |
| user_id | UUID | FK → users(id), NOT NULL | Owner user |
| account_type | VARCHAR(50) | NOT NULL, CHECK | Type (standard, premium, business) |
| risk_profile | VARCHAR(20) | NOT NULL, CHECK | Risk level (low, medium, high) |
| status | VARCHAR(20) | NOT NULL, CHECK | Status (active, suspended, closed) |
| created_at | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Creation timestamp |
| updated_at | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Last update timestamp |

**Indexes:**
- `idx_accounts_user_id` - User's accounts lookup
- `idx_accounts_risk_profile` - Risk-based queries
- `idx_accounts_status` - Status filtering

### Transactions (Partitioned)

Represents financial transactions. **Partitioned by month** for performance.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | UUID | PK (composite), DEFAULT uuid_generate_v4() | Unique identifier |
| account_id | UUID | NOT NULL | Associated account |
| amount | DECIMAL(15,2) | NOT NULL | Transaction amount |
| currency | VARCHAR(3) | NOT NULL, DEFAULT 'USD' | Currency code |
| merchant | VARCHAR(255) | NULL | Merchant name |
| merchant_category | VARCHAR(100) | NULL | Merchant category |
| location | VARCHAR(255) | NULL | Transaction location |
| country | VARCHAR(3) | NULL | Country code |
| channel | VARCHAR(20) | NOT NULL, CHECK | Channel (online, pos, atm) |
| status | VARCHAR(20) | NOT NULL, CHECK | Status (pending, processed, flagged, blocked) |
| idempotency_key | VARCHAR(255) | NOT NULL | Deduplication key |
| metadata | JSONB | DEFAULT '{}' | Additional data |
| created_at | TIMESTAMPTZ | PK (composite), NOT NULL | Creation timestamp |
| processed_at | TIMESTAMPTZ | NULL | Processing completion time |

**Partition Key:** `created_at` (RANGE by month)

**Indexes:**
- `idx_transactions_account_created` - Account history queries (composite)
- `idx_transactions_status` - Status filtering
- `idx_transactions_idempotency` - Deduplication check
- `idx_transactions_merchant` - Merchant search (GIN trigram)
- `idx_transactions_location` - Location queries
- `idx_transactions_country` - Country filtering

**Partitions:**
```sql
transactions_2026_01 FOR VALUES FROM ('2026-01-01') TO ('2026-02-01')
transactions_2026_02 FOR VALUES FROM ('2026-02-01') TO ('2026-03-01')
-- ... continues for each month
```

### Risk Scores

Stores computed risk scores for transactions.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | UUID | PK, DEFAULT uuid_generate_v4() | Unique identifier |
| transaction_id | UUID | FK (composite), NOT NULL | Associated transaction |
| transaction_created_at | TIMESTAMPTZ | FK (composite), NOT NULL | Transaction timestamp |
| score | DECIMAL(5,2) | NOT NULL, CHECK 0-100 | Risk score |
| risk_level | VARCHAR(20) | NOT NULL, CHECK | Level (low, medium, high, critical) |
| rules_triggered | TEXT[] | DEFAULT '{}' | Array of triggered rule IDs |
| features | JSONB | DEFAULT '{}' | Computed features |
| model_version | VARCHAR(50) | NOT NULL | Scoring model version |
| processing_time_ms | INTEGER | NOT NULL, DEFAULT 0 | Processing duration |
| created_at | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Creation timestamp |

**Foreign Key:** Composite reference to `transactions(id, created_at)` for partition pruning.

**Indexes:**
- `idx_risk_scores_transaction` - Transaction lookup
- `idx_risk_scores_risk_level` - Risk level filtering
- `idx_risk_scores_score` - Score-based sorting (DESC)
- `idx_risk_scores_created_at` - Time-based queries
- `idx_risk_scores_rules` - Rules analysis (GIN)

### Audit Logs

Immutable audit trail for all system events.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | UUID | PK, DEFAULT uuid_generate_v4() | Unique identifier |
| event_type | VARCHAR(50) | NOT NULL | Event category |
| entity_id | UUID | NULL | Related entity ID |
| entity_type | VARCHAR(50) | NULL | Related entity type |
| user_id | UUID | FK → users(id), NULL | Acting user |
| action | VARCHAR(50) | NOT NULL | Action performed |
| payload | JSONB | DEFAULT '{}' | Event details |
| ip_address | INET | NULL | Client IP address |
| user_agent | TEXT | NULL | Client user agent |
| request_id | VARCHAR(100) | NULL | Request correlation ID |
| created_at | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Event timestamp |

**Indexes:**
- `idx_audit_logs_event_type` - Event type filtering
- `idx_audit_logs_entity` - Entity lookup (composite)
- `idx_audit_logs_user_id` - User activity
- `idx_audit_logs_created_at` - Time-based queries
- `idx_audit_logs_request_id` - Request tracing

### Rules

Configurable scoring rules stored in database.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | VARCHAR(100) | PK | Rule identifier |
| name | VARCHAR(255) | NOT NULL | Display name |
| description | TEXT | NULL | Rule description |
| condition | JSONB | NOT NULL | Rule condition definition |
| score_impact | DECIMAL(5,2) | NOT NULL | Score contribution |
| risk_level | VARCHAR(20) | NOT NULL, CHECK | Associated risk level |
| priority | INTEGER | NOT NULL, DEFAULT 100 | Evaluation order |
| enabled | BOOLEAN | NOT NULL, DEFAULT true | Active flag |
| created_at | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Creation timestamp |
| updated_at | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Last update timestamp |

**Rule Condition Format:**
```json
{
  "type": "threshold",
  "field": "amount",
  "operator": ">",
  "value": 10000
}
```

```json
{
  "type": "compound",
  "operator": "AND",
  "conditions": [
    {"field": "is_new_location", "operator": "=", "value": true},
    {"field": "amount", "operator": ">", "value": 1000}
  ]
}
```

### Daily Aggregates

Pre-computed daily statistics for fast analytics.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| date | DATE | PK (composite), NOT NULL | Aggregation date |
| account_id | UUID | PK (composite), NULL | Account (NULL for global) |
| total_transactions | INTEGER | NOT NULL, DEFAULT 0 | Transaction count |
| total_amount | DECIMAL(20,2) | NOT NULL, DEFAULT 0 | Total amount |
| flagged_count | INTEGER | NOT NULL, DEFAULT 0 | Flagged transactions |
| blocked_count | INTEGER | NOT NULL, DEFAULT 0 | Blocked transactions |
| avg_risk_score | DECIMAL(5,2) | NULL | Average risk score |
| high_risk_count | INTEGER | NOT NULL, DEFAULT 0 | High risk count |
| critical_risk_count | INTEGER | NOT NULL, DEFAULT 0 | Critical risk count |
| top_rules_triggered | JSONB | DEFAULT '[]' | Top rules with counts |
| created_at | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Creation timestamp |
| updated_at | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Last update timestamp |

## Data Relationships

```
users (1) ──────────────< accounts (N)
                              │
                              │ (1)
                              │
                              ▼
                         transactions (N)
                              │
                              │ (1)
                              │
                              ▼
                         risk_scores (1)

users (1) ──────────────< audit_logs (N)
```

## Risk Features Schema

The `features` JSONB column in `risk_scores` contains:

```json
{
  "rolling_avg_spend_7d": 450.00,
  "rolling_avg_spend_30d": 520.00,
  "transaction_velocity_1h": 3,
  "transaction_velocity_24h": 15,
  "unique_locations_7d": 2,
  "location_change_count": 1,
  "amount_deviation": 2.5,
  "anomaly_ratio": 0.05,
  "is_new_merchant": false,
  "is_new_location": true,
  "is_high_risk_country": false,
  "time_since_last_tx_hours": 4.5
}
```

## Partition Management

### Creating Future Partitions

```sql
-- Function to create partition for a given month
SELECT create_transaction_partition('2027-01-01'::date);

-- This creates:
-- transactions_2027_01 FOR VALUES FROM ('2027-01-01') TO ('2027-02-01')
```

### Archiving Old Partitions

```sql
-- Detach old partition (for archiving)
ALTER TABLE transactions DETACH PARTITION transactions_2025_01;

-- Export to cold storage
COPY transactions_2025_01 TO '/backup/transactions_2025_01.csv' CSV HEADER;

-- Drop if no longer needed
DROP TABLE transactions_2025_01;
```

## Query Patterns

### High-Performance Queries

**Account Transaction History:**
```sql
SELECT * FROM transactions
WHERE account_id = $1
  AND created_at >= $2
  AND created_at < $3
ORDER BY created_at DESC
LIMIT 100;
-- Uses: idx_transactions_account_created + partition pruning
```

**Daily Risk Summary:**
```sql
SELECT 
    COUNT(*) as total,
    AVG(rs.score) as avg_score,
    COUNT(CASE WHEN t.status = 'flagged' THEN 1 END) as flagged
FROM transactions t
JOIN risk_scores rs ON t.id = rs.transaction_id 
    AND t.created_at = rs.transaction_created_at
WHERE t.created_at >= '2026-02-03' 
  AND t.created_at < '2026-02-04';
-- Uses: partition pruning on both tables
```

### Batch Operations

**Batch Insert (with ON CONFLICT):**
```sql
INSERT INTO transactions (...)
VALUES ($1, $2, ...), ($3, $4, ...), ...
ON CONFLICT (idempotency_key) DO NOTHING;
```

## Data Retention

| Table | Retention | Archive Strategy |
|-------|-----------|------------------|
| transactions | 2 years | Monthly partitions to cold storage |
| risk_scores | 2 years | Follows transaction retention |
| audit_logs | 7 years | Yearly archive to S3/GCS |
| daily_aggregates | Indefinite | Compact, keep forever |
| users | Until deleted | Soft delete, hard delete after 90 days |

## Migration Strategy

1. **Initial Setup:**
   - Run `001_initial_schema.sql`
   - Run `002_create_partitions.sql`
   - Run `003_seed_rules.sql`

2. **Adding New Partitions:**
   - Schedule monthly job to create next quarter's partitions
   - Use `create_transaction_partition()` function

3. **Schema Changes:**
   - Use numbered migration files
   - Always include rollback (DOWN) section
   - Test on staging first
