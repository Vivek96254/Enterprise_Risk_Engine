# Architecture Documentation

## System Overview

The Enterprise Risk Engine is designed as a **modular monolith** that can be split into microservices as scaling needs grow. This architecture provides the benefits of a monolithic codebase (simplicity, easier debugging) while maintaining clear service boundaries.

## Core Components

### 1. API Gateway Layer

**Responsibilities:**
- HTTP request routing
- JWT authentication and authorization
- Request validation and sanitization
- Rate limiting (future)
- Request ID generation for tracing
- CORS handling

**Technology:** Gin (Go HTTP framework)

```
Request Flow:
Client → CORS → RequestID → Logging → Auth → Handler → Response
```

### 2. Transaction Ingestion Module

**Location:** `internal/ingestion/`

**Responsibilities:**
- Validate incoming transactions
- Deduplicate using idempotency keys
- Persist to PostgreSQL
- Publish events to Redis Streams
- Handle batch uploads

**Key Design Decisions:**
- Idempotency keys prevent duplicate processing
- Minimal validation at ingestion (full validation in worker)
- Async processing via message queue
- Batch API for high-throughput scenarios

### 3. Event Queue (Redis Streams)

**Location:** `internal/queue/`

**Why Redis Streams over Kafka:**
- Free tier friendly (no Kafka infrastructure costs)
- Consumer groups for parallel processing
- Built-in acknowledgment mechanism
- Simpler operational overhead
- Sufficient for 10K+ TPS

**Message Format:**
```json
{
  "transaction_id": "uuid",
  "account_id": "uuid",
  "amount": 1500.00,
  "currency": "USD",
  "merchant": "Amazon",
  "location": "New York",
  "country": "US",
  "channel": "online",
  "timestamp": "2026-02-03T10:30:00Z",
  "retry_count": 0
}
```

**Consumer Group Configuration:**
- Group Name: `scoring-workers`
- Pending timeout: 30 seconds
- Auto-claim abandoned messages
- Dead letter queue for permanent failures

### 4. Scoring Worker Module

**Location:** `internal/scoring/`

**Responsibilities:**
- Consume events from Redis Streams
- Fetch historical transaction data
- Compute risk features
- Apply scoring rules
- Persist risk scores
- Update transaction status
- Update account risk profiles

**Worker Pool Architecture:**
```
WorkerPool
├── Worker-0
│   ├── Goroutine-0 (consumer)
│   ├── Goroutine-1 (consumer)
│   └── Goroutine-N (consumer)
├── Worker-1
│   └── ...
└── Worker-N
```

**Processing Flow:**
1. Consume batch from stream
2. For each message:
   - Fetch transaction from DB
   - Compute features (rolling averages, velocity, etc.)
   - Evaluate rules
   - Calculate final score
   - Determine risk level
   - Update transaction status
   - Save risk score
   - Update cache
3. Acknowledge batch

### 5. Rule Engine

**Location:** `internal/scoring/engine.go`

**Rule Types:**
1. **Threshold Rules**: Simple value comparisons
2. **Compound Rules**: Multiple conditions with AND/OR
3. **Time-based Rules**: Hour/day restrictions
4. **Velocity Rules**: Rate-based checks

**Rule Evaluation:**
```go
for _, rule := range rules {
    if rule.Evaluate(features, transaction) {
        totalScore += rule.ScoreImpact
        triggeredRules = append(triggeredRules, rule.ID)
    }
}
```

**Future Enhancement:** JSON-based rule configuration from database

### 6. Analytics Module

**Location:** `internal/analytics/`

**Capabilities:**
- Daily risk summaries
- Account risk profiles
- Risk distribution analysis
- Top triggered rules
- Hourly volume analysis
- System metrics

**Caching Strategy:**
- Recent data: 5 minute TTL
- Historical data: 1 hour TTL
- Account profiles: 5 minute TTL

### 7. Data Layer

**Location:** `internal/repositories/`

**Repository Pattern:**
```
Repository Interface
├── UserRepository
├── AccountRepository
├── TransactionRepository
├── RiskScoreRepository
└── AuditRepository
```

**Connection Management:**
- pgx connection pool
- Configurable pool size
- Health checks
- Transaction support

## Data Flow Diagrams

### Transaction Processing Flow

