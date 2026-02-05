package scoring

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/enterprise/risk-engine/internal/models"
	"github.com/enterprise/risk-engine/internal/repositories"
)

// BacktestService provides backtesting capabilities for the scoring engine
type BacktestService struct {
	engine  *ScoringEngine
	txRepo  *repositories.TransactionRepository
}

// NewBacktestService creates a new backtest service
func NewBacktestService(engine *ScoringEngine, txRepo *repositories.TransactionRepository) *BacktestService {
	return &BacktestService{
		engine: engine,
		txRepo: txRepo,
	}
}

// BacktestRequest represents a backtest request
type BacktestRequest struct {
	AccountID  string     `json:"account_id"`
	StartDate  time.Time  `json:"start_date"`
	EndDate    time.Time  `json:"end_date"`
	RuleSet    string     `json:"rule_set,omitempty"` // For A/B testing different rule sets
	SampleSize int        `json:"sample_size,omitempty"` // Limit number of transactions
}

// BacktestResult represents the result of backtesting
type BacktestResult struct {
	TotalTransactions   int                    `json:"total_transactions"`
	ProcessedCount      int                    `json:"processed_count"`
	FailedCount         int                    `json:"failed_count"`
	AverageScore        float64                `json:"average_score"`
	RiskDistribution    map[string]int         `json:"risk_distribution"`
	TopTriggeredRules   []models.RuleCount     `json:"top_triggered_rules"`
	ProcessingTimeMs    int64                  `json:"processing_time_ms"`
	TransactionResults  []TransactionBacktest  `json:"transaction_results,omitempty"`
	ComparisonWithLive  *BacktestComparison    `json:"comparison_with_live,omitempty"`
}

// TransactionBacktest represents a single transaction backtest result
type TransactionBacktest struct {
	TransactionID   string    `json:"transaction_id"`
	OriginalScore   float64   `json:"original_score,omitempty"`
	BacktestScore   float64   `json:"backtest_score"`
	OriginalLevel   string    `json:"original_level,omitempty"`
	BacktestLevel   string    `json:"backtest_level"`
	RulesTriggered  []string  `json:"rules_triggered"`
	ScoreDiff       float64   `json:"score_diff"`
}

// BacktestComparison compares backtest results with live scoring
type BacktestComparison struct {
	MatchingScores      int     `json:"matching_scores"`
	DifferentScores     int     `json:"different_scores"`
	AvgScoreDifference  float64 `json:"avg_score_difference"`
	UpgradedRisk        int     `json:"upgraded_risk"`   // Backtest scored higher
	DowngradedRisk      int     `json:"downgraded_risk"` // Backtest scored lower
}

// RunBacktest runs a backtest on historical transactions
func (s *BacktestService) RunBacktest(ctx context.Context, req *BacktestRequest) (*BacktestResult, error) {
	startTime := time.Now()

	log.Info().
		Str("account_id", req.AccountID).
		Time("start_date", req.StartDate).
		Time("end_date", req.EndDate).
		Msg("Starting backtest")

	result := &BacktestResult{
		RiskDistribution:   make(map[string]int),
		TopTriggeredRules:  make([]models.RuleCount, 0),
		TransactionResults: make([]TransactionBacktest, 0),
	}

	// Get historical transactions
	var transactions []*models.Transaction
	var err error

	if req.AccountID != "" {
		accountID, parseErr := uuid.Parse(req.AccountID)
		if parseErr != nil {
			return nil, fmt.Errorf("invalid account_id: %w", parseErr)
		}
		transactions, _, err = s.txRepo.GetByAccountID(ctx, accountID, 1, req.SampleSize, &req.StartDate, &req.EndDate)
	} else {
		// Get all transactions in date range (for system-wide backtest)
		transactions, err = s.getTransactionsInRange(ctx, req.StartDate, req.EndDate, req.SampleSize)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to fetch transactions: %w", err)
	}

	result.TotalTransactions = len(transactions)

	// Track rule triggers
	ruleTriggers := make(map[string]int)
	var totalScore float64
	var scoreDiffs []float64

	for _, tx := range transactions {
		// Create event for scoring
		event := &models.TransactionEvent{
			TransactionID: tx.ID.String(),
			AccountID:     tx.AccountID.String(),
			Amount:        tx.Amount,
			Currency:      tx.Currency,
			Merchant:      tx.Merchant,
			Location:      tx.Location,
			Country:       tx.Country,
			Channel:       tx.Channel,
			Timestamp:     tx.CreatedAt,
		}

		// Score using current engine (without persisting)
		score, err := s.engine.ScoreTransactionDryRun(ctx, event)
		if err != nil {
			result.FailedCount++
			log.Warn().Err(err).Str("tx_id", tx.ID.String()).Msg("Failed to backtest transaction")
			continue
		}

		result.ProcessedCount++
		totalScore += score.Score
		result.RiskDistribution[score.RiskLevel]++

		// Track triggered rules
		for _, ruleID := range score.RulesTriggered {
			ruleTriggers[ruleID]++
		}

		// Get original score if exists
		originalScore, _ := s.engine.riskScoreRepo.GetByTransactionID(ctx, tx.ID)
		
		txResult := TransactionBacktest{
			TransactionID:  tx.ID.String(),
			BacktestScore:  score.Score,
			BacktestLevel:  score.RiskLevel,
			RulesTriggered: score.RulesTriggered,
		}

		if originalScore != nil {
			txResult.OriginalScore = originalScore.Score
			txResult.OriginalLevel = originalScore.RiskLevel
			txResult.ScoreDiff = score.Score - originalScore.Score
			scoreDiffs = append(scoreDiffs, txResult.ScoreDiff)
		}

		// Only include detailed results if sample size is reasonable
		if req.SampleSize <= 100 || result.ProcessedCount <= 100 {
			result.TransactionResults = append(result.TransactionResults, txResult)
		}
	}

	// Calculate averages
	if result.ProcessedCount > 0 {
		result.AverageScore = totalScore / float64(result.ProcessedCount)
	}

	// Build top triggered rules
	for ruleID, count := range ruleTriggers {
		result.TopTriggeredRules = append(result.TopTriggeredRules, models.RuleCount{
			RuleID: ruleID,
			Count:  count,
		})
	}

	// Sort by count (descending)
	sortRuleCounts(result.TopTriggeredRules)

	// Limit to top 10
	if len(result.TopTriggeredRules) > 10 {
		result.TopTriggeredRules = result.TopTriggeredRules[:10]
	}

	// Build comparison if we have original scores
	if len(scoreDiffs) > 0 {
		result.ComparisonWithLive = s.buildComparison(result.TransactionResults, scoreDiffs)
	}

	result.ProcessingTimeMs = time.Since(startTime).Milliseconds()

	log.Info().
		Int("total", result.TotalTransactions).
		Int("processed", result.ProcessedCount).
		Float64("avg_score", result.AverageScore).
		Int64("processing_ms", result.ProcessingTimeMs).
		Msg("Backtest completed")

	return result, nil
}

