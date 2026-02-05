-- Migration: 003_seed_rules
-- Description: Seed default risk scoring rules
-- Created: 2026-02-03

BEGIN;

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
 40.0, 'critical', 5, true),

('RULE_RAPID_SMALL_TRANSACTIONS', 'Rapid Small Transactions', 'Many small transactions in quick succession (potential structuring)',
 '{"type": "compound", "operator": "AND", "conditions": [{"field": "transaction_velocity_1h", "operator": ">", "value": 5}, {"field": "amount", "operator": "<", "value": 100}]}',
 25.0, 'high', 25, true),

('RULE_CROSS_BORDER', 'Cross Border Transaction', 'Transaction from different country than account origin',
 '{"type": "threshold", "field": "is_cross_border", "operator": "=", "value": true}',
 10.0, 'low', 70, true)
ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    description = EXCLUDED.description,
    condition = EXCLUDED.condition,
    score_impact = EXCLUDED.score_impact,
    risk_level = EXCLUDED.risk_level,
    priority = EXCLUDED.priority,
    updated_at = NOW();

COMMIT;
