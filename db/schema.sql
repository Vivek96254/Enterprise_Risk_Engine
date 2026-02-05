-- Enterprise Risk Engine Database Schema
-- PostgreSQL 14+ with partitioning support

-- Enable required extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pg_trgm";

-- ============================================
-- USERS TABLE
-- ============================================
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    email VARCHAR(255) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    role VARCHAR(50) NOT NULL DEFAULT 'user' CHECK (role IN ('admin', 'analyst', 'user')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX idx_users_email ON users(email) WHERE deleted_at IS NULL;
CREATE INDEX idx_users_created_at ON users(created_at);

-- ============================================
-- ACCOUNTS TABLE
-- ============================================
CREATE TABLE IF NOT EXISTS accounts (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    account_type VARCHAR(50) NOT NULL DEFAULT 'standard' CHECK (account_type IN ('standard', 'premium', 'business')),
    risk_profile VARCHAR(20) NOT NULL DEFAULT 'low' CHECK (risk_profile IN ('low', 'medium', 'high')),
    status VARCHAR(20) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended', 'closed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_accounts_user_id ON accounts(user_id);
CREATE INDEX idx_accounts_risk_profile ON accounts(risk_profile);
CREATE INDEX idx_accounts_status ON accounts(status);

-- ============================================
-- TRANSACTIONS TABLE (PARTITIONED BY MONTH)
-- ============================================
CREATE TABLE IF NOT EXISTS transactions (
    id UUID NOT NULL DEFAULT uuid_generate_v4(),
    account_id UUID NOT NULL,
    amount DECIMAL(15, 2) NOT NULL,
    currency VARCHAR(3) NOT NULL DEFAULT 'USD',
    merchant VARCHAR(255),
    merchant_category VARCHAR(100),
    location VARCHAR(255),
    country VARCHAR(3),
    channel VARCHAR(20) NOT NULL DEFAULT 'online' CHECK (channel IN ('online', 'pos', 'atm')),
    status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processed', 'flagged', 'blocked')),
    idempotency_key VARCHAR(255) NOT NULL,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMPTZ,
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

-- Create partitions for the current and next 12 months
-- These will be created dynamically by the application or via cron
CREATE TABLE IF NOT EXISTS transactions_2026_01 PARTITION OF transactions
    FOR VALUES FROM ('2026-01-01') TO ('2026-02-01');

CREATE TABLE IF NOT EXISTS transactions_2026_02 PARTITION OF transactions
    FOR VALUES FROM ('2026-02-01') TO ('2026-03-01');

CREATE TABLE IF NOT EXISTS transactions_2026_03 PARTITION OF transactions
    FOR VALUES FROM ('2026-03-01') TO ('2026-04-01');

CREATE TABLE IF NOT EXISTS transactions_2026_04 PARTITION OF transactions
    FOR VALUES FROM ('2026-04-01') TO ('2026-05-01');

CREATE TABLE IF NOT EXISTS transactions_2026_05 PARTITION OF transactions
    FOR VALUES FROM ('2026-05-01') TO ('2026-06-01');

CREATE TABLE IF NOT EXISTS transactions_2026_06 PARTITION OF transactions
    FOR VALUES FROM ('2026-06-01') TO ('2026-07-01');

CREATE TABLE IF NOT EXISTS transactions_2026_07 PARTITION OF transactions
    FOR VALUES FROM ('2026-07-01') TO ('2026-08-01');

CREATE TABLE IF NOT EXISTS transactions_2026_08 PARTITION OF transactions
    FOR VALUES FROM ('2026-08-01') TO ('2026-09-01');

CREATE TABLE IF NOT EXISTS transactions_2026_09 PARTITION OF transactions
    FOR VALUES FROM ('2026-09-01') TO ('2026-10-01');

CREATE TABLE IF NOT EXISTS transactions_2026_10 PARTITION OF transactions
    FOR VALUES FROM ('2026-10-01') TO ('2026-11-01');

CREATE TABLE IF NOT EXISTS transactions_2026_11 PARTITION OF transactions
    FOR VALUES FROM ('2026-11-01') TO ('2026-12-01');

CREATE TABLE IF NOT EXISTS transactions_2026_12 PARTITION OF transactions
    FOR VALUES FROM ('2026-12-01') TO ('2027-01-01');

-- Indexes on partitioned table (created on parent, propagates to children)
CREATE INDEX idx_transactions_account_created ON transactions(account_id, created_at DESC);
CREATE INDEX idx_transactions_status ON transactions(status);
CREATE INDEX idx_transactions_idempotency ON transactions(idempotency_key);
CREATE INDEX idx_transactions_merchant ON transactions USING gin(merchant gin_trgm_ops);
CREATE INDEX idx_transactions_location ON transactions(location);
CREATE INDEX idx_transactions_country ON transactions(country);

-- ============================================
-- RISK SCORES TABLE
-- ============================================
CREATE TABLE IF NOT EXISTS risk_scores (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    transaction_id UUID NOT NULL,
    transaction_created_at TIMESTAMPTZ NOT NULL,
    score DECIMAL(5, 2) NOT NULL CHECK (score >= 0 AND score <= 100),
    risk_level VARCHAR(20) NOT NULL CHECK (risk_level IN ('low', 'medium', 'high', 'critical')),
    rules_triggered TEXT[] DEFAULT '{}',
    features JSONB DEFAULT '{}',
    model_version VARCHAR(50) NOT NULL DEFAULT 'v1.0.0',
    processing_time_ms INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (transaction_id, transaction_created_at) REFERENCES transactions(id, created_at) ON DELETE CASCADE
);

CREATE INDEX idx_risk_scores_transaction ON risk_scores(transaction_id);
CREATE INDEX idx_risk_scores_risk_level ON risk_scores(risk_level);
CREATE INDEX idx_risk_scores_score ON risk_scores(score DESC);
CREATE INDEX idx_risk_scores_created_at ON risk_scores(created_at DESC);
CREATE INDEX idx_risk_scores_rules ON risk_scores USING gin(rules_triggered);

-- ============================================
-- AUDIT LOGS TABLE (APPEND-ONLY)
-- ============================================
CREATE TABLE IF NOT EXISTS audit_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    event_type VARCHAR(50) NOT NULL,
    entity_id UUID,
    entity_type VARCHAR(50),
    user_id UUID REFERENCES users(id),
    action VARCHAR(50) NOT NULL,
    payload JSONB DEFAULT '{}',
    ip_address INET,
    user_agent TEXT,
    request_id VARCHAR(100),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_audit_logs_event_type ON audit_logs(event_type);
CREATE INDEX idx_audit_logs_entity ON audit_logs(entity_type, entity_id);
CREATE INDEX idx_audit_logs_user_id ON audit_logs(user_id);
CREATE INDEX idx_audit_logs_created_at ON audit_logs(created_at DESC);
CREATE INDEX idx_audit_logs_request_id ON audit_logs(request_id);

-- ============================================
-- RULES TABLE (JSON-CONFIGURABLE RULES)
-- ============================================
CREATE TABLE IF NOT EXISTS rules (
    id VARCHAR(100) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    condition JSONB NOT NULL,
    score_impact DECIMAL(5, 2) NOT NULL,
    risk_level VARCHAR(20) NOT NULL CHECK (risk_level IN ('low', 'medium', 'high', 'critical')),
    priority INTEGER NOT NULL DEFAULT 100,
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_rules_enabled ON rules(enabled, priority);

-- ============================================
-- DAILY AGGREGATES TABLE (PRE-COMPUTED STATS)
-- ============================================
CREATE TABLE IF NOT EXISTS daily_aggregates (
    date DATE NOT NULL,
    account_id UUID,
    total_transactions INTEGER NOT NULL DEFAULT 0,
    total_amount DECIMAL(20, 2) NOT NULL DEFAULT 0,
    flagged_count INTEGER NOT NULL DEFAULT 0,
    blocked_count INTEGER NOT NULL DEFAULT 0,
    avg_risk_score DECIMAL(5, 2),
    high_risk_count INTEGER NOT NULL DEFAULT 0,
    critical_risk_count INTEGER NOT NULL DEFAULT 0,
    top_rules_triggered JSONB DEFAULT '[]',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (date, account_id)
);

CREATE INDEX idx_daily_aggregates_date ON daily_aggregates(date DESC);
CREATE INDEX idx_daily_aggregates_account ON daily_aggregates(account_id);

-- ============================================
-- FUNCTIONS & TRIGGERS
-- ============================================

-- Function to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Apply updated_at trigger to relevant tables
CREATE TRIGGER update_users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_accounts_updated_at
    BEFORE UPDATE ON accounts
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_rules_updated_at
    BEFORE UPDATE ON rules
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_daily_aggregates_updated_at
    BEFORE UPDATE ON daily_aggregates
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Function to create future partitions
CREATE OR REPLACE FUNCTION create_transaction_partition(partition_date DATE)
RETURNS VOID AS $$
DECLARE
    partition_name TEXT;
    start_date DATE;
    end_date DATE;
BEGIN
    partition_name := 'transactions_' || TO_CHAR(partition_date, 'YYYY_MM');
    start_date := DATE_TRUNC('month', partition_date);
    end_date := start_date + INTERVAL '1 month';
    
    EXECUTE FORMAT(
        'CREATE TABLE IF NOT EXISTS %I PARTITION OF transactions FOR VALUES FROM (%L) TO (%L)',
        partition_name,
        start_date,
        end_date
    );
END;
$$ LANGUAGE plpgsql;

-- ============================================
-- SEED DEFAULT RULES
-- ============================================
INSERT INTO rules (id, name, description, condition, score_impact, risk_level, priority, enabled) VALUES
('RULE_SPIKE_ANOMALY', 'Spike Anomaly', 'Transaction amount significantly higher than average', 
 '{"type": "threshold", "field": "amount_deviation", "operator": ">", "value": 3.0}', 
 30.0, 'high', 10, true),

('RULE_NEW_LOCATION_HIGH_AMOUNT', 'New Location High Amount', 'High amount transaction from a new location',
 '{"type": "compound", "operator": "AND", "conditions": [{"field": "is_new_location", "operator": "=", "value": true}, {"field": "amount", "operator": ">", "value": 1000}]}',
 25.0, 'medium', 20, true),

('RULE_VELOCITY_BURST', 'Velocity Burst', 'Too many transactions in short time period',
 '{"type": "threshold", "field": "transaction_velocity_1h", "operator": ">", "value": 10}',
 20.0, 'medium', 30, true),

('RULE_HIGH_RISK_COUNTRY', 'High Risk Country', 'Transaction from high-risk country',
 '{"type": "threshold", "field": "is_high_risk_country", "operator": "=", "value": true}',
 35.0, 'high', 15, true),

('RULE_LOCATION_HOPPING', 'Location Hopping', 'Multiple location changes in short period',
 '{"type": "threshold", "field": "location_change_count", "operator": ">", "value": 3}',
 15.0, 'medium', 40, true),

('RULE_NEW_MERCHANT_HIGH_AMOUNT', 'New Merchant High Amount', 'High amount at new merchant',
 '{"type": "compound", "operator": "AND", "conditions": [{"field": "is_new_merchant", "operator": "=", "value": true}, {"field": "amount", "operator": ">", "value": 500}]}',
 15.0, 'medium', 50, true),

('RULE_NIGHT_TRANSACTION', 'Night Transaction', 'Transaction during unusual hours (midnight to 5am)',
 '{"type": "time_range", "field": "hour", "start": 0, "end": 5}',
 10.0, 'low', 60, true),

('RULE_CRITICAL_AMOUNT', 'Critical Amount', 'Extremely high transaction amount',
 '{"type": "threshold", "field": "amount", "operator": ">", "value": 10000}',
 40.0, 'critical', 5, true)
ON CONFLICT (id) DO NOTHING;

-- ============================================
-- VIEWS FOR ANALYTICS
-- ============================================

-- View for recent flagged transactions
CREATE OR REPLACE VIEW v_recent_flagged_transactions AS
SELECT 
    t.id,
    t.account_id,
    t.amount,
    t.currency,
    t.merchant,
    t.location,
    t.country,
    t.channel,
    t.status,
    t.created_at,
    rs.score,
    rs.risk_level,
    rs.rules_triggered
FROM transactions t
JOIN risk_scores rs ON t.id = rs.transaction_id AND t.created_at = rs.transaction_created_at
WHERE t.status IN ('flagged', 'blocked')
ORDER BY t.created_at DESC;

-- View for account risk summary
CREATE OR REPLACE VIEW v_account_risk_summary AS
SELECT 
    a.id AS account_id,
    a.risk_profile,
    a.status,
    COUNT(t.id) AS total_transactions_30d,
    COALESCE(AVG(t.amount), 0) AS avg_transaction_amount,
    COUNT(CASE WHEN t.status = 'flagged' THEN 1 END) AS flagged_count_30d,
    COUNT(CASE WHEN t.status = 'blocked' THEN 1 END) AS blocked_count_30d,
    COALESCE(AVG(rs.score), 0) AS avg_risk_score,
    MAX(t.created_at) AS last_transaction_at
FROM accounts a
LEFT JOIN transactions t ON a.id = t.account_id 
    AND t.created_at >= NOW() - INTERVAL '30 days'
LEFT JOIN risk_scores rs ON t.id = rs.transaction_id AND t.created_at = rs.transaction_created_at
GROUP BY a.id, a.risk_profile, a.status;

-- Grant permissions (adjust based on your setup)
-- GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO risk_engine_app;
-- GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO risk_engine_app;