```
┌─────────┐     ┌─────────┐     ┌─────────┐     ┌─────────┐
│  Client │────▶│   API   │────▶│   DB    │────▶│  Redis  │
└─────────┘     └─────────┘     └─────────┘     └────┬────┘
                                                     │
                    ┌────────────────────────────────┘
                    │
                    ▼
              ┌─────────┐     ┌─────────┐     ┌─────────┐
              │ Worker  │────▶│   DB    │────▶│  Cache  │
              └─────────┘     └─────────┘     └─────────┘
```

### Authentication Flow

```
┌─────────┐     ┌─────────┐     ┌─────────┐
│  Client │────▶│  Login  │────▶│   DB    │
└─────────┘     └────┬────┘     └─────────┘
                     │
                     ▼
              ┌─────────────┐
              │ JWT Token   │
              └──────┬──────┘
                     │
                     ▼
              ┌─────────────┐
              │ Protected   │
              │ Endpoints   │
              └─────────────┘
```

## Security Architecture

### Authentication
- JWT tokens with HS256 signing
- 24-hour token expiration
- Role-based access control (admin, analyst, user)

### Authorization Levels
| Role | Capabilities |
|------|-------------|
| admin | Full access, system metrics, rule management |
| analyst | Read all data, analytics access |
| user | Own account data only |

### Data Protection
- Password hashing with bcrypt (cost factor 12)
- Prepared statements (SQL injection prevention)
- Input validation at API layer
- Audit logging for all operations

## Deployment Architecture

### Single Instance (Development)
```
┌─────────────────────────────────────┐
│           Docker Compose            │
├─────────────────────────────────────┤
│  ┌─────────┐  ┌─────────┐          │
│  │   API   │  │ Worker  │          │
│  └────┬────┘  └────┬────┘          │
│       │            │               │
│  ┌────┴────────────┴────┐          │
│  │      PostgreSQL      │          │
│  └──────────────────────┘          │
│  ┌──────────────────────┐          │
│  │        Redis         │          │
│  └──────────────────────┘          │
└─────────────────────────────────────┘
```

### Production (Render.com)
```
┌─────────────────────────────────────────────────────┐
│                    Render.com                        │
├─────────────────────────────────────────────────────┤
│                                                     │
│  ┌─────────────┐        ┌─────────────┐            │
│  │  Web Svc    │        │  Worker Svc │            │
│  │  (API)      │        │  (Scoring)  │            │
│  └──────┬──────┘        └──────┬──────┘            │
│         │                      │                    │
│         └──────────┬───────────┘                   │
│                    │                               │
│  ┌─────────────────┴─────────────────┐            │
│  │         Render PostgreSQL          │            │
│  └────────────────────────────────────┘            │
│                                                     │
│  ┌────────────────────────────────────┐            │
│  │         Upstash Redis              │            │
│  │         (External)                 │            │
│  └────────────────────────────────────┘            │
└─────────────────────────────────────────────────────┘
```

## Observability

### Structured Logging
```json
{
  "level": "info",
  "time": 1706954400,
  "method": "POST",
  "path": "/api/v1/transactions",
  "status": 201,
  "latency": "45ms",
  "request_id": "abc123",
  "client_ip": "192.168.1.1"
}
```

### Metrics Exposed
- Transactions per second
- Average processing time
- Queue depth
- Database connections
- Error rate
- Worker status

### Health Checks
- `/health` endpoint for load balancers
- Database connectivity check
- Redis connectivity check

## Error Handling

### Retry Strategy
1. **Transient Failures**: Automatic retry with exponential backoff
2. **Permanent Failures**: Send to dead letter queue
3. **Max Retries**: 3 attempts before DLQ

### Dead Letter Queue
- Failed messages preserved for investigation
- Contains original message + error details
- Manual replay capability (future)

## Future Microservices Split

When scaling requires it, the monolith can be split:

```
Current Monolith
├── API Server
│   ├── Auth Module      → Auth Service
│   ├── Ingestion Module → Ingestion Service
│   └── Analytics Module → Analytics Service
└── Worker
    └── Scoring Module   → Scoring Service
```

**Shared Components:**
- Database (with service-specific schemas)
- Redis (with namespaced keys)
- Message formats (protobuf for gRPC)
