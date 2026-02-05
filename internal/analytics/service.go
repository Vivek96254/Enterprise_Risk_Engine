package analytics

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/enterprise/risk-engine/internal/models"
	"github.com/enterprise/risk-engine/internal/queue"
	"github.com/enterprise/risk-engine/internal/repositories"
)

// AnalyticsService provides analytics and reporting functionality
type AnalyticsService struct {
	txRepo        *repositories.TransactionRepository
	riskScoreRepo *repositories.RiskScoreRepository
	accountRepo   *repositories.AccountRepository
	db            *repositories.Database
	cacheClient   *queue.CacheClient
}

// NewAnalyticsService creates a new analytics service
func NewAnalyticsService(
	txRepo *repositories.TransactionRepository,
	riskScoreRepo *repositories.RiskScoreRepository,
	accountRepo *repositories.AccountRepository,
	db *repositories.Database,
	cacheClient *queue.CacheClient,
) *AnalyticsService {
	return &AnalyticsService{
		txRepo:        txRepo,
		riskScoreRepo: riskScoreRepo,
		accountRepo:   accountRepo,
		db:            db,
		cacheClient:   cacheClient,
	}
}

// GetRiskSummary returns risk summary for a specific date
func (s *AnalyticsService) GetRiskSummary(ctx context.Context, date time.Time) (*models.RiskSummary, error) {
	// Try cache first
	cacheKey := fmt.Sprintf("risk_summary:%s", date.Format("2006-01-02"))
	var cached models.RiskSummary
	if s.cacheClient != nil {
		if err := s.cacheClient.Get(ctx, cacheKey, &cached); err == nil {
			return &cached, nil
		}
	}

	// Fetch from database
	summary, err := s.riskScoreRepo.GetDailySummary(ctx, date)
	if err != nil {
		return nil, fmt.Errorf("failed to get risk summary: %w", err)
	}

	// Cache the result (cache for 5 minutes for recent dates, longer for historical)
	if s.cacheClient != nil {
		cacheDuration := 5 * time.Minute
		if time.Since(date) > 24*time.Hour {
			cacheDuration = 1 * time.Hour
		}
		if err := s.cacheClient.Set(ctx, cacheKey, summary, cacheDuration); err != nil {
			log.Warn().Err(err).Msg("Failed to cache risk summary")
		}
	}

	return summary, nil
}

// GetRiskSummaryRange returns risk summaries for a date range
func (s *AnalyticsService) GetRiskSummaryRange(ctx context.Context, startDate, endDate time.Time) ([]*models.RiskSummary, error) {
	var summaries []*models.RiskSummary

	for d := startDate; !d.After(endDate); d = d.AddDate(0, 0, 1) {
		summary, err := s.GetRiskSummary(ctx, d)
		if err != nil {
			log.Warn().Err(err).Time("date", d).Msg("Failed to get summary for date")
			continue
		}
		summaries = append(summaries, summary)
	}

	return summaries, nil
}

// GetAccountRiskProfile returns the risk profile for an account
func (s *AnalyticsService) GetAccountRiskProfile(ctx context.Context, accountID string) (*models.AccountRiskProfile, error) {
	id, err := uuid.Parse(accountID)
	if err != nil {
		return nil, fmt.Errorf("invalid account_id: %w", err)
	}

	// Try cache first
	cacheKey := fmt.Sprintf("account_risk_profile:%s", accountID)
	var cached models.AccountRiskProfile
	if s.cacheClient != nil {
		if err := s.cacheClient.Get(ctx, cacheKey, &cached); err == nil {
			return &cached, nil
		}
	}

	profile, err := s.riskScoreRepo.GetAccountRiskProfile(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get account risk profile: %w", err)
	}

	// Cache for 5 minutes
	if s.cacheClient != nil {
		if err := s.cacheClient.Set(ctx, cacheKey, profile, 5*time.Minute); err != nil {
			log.Warn().Err(err).Msg("Failed to cache account risk profile")
		}
	}

	return profile, nil
}

