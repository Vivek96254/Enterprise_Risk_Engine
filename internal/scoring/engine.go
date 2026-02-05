package scoring

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/enterprise/risk-engine/internal/models"
	"github.com/enterprise/risk-engine/internal/queue"
	"github.com/enterprise/risk-engine/internal/repositories"
)

// High-risk countries list (example)
var highRiskCountries = map[string]bool{
	"NK": true, "IR": true, "SY": true, "CU": true,
	"VE": true, "MM": true, "BY": true, "ZW": true,
}

// ScoringEngine computes risk scores for transactions
type ScoringEngine struct {
	txRepo        *repositories.TransactionRepository
	accountRepo   *repositories.AccountRepository
	riskScoreRepo *repositories.RiskScoreRepository
	cacheClient   *queue.CacheClient
	rules         []Rule
	modelVersion  string
	abTestManager *ABTestManager
	mlScorer      *MLScorer
	
	// Scoring weights for hybrid model
	ruleWeight       float64
	mlWeight         float64
	behavioralWeight float64
}

// Rule represents a scoring rule
type Rule struct {
	ID          string
	Name        string
	Evaluate    func(features *models.RiskFeatures, tx *models.Transaction) bool
	ScoreImpact float64
	RiskLevel   string
	Priority    int
}

// NewScoringEngine creates a new scoring engine
func NewScoringEngine(
	txRepo *repositories.TransactionRepository,
	accountRepo *repositories.AccountRepository,
	riskScoreRepo *repositories.RiskScoreRepository,
	cacheClient *queue.CacheClient,
) *ScoringEngine {
	engine := &ScoringEngine{
		txRepo:        txRepo,
		accountRepo:   accountRepo,
		riskScoreRepo: riskScoreRepo,
		cacheClient:   cacheClient,
		modelVersion:  "v2.0.0-hybrid",
		abTestManager: NewABTestManager(cacheClient),
		
		// Hybrid scoring weights (Rule + Behavioral + ML)
		// Final Score = (ruleWeight * RuleScore) + (behavioralWeight * BehavioralScore) + (mlWeight * MLScore)
		ruleWeight:       0.50, // 50% from rule engine
		behavioralWeight: 0.35, // 35% from behavioral analysis
		mlWeight:         0.15, // 15% from ML (when available)
	}

	// Initialize ML scorer
	engine.mlScorer = NewMLScorer(txRepo, MLScorerConfig{
		Enabled:      true,
		ModelVersion: "behavioral-v1",
	})

	// Initialize built-in rules
	engine.initializeRules()

	return engine
}

// GetABTestManager returns the A/B test manager
func (e *ScoringEngine) GetABTestManager() *ABTestManager {
	return e.abTestManager
}

