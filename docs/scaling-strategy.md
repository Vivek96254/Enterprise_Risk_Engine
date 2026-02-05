# Scaling Strategy Documentation

## Overview

This document outlines the scaling strategies for the Enterprise Risk Engine, covering horizontal scaling, performance optimization, and failure handling.

## Current Capacity

### Single Instance Baseline
| Metric | Value |
|--------|-------|
| API Throughput | ~100 TPS |
| Worker Processing | ~50 TPS |
| Database Connections | 25 max |
| Memory Usage | ~128MB per service |

## Horizontal Scaling

### API Server Scaling

The API server is **stateless** and can be horizontally scaled behind a load balancer.

```
                    ┌─────────────┐
                    │   Load      │
                    │  Balancer   │
                    └──────┬──────┘
           ┌───────────────┼───────────────┐
           │               │               │
           ▼               ▼               ▼
    ┌─────────────┐ ┌─────────────┐ ┌─────────────┐
    │  API Srv 1  │ │  API Srv 2  │ │  API Srv N  │
    └─────────────┘ └─────────────┘ └─────────────┘
```

**Scaling Triggers:**
- CPU > 70% for 5 minutes
- Memory > 80%
- Response latency > 200ms (p95)
- Request queue depth > 100

**Configuration per Instance:**
```yaml
resources:
  cpu: 0.5 cores
  memory: 256MB
env:
  DB_MAX_OPEN_CONNS: 10  # Reduced per instance
  DB_MAX_IDLE_CONNS: 2
```

### Worker Scaling

Workers use **Redis Streams Consumer Groups** for coordinated parallel processing.

```
                    ┌─────────────────────┐
                    │    Redis Streams    │
                    │  Consumer Group:    │
                    │  "scoring-workers"  │
                    └──────────┬──────────┘
           ┌───────────────────┼───────────────────┐
           │                   │                   │
           ▼                   ▼                   ▼
    ┌─────────────┐     ┌─────────────┐     ┌─────────────┐
    │  Worker 1   │     │  Worker 2   │     │  Worker N   │
    │ Consumers:5 │     │ Consumers:5 │     │ Consumers:5 │
    └─────────────┘     └─────────────┘     └─────────────┘
```

**Consumer Group Benefits:**
- Messages delivered to exactly one consumer
- Automatic load balancing
- Pending message tracking
- Dead letter queue support

**Scaling Triggers:**
- Queue depth > 1000 messages
- Processing lag > 30 seconds
- Worker CPU > 70%

**Scaling Command (Docker Compose):**
```bash
# Scale to 5 workers
docker-compose up -d --scale worker=5
```

### Database Scaling

#### Connection Pool Management

Total connections = (API instances × connections per API) + (Workers × connections per worker)

```
Example: 4 API + 3 Workers
- API: 4 × 10 = 40 connections
- Workers: 3 × 5 = 15 connections
- Total: 55 connections (within PostgreSQL default 100)
```

#### Read Replicas (Future)

For heavy analytics workloads:

```
                    ┌─────────────────┐
                    │  Primary (RW)   │
                    └────────┬────────┘
                             │ Replication
           ┌─────────────────┼─────────────────┐
           ▼                 ▼                 ▼
    ┌─────────────┐   ┌─────────────┐   ┌─────────────┐
    │  Replica 1  │   │  Replica 2  │   │  Replica N  │
    │  (Read)     │   │  (Read)     │   │  (Read)     │
    └─────────────┘   └─────────────┘   └─────────────┘
```

**Routing Strategy:**
- Writes → Primary
- Reads (analytics) → Replicas
- Reads (recent data) → Primary (consistency)

#### Partitioning Benefits

Monthly partitions enable:
- **Partition Pruning**: Queries only scan relevant partitions
- **Parallel Scans**: Multiple partitions scanned concurrently
- **Easy Archival**: Detach and archive old partitions
- **Maintenance**: VACUUM/ANALYZE per partition

```sql
-- Query only touches February 2026 partition
SELECT * FROM transactions
WHERE created_at >= '2026-02-01' AND created_at < '2026-03-01';
```

### Redis Scaling

#### Current Setup (Single Instance)
- Streams for message queue
- Cache for risk scores and profiles
- ~256MB memory limit

#### Redis Cluster (Future)

For high availability and throughput:

```
┌─────────────────────────────────────────────────────┐
│                  Redis Cluster                       │
├─────────────────────────────────────────────────────┤
│  ┌─────────┐  ┌─────────┐  ┌─────────┐            │
│  │ Master1 │  │ Master2 │  │ Master3 │            │
│  │ Slot    │  │ Slot    │  │ Slot    │            │
│  │ 0-5460  │  │ 5461-   │  │ 10923-  │            │
│  │         │  │ 10922   │  │ 16383   │            │
│  └────┬────┘  └────┬────┘  └────┬────┘            │
│       │            │            │                  │
│  ┌────┴────┐  ┌────┴────┐  ┌────┴────┐            │
│  │ Replica │  │ Replica │  │ Replica │            │
│  └─────────┘  └─────────┘  └─────────┘            │
└─────────────────────────────────────────────────────┘
```

## Load Smoothing

### Queue-Based Buffering

Redis Streams act as a buffer during traffic spikes:

```
Traffic Spike
     │
     ▼
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│    API      │────▶│   Redis     │────▶│   Workers   │
│ (Accepts    │     │  (Buffers   │     │ (Processes  │
│  all)       │     │   spikes)   │     │  steadily)  │
└─────────────┘     └─────────────┘     └─────────────┘
```

**Benefits:**
- API remains responsive during spikes
- Workers process at sustainable rate
- No data loss during overload

### Batch Processing