// GetFlaggedTransactions returns flagged/blocked transactions with pagination
func (s *AnalyticsService) GetFlaggedTransactions(ctx context.Context, page, pageSize int) (*FlaggedTransactionsResponse, error) {
	transactions, total, err := s.txRepo.GetFlagged(ctx, page, pageSize)
	if err != nil {
		return nil, fmt.Errorf("failed to get flagged transactions: %w", err)
	}

	// Enrich with risk scores
	var enriched []FlaggedTransaction
	for _, tx := range transactions {
		score, _ := s.riskScoreRepo.GetByTransactionID(ctx, tx.ID)

		ft := FlaggedTransaction{
			Transaction: tx,
		}
		if score != nil {
			ft.RiskScore = score.Score
			ft.RiskLevel = score.RiskLevel
			ft.RulesTriggered = score.RulesTriggered
		}
		enriched = append(enriched, ft)
	}

	return &FlaggedTransactionsResponse{
		Transactions: enriched,
		Pagination: models.Pagination{
			Page:     page,
			PageSize: pageSize,
			Total:    total,
		},
	}, nil
}

// GetSystemMetrics returns current system metrics
func (s *AnalyticsService) GetSystemMetrics(ctx context.Context, streamClient *queue.RedisStreamClient) (*models.SystemMetrics, error) {
	metrics := &models.SystemMetrics{
		Timestamp: time.Now(),
	}

	// Get database stats
	dbStats := s.db.Stats()
	metrics.DBConnectionsActive = int(dbStats.AcquiredConns())
	metrics.DBConnectionsIdle = int(dbStats.IdleConns())

	// Get queue depth
	if streamClient != nil {
		info, err := streamClient.GetStreamInfo(ctx)
		if err == nil {
			metrics.QueueDepth = int(info.PendingCount)
		}
	}

	// Calculate transactions per second (from last minute)
	tps, err := s.calculateTPS(ctx)
	if err == nil {
		metrics.TransactionsPerSec = tps
	}

	// Calculate average processing time
	avgTime, err := s.calculateAvgProcessingTime(ctx)
	if err == nil {
		metrics.AvgProcessingTimeMs = avgTime
	}

	// Calculate error rate
	errorRate, err := s.calculateErrorRate(ctx)
	if err == nil {
		metrics.ErrorRate = errorRate
	}

	return metrics, nil
}

// calculateTPS calculates transactions per second over the last minute
func (s *AnalyticsService) calculateTPS(ctx context.Context) (float64, error) {
	query := `
		SELECT COUNT(*) 
		FROM transactions 
		WHERE created_at >= NOW() - INTERVAL '1 minute'
	`

	var count int
	err := s.db.Pool.QueryRow(ctx, query).Scan(&count)
	if err != nil {
		return 0, err
	}

	return float64(count) / 60.0, nil
}

// calculateAvgProcessingTime calculates average processing time
func (s *AnalyticsService) calculateAvgProcessingTime(ctx context.Context) (float64, error) {
	query := `
		SELECT COALESCE(AVG(processing_time_ms), 0)
		FROM risk_scores
		WHERE created_at >= NOW() - INTERVAL '5 minutes'
	`

	var avgTime float64
	err := s.db.Pool.QueryRow(ctx, query).Scan(&avgTime)
	if err != nil {
		return 0, err
	}

	return avgTime, nil
}

// calculateErrorRate calculates the error rate
func (s *AnalyticsService) calculateErrorRate(ctx context.Context) (float64, error) {
	query := `
		SELECT 
			COUNT(CASE WHEN status IN ('flagged', 'blocked') THEN 1 END)::float / 
			NULLIF(COUNT(*), 0)
		FROM transactions
		WHERE created_at >= NOW() - INTERVAL '1 hour'
	`

	var errorRate *float64
	err := s.db.Pool.QueryRow(ctx, query).Scan(&errorRate)
	if err != nil {
		return 0, err
	}

	if errorRate == nil {
		return 0, nil
	}

	return *errorRate, nil
}