// initializeRules sets up the default scoring rules
func (e *ScoringEngine) initializeRules() {
	e.rules = []Rule{
		{
			ID:          "RULE_CRITICAL_AMOUNT",
			Name:        "Critical Amount",
			ScoreImpact: 40.0,
			RiskLevel:   models.RiskLevelCritical,
			Priority:    5,
			Evaluate: func(features *models.RiskFeatures, tx *models.Transaction) bool {
				return tx.Amount > 10000
			},
		},
		{
			ID:          "RULE_SPIKE_ANOMALY",
			Name:        "Spike Anomaly",
			ScoreImpact: 30.0,
			RiskLevel:   models.RiskLevelHigh,
			Priority:    10,
			Evaluate: func(features *models.RiskFeatures, tx *models.Transaction) bool {
				return features.AmountDeviation > 3.0
			},
		},
		{
			ID:          "RULE_HIGH_RISK_COUNTRY",
			Name:        "High Risk Country",
			ScoreImpact: 35.0,
			RiskLevel:   models.RiskLevelHigh,
			Priority:    15,
			Evaluate: func(features *models.RiskFeatures, tx *models.Transaction) bool {
				return features.IsHighRiskCountry
			},
		},
		{
			ID:          "RULE_NEW_LOCATION_HIGH_AMOUNT",
			Name:        "New Location High Amount",
			ScoreImpact: 25.0,
			RiskLevel:   models.RiskLevelMedium,
			Priority:    20,
			Evaluate: func(features *models.RiskFeatures, tx *models.Transaction) bool {
				return features.IsNewLocation && tx.Amount > 1000
			},
		},
		{
			ID:          "RULE_RAPID_SMALL_TRANSACTIONS",
			Name:        "Rapid Small Transactions",
			ScoreImpact: 25.0,
			RiskLevel:   models.RiskLevelHigh,
			Priority:    25,
			Evaluate: func(features *models.RiskFeatures, tx *models.Transaction) bool {
				return features.TransactionVelocity1h > 5 && tx.Amount < 100
			},
		},
		{
			ID:          "RULE_VELOCITY_BURST",
			Name:        "Velocity Burst",
			ScoreImpact: 20.0,
			RiskLevel:   models.RiskLevelMedium,
			Priority:    30,
			Evaluate: func(features *models.RiskFeatures, tx *models.Transaction) bool {
				return features.TransactionVelocity1h > 10
			},
		},
		{
			ID:          "RULE_LOCATION_HOPPING",
			Name:        "Location Hopping",
			ScoreImpact: 15.0,
			RiskLevel:   models.RiskLevelMedium,
			Priority:    40,
			Evaluate: func(features *models.RiskFeatures, tx *models.Transaction) bool {
				return features.LocationChangeCount > 3
			},
		},
		{
			ID:          "RULE_NEW_MERCHANT_HIGH_AMOUNT",
			Name:        "New Merchant High Amount",
			ScoreImpact: 15.0,
			RiskLevel:   models.RiskLevelMedium,
			Priority:    50,
			Evaluate: func(features *models.RiskFeatures, tx *models.Transaction) bool {
				return features.IsNewMerchant && tx.Amount > 500
			},
		},
		{
			ID:          "RULE_NIGHT_TRANSACTION",
			Name:        "Night Transaction",
			ScoreImpact: 10.0,
			RiskLevel:   models.RiskLevelLow,
			Priority:    60,
			Evaluate: func(features *models.RiskFeatures, tx *models.Transaction) bool {
				hour := tx.CreatedAt.Hour()
				return hour >= 0 && hour < 5
			},
		},
		// Modern fraud pattern rules
		{
			ID:          "RULE_SEQUENCE_EXFIL_PATTERN",
			Name:        "Sequence Exfiltration Pattern",
			ScoreImpact: 35.0,
			RiskLevel:   models.RiskLevelHigh,
			Priority:    8,
			Evaluate: func(features *models.RiskFeatures, tx *models.Transaction) bool {
				// Small probe transaction followed by large transaction within short time
				// Pattern: test with small amount, then exfiltrate with large amount
				return features.FollowsProbePattern && tx.Amount > 1000
			},
		},
		{
			ID:          "RULE_PEER_GROUP_ANOMALY",
			Name:        "Peer Group Anomaly",
			ScoreImpact: 25.0,
			RiskLevel:   models.RiskLevelMedium,
			Priority:    22,
			Evaluate: func(features *models.RiskFeatures, tx *models.Transaction) bool {
				// User deviates significantly from similar accounts
				// 3 standard deviations from peer group average
				return features.PeerGroupDeviation > 3.0
			},
		},
		{
			ID:          "RULE_SHARED_BENEFICIARY_NETWORK",
			Name:        "Shared Beneficiary Network",
			ScoreImpact: 30.0,
			RiskLevel:   models.RiskLevelHigh,
			Priority:    12,
			Evaluate: func(features *models.RiskFeatures, tx *models.Transaction) bool {
				// Multiple accounts sending to same target (mule account pattern)
				return features.SharedBeneficiaryCount > 3
			},
		},
		{
			ID:          "RULE_RAPID_DEVICE_SWITCH",
			Name:        "Rapid Device Switch",
			ScoreImpact: 25.0,
			RiskLevel:   models.RiskLevelMedium,
			Priority:    18,
			Evaluate: func(features *models.RiskFeatures, tx *models.Transaction) bool {
				// New device combined with high-value transaction
				return features.IsNewDevice && tx.Amount > 500
			},
		},
		{
			ID:          "RULE_GEO_IMPOSSIBLE_TRAVEL",
			Name:        "Impossible Travel",
			ScoreImpact: 40.0,
			RiskLevel:   models.RiskLevelCritical,
			Priority:    3,
			Evaluate: func(features *models.RiskFeatures, tx *models.Transaction) bool {
				// Transaction from location impossible to reach in time
				// Speed > 900 km/h (faster than commercial flight)
				if features.DistanceFromLastTx > 0 && features.TimeSinceLastTx > 0 {
					speed := features.DistanceFromLastTx / features.TimeSinceLastTx
					return speed > 900
				}
				return false
			},
		},
		{
			ID:          "RULE_RAPID_CHANNEL_SWITCH",
			Name:        "Rapid Channel Switching",
			ScoreImpact: 15.0,
			RiskLevel:   models.RiskLevelMedium,
			Priority:    45,
			Evaluate: func(features *models.RiskFeatures, tx *models.Transaction) bool {
				// Switching between online, POS, ATM rapidly
				return features.ChannelSwitchCount > 3
			},
		},
		{
			ID:          "RULE_BEHAVIORAL_ANOMALY",
			Name:        "Behavioral Pattern Anomaly",
			ScoreImpact: 20.0,
			RiskLevel:   models.RiskLevelMedium,
			Priority:    35,
			Evaluate: func(features *models.RiskFeatures, tx *models.Transaction) bool {
				// Composite behavioral anomaly score exceeds threshold
				return features.BehavioralAnomalyScore > 50
			},
		},
	}
}

