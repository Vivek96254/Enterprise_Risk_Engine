package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// User represents a system user
type User struct {
	ID           uuid.UUID  `json:"id"`
	Email        string     `json:"email"`
	PasswordHash string     `json:"-"`
	Role         string     `json:"role"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	DeletedAt    *time.Time `json:"deleted_at,omitempty"`
}

// Account represents a financial account
type Account struct {
	ID          uuid.UUID `json:"id"`
	UserID      uuid.UUID `json:"user_id"`
	AccountType string    `json:"account_type"`
	RiskProfile string    `json:"risk_profile"` // low, medium, high
	Status      string    `json:"status"`       // active, suspended, closed
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// RiskProfile enum values
const (
	RiskProfileLow    = "low"
	RiskProfileMedium = "medium"
	RiskProfileHigh   = "high"
)

// AccountStatus enum values
const (
	AccountStatusActive    = "active"
	AccountStatusSuspended = "suspended"
	AccountStatusClosed    = "closed"
)

// Transaction represents a financial transaction
type Transaction struct {
	ID              uuid.UUID  `json:"id"`
	AccountID       uuid.UUID  `json:"account_id"`
	Amount          float64    `json:"amount"`
	Currency        string     `json:"currency"`
	Merchant        string     `json:"merchant"`
	MerchantCategory string    `json:"merchant_category"`
	Location        string     `json:"location"`
	Country         string     `json:"country"`
	Channel         string     `json:"channel"` // online, pos, atm
	Status          string     `json:"status"`  // pending, processed, flagged, blocked
	IdempotencyKey  string     `json:"idempotency_key"`
	Metadata        JSONB      `json:"metadata,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	ProcessedAt     *time.Time `json:"processed_at,omitempty"`
}

// TransactionStatus enum values
const (
	TransactionStatusPending   = "pending"
	TransactionStatusProcessed = "processed"
	TransactionStatusFlagged   = "flagged"
	TransactionStatusBlocked   = "blocked"
)

// TransactionChannel enum values
const (
	ChannelOnline = "online"
	ChannelPOS    = "pos"
	ChannelATM    = "atm"
)

// RiskScore represents the computed risk score for a transaction
type RiskScore struct {
	ID               uuid.UUID `json:"id"`
	TransactionID    uuid.UUID `json:"transaction_id"`
	Score            float64   `json:"score"`             // 0-100 (final composite score)
	RuleScore        float64   `json:"rule_score"`        // Score from rule engine
	MLScore          *float64  `json:"ml_score"`          // Score from ML model (nullable)
	BehavioralScore  *float64  `json:"behavioral_score"`  // Score from behavioral analysis
	RiskLevel        string    `json:"risk_level"`        // low, medium, high, critical
	RulesTriggered   []string  `json:"rules_triggered"`   // list of rule IDs
	AnomaliesDetected []string `json:"anomalies_detected"` // list of anomaly types
	Features         JSONB     `json:"features"`          // computed features
	ModelVersion     string    `json:"model_version"`
	ScoringPath      string    `json:"scoring_path"`      // "fast" or "full"
	ProcessingTimeMs int64     `json:"processing_time_ms"`
	CreatedAt        time.Time `json:"created_at"`
}

// RiskLevel enum values
const (
	RiskLevelLow      = "low"
	RiskLevelMedium   = "medium"
	RiskLevelHigh     = "high"
	RiskLevelCritical = "critical"
)

// AuditLog represents an audit trail entry
type AuditLog struct {
	ID        uuid.UUID `json:"id"`
	EventType string    `json:"event_type"`
	EntityID  uuid.UUID `json:"entity_id"`
	EntityType string   `json:"entity_type"`
	UserID    *uuid.UUID `json:"user_id,omitempty"`
	Action    string    `json:"action"`
	Payload   JSONB     `json:"payload"`
	IPAddress string    `json:"ip_address"`
	UserAgent string    `json:"user_agent"`
	RequestID string    `json:"request_id"`
	CreatedAt time.Time `json:"created_at"`
}

// AuditEventType enum values
const (
	AuditEventTransaction     = "transaction"
	AuditEventRiskScore       = "risk_score"
	AuditEventAccountUpdate   = "account_update"
	AuditEventUserLogin       = "user_login"
	AuditEventUserLogout      = "user_logout"
	AuditEventRuleUpdate      = "rule_update"
)

// TransactionEvent is the event published to Redis Streams
type TransactionEvent struct {
	TransactionID string    `json:"transaction_id"`
	AccountID     string    `json:"account_id"`
	Amount        float64   `json:"amount"`
	Currency      string    `json:"currency"`
	Merchant      string    `json:"merchant"`
	Location      string    `json:"location"`
	Country       string    `json:"country"`
	Channel       string    `json:"channel"`
	Timestamp     time.Time `json:"timestamp"`
	RetryCount    int       `json:"retry_count"`
}