**API Batch Endpoint:**
- Up to 1000 transactions per request
- Reduces HTTP overhead
- Atomic batch inserts

**Worker Batch Processing:**
- Configurable batch size (default: 100)
- Batch acknowledgment
- Reduced Redis round trips

## Performance Optimizations

### Database Optimizations

| Optimization | Impact | Implementation |
|--------------|--------|----------------|
| Connection Pooling | -90% connection overhead | pgx pool |
| Batch Inserts | -80% insert latency | Batch API |
| Prepared Statements | -30% query overhead | pgx |
| Composite Indexes | -70% query time | (account_id, created_at) |
| Partitioning | -90% scan time | Monthly partitions |

### Caching Strategy

| Data Type | TTL | Hit Rate Target |
|-----------|-----|-----------------|
| Risk Scores | 24h | 80% |
| Account Profiles | 5m | 70% |
| Daily Summaries (recent) | 5m | 90% |
| Daily Summaries (historical) | 1h | 95% |

### Query Optimization

**Before (Full Scan):**
```sql
SELECT * FROM transactions WHERE account_id = $1;
-- Scans all partitions
```

**After (Partition Pruning):**
```sql
SELECT * FROM transactions 
WHERE account_id = $1 
  AND created_at >= NOW() - INTERVAL '30 days';
-- Only scans recent partitions
```

## Failure Handling

### Retry Strategy

```
Attempt 1 → Failure → Wait 1s
Attempt 2 → Failure → Wait 2s
Attempt 3 → Failure → Wait 4s
Attempt 4 → Dead Letter Queue
```

**Implementation:**
```go
type WorkerConfig struct {
    RetryAttempts    int           // 3
    RetryBackoff     time.Duration // Exponential
    DeadLetterStream string        // "transactions-dlq"
}
```

### Dead Letter Queue

Failed messages are preserved for:
- Manual investigation
- Replay after fixes
- Pattern analysis

```json
{
  "stream": "transactions-dlq",
  "message": {
    "transaction_id": "...",
    "account_id": "...",
    "retry_count": 3
  },
  "error": "database connection timeout",
  "failed_at": "2026-02-03T10:30:00Z"
}
```

### Circuit Breaker (Future)

Prevent cascade failures:

```
┌─────────────┐
│   Closed    │ ← Normal operation
└──────┬──────┘
       │ Failures > threshold
       ▼
┌─────────────┐
│    Open     │ ← Reject requests immediately
└──────┬──────┘
       │ After timeout
       ▼
┌─────────────┐
│ Half-Open   │ ← Test with limited requests
└──────┬──────┘
       │ Success
       ▼
┌─────────────┐
│   Closed    │
└─────────────┘
```

### Idempotency

All transaction ingestion is idempotent:

```go
// Same idempotency_key returns existing transaction
if existing, _ := repo.GetByIdempotencyKey(ctx, key); existing != nil {
    return existing, nil // No duplicate processing
}
```

## Capacity Planning

### Traffic Projections

| Scenario | TPS | API Instances | Workers | DB Connections |
|----------|-----|---------------|---------|----------------|
| Low | 50 | 1 | 1 | 30 |
| Medium | 200 | 2 | 2 | 50 |
| High | 500 | 4 | 5 | 80 |
| Peak | 1000 | 8 | 10 | 150 |

### Resource Requirements

**Per API Instance:**
- CPU: 0.5 cores
- Memory: 256MB
- Network: 100 Mbps

**Per Worker Instance:**
- CPU: 0.5 cores
- Memory: 256MB
- Network: 50 Mbps

### Cost Estimation (Render.com)

| Tier | Services | Monthly Cost |
|------|----------|--------------|
| Free | 1 API + 1 Worker + DB | $0 |
| Starter | 2 API + 2 Workers + DB | ~$50 |
| Pro | 4 API + 5 Workers + DB + Redis | ~$200 |

## Monitoring & Alerts

### Key Metrics

| Metric | Warning | Critical |
|--------|---------|----------|
| API Latency (p95) | > 200ms | > 500ms |
| Queue Depth | > 500 | > 2000 |
| Error Rate | > 1% | > 5% |
| DB Connections | > 70% | > 90% |
| Worker Lag | > 30s | > 120s |

### Scaling Alerts

```yaml
alerts:
  - name: HighQueueDepth
    condition: queue_depth > 1000 for 5m
    action: scale_workers_up
    
  - name: HighAPILatency
    condition: p95_latency > 300ms for 5m
    action: scale_api_up
    
  - name: LowQueueDepth
    condition: queue_depth < 10 for 30m
    action: scale_workers_down
```

## AWS Migration Path

| Current | AWS Service | Benefits |
|---------|-------------|----------|
| Redis Streams | Amazon MSK (Kafka) | Higher throughput, better durability |
| Render PostgreSQL | Amazon RDS | Multi-AZ, automated backups |
| Docker on Render | Amazon ECS/EKS | Auto-scaling, better orchestration |
| Manual Metrics | CloudWatch + Prometheus | Comprehensive observability |
| Single Region | Multi-Region | Disaster recovery |

### Migration Steps

1. **Phase 1: Database Migration**
   - Set up RDS with read replicas
   - Use DMS for zero-downtime migration
   - Update connection strings

2. **Phase 2: Message Queue Migration**
   - Set up MSK cluster
   - Run Redis Streams and Kafka in parallel
   - Migrate consumers gradually

3. **Phase 3: Compute Migration**
   - Containerize with ECS/EKS
   - Set up auto-scaling policies
   - Configure load balancers

4. **Phase 4: Observability**
   - Deploy Prometheus + Grafana
   - Set up CloudWatch alarms
   - Implement distributed tracing