// ScoreTransaction computes the risk score for a transaction
func (e *ScoringEngine) ScoreTransaction(ctx context.Context, event *models.TransactionEvent) (*models.RiskScore, error) {
	startTime := time.Now()

	// Parse IDs
	txID, err := uuid.Parse(event.TransactionID)
	if err != nil {
		return nil, fmt.Errorf("invalid transaction_id: %w", err)
	}

	accountID, err := uuid.Parse(event.AccountID)
	if err != nil {
		return nil, fmt.Errorf("invalid account_id: %w", err)
	}

	// Get transaction details
	tx, err := e.txRepo.GetByID(ctx, txID)
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction: %w", err)
	}

	// Compute base features
	features, err := e.computeFeatures(ctx, accountID, tx)
	if err != nil {
		return nil, fmt.Errorf("failed to compute features: %w", err)
	}

	// Enhance features for ML/behavioral scoring
	e.mlScorer.ComputeEnhancedFeatures(ctx, accountID, tx, features)

	// Check for active A/B tests and assign group
	var abDecision *ABTestDecision
	activeExperiments := e.abTestManager.GetActiveExperiments()
	
	// Apply rules and compute rule score (potentially with A/B test modifications)
	var ruleScore float64
	var triggeredRules []string
	modelVersion := e.modelVersion

	if len(activeExperiments) > 0 {
		exp := activeExperiments[0]
		abDecision, err = e.abTestManager.AssignGroup(exp.ID, event.AccountID)
		if err == nil {
			if abDecision.Group == "test" {
				ruleScore, triggeredRules = e.applyRulesForABTest(features, tx, exp.TestRules)
				modelVersion = e.modelVersion + "-test-" + exp.ID[:8]
			} else {
				ruleScore, triggeredRules = e.applyRulesForABTest(features, tx, exp.ControlRules)
				modelVersion = e.modelVersion + "-control-" + exp.ID[:8]
			}
		} else {
			ruleScore, triggeredRules = e.applyRules(features, tx)
		}
	} else {
		ruleScore, triggeredRules = e.applyRules(features, tx)
	}

	// Compute ML and behavioral scores
	mlResult := e.mlScorer.Score(ctx, features, tx)

	// Compute final hybrid score
	// Final Score = (ruleWeight * RuleScore) + (behavioralWeight * BehavioralScore) + (mlWeight * MLScore)
	var finalScore float64
	if mlResult.MLScore != nil {
		finalScore = (e.ruleWeight * ruleScore) + 
		             (e.behavioralWeight * mlResult.BehavioralScore) + 
		             (e.mlWeight * *mlResult.MLScore)
	} else {
		// If no ML score, redistribute weight to rule and behavioral
		adjustedRuleWeight := e.ruleWeight + (e.mlWeight * 0.6)      // 60% of ML weight to rules
		adjustedBehavioralWeight := e.behavioralWeight + (e.mlWeight * 0.4) // 40% to behavioral
		finalScore = (adjustedRuleWeight * ruleScore) + (adjustedBehavioralWeight * mlResult.BehavioralScore)
	}

	// Normalize final score
	finalScore = math.Round(math.Min(finalScore, 100)*100) / 100

	// Determine risk level based on final score
	riskLevel := e.determineRiskLevel(finalScore)

	// Determine transaction status based on risk
	status := e.determineTransactionStatus(finalScore, riskLevel)

	// Determine scoring path (for fast-path optimization)
	scoringPath := "full"
	if ruleScore < 20 && mlResult.BehavioralScore < 15 {
		scoringPath = "fast" // Low risk, could skip some checks
	}

	// Update transaction status
	if err := e.txRepo.UpdateStatus(ctx, tx.ID, tx.CreatedAt, status); err != nil {
		log.Error().Err(err).Str("transaction_id", tx.ID.String()).Msg("Failed to update transaction status")
	}

	// Create risk score record with hybrid scores
	processingTime := time.Since(startTime)
	riskScore := &models.RiskScore{
		TransactionID:     tx.ID,
		Score:             finalScore,
		RuleScore:         ruleScore,
		MLScore:           mlResult.MLScore,
		BehavioralScore:   &mlResult.BehavioralScore,
		RiskLevel:         riskLevel,
		RulesTriggered:    triggeredRules,
		AnomaliesDetected: mlResult.AnomaliesDetected,
		Features:          e.featuresToJSONB(features),
		ModelVersion:      modelVersion,
		ScoringPath:       scoringPath,
		ProcessingTimeMs:  processingTime.Milliseconds(),
	}

	// Add A/B test info to features if applicable
	if abDecision != nil {
		riskScore.Features["ab_test_experiment"] = abDecision.ExperimentID
		riskScore.Features["ab_test_group"] = abDecision.Group
	}

	// Add scoring breakdown to features
	riskScore.Features["score_breakdown"] = map[string]interface{}{
		"rule_score":       ruleScore,
		"behavioral_score": mlResult.BehavioralScore,
		"ml_score":         mlResult.MLScore,
		"rule_weight":      e.ruleWeight,
		"behavioral_weight": e.behavioralWeight,
		"ml_weight":        e.mlWeight,
	}

	if err := e.riskScoreRepo.CreateWithTransactionTime(ctx, riskScore, tx.CreatedAt); err != nil {
		return nil, fmt.Errorf("failed to save risk score: %w", err)
	}

	// Record A/B test result if applicable
	if abDecision != nil {
		e.abTestManager.RecordResult(abDecision.ExperimentID, abDecision, riskScore, tx)
	}

	// Update account risk profile if needed
	e.updateAccountRiskProfile(ctx, accountID, riskLevel)

	// Cache the result
	e.cacheRiskScore(ctx, tx.ID.String(), riskScore)

	logEvent := log.Info().
		Str("transaction_id", tx.ID.String()).
		Float64("final_score", finalScore).
		Float64("rule_score", ruleScore).
		Float64("behavioral_score", mlResult.BehavioralScore).
		Str("risk_level", riskLevel).
		Str("scoring_path", scoringPath).
		Strs("rules_triggered", triggeredRules).
		Strs("anomalies_detected", mlResult.AnomaliesDetected).
		Int64("processing_time_ms", processingTime.Milliseconds())
	
	if mlResult.MLScore != nil {
		logEvent = logEvent.Float64("ml_score", *mlResult.MLScore)
	}
	
	if abDecision != nil {
		logEvent = logEvent.
			Str("ab_experiment", abDecision.ExperimentID).
			Str("ab_group", abDecision.Group)
	}
	
	logEvent.Msg("Transaction scored (hybrid)")

	return riskScore, nil
}

