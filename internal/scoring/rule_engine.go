package scoring

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/enterprise/risk-engine/internal/models"
)

// RuleEngine evaluates JSON-configured rules from the database
type RuleEngine struct {
	mu           sync.RWMutex
	rules        []DBRule
	lastReload   time.Time
	reloadPeriod time.Duration
}

// DBRule represents a rule loaded from the database
type DBRule struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Condition   RuleCondition   `json:"condition"`
	ScoreImpact float64         `json:"score_impact"`
	RiskLevel   string          `json:"risk_level"`
	Priority    int             `json:"priority"`
	Enabled     bool            `json:"enabled"`
}

// RuleCondition represents a rule condition
type RuleCondition struct {
	Type       string          `json:"type"`       // threshold, compound, time_range
	Field      string          `json:"field"`      // field to check
	Operator   string          `json:"operator"`   // >, <, =, >=, <=, !=, AND, OR
	Value      interface{}     `json:"value"`      // value to compare
	Conditions []RuleCondition `json:"conditions"` // for compound rules
	Start      int             `json:"start"`      // for time_range
	End        int             `json:"end"`        // for time_range
}

// RuleRepository interface for fetching rules
type RuleRepository interface {
	GetEnabledRules(ctx context.Context) ([]models.Rule, error)
}

// NewRuleEngine creates a new rule engine
func NewRuleEngine(reloadPeriod time.Duration) *RuleEngine {
	return &RuleEngine{
		rules:        make([]DBRule, 0),
		reloadPeriod: reloadPeriod,
	}
}

// LoadRulesFromDB loads rules from the database
func (re *RuleEngine) LoadRulesFromDB(ctx context.Context, db interface{ Query(context.Context, string, ...interface{}) (interface{ Close(); Next() bool; Scan(...interface{}) error }, error) }) error {
	// Query for loading rules from database
	// In production, this would execute:
	// SELECT id, name, description, condition, score_impact, risk_level, priority, enabled
	// FROM rules WHERE enabled = true ORDER BY priority ASC

	// This is a simplified version - in production, use proper repository
	re.mu.Lock()
	defer re.mu.Unlock()

	// For now, we'll use the hardcoded rules as default
	// In production, this would query the database
	re.rules = getDefaultDBRules()
	re.lastReload = time.Now()

	log.Info().Int("rule_count", len(re.rules)).Msg("Rules loaded from database")
	return nil
}

