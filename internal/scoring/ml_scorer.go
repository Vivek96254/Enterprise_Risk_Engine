package scoring

import (
	"context"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/enterprise/risk-engine/internal/models"
	"github.com/enterprise/risk-engine/internal/repositories"
)

// MLScorer provides ML-based and behavioral anomaly scoring
// This is designed as a pluggable interface for future ML model integration
type MLScorer struct {
	txRepo       *repositories.TransactionRepository
	modelVersion string
	enabled      bool
}

// MLScorerConfig configures the ML scorer
type MLScorerConfig struct {
	Enabled      bool
	ModelVersion string
	// Future: model endpoint, API key, etc.
}

// MLScoreResult contains the ML scoring output
type MLScoreResult struct {
	MLScore           *float64 `json:"ml_score"`           // From ML model (nullable if not available)
	BehavioralScore   float64  `json:"behavioral_score"`   // From statistical analysis
	AnomaliesDetected []string `json:"anomalies_detected"` // List of detected anomalies
	Confidence        float64  `json:"confidence"`         // Model confidence (0-1)
}

// AnomalyType represents types of anomalies detected
type AnomalyType string

const (
	AnomalySpendingSpike      AnomalyType = "SPENDING_SPIKE"
	AnomalyVelocityBurst      AnomalyType = "VELOCITY_BURST"
	AnomalyGeoImpossible      AnomalyType = "GEO_IMPOSSIBLE_TRAVEL"
	AnomalyPeerDeviation      AnomalyType = "PEER_GROUP_DEVIATION"
	AnomalySequenceExfil      AnomalyType = "SEQUENCE_EXFIL_PATTERN"
	AnomalyTimePattern        AnomalyType = "UNUSUAL_TIME_PATTERN"
	AnomalyChannelSwitch      AnomalyType = "RAPID_CHANNEL_SWITCH"
	AnomalyNewDeviceHighValue AnomalyType = "NEW_DEVICE_HIGH_VALUE"
)

// NewMLScorer creates a new ML scorer
func NewMLScorer(txRepo *repositories.TransactionRepository, config MLScorerConfig) *MLScorer {
	return &MLScorer{
		txRepo:       txRepo,
		modelVersion: config.ModelVersion,
		enabled:      config.Enabled,
	}
}

// Score computes ML and behavioral scores for a transaction
func (s *MLScorer) Score(ctx context.Context, features *models.RiskFeatures, tx *models.Transaction) *MLScoreResult {
	result := &MLScoreResult{
		AnomaliesDetected: make([]string, 0),
		Confidence:        1.0, // Default confidence for rule-based behavioral scoring
	}

	// Compute behavioral anomaly score using statistical methods
	behavioralScore, anomalies := s.computeBehavioralScore(features, tx)
	result.BehavioralScore = behavioralScore
	result.AnomaliesDetected = anomalies

	// ML Score placeholder - would call external ML service in production
	// For now, we use a simple ensemble of z-scores as a "lightweight ML" approach
	if s.enabled {
		mlScore := s.computeLightweightMLScore(features, tx)
		result.MLScore = &mlScore
		result.Confidence = 0.85 // Simulated confidence
	}

	return result
}

