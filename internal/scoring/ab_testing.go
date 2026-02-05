package scoring

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/enterprise/risk-engine/internal/models"
	"github.com/enterprise/risk-engine/internal/queue"
)

// ABTestManager manages A/B testing experiments for the scoring engine
type ABTestManager struct {
	mu          sync.RWMutex
	experiments map[string]*Experiment
	results     map[string]*ExperimentResults
	cacheClient *queue.CacheClient
}

// Experiment represents an A/B test experiment
type Experiment struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Description  string            `json:"description"`
	Status       ExperimentStatus  `json:"status"`
	ControlRules []string          `json:"control_rules"`  // Rule IDs for control group
	TestRules    []string          `json:"test_rules"`     // Rule IDs for test group (can be modified rules)
	TrafficSplit float64           `json:"traffic_split"`  // 0.0-1.0, percentage going to test group
	StartTime    time.Time         `json:"start_time"`
	EndTime      *time.Time        `json:"end_time,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// ExperimentStatus represents the status of an experiment
type ExperimentStatus string

const (
	ExperimentStatusDraft     ExperimentStatus = "draft"
	ExperimentStatusRunning   ExperimentStatus = "running"
	ExperimentStatusPaused    ExperimentStatus = "paused"
	ExperimentStatusCompleted ExperimentStatus = "completed"
)

// ExperimentResults tracks the results of an A/B test
type ExperimentResults struct {
	ExperimentID string    `json:"experiment_id"`
	Control      GroupStats `json:"control"`
	Test         GroupStats `json:"test"`
	StartTime    time.Time  `json:"start_time"`
	LastUpdated  time.Time  `json:"last_updated"`
}

// GroupStats holds statistics for a test group
type GroupStats struct {
	TotalTransactions int              `json:"total_transactions"`
	TotalAmount       float64          `json:"total_amount"`
	AvgRiskScore      float64          `json:"avg_risk_score"`
	RiskDistribution  map[string]int   `json:"risk_distribution"`
	FlaggedCount      int              `json:"flagged_count"`
	BlockedCount      int              `json:"blocked_count"`
	RulesTriggered    map[string]int   `json:"rules_triggered"`
	ScoreSum          float64          `json:"-"` // Internal for calculating avg
}

// ABTestDecision represents which group a transaction was assigned to
type ABTestDecision struct {
	ExperimentID string `json:"experiment_id"`
	Group        string `json:"group"` // "control" or "test"
	RuleSet      string `json:"rule_set"`
}

// NewABTestManager creates a new A/B test manager
func NewABTestManager(cacheClient *queue.CacheClient) *ABTestManager {
	return &ABTestManager{
		experiments: make(map[string]*Experiment),
		results:     make(map[string]*ExperimentResults),
		cacheClient: cacheClient,
	}
}

// CreateExperiment creates a new A/B test experiment
func (m *ABTestManager) CreateExperiment(exp *Experiment) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if exp.ID == "" {
		exp.ID = uuid.New().String()
	}
	exp.Status = ExperimentStatusDraft
	exp.CreatedAt = time.Now()
	exp.UpdatedAt = time.Now()

	// Validate traffic split
	if exp.TrafficSplit < 0 || exp.TrafficSplit > 1 {
		return fmt.Errorf("traffic_split must be between 0.0 and 1.0")
	}

	m.experiments[exp.ID] = exp
	m.results[exp.ID] = &ExperimentResults{
		ExperimentID: exp.ID,
		Control: GroupStats{
			RiskDistribution: make(map[string]int),
			RulesTriggered:   make(map[string]int),
		},
		Test: GroupStats{
			RiskDistribution: make(map[string]int),
			RulesTriggered:   make(map[string]int),
		},
		StartTime:   time.Now(),
		LastUpdated: time.Now(),
	}

	log.Info().
		Str("experiment_id", exp.ID).
		Str("name", exp.Name).
		Float64("traffic_split", exp.TrafficSplit).
		Msg("A/B test experiment created")

	return nil
}

// StartExperiment starts an experiment
func (m *ABTestManager) StartExperiment(experimentID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	exp, exists := m.experiments[experimentID]
	if !exists {
		return fmt.Errorf("experiment not found: %s", experimentID)
	}

	if exp.Status == ExperimentStatusRunning {
		return fmt.Errorf("experiment is already running")
	}

	exp.Status = ExperimentStatusRunning
	exp.StartTime = time.Now()
	exp.UpdatedAt = time.Now()

	// Reset results
	m.results[experimentID] = &ExperimentResults{
		ExperimentID: experimentID,
		Control: GroupStats{
			RiskDistribution: make(map[string]int),
			RulesTriggered:   make(map[string]int),
		},
		Test: GroupStats{
			RiskDistribution: make(map[string]int),
			RulesTriggered:   make(map[string]int),
		},
		StartTime:   time.Now(),
		LastUpdated: time.Now(),
	}

	log.Info().Str("experiment_id", experimentID).Msg("A/B test experiment started")
	return nil
}

// StopExperiment stops an experiment
func (m *ABTestManager) StopExperiment(experimentID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	exp, exists := m.experiments[experimentID]
	if !exists {
		return fmt.Errorf("experiment not found: %s", experimentID)
	}

	exp.Status = ExperimentStatusCompleted
	now := time.Now()
	exp.EndTime = &now
	exp.UpdatedAt = now

	log.Info().Str("experiment_id", experimentID).Msg("A/B test experiment stopped")
	return nil
}

// PauseExperiment pauses an experiment
func (m *ABTestManager) PauseExperiment(experimentID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	exp, exists := m.experiments[experimentID]
	if !exists {
		return fmt.Errorf("experiment not found: %s", experimentID)
	}

	exp.Status = ExperimentStatusPaused
	exp.UpdatedAt = time.Now()

	log.Info().Str("experiment_id", experimentID).Msg("A/B test experiment paused")
	return nil
}

// GetActiveExperiments returns all running experiments
func (m *ABTestManager) GetActiveExperiments() []*Experiment {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var active []*Experiment
	for _, exp := range m.experiments {
		if exp.Status == ExperimentStatusRunning {
			active = append(active, exp)
		}
	}
	return active
}

// GetExperiment returns a specific experiment
func (m *ABTestManager) GetExperiment(experimentID string) (*Experiment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	exp, exists := m.experiments[experimentID]
	if !exists {
		return nil, fmt.Errorf("experiment not found: %s", experimentID)
	}
	return exp, nil
}

// GetAllExperiments returns all experiments
func (m *ABTestManager) GetAllExperiments() []*Experiment {
	m.mu.RLock()
	defer m.mu.RUnlock()

	experiments := make([]*Experiment, 0, len(m.experiments))
	for _, exp := range m.experiments {
		experiments = append(experiments, exp)
	}
	return experiments
}

// AssignGroup determines which group a transaction should be assigned to
// Uses consistent hashing based on account_id to ensure same account always gets same group
func (m *ABTestManager) AssignGroup(experimentID, accountID string) (*ABTestDecision, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	exp, exists := m.experiments[experimentID]
	if !exists {
		return nil, fmt.Errorf("experiment not found: %s", experimentID)
	}

	if exp.Status != ExperimentStatusRunning {
		return nil, fmt.Errorf("experiment is not running")
	}

	// Use consistent hashing to assign group
	// This ensures the same account always gets the same group
	hash := sha256.Sum256([]byte(experimentID + ":" + accountID))
	hashHex := hex.EncodeToString(hash[:])
	
	// Convert first 8 chars of hash to a number between 0-1
	hashValue := 0.0
	for i := 0; i < 8; i++ {
		hashValue = hashValue*16 + float64(hexCharToInt(hashHex[i]))
	}
	hashValue = hashValue / math.Pow(16, 8)

	decision := &ABTestDecision{
		ExperimentID: experimentID,
	}

	if hashValue < exp.TrafficSplit {
		decision.Group = "test"
		decision.RuleSet = "test"
	} else {
		decision.Group = "control"
		decision.RuleSet = "control"
	}

	return decision, nil
}

// RecordResult records a scoring result for an experiment
func (m *ABTestManager) RecordResult(experimentID string, decision *ABTestDecision, score *models.RiskScore, tx *models.Transaction) {
	m.mu.Lock()
	defer m.mu.Unlock()

	results, exists := m.results[experimentID]
	if !exists {
		return
	}

	var stats *GroupStats
	if decision.Group == "test" {
		stats = &results.Test
	} else {
		stats = &results.Control
	}

	stats.TotalTransactions++
	stats.TotalAmount += tx.Amount
	stats.ScoreSum += score.Score
	stats.AvgRiskScore = stats.ScoreSum / float64(stats.TotalTransactions)
	stats.RiskDistribution[score.RiskLevel]++

	if score.RiskLevel == models.RiskLevelHigh {
		stats.FlaggedCount++
	} else if score.RiskLevel == models.RiskLevelCritical {
		stats.BlockedCount++
	}

	for _, ruleID := range score.RulesTriggered {
		stats.RulesTriggered[ruleID]++
	}

	results.LastUpdated = time.Now()
}

// GetResults returns the results of an experiment
func (m *ABTestManager) GetResults(experimentID string) (*ExperimentResults, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	results, exists := m.results[experimentID]
	if !exists {
		return nil, fmt.Errorf("results not found for experiment: %s", experimentID)
	}

	return results, nil
}

// GetStatisticalSignificance calculates if the results are statistically significant
func (m *ABTestManager) GetStatisticalSignificance(experimentID string) (*SignificanceResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	results, exists := m.results[experimentID]
	if !exists {
		return nil, fmt.Errorf("results not found for experiment: %s", experimentID)
	}

	return calculateSignificance(results), nil
}

// SignificanceResult contains statistical significance analysis
type SignificanceResult struct {
	IsSignificant       bool    `json:"is_significant"`
	ConfidenceLevel     float64 `json:"confidence_level"`     // e.g., 0.95 for 95%
	PValue              float64 `json:"p_value"`
	ScoreDifference     float64 `json:"score_difference"`     // Test avg - Control avg
	ScoreDifferencePct  float64 `json:"score_difference_pct"` // Percentage difference
	FlagRateDifference  float64 `json:"flag_rate_difference"`
	SampleSizeControl   int     `json:"sample_size_control"`
	SampleSizeTest      int     `json:"sample_size_test"`
	Recommendation      string  `json:"recommendation"`
}

func calculateSignificance(results *ExperimentResults) *SignificanceResult {
	sig := &SignificanceResult{
		SampleSizeControl: results.Control.TotalTransactions,
		SampleSizeTest:    results.Test.TotalTransactions,
		ConfidenceLevel:   0.95,
	}

	// Need minimum sample size for significance
	minSampleSize := 100
	if sig.SampleSizeControl < minSampleSize || sig.SampleSizeTest < minSampleSize {
		sig.IsSignificant = false
		sig.Recommendation = fmt.Sprintf("Need at least %d samples in each group. Control: %d, Test: %d",
			minSampleSize, sig.SampleSizeControl, sig.SampleSizeTest)
		return sig
	}

	// Calculate score difference
	sig.ScoreDifference = results.Test.AvgRiskScore - results.Control.AvgRiskScore
	if results.Control.AvgRiskScore > 0 {
		sig.ScoreDifferencePct = (sig.ScoreDifference / results.Control.AvgRiskScore) * 100
	}

	// Calculate flag rate difference
	controlFlagRate := float64(results.Control.FlaggedCount+results.Control.BlockedCount) / float64(results.Control.TotalTransactions)
	testFlagRate := float64(results.Test.FlaggedCount+results.Test.BlockedCount) / float64(results.Test.TotalTransactions)
	sig.FlagRateDifference = testFlagRate - controlFlagRate

	// Simplified significance test (Z-test approximation)
	// In production, use proper statistical library
	pooledProportion := float64(results.Control.FlaggedCount+results.Control.BlockedCount+results.Test.FlaggedCount+results.Test.BlockedCount) /
		float64(results.Control.TotalTransactions+results.Test.TotalTransactions)
	
	if pooledProportion > 0 && pooledProportion < 1 {
		standardError := math.Sqrt(pooledProportion * (1 - pooledProportion) * 
			(1/float64(results.Control.TotalTransactions) + 1/float64(results.Test.TotalTransactions)))
		
		if standardError > 0 {
			zScore := math.Abs(sig.FlagRateDifference) / standardError
			// Approximate p-value from z-score (two-tailed)
			sig.PValue = 2 * (1 - normalCDF(zScore))
			sig.IsSignificant = sig.PValue < 0.05
		}
	}

	// Generate recommendation
	if !sig.IsSignificant {
		sig.Recommendation = "Results are not statistically significant. Continue running the experiment."
	} else if sig.ScoreDifference > 0 {
		sig.Recommendation = fmt.Sprintf("Test group shows %.1f%% higher risk scores. Consider if this aligns with your goals.", sig.ScoreDifferencePct)
	} else {
		sig.Recommendation = fmt.Sprintf("Test group shows %.1f%% lower risk scores. Evaluate false negative risk.", math.Abs(sig.ScoreDifferencePct))
	}

	return sig
}

// normalCDF approximates the cumulative distribution function of the standard normal distribution
func normalCDF(x float64) float64 {
	// Approximation using error function
	return 0.5 * (1 + erf(x/math.Sqrt2))
}

// erf approximates the error function
func erf(x float64) float64 {
	// Horner form coefficients for approximation
	a1 := 0.254829592
	a2 := -0.284496736
	a3 := 1.421413741
	a4 := -1.453152027
	a5 := 1.061405429
	p := 0.3275911

	sign := 1.0
	if x < 0 {
		sign = -1.0
	}
	x = math.Abs(x)

	t := 1.0 / (1.0 + p*x)
	y := 1.0 - (((((a5*t+a4)*t)+a3)*t+a2)*t+a1)*t*math.Exp(-x*x)

	return sign * y
}

func hexCharToInt(c byte) float64 {
	if c >= '0' && c <= '9' {
		return float64(c - '0')
	}
	if c >= 'a' && c <= 'f' {
		return float64(c - 'a' + 10)
	}
	if c >= 'A' && c <= 'F' {
		return float64(c - 'A' + 10)
	}
	return 0
}

// DeleteExperiment removes an experiment
func (m *ABTestManager) DeleteExperiment(experimentID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.experiments[experimentID]; !exists {
		return fmt.Errorf("experiment not found: %s", experimentID)
	}

	delete(m.experiments, experimentID)
	delete(m.results, experimentID)

	log.Info().Str("experiment_id", experimentID).Msg("A/B test experiment deleted")
	return nil
}

// ExportResults exports experiment results as JSON
func (m *ABTestManager) ExportResults(experimentID string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	exp, expExists := m.experiments[experimentID]
	results, resExists := m.results[experimentID]

	if !expExists || !resExists {
		return nil, fmt.Errorf("experiment not found: %s", experimentID)
	}

	export := struct {
		Experiment   *Experiment          `json:"experiment"`
		Results      *ExperimentResults   `json:"results"`
		Significance *SignificanceResult  `json:"significance"`
		ExportedAt   time.Time            `json:"exported_at"`
	}{
		Experiment:   exp,
		Results:      results,
		Significance: calculateSignificance(results),
		ExportedAt:   time.Now(),
	}

	return json.MarshalIndent(export, "", "  ")
}

// SaveToDB persists experiment to database (placeholder for future implementation)
func (m *ABTestManager) SaveToDB(ctx context.Context, experimentID string) error {
	// TODO: Implement database persistence
	// This would save to an experiments table
	log.Debug().Str("experiment_id", experimentID).Msg("SaveToDB called (not yet implemented)")
	return nil
}

// LoadFromDB loads experiments from database (placeholder for future implementation)
func (m *ABTestManager) LoadFromDB(ctx context.Context) error {
	// TODO: Implement database loading
	// This would load from an experiments table
	log.Debug().Msg("LoadFromDB called (not yet implemented)")
	return nil
}