// applyRulesForABTest applies specific rules for A/B testing
func (e *ScoringEngine) applyRulesForABTest(features *models.RiskFeatures, tx *models.Transaction, ruleIDs []string) (float64, []string) {
	// If no specific rules defined, use all rules
	if len(ruleIDs) == 0 {
		return e.applyRules(features, tx)
	}

	// Create a set of allowed rule IDs
	allowedRules := make(map[string]bool)
	for _, id := range ruleIDs {
		allowedRules[id] = true
	}

	var totalScore float64
	var triggeredRules []string

	for _, rule := range e.rules {
		// Skip rules not in the allowed set
		if !allowedRules[rule.ID] {
			continue
		}

		if rule.Evaluate(features, tx) {
			totalScore += rule.ScoreImpact
			triggeredRules = append(triggeredRules, rule.ID)
		}
	}

	// Cap score at 100
	if totalScore > 100 {
		totalScore = 100
	}

	return math.Round(totalScore*100) / 100, triggeredRules
}

// computeFeatures computes risk features for a transaction
func (e *ScoringEngine) computeFeatures(ctx context.Context, accountID uuid.UUID, tx *models.Transaction) (*models.RiskFeatures, error) {
	features := &models.RiskFeatures{}

	// Get recent transactions for the account
	since7d := time.Now().Add(-7 * 24 * time.Hour)
	since30d := time.Now().Add(-30 * 24 * time.Hour)
	since1h := time.Now().Add(-1 * time.Hour)
	since24h := time.Now().Add(-24 * time.Hour)

	// Get transaction statistics
	stats, err := e.txRepo.GetTransactionStats(ctx, accountID, 30)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to get transaction stats")
	} else {
		if avgAmount, ok := stats["avg_amount"].(float64); ok {
			features.RollingAvgSpend30d = avgAmount
		}
		if stddev, ok := stats["stddev_amount"].(float64); ok && stddev > 0 {
			features.AmountDeviation = (tx.Amount - features.RollingAvgSpend30d) / stddev
		}
		if uniqueLocations, ok := stats["unique_locations"].(int); ok {
			features.UniqueLocations7d = uniqueLocations
		}
	}

	// Get 7-day average
	stats7d, err := e.txRepo.GetTransactionStats(ctx, accountID, 7)
	if err == nil {
		if avgAmount, ok := stats7d["avg_amount"].(float64); ok {
			features.RollingAvgSpend7d = avgAmount
		}
	}

	// Calculate velocity (transactions per hour and per day)
	recentTx1h, _ := e.txRepo.GetRecentByAccount(ctx, accountID, since1h)
	features.TransactionVelocity1h = len(recentTx1h)

	recentTx24h, _ := e.txRepo.GetRecentByAccount(ctx, accountID, since24h)
	features.TransactionVelocity24h = len(recentTx24h)

	// Check for location changes
	recentTx7d, _ := e.txRepo.GetRecentByAccount(ctx, accountID, since7d)
	locations := make(map[string]bool)
	var lastLocation string
	locationChanges := 0

	for _, t := range recentTx7d {
		if t.Location != "" {
			locations[t.Location] = true
			if lastLocation != "" && lastLocation != t.Location {
				locationChanges++
			}
			lastLocation = t.Location
		}
	}

	features.UniqueLocations7d = len(locations)
	features.LocationChangeCount = locationChanges

	// Check if new location
	if tx.Location != "" {
		features.IsNewLocation = !locations[tx.Location]
	}

	// Check if new merchant
	recentMerchants := make(map[string]bool)
	for _, t := range recentTx7d {
		if t.Merchant != "" {
			recentMerchants[t.Merchant] = true
		}
	}
	if tx.Merchant != "" {
		features.IsNewMerchant = !recentMerchants[tx.Merchant]
	}

	// Check for high-risk country
	if tx.Country != "" {
		features.IsHighRiskCountry = highRiskCountries[tx.Country]
	}

	// Calculate time since last transaction
	if len(recentTx24h) > 1 {
		lastTx := recentTx24h[1] // Index 0 is current transaction
		features.TimeSinceLastTx = time.Since(lastTx.CreatedAt).Hours()
	}

	// Calculate anomaly ratio (flagged transactions / total)
	flaggedCount := 0
	for _, t := range recentTx7d {
		if t.Status == models.TransactionStatusFlagged || t.Status == models.TransactionStatusBlocked {
			flaggedCount++
		}
	}
	if len(recentTx7d) > 0 {
		features.AnomalyRatio = float64(flaggedCount) / float64(len(recentTx7d))
	}

	// Get 30-day transactions for additional stats
	_ = since30d // Used in stats query

	return features, nil
}