// computeBehavioralScore calculates anomaly score using statistical methods
func (s *MLScorer) computeBehavioralScore(features *models.RiskFeatures, tx *models.Transaction) (float64, []string) {
	var totalScore float64
	var anomalies []string

	// 1. Spending Z-Score Analysis
	// If spending is > 2.5 standard deviations from mean, flag it
	if features.SpendingZScore > 2.5 {
		score := math.Min(features.SpendingZScore*10, 30) // Cap at 30
		totalScore += score
		anomalies = append(anomalies, string(AnomalySpendingSpike))
		log.Debug().Float64("z_score", features.SpendingZScore).Msg("Spending spike detected")
	}

	// 2. Velocity Z-Score Analysis
	if features.VelocityZScore > 2.0 {
		score := math.Min(features.VelocityZScore*8, 25)
		totalScore += score
		anomalies = append(anomalies, string(AnomalyVelocityBurst))
	}

	// 3. Peer Group Deviation
	// Compare against similar accounts (by account type, tenure, etc.)
	if features.PeerGroupDeviation > 3.0 {
		score := math.Min(features.PeerGroupDeviation*7, 25)
		totalScore += score
		anomalies = append(anomalies, string(AnomalyPeerDeviation))
	}

	// 4. Sequence/Exfiltration Pattern Detection
	// Small probe transaction followed by large transaction
	if features.FollowsProbePattern {
		totalScore += 35
		anomalies = append(anomalies, string(AnomalySequenceExfil))
	}

	// 5. Impossible Travel Detection
	// If distance from last transaction is impossible given time elapsed
	if features.DistanceFromLastTx > 0 && features.TimeSinceLastTx > 0 {
		// Speed in km/h
		speed := features.DistanceFromLastTx / features.TimeSinceLastTx
		if speed > 900 { // Faster than commercial flight
			totalScore += 30
			anomalies = append(anomalies, string(AnomalyGeoImpossible))
		}
	}

	// 6. Unusual Time Pattern
	if features.IsUnusualHour && features.DayOfWeekAnomaly {
		totalScore += 10
		anomalies = append(anomalies, string(AnomalyTimePattern))
	}

	// 7. Rapid Channel Switching
	if features.ChannelSwitchCount > 3 {
		totalScore += 15
		anomalies = append(anomalies, string(AnomalyChannelSwitch))
	}

	// 8. New Device + High Value
	if features.IsNewDevice && tx.Amount > 1000 {
		totalScore += 20
		anomalies = append(anomalies, string(AnomalyNewDeviceHighValue))
	}

	// Normalize to 0-100
	if totalScore > 100 {
		totalScore = 100
	}

	return math.Round(totalScore*100) / 100, anomalies
}

// computeLightweightMLScore provides a simple ML-like score using ensemble of features
// This simulates what a real ML model would do - in production, replace with actual model call
func (s *MLScorer) computeLightweightMLScore(features *models.RiskFeatures, tx *models.Transaction) float64 {
	// Weighted ensemble of normalized features
	// This mimics a simple logistic regression or random forest output

	weights := map[string]float64{
		"spending_z":      0.20,
		"velocity_z":      0.15,
		"peer_deviation":  0.15,
		"location_risk":   0.10,
		"time_risk":       0.10,
		"merchant_risk":   0.10,
		"behavioral":      0.20,
	}

	var score float64

	// Spending anomaly (sigmoid transformation of z-score)
	spendingRisk := sigmoid(features.SpendingZScore - 2) * 100
	score += weights["spending_z"] * spendingRisk

	// Velocity anomaly
	velocityRisk := sigmoid(features.VelocityZScore - 1.5) * 100
	score += weights["velocity_z"] * velocityRisk

	// Peer deviation
	peerRisk := sigmoid(features.PeerGroupDeviation - 2) * 100
	score += weights["peer_deviation"] * peerRisk

	// Location risk factors
	locationRisk := 0.0
	if features.IsNewLocation {
		locationRisk += 30
	}
	if features.IsHighRiskCountry {
		locationRisk += 50
	}
	if features.DistanceFromLastTx > 500 && features.TimeSinceLastTx < 2 {
		locationRisk += 40
	}
	score += weights["location_risk"] * math.Min(locationRisk, 100)

	// Time-based risk
	timeRisk := 0.0
	if features.IsUnusualHour {
		timeRisk += 30
	}
	if features.DayOfWeekAnomaly {
		timeRisk += 20
	}
	score += weights["time_risk"] * math.Min(timeRisk, 100)

	// Merchant risk
	score += weights["merchant_risk"] * features.MerchantRiskScore

	// Behavioral composite
	score += weights["behavioral"] * features.BehavioralAnomalyScore

	return math.Round(score*100) / 100
}

// sigmoid function for smooth risk transformation
func sigmoid(x float64) float64 {
	return 1 / (1 + math.Exp(-x))
}

