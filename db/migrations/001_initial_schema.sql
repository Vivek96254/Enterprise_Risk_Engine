-- Migration: 001_initial_schema
-- Description: Initial database schema for Enterprise Risk Engine
-- Created: 2026-02-03

-- UP Migration
BEGIN;

-- Enable required extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pg_trgm";

-- Users table
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    email VARCHAR(255) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    role VARCHAR(50) NOT NULL DEFAULT 'user' CHECK (role IN ('admin', 'analyst', 'user')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_users_email ON users(email) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_users_created_at ON users(created_at);

-- Accounts table
CREATE TABLE IF NOT EXISTS accounts (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    account_type VARCHAR(50) NOT NULL DEFAULT 'standard' CHECK (account_type IN ('standard', 'premium', 'business')),
    risk_profile VARCHAR(20) NOT NULL DEFAULT 'low' CHECK (risk_profile IN ('low', 'medium', 'high')),
    status VARCHAR(20) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended', 'closed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_accounts_user_id ON accounts(user_id);
CREATE INDEX IF NOT EXISTS idx_accounts_risk_profile ON accounts(risk_profile);
CREATE INDEX IF NOT EXISTS idx_accounts_status ON accounts(status);

-- Partitioned transactions table
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

CREATE INDEX IF NOT EXISTS idx_transactions_account_created ON transactions(account_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_transactions_status ON transactions(status);
CREATE INDEX IF NOT EXISTS idx_transactions_idempotency ON transactions(idempotency_key);
CREATE INDEX IF NOT EXISTS idx_transactions_location ON transactions(location);
CREATE INDEX IF NOT EXISTS idx_transactions_country ON transactions(country);

-- Risk scores table
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

CREATE INDEX IF NOT EXISTS idx_risk_scores_transaction ON risk_scores(transaction_id);
CREATE INDEX IF NOT EXISTS idx_risk_scores_risk_level ON risk_scores(risk_level);
CREATE INDEX IF NOT EXISTS idx_risk_scores_score ON risk_scores(score DESC);
CREATE INDEX IF NOT EXISTS idx_risk_scores_created_at ON risk_scores(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_risk_scores_rules_triggered ON risk_scores USING GIN(rules_triggered);

-- Audit logs table
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

CREATE INDEX IF NOT EXISTS idx_audit_logs_event_type ON audit_logs(event_type);
CREATE INDEX IF NOT EXISTS idx_audit_logs_entity ON audit_logs(entity_type, entity_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_user_id ON audit_logs(user_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at ON audit_logs(created_at DESC);

-- Rules table
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

CREATE INDEX IF NOT EXISTS idx_rules_enabled ON rules(enabled, priority);

-- Daily aggregates table
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

CREATE INDEX IF NOT EXISTS idx_daily_aggregates_date ON daily_aggregates(date DESC);
CREATE INDEX IF NOT EXISTS idx_daily_aggregates_account ON daily_aggregates(account_id);

-- Updated_at trigger function
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Apply triggers
DROP TRIGGER IF EXISTS update_users_updated_at ON users;
CREATE TRIGGER update_users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

DROP TRIGGER IF EXISTS update_accounts_updated_at ON accounts;
CREATE TRIGGER update_accounts_updated_at
    BEFORE UPDATE ON accounts
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

DROP TRIGGER IF EXISTS update_rules_updated_at ON rules;
CREATE TRIGGER update_rules_updated_at
    BEFORE UPDATE ON rules
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

DROP TRIGGER IF EXISTS update_daily_aggregates_updated_at ON daily_aggregates;
CREATE TRIGGER update_daily_aggregates_updated_at
    BEFORE UPDATE ON daily_aggregates
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

COMMIT;

-- DOWN Migration (uncomment to rollback)
-- BEGIN;
-- DROP TABLE IF EXISTS daily_aggregates CASCADE;
-- DROP TABLE IF EXISTS rules CASCADE;
-- DROP TABLE IF EXISTS audit_logs CASCADE;
-- DROP TABLE IF EXISTS risk_scores CASCADE;
-- DROP TABLE IF EXISTS transactions CASCADE;
-- DROP TABLE IF EXISTS accounts CASCADE;
-- DROP TABLE IF EXISTS users CASCADE;
-- DROP FUNCTION IF EXISTS update_updated_at_column();
-- COMMIT;
