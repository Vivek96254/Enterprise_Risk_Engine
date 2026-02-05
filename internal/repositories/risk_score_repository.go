package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/lib/pq"

	"github.com/enterprise/risk-engine/internal/models"
)

var (
	ErrRiskScoreNotFound = errors.New("risk score not found")
)

// RiskScoreRepository handles risk score database operations
type RiskScoreRepository struct {
	db *Database
}

// NewRiskScoreRepository creates a new risk score repository
func NewRiskScoreRepository(db *Database) *RiskScoreRepository {
	return &RiskScoreRepository{db: db}
}

// Create creates a new risk score
func (r *RiskScoreRepository) Create(ctx context.Context, score *models.RiskScore) error {
	query := `
		INSERT INTO risk_scores (
			id, transaction_id, transaction_created_at, score, risk_level,
			rules_triggered, features, model_version, processing_time_ms, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`

	score.ID = uuid.New()
	score.CreatedAt = time.Now()

	featuresBytes, _ := score.Features.Value()

	_, err := r.db.Pool.Exec(ctx, query,
		score.ID,
		score.TransactionID,
		score.CreatedAt, // transaction_created_at - using score.CreatedAt as placeholder
		score.Score,
		score.RiskLevel,
		pq.Array(score.RulesTriggered),
		featuresBytes,
		score.ModelVersion,
		score.ProcessingTimeMs,
		score.CreatedAt,
	)

	return err
}

// CreateWithTransactionTime creates a risk score with specific transaction time
func (r *RiskScoreRepository) CreateWithTransactionTime(ctx context.Context, score *models.RiskScore, transactionCreatedAt time.Time) error {
	query := `
		INSERT INTO risk_scores (
			id, transaction_id, transaction_created_at, score, risk_level,
			rules_triggered, features, model_version, processing_time_ms, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`

	score.ID = uuid.New()
	score.CreatedAt = time.Now()

	featuresBytes, _ := score.Features.Value()

	_, err := r.db.Pool.Exec(ctx, query,
		score.ID,
		score.TransactionID,
		transactionCreatedAt,
		score.Score,
		score.RiskLevel,
		pq.Array(score.RulesTriggered),
		featuresBytes,
		score.ModelVersion,
		score.ProcessingTimeMs,
		score.CreatedAt,
	)

	return err
}

// GetByTransactionID retrieves a risk score by transaction ID
func (r *RiskScoreRepository) GetByTransactionID(ctx context.Context, transactionID uuid.UUID) (*models.RiskScore, error) {
	query := `
		SELECT id, transaction_id, score, risk_level, rules_triggered,
			   features, model_version, processing_time_ms, created_at
		FROM risk_scores
		WHERE transaction_id = $1
	`

	score := &models.RiskScore{}
	var rulesTriggered []string
	var featuresBytes []byte

	err := r.db.Pool.QueryRow(ctx, query, transactionID).Scan(
		&score.ID,
		&score.TransactionID,
		&score.Score,
		&score.RiskLevel,
		&rulesTriggered, // pgx can handle []string directly
		&featuresBytes,
		&score.ModelVersion,
		&score.ProcessingTimeMs,
		&score.CreatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRiskScoreNotFound
		}
		return nil, err
	}

	score.RulesTriggered = rulesTriggered
	score.Features.Scan(featuresBytes)
	return score, nil
}