// RiskFeatures represents computed risk features
type RiskFeatures struct {
	// Spending patterns
	RollingAvgSpend7d      float64 `json:"rolling_avg_spend_7d"`
	RollingAvgSpend30d     float64 `json:"rolling_avg_spend_30d"`
	RollingStdDev30d       float64 `json:"rolling_std_dev_30d"`
	SpendingZScore         float64 `json:"spending_z_score"`         // How many std devs from mean
	
	// Velocity metrics
	TransactionVelocity1h  int     `json:"transaction_velocity_1h"`
	TransactionVelocity24h int     `json:"transaction_velocity_24h"`
	VelocityZScore         float64 `json:"velocity_z_score"`         // Velocity anomaly score
	
	// Location patterns
	UniqueLocations7d      int     `json:"unique_locations_7d"`
	LocationChangeCount    int     `json:"location_change_count"`
	IsNewLocation          bool    `json:"is_new_location"`
	IsHighRiskCountry      bool    `json:"is_high_risk_country"`
	DistanceFromLastTx     float64 `json:"distance_from_last_tx_km"` // Geo distance
	
	// Merchant patterns
	IsNewMerchant          bool    `json:"is_new_merchant"`
	MerchantRiskScore      float64 `json:"merchant_risk_score"`      // Historical risk of merchant
	
	// Temporal patterns
	TimeSinceLastTx        float64 `json:"time_since_last_tx_hours"`
	IsUnusualHour          bool    `json:"is_unusual_hour"`          // Based on user's pattern
	DayOfWeekAnomaly       bool    `json:"day_of_week_anomaly"`      // Unusual day pattern
	
	// Behavioral anomalies
	AmountDeviation        float64 `json:"amount_deviation"`
	AnomalyRatio           float64 `json:"anomaly_ratio"`
	BehavioralAnomalyScore float64 `json:"behavioral_anomaly_score"` // Composite behavioral score
	
	// Sequence patterns (for modern fraud detection)
	RecentSmallTxCount     int     `json:"recent_small_tx_count"`    // Small txns in last 10 min
	FollowsProbePattern    bool    `json:"follows_probe_pattern"`    // Small tx followed by large
	SharedBeneficiaryCount int     `json:"shared_beneficiary_count"` // Accounts sharing same target
	
	// Peer group comparison
	PeerGroupAvgSpend      float64 `json:"peer_group_avg_spend"`     // Similar accounts' avg
	PeerGroupDeviation     float64 `json:"peer_group_deviation"`     // Deviation from peer group
	
	// Device/channel patterns
	IsNewDevice            bool    `json:"is_new_device"`
	ChannelSwitchCount     int     `json:"channel_switch_count"`     // online→pos→atm changes
}

// Rule represents a scoring rule
type Rule struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Condition   string  `json:"condition"` // JSON condition expression
	ScoreImpact float64 `json:"score_impact"`
	RiskLevel   string  `json:"risk_level"`
	Priority    int     `json:"priority"`
	Enabled     bool    `json:"enabled"`
}

// JSONB is a helper type for PostgreSQL JSONB columns
type JSONB map[string]interface{}

func (j JSONB) Value() ([]byte, error) {
	return json.Marshal(j)
}

func (j *JSONB) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(bytes, j)
}

// Pagination represents pagination parameters
type Pagination struct {
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
	Total    int `json:"total"`
}

// PaginatedResponse wraps paginated results
type PaginatedResponse struct {
	Data       interface{} `json:"data"`
	Pagination Pagination  `json:"pagination"`
}

// RiskSummary represents aggregated risk statistics
type RiskSummary struct {
	Date               string  `json:"date"`
	TotalTransactions  int     `json:"total_transactions"`
	TotalAmount        float64 `json:"total_amount"`
	FlaggedCount       int     `json:"flagged_count"`
	BlockedCount       int     `json:"blocked_count"`
	AvgRiskScore       float64 `json:"avg_risk_score"`
	HighRiskCount      int     `json:"high_risk_count"`
	CriticalRiskCount  int     `json:"critical_risk_count"`
	TopRulesTriggered  []RuleCount `json:"top_rules_triggered"`
}

// RuleCount represents a rule and its trigger count
type RuleCount struct {
	RuleID string `json:"rule_id"`
	Count  int    `json:"count"`
}

// AccountRiskProfile represents an account's risk profile
type AccountRiskProfile struct {
	AccountID           uuid.UUID `json:"account_id"`
	CurrentRiskLevel    string    `json:"current_risk_level"`
	AvgTransactionAmount float64  `json:"avg_transaction_amount"`
	TransactionCount30d int       `json:"transaction_count_30d"`
	FlaggedCount30d     int       `json:"flagged_count_30d"`
	LastTransactionAt   *time.Time `json:"last_transaction_at"`
	RiskTrend           string    `json:"risk_trend"` // increasing, stable, decreasing
}

// SystemMetrics represents system health metrics
type SystemMetrics struct {
	Timestamp           time.Time `json:"timestamp"`
	TransactionsPerSec  float64   `json:"transactions_per_sec"`
	AvgProcessingTimeMs float64   `json:"avg_processing_time_ms"`
	QueueDepth          int       `json:"queue_depth"`
	ActiveWorkers       int       `json:"active_workers"`
	DBConnectionsActive int       `json:"db_connections_active"`
	DBConnectionsIdle   int       `json:"db_connections_idle"`
	RedisMemoryUsedMB   float64   `json:"redis_memory_used_mb"`
	ErrorRate           float64   `json:"error_rate"`
}