// GetRiskDistribution returns the distribution of risk levels
func (s *AnalyticsService) GetRiskDistribution(ctx context.Context, days int) (*RiskDistribution, error) {
	query := `
		SELECT 
			risk_level,
			COUNT(*) as count
		FROM risk_scores
		WHERE created_at >= NOW() - ($1::text || ' days')::interval
		GROUP BY risk_level
		ORDER BY 
			CASE risk_level 
				WHEN 'critical' THEN 1 
				WHEN 'high' THEN 2 
				WHEN 'medium' THEN 3 
				WHEN 'low' THEN 4 
			END
	`

	rows, err := s.db.Pool.Query(ctx, query, fmt.Sprintf("%d", days))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	distribution := &RiskDistribution{
		Period: fmt.Sprintf("%d days", days),
		Levels: make(map[string]int),
	}

	var total int
	for rows.Next() {
		var level string
		var count int
		if err := rows.Scan(&level, &count); err != nil {
			return nil, err
		}
		distribution.Levels[level] = count
		total += count
	}
	distribution.Total = total

	return distribution, nil
}

// GetTopTriggeredRules returns the most frequently triggered rules
// for high / critical (flagged or blocked) transactions within the
// given time window.
// The count is the number of DISTINCT flagged/blocked transactions
// where this rule was present, so it can be safely compared against
// the total flagged/blocked transaction count.
func (s *AnalyticsService) GetTopTriggeredRules(ctx context.Context, days, limit int) ([]models.RuleCount, error) {
	query := `
		SELECT 
			rule_id,
			COUNT(DISTINCT transaction_id) AS count
		FROM (
			SELECT 
				transaction_id,
				unnest(rules_triggered) AS rule_id
			FROM risk_scores
			WHERE 
				created_at >= NOW() - ($1::text || ' days')::interval
				AND risk_level IN ('high', 'critical')
		) t
		GROUP BY rule_id
		ORDER BY count DESC
		LIMIT $2
	`

	rows, err := s.db.Pool.Query(ctx, query, fmt.Sprintf("%d", days), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []models.RuleCount
	for rows.Next() {
		var rc models.RuleCount
		if err := rows.Scan(&rc.RuleID, &rc.Count); err != nil {
			return nil, err
		}
		rules = append(rules, rc)
	}

	return rules, nil
}

// GetHourlyTransactionVolume returns transaction volume by hour
func (s *AnalyticsService) GetHourlyTransactionVolume(ctx context.Context, date time.Time) ([]HourlyVolume, error) {
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	query := `
		SELECT 
			EXTRACT(HOUR FROM created_at) as hour,
			COUNT(*) as count,
			COALESCE(SUM(amount), 0) as total_amount
		FROM transactions
		WHERE created_at >= $1 AND created_at < $2
		GROUP BY EXTRACT(HOUR FROM created_at)
		ORDER BY hour
	`

	rows, err := s.db.Pool.Query(ctx, query, startOfDay, endOfDay)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var volumes []HourlyVolume
	for rows.Next() {
		var hv HourlyVolume
		if err := rows.Scan(&hv.Hour, &hv.Count, &hv.TotalAmount); err != nil {
			return nil, err
		}
		volumes = append(volumes, hv)
	}

	return volumes, nil
}

// Response types

// FlaggedTransaction includes transaction with risk details
type FlaggedTransaction struct {
	Transaction    *models.Transaction `json:"transaction"`
	RiskScore      float64             `json:"risk_score"`
	RiskLevel      string              `json:"risk_level"`
	RulesTriggered []string            `json:"rules_triggered"`
}

// FlaggedTransactionsResponse is the response for flagged transactions
type FlaggedTransactionsResponse struct {
	Transactions []FlaggedTransaction `json:"transactions"`
	Pagination   models.Pagination    `json:"pagination"`
}

// RiskDistribution represents risk level distribution
type RiskDistribution struct {
	Period string         `json:"period"`
	Levels map[string]int `json:"levels"`
	Total  int            `json:"total"`
}

// HourlyVolume represents transaction volume for an hour
type HourlyVolume struct {
	Hour        int     `json:"hour"`
	Count       int     `json:"count"`
	TotalAmount float64 `json:"total_amount"`
}