// getDefaultDBRules returns the default rules in DB format
func getDefaultDBRules() []DBRule {
	return []DBRule{
		{
			ID:          "RULE_CRITICAL_AMOUNT",
			Name:        "Critical Amount",
			Description: "Extremely high transaction amount",
			Condition: RuleCondition{
				Type:     "threshold",
				Field:    "amount",
				Operator: ">",
				Value:    float64(10000),
			},
			ScoreImpact: 40.0,
			RiskLevel:   models.RiskLevelCritical,
			Priority:    5,
			Enabled:     true,
		},
		{
			ID:          "RULE_SPIKE_ANOMALY",
			Name:        "Spike Anomaly",
			Description: "Transaction amount significantly higher than average",
			Condition: RuleCondition{
				Type:     "threshold",
				Field:    "amount_deviation",
				Operator: ">",
				Value:    float64(3.0),
			},
			ScoreImpact: 30.0,
			RiskLevel:   models.RiskLevelHigh,
			Priority:    10,
			Enabled:     true,
		},
		{
			ID:          "RULE_HIGH_RISK_COUNTRY",
			Name:        "High Risk Country",
			Description: "Transaction from high-risk country",
			Condition: RuleCondition{
				Type:     "threshold",
				Field:    "is_high_risk_country",
				Operator: "=",
				Value:    true,
			},
			ScoreImpact: 35.0,
			RiskLevel:   models.RiskLevelHigh,
			Priority:    15,
			Enabled:     true,
		},
		{
			ID:          "RULE_NEW_LOCATION_HIGH_AMOUNT",
			Name:        "New Location High Amount",
			Description: "High amount transaction from a new location",
			Condition: RuleCondition{
				Type:     "compound",
				Operator: "AND",
				Conditions: []RuleCondition{
					{Type: "threshold", Field: "is_new_location", Operator: "=", Value: true},
					{Type: "threshold", Field: "amount", Operator: ">", Value: float64(1000)},
				},
			},
			ScoreImpact: 25.0,
			RiskLevel:   models.RiskLevelMedium,
			Priority:    20,
			Enabled:     true,
		},
		{
			ID:          "RULE_RAPID_SMALL_TRANSACTIONS",
			Name:        "Rapid Small Transactions",
			Description: "Many small transactions in quick succession",
			Condition: RuleCondition{
				Type:     "compound",
				Operator: "AND",
				Conditions: []RuleCondition{
					{Type: "threshold", Field: "transaction_velocity_1h", Operator: ">", Value: float64(5)},
					{Type: "threshold", Field: "amount", Operator: "<", Value: float64(100)},
				},
			},
			ScoreImpact: 25.0,
			RiskLevel:   models.RiskLevelHigh,
			Priority:    25,
			Enabled:     true,
		},
		{
			ID:          "RULE_VELOCITY_BURST",
			Name:        "Velocity Burst",
			Description: "Too many transactions in short time period",
			Condition: RuleCondition{
				Type:     "threshold",
				Field:    "transaction_velocity_1h",
				Operator: ">",
				Value:    float64(10),
			},
			ScoreImpact: 20.0,
			RiskLevel:   models.RiskLevelMedium,
			Priority:    30,
			Enabled:     true,
		},
		{
			ID:          "RULE_LOCATION_HOPPING",
			Name:        "Location Hopping",
			Description: "Multiple location changes in short period",
			Condition: RuleCondition{
				Type:     "threshold",
				Field:    "location_change_count",
				Operator: ">",
				Value:    float64(3),
			},
			ScoreImpact: 15.0,
			RiskLevel:   models.RiskLevelMedium,
			Priority:    40,
			Enabled:     true,
		},
		{
			ID:          "RULE_NEW_MERCHANT_HIGH_AMOUNT",
			Name:        "New Merchant High Amount",
			Description: "High amount at new merchant",
			Condition: RuleCondition{
				Type:     "compound",
				Operator: "AND",
				Conditions: []RuleCondition{
					{Type: "threshold", Field: "is_new_merchant", Operator: "=", Value: true},
					{Type: "threshold", Field: "amount", Operator: ">", Value: float64(500)},
				},
			},
			ScoreImpact: 15.0,
			RiskLevel:   models.RiskLevelMedium,
			Priority:    50,
			Enabled:     true,
		},
		{
			ID:          "RULE_NIGHT_TRANSACTION",
			Name:        "Night Transaction",
			Description: "Transaction during unusual hours (midnight to 5am)",
			Condition: RuleCondition{
				Type:  "time_range",
				Field: "hour",
				Start: 0,
				End:   5,
			},
			ScoreImpact: 10.0,
			RiskLevel:   models.RiskLevelLow,
			Priority:    60,
			Enabled:     true,
		},
	}
}

// Evaluate evaluates all rules against features and transaction
func (re *RuleEngine) Evaluate(features *models.RiskFeatures, tx *models.Transaction) (float64, []string) {
	re.mu.RLock()
	defer re.mu.RUnlock()

	var totalScore float64
	var triggeredRules []string

	// Build evaluation context
	ctx := buildEvaluationContext(features, tx)

	for _, rule := range re.rules {
		if !rule.Enabled {
			continue
		}

		if re.evaluateCondition(rule.Condition, ctx) {
			totalScore += rule.ScoreImpact
			triggeredRules = append(triggeredRules, rule.ID)
		}
	}

	// Cap at 100
	if totalScore > 100 {
		totalScore = 100
	}

	return totalScore, triggeredRules
}

// evaluationContext holds all values for rule evaluation
type evaluationContext struct {
	Amount               float64
	AmountDeviation      float64
	TransactionVelocity1h int
	TransactionVelocity24h int
	LocationChangeCount  int
	IsNewLocation        bool
	IsNewMerchant        bool
	IsHighRiskCountry    bool
	Hour                 int
}

func buildEvaluationContext(features *models.RiskFeatures, tx *models.Transaction) evaluationContext {
	return evaluationContext{
		Amount:               tx.Amount,
		AmountDeviation:      features.AmountDeviation,
		TransactionVelocity1h: features.TransactionVelocity1h,
		TransactionVelocity24h: features.TransactionVelocity24h,
		LocationChangeCount:  features.LocationChangeCount,
		IsNewLocation:        features.IsNewLocation,
		IsNewMerchant:        features.IsNewMerchant,
		IsHighRiskCountry:    features.IsHighRiskCountry,
		Hour:                 tx.CreatedAt.Hour(),
	}
}