// GetByRiskLevel retrieves risk scores by risk level with pagination
func (r *RiskScoreRepository) GetByRiskLevel(ctx context.Context, riskLevel string, page, pageSize int) ([]*models.RiskScore, int, error) {
	offset := (page - 1) * pageSize

	countQuery := `SELECT COUNT(*) FROM risk_scores WHERE risk_level = $1`
	var total int
	if err := r.db.Pool.QueryRow(ctx, countQuery, riskLevel).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
		SELECT id, transaction_id, score, risk_level, rules_triggered,
			   features, model_version, processing_time_ms, created_at
		FROM risk_scores
		WHERE risk_level = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.db.Pool.Query(ctx, query, riskLevel, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	return r.scanRiskScores(rows, total)
}

// GetDailySummary retrieves daily risk summary
func (r *RiskScoreRepository) GetDailySummary(ctx context.Context, date time.Time) (*models.RiskSummary, error) {
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	query := `
		SELECT 
			COUNT(*) as total_transactions,
			COALESCE(SUM(t.amount), 0) as total_amount,
			COUNT(CASE WHEN t.status = 'flagged' THEN 1 END) as flagged_count,
			COUNT(CASE WHEN t.status = 'blocked' THEN 1 END) as blocked_count,
			COALESCE(AVG(rs.score), 0) as avg_risk_score,
			COUNT(CASE WHEN rs.risk_level = 'high' THEN 1 END) as high_risk_count,
			COUNT(CASE WHEN rs.risk_level = 'critical' THEN 1 END) as critical_risk_count
		FROM transactions t
		LEFT JOIN risk_scores rs ON t.id = rs.transaction_id AND t.created_at = rs.transaction_created_at
		WHERE t.created_at >= $1 AND t.created_at < $2
	`

	summary := &models.RiskSummary{
		Date: date.Format("2006-01-02"),
	}

	err := r.db.Pool.QueryRow(ctx, query, startOfDay, endOfDay).Scan(
		&summary.TotalTransactions,
		&summary.TotalAmount,
		&summary.FlaggedCount,
		&summary.BlockedCount,
		&summary.AvgRiskScore,
		&summary.HighRiskCount,
		&summary.CriticalRiskCount,
	)

	if err != nil {
		return nil, err
	}

	// Get top rules triggered
	rulesQuery := `
		SELECT unnest(rules_triggered) as rule_id, COUNT(*) as count
		FROM risk_scores
		WHERE created_at >= $1 AND created_at < $2
		GROUP BY rule_id
		ORDER BY count DESC
		LIMIT 10
	`

	rulesRows, err := r.db.Pool.Query(ctx, rulesQuery, startOfDay, endOfDay)
	if err != nil {
		return nil, err
	}
	defer rulesRows.Close()

	for rulesRows.Next() {
		var ruleCount models.RuleCount
		if err := rulesRows.Scan(&ruleCount.RuleID, &ruleCount.Count); err != nil {
			return nil, err
		}
		summary.TopRulesTriggered = append(summary.TopRulesTriggered, ruleCount)
	}

	return summary, nil
}

// GetAccountRiskProfile retrieves risk profile for an account
func (r *RiskScoreRepository) GetAccountRiskProfile(ctx context.Context, accountID uuid.UUID) (*models.AccountRiskProfile, error) {
	query := `
		SELECT 
			a.id as account_id,
			a.risk_profile as current_risk_level,
			COALESCE(AVG(t.amount), 0) as avg_transaction_amount,
			COUNT(t.id) as transaction_count_30d,
			COUNT(CASE WHEN t.status = 'flagged' THEN 1 END) as flagged_count_30d,
			MAX(t.created_at) as last_transaction_at
		FROM accounts a
		LEFT JOIN transactions t ON a.id = t.account_id AND t.created_at >= NOW() - INTERVAL '30 days'
		WHERE a.id = $1
		GROUP BY a.id, a.risk_profile
	`

	profile := &models.AccountRiskProfile{}

	err := r.db.Pool.QueryRow(ctx, query, accountID).Scan(
		&profile.AccountID,
		&profile.CurrentRiskLevel,
		&profile.AvgTransactionAmount,
		&profile.TransactionCount30d,
		&profile.FlaggedCount30d,
		&profile.LastTransactionAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAccountNotFound
		}
		return nil, err
	}

	// Determine risk trend based on recent flagged transactions
	profile.RiskTrend = "stable"
	if profile.FlaggedCount30d > 5 {
		profile.RiskTrend = "increasing"
	} else if profile.FlaggedCount30d == 0 && profile.TransactionCount30d > 10 {
		profile.RiskTrend = "decreasing"
	}

	return profile, nil
}

func (r *RiskScoreRepository) scanRiskScores(rows pgx.Rows, total int) ([]*models.RiskScore, int, error) {
	var scores []*models.RiskScore
	for rows.Next() {
		score := &models.RiskScore{}
		var rulesTriggered []string
		var featuresBytes []byte

		if err := rows.Scan(
			&score.ID,
			&score.TransactionID,
			&score.Score,
			&score.RiskLevel,
			&rulesTriggered, // pgx handles []string directly
			&featuresBytes,
			&score.ModelVersion,
			&score.ProcessingTimeMs,
			&score.CreatedAt,
		); err != nil {
			return nil, 0, err
		}

		score.RulesTriggered = rulesTriggered
		score.Features.Scan(featuresBytes)
		scores = append(scores, score)
	}

	return scores, total, nil
}