// ScoreTransactionDryRun scores a transaction without persisting results
func (e *ScoringEngine) ScoreTransactionDryRun(ctx context.Context, event *models.TransactionEvent) (*models.RiskScore, error) {
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

	// Compute features
	features, err := e.computeFeatures(ctx, accountID, tx)
	if err != nil {
		return nil, fmt.Errorf("failed to compute features: %w", err)
	}

	// Apply rules and compute score
	score, triggeredRules := e.applyRules(features, tx)

	// Determine risk level
	riskLevel := e.determineRiskLevel(score)

	// Return without persisting
	return &models.RiskScore{
		TransactionID:  tx.ID,
		Score:          score,
		RiskLevel:      riskLevel,
		RulesTriggered: triggeredRules,
		Features:       e.featuresToJSONB(features),
		ModelVersion:   e.modelVersion + "-backtest",
	}, nil
}

func (s *BacktestService) getTransactionsInRange(ctx context.Context, start, end time.Time, limit int) ([]*models.Transaction, error) {
	// This would be a new repository method - simplified here
	if limit == 0 {
		limit = 1000 // Default limit
	}
	// For now, return empty - in production, implement proper query
	return []*models.Transaction{}, nil
}

func (s *BacktestService) buildComparison(results []TransactionBacktest, scoreDiffs []float64) *BacktestComparison {
	comparison := &BacktestComparison{}

	var totalDiff float64
	for _, result := range results {
		if result.OriginalScore == 0 && result.BacktestScore == 0 {
			comparison.MatchingScores++
		} else if result.ScoreDiff == 0 {
			comparison.MatchingScores++
		} else {
			comparison.DifferentScores++
			if result.ScoreDiff > 0 {
				comparison.UpgradedRisk++
			} else {
				comparison.DowngradedRisk++
			}
		}
		totalDiff += absFloat(result.ScoreDiff)
	}

	if len(results) > 0 {
		comparison.AvgScoreDifference = totalDiff / float64(len(results))
	}

	return comparison
}

func sortRuleCounts(rules []models.RuleCount) {
	// Simple bubble sort for small arrays
	for i := 0; i < len(rules)-1; i++ {
		for j := 0; j < len(rules)-i-1; j++ {
			if rules[j].Count < rules[j+1].Count {
				rules[j], rules[j+1] = rules[j+1], rules[j]
			}
		}
	}
}

func absFloat(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

// ABTestConfig represents A/B test configuration
type ABTestConfig struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	ControlSet  string   `json:"control_set"`  // Rule set for control group
	TestSet     string   `json:"test_set"`     // Rule set for test group
	SplitRatio  float64  `json:"split_ratio"`  // Percentage for test group (0.0-1.0)
	Enabled     bool     `json:"enabled"`
}

// ABTestResult represents A/B test comparison results
type ABTestResult struct {
	ControlResult *BacktestResult `json:"control_result"`
	TestResult    *BacktestResult `json:"test_result"`
	Improvement   float64         `json:"improvement_pct"` // Positive = test is better
	StatSignificant bool          `json:"statistically_significant"`
}