// ComputeEnhancedFeatures computes additional features for ML scoring
func (s *MLScorer) ComputeEnhancedFeatures(ctx context.Context, accountID uuid.UUID, tx *models.Transaction, baseFeatures *models.RiskFeatures) {
	// Compute Z-scores
	if baseFeatures.RollingStdDev30d > 0 {
		baseFeatures.SpendingZScore = (tx.Amount - baseFeatures.RollingAvgSpend30d) / baseFeatures.RollingStdDev30d
	}

	// Compute velocity z-score (simplified - would use historical velocity stats)
	avgVelocity := 3.0 // Assume average 3 transactions per hour
	stdVelocity := 2.0 // Assume std dev of 2
	if stdVelocity > 0 {
		baseFeatures.VelocityZScore = (float64(baseFeatures.TransactionVelocity1h) - avgVelocity) / stdVelocity
	}

	// Detect probe pattern (small tx followed by large tx within 10 minutes)
	if baseFeatures.RecentSmallTxCount > 0 && tx.Amount > 1000 {
		baseFeatures.FollowsProbePattern = true
	}

	// Unusual hour detection (based on user's historical pattern)
	hour := tx.CreatedAt.Hour()
	baseFeatures.IsUnusualHour = hour >= 0 && hour < 6

	// Day of week anomaly (simplified - would use user's pattern)
	dayOfWeek := tx.CreatedAt.Weekday()
	baseFeatures.DayOfWeekAnomaly = dayOfWeek == time.Saturday || dayOfWeek == time.Sunday

	// Compute behavioral anomaly composite
	baseFeatures.BehavioralAnomalyScore = s.computeBehavioralComposite(baseFeatures)
}

// computeBehavioralComposite creates a single behavioral risk score
func (s *MLScorer) computeBehavioralComposite(features *models.RiskFeatures) float64 {
	var score float64

	// Weight different behavioral signals
	if math.Abs(features.SpendingZScore) > 2 {
		score += math.Min(math.Abs(features.SpendingZScore)*10, 30)
	}

	if math.Abs(features.VelocityZScore) > 1.5 {
		score += math.Min(math.Abs(features.VelocityZScore)*8, 25)
	}

	if features.LocationChangeCount > 2 {
		score += float64(features.LocationChangeCount) * 5
	}

	if features.ChannelSwitchCount > 1 {
		score += float64(features.ChannelSwitchCount) * 7
	}

	return math.Min(score, 100)
}

// MLScorerInterface defines the interface for pluggable ML scorers
// This allows swapping in different ML implementations
type MLScorerInterface interface {
	Score(ctx context.Context, features *models.RiskFeatures, tx *models.Transaction) *MLScoreResult
	ComputeEnhancedFeatures(ctx context.Context, accountID uuid.UUID, tx *models.Transaction, baseFeatures *models.RiskFeatures)
}

// ExternalMLScorer would call an external ML service (placeholder for future)
type ExternalMLScorer struct {
	endpoint   string
	apiKey     string
	timeout    time.Duration
	httpClient interface{} // *http.Client in real implementation
}

// NewExternalMLScorer creates a scorer that calls an external ML API
func NewExternalMLScorer(endpoint, apiKey string, timeout time.Duration) *ExternalMLScorer {
	return &ExternalMLScorer{
		endpoint: endpoint,
		apiKey:   apiKey,
		timeout:  timeout,
	}
}

// Score calls the external ML service (placeholder implementation)
func (s *ExternalMLScorer) Score(ctx context.Context, features *models.RiskFeatures, tx *models.Transaction) *MLScoreResult {
	// In production, this would:
	// 1. Serialize features to JSON
	// 2. Call external ML API (e.g., SageMaker, Vertex AI, custom service)
	// 3. Parse response and return MLScoreResult
	
	log.Debug().Str("endpoint", s.endpoint).Msg("External ML scorer called (placeholder)")
	
	return &MLScoreResult{
		MLScore:           nil, // Would be populated from API response
		BehavioralScore:   0,
		AnomaliesDetected: []string{},
		Confidence:        0,
	}
}