// applyRules applies all rules and returns the score and triggered rules
func (e *ScoringEngine) applyRules(features *models.RiskFeatures, tx *models.Transaction) (float64, []string) {
	var totalScore float64
	var triggeredRules []string

	for _, rule := range e.rules {
		if rule.Evaluate(features, tx) {
			totalScore += rule.ScoreImpact
			triggeredRules = append(triggeredRules, rule.ID)
		}
	}

	// Cap score at 100
	if totalScore > 100 {
		totalScore = 100
	}

	return math.Round(totalScore*100) / 100, triggeredRules
}

// determineRiskLevel determines the risk level based on score
func (e *ScoringEngine) determineRiskLevel(score float64) string {
	switch {
	case score >= 70:
		return models.RiskLevelCritical
	case score >= 50:
		return models.RiskLevelHigh
	case score >= 25:
		return models.RiskLevelMedium
	default:
		return models.RiskLevelLow
	}
}

// determineTransactionStatus determines the transaction status based on risk
func (e *ScoringEngine) determineTransactionStatus(score float64, riskLevel string) string {
	switch riskLevel {
	case models.RiskLevelCritical:
		return models.TransactionStatusBlocked
	case models.RiskLevelHigh:
		return models.TransactionStatusFlagged
	default:
		return models.TransactionStatusProcessed
	}
}