func (re *RuleEngine) evaluateCondition(cond RuleCondition, ctx evaluationContext) bool {
	switch cond.Type {
	case "threshold":
		return re.evaluateThreshold(cond, ctx)
	case "compound":
		return re.evaluateCompound(cond, ctx)
	case "time_range":
		return re.evaluateTimeRange(cond, ctx)
	default:
		return false
	}
}

func (re *RuleEngine) evaluateThreshold(cond RuleCondition, ctx evaluationContext) bool {
	fieldValue := re.getFieldValue(cond.Field, ctx)
	condValue := cond.Value

	switch cond.Operator {
	case ">":
		return compareFloat(fieldValue, condValue, func(a, b float64) bool { return a > b })
	case "<":
		return compareFloat(fieldValue, condValue, func(a, b float64) bool { return a < b })
	case ">=":
		return compareFloat(fieldValue, condValue, func(a, b float64) bool { return a >= b })
	case "<=":
		return compareFloat(fieldValue, condValue, func(a, b float64) bool { return a <= b })
	case "=", "==":
		return compareEqual(fieldValue, condValue)
	case "!=":
		return !compareEqual(fieldValue, condValue)
	default:
		return false
	}
}

func (re *RuleEngine) evaluateCompound(cond RuleCondition, ctx evaluationContext) bool {
	if len(cond.Conditions) == 0 {
		return false
	}

	switch cond.Operator {
	case "AND":
		for _, subCond := range cond.Conditions {
			if !re.evaluateCondition(subCond, ctx) {
				return false
			}
		}
		return true
	case "OR":
		for _, subCond := range cond.Conditions {
			if re.evaluateCondition(subCond, ctx) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func (re *RuleEngine) evaluateTimeRange(cond RuleCondition, ctx evaluationContext) bool {
	hour := ctx.Hour
	return hour >= cond.Start && hour < cond.End
}

func (re *RuleEngine) getFieldValue(field string, ctx evaluationContext) interface{} {
	switch field {
	case "amount":
		return ctx.Amount
	case "amount_deviation":
		return ctx.AmountDeviation
	case "transaction_velocity_1h":
		return float64(ctx.TransactionVelocity1h)
	case "transaction_velocity_24h":
		return float64(ctx.TransactionVelocity24h)
	case "location_change_count":
		return float64(ctx.LocationChangeCount)
	case "is_new_location":
		return ctx.IsNewLocation
	case "is_new_merchant":
		return ctx.IsNewMerchant
	case "is_high_risk_country":
		return ctx.IsHighRiskCountry
	case "hour":
		return float64(ctx.Hour)
	default:
		return nil
	}
}

func compareFloat(a, b interface{}, cmp func(float64, float64) bool) bool {
	aFloat, aOk := toFloat64(a)
	bFloat, bOk := toFloat64(b)
	if !aOk || !bOk {
		return false
	}
	return cmp(aFloat, bFloat)
}

func compareEqual(a, b interface{}) bool {
	// Handle bool comparison
	if aBool, ok := a.(bool); ok {
		if bBool, ok := b.(bool); ok {
			return aBool == bBool
		}
	}
	// Handle numeric comparison
	aFloat, aOk := toFloat64(a)
	bFloat, bOk := toFloat64(b)
	if aOk && bOk {
		return aFloat == bFloat
	}
	// Fallback to string comparison
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

func toFloat64(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case int32:
		return float64(val), true
	case json.Number:
		f, err := val.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

// GetRules returns the current rules (for API exposure)
func (re *RuleEngine) GetRules() []DBRule {
	re.mu.RLock()
	defer re.mu.RUnlock()
	
	rules := make([]DBRule, len(re.rules))
	copy(rules, re.rules)
	return rules
}

// UpdateRule updates a single rule (for hot-reload)
func (re *RuleEngine) UpdateRule(rule DBRule) {
	re.mu.Lock()
	defer re.mu.Unlock()

	for i, r := range re.rules {
		if r.ID == rule.ID {
			re.rules[i] = rule
			log.Info().Str("rule_id", rule.ID).Msg("Rule updated")
			return
		}
	}
	// Add new rule
	re.rules = append(re.rules, rule)
	log.Info().Str("rule_id", rule.ID).Msg("New rule added")
}