// updateAccountRiskProfile updates the account risk profile based on scoring
func (e *ScoringEngine) updateAccountRiskProfile(ctx context.Context, accountID uuid.UUID, riskLevel string) {
	// Only escalate risk profile, don't de-escalate automatically
	if riskLevel == models.RiskLevelCritical || riskLevel == models.RiskLevelHigh {
		account, err := e.accountRepo.GetByID(ctx, accountID)
		if err != nil {
			return
		}

		newProfile := models.RiskProfileMedium
		if riskLevel == models.RiskLevelCritical {
			newProfile = models.RiskProfileHigh
		}

		// Only update if escalating
		if account.RiskProfile == models.RiskProfileLow ||
			(account.RiskProfile == models.RiskProfileMedium && newProfile == models.RiskProfileHigh) {
			if err := e.accountRepo.UpdateRiskProfile(ctx, accountID, newProfile); err != nil {
				log.Error().Err(err).Str("account_id", accountID.String()).Msg("Failed to update account risk profile")
			}
		}
	}
}

// featuresToJSONB converts features to JSONB
func (e *ScoringEngine) featuresToJSONB(features *models.RiskFeatures) models.JSONB {
	data, _ := json.Marshal(features)
	var jsonb models.JSONB
	json.Unmarshal(data, &jsonb)
	return jsonb
}

// cacheRiskScore caches the risk score
func (e *ScoringEngine) cacheRiskScore(ctx context.Context, txID string, score *models.RiskScore) {
	if e.cacheClient == nil {
		return
	}

	key := fmt.Sprintf("risk_score:%s", txID)
	if err := e.cacheClient.Set(ctx, key, score, 24*time.Hour); err != nil {
		log.Warn().Err(err).Str("transaction_id", txID).Msg("Failed to cache risk score")
	}
}

// GetCachedRiskScore retrieves a cached risk score
func (e *ScoringEngine) GetCachedRiskScore(ctx context.Context, txID string) (*models.RiskScore, error) {
	if e.cacheClient == nil {
		return nil, fmt.Errorf("cache not available")
	}

	key := fmt.Sprintf("risk_score:%s", txID)
	var score models.RiskScore
	if err := e.cacheClient.Get(ctx, key, &score); err != nil {
		return nil, err
	}
	return &score, nil
}
