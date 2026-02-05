package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/enterprise/risk-engine/internal/models"
)

var (
	ErrTransactionNotFound    = errors.New("transaction not found")
	ErrDuplicateTransaction   = errors.New("duplicate transaction (idempotency key exists)")
)

// TransactionRepository handles transaction database operations
type TransactionRepository struct {
	db *Database
}

// NewTransactionRepository creates a new transaction repository
func NewTransactionRepository(db *Database) *TransactionRepository {
	return &TransactionRepository{db: db}
}

// Create creates a new transaction
func (r *TransactionRepository) Create(ctx context.Context, tx *models.Transaction) error {
	query := `
		INSERT INTO transactions (
			id, account_id, amount, currency, merchant, merchant_category,
			location, country, channel, status, idempotency_key, metadata, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`

	tx.ID = uuid.New()
	tx.CreatedAt = time.Now()
	tx.Status = models.TransactionStatusPending

	metadataBytes, _ := tx.Metadata.Value()

	_, err := r.db.Pool.Exec(ctx, query,
		tx.ID,
		tx.AccountID,
		tx.Amount,
		tx.Currency,
		tx.Merchant,
		tx.MerchantCategory,
		tx.Location,
		tx.Country,
		tx.Channel,
		tx.Status,
		tx.IdempotencyKey,
		metadataBytes,
		tx.CreatedAt,
	)

	if err != nil {
		if isDuplicateKeyError(err) {
			return ErrDuplicateTransaction
		}
		return err
	}

	return nil
}

// CreateBatch creates multiple transactions in a batch
func (r *TransactionRepository) CreateBatch(ctx context.Context, transactions []*models.Transaction) error {
	if len(transactions) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	query := `
		INSERT INTO transactions (
			id, account_id, amount, currency, merchant, merchant_category,
			location, country, channel, status, idempotency_key, metadata, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (idempotency_key) DO NOTHING
	`

	for _, tx := range transactions {
		tx.ID = uuid.New()
		tx.CreatedAt = time.Now()
		tx.Status = models.TransactionStatusPending
		metadataBytes, _ := tx.Metadata.Value()

		batch.Queue(query,
			tx.ID,
			tx.AccountID,
			tx.Amount,
			tx.Currency,
			tx.Merchant,
			tx.MerchantCategory,
			tx.Location,
			tx.Country,
			tx.Channel,
			tx.Status,
			tx.IdempotencyKey,
			metadataBytes,
			tx.CreatedAt,
		)
	}

	br := r.db.Pool.SendBatch(ctx, batch)
	defer br.Close()

	for range transactions {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}

	return nil
}

// GetByID retrieves a transaction by ID
func (r *TransactionRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Transaction, error) {
	query := `
		SELECT id, account_id, amount, currency, merchant, merchant_category,
			   location, country, channel, status, idempotency_key, metadata,
			   created_at, processed_at
		FROM transactions
		WHERE id = $1
	`

	tx := &models.Transaction{}
	var metadataBytes []byte

	err := r.db.Pool.QueryRow(ctx, query, id).Scan(
		&tx.ID,
		&tx.AccountID,
		&tx.Amount,
		&tx.Currency,
		&tx.Merchant,
		&tx.MerchantCategory,
		&tx.Location,
		&tx.Country,
		&tx.Channel,
		&tx.Status,
		&tx.IdempotencyKey,
		&metadataBytes,
		&tx.CreatedAt,
		&tx.ProcessedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTransactionNotFound
		}
		return nil, err
	}

	tx.Metadata.Scan(metadataBytes)
	return tx, nil
}

// GetByIdempotencyKey retrieves a transaction by idempotency key
func (r *TransactionRepository) GetByIdempotencyKey(ctx context.Context, key string) (*models.Transaction, error) {
	query := `
		SELECT id, account_id, amount, currency, merchant, merchant_category,
			   location, country, channel, status, idempotency_key, metadata,
			   created_at, processed_at
		FROM transactions
		WHERE idempotency_key = $1
	`

	tx := &models.Transaction{}
	var metadataBytes []byte

	err := r.db.Pool.QueryRow(ctx, query, key).Scan(
		&tx.ID,
		&tx.AccountID,
		&tx.Amount,
		&tx.Currency,
		&tx.Merchant,
		&tx.MerchantCategory,
		&tx.Location,
		&tx.Country,
		&tx.Channel,
		&tx.Status,
		&tx.IdempotencyKey,
		&metadataBytes,
		&tx.CreatedAt,
		&tx.ProcessedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTransactionNotFound
		}
		return nil, err
	}

	tx.Metadata.Scan(metadataBytes)
	return tx, nil
}

// UpdateStatus updates a transaction's status
func (r *TransactionRepository) UpdateStatus(ctx context.Context, id uuid.UUID, createdAt time.Time, status string) error {
	query := `
		UPDATE transactions
		SET status = $3, processed_at = $4
		WHERE id = $1 AND created_at = $2
	`

	processedAt := time.Now()
	result, err := r.db.Pool.Exec(ctx, query, id, createdAt, status, processedAt)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrTransactionNotFound
	}

	return nil
}

// GetByAccountID retrieves transactions for an account with pagination
func (r *TransactionRepository) GetByAccountID(ctx context.Context, accountID uuid.UUID, page, pageSize int, startDate, endDate *time.Time) ([]*models.Transaction, int, error) {
	offset := (page - 1) * pageSize

	countQuery := `
		SELECT COUNT(*) FROM transactions
		WHERE account_id = $1
		AND ($2::timestamptz IS NULL OR created_at >= $2)
		AND ($3::timestamptz IS NULL OR created_at <= $3)
	`
	var total int
	if err := r.db.Pool.QueryRow(ctx, countQuery, accountID, startDate, endDate).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
		SELECT id, account_id, amount, currency, merchant, merchant_category,
			   location, country, channel, status, idempotency_key, metadata,
			   created_at, processed_at
		FROM transactions
		WHERE account_id = $1
		AND ($4::timestamptz IS NULL OR created_at >= $4)
		AND ($5::timestamptz IS NULL OR created_at <= $5)
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.db.Pool.Query(ctx, query, accountID, pageSize, offset, startDate, endDate)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	return r.scanTransactions(rows, total)
}

// GetFlagged retrieves flagged/blocked transactions with pagination
func (r *TransactionRepository) GetFlagged(ctx context.Context, page, pageSize int) ([]*models.Transaction, int, error) {
	offset := (page - 1) * pageSize

	countQuery := `SELECT COUNT(*) FROM transactions WHERE status IN ('flagged', 'blocked')`
	var total int
	if err := r.db.Pool.QueryRow(ctx, countQuery).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
		SELECT id, account_id, amount, currency, merchant, merchant_category,
			   location, country, channel, status, idempotency_key, metadata,
			   created_at, processed_at
		FROM transactions
		WHERE status IN ('flagged', 'blocked')
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := r.db.Pool.Query(ctx, query, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	return r.scanTransactions(rows, total)
}

// GetRecent retrieves all recent transactions across all accounts
func (r *TransactionRepository) GetRecent(ctx context.Context, page, pageSize int) ([]*models.Transaction, int, error) {
	offset := (page - 1) * pageSize

	countQuery := `SELECT COUNT(*) FROM transactions WHERE created_at >= NOW() - INTERVAL '7 days'`
	var total int
	if err := r.db.Pool.QueryRow(ctx, countQuery).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
		SELECT id, account_id, amount, currency, merchant, merchant_category,
			   location, country, channel, status, idempotency_key, metadata,
			   created_at, processed_at
		FROM transactions
		WHERE created_at >= NOW() - INTERVAL '7 days'
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := r.db.Pool.Query(ctx, query, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	return r.scanTransactions(rows, total)
}

// GetRecentByAccount retrieves recent transactions for risk calculation
func (r *TransactionRepository) GetRecentByAccount(ctx context.Context, accountID uuid.UUID, since time.Time) ([]*models.Transaction, error) {
	query := `
		SELECT id, account_id, amount, currency, merchant, merchant_category,
			   location, country, channel, status, idempotency_key, metadata,
			   created_at, processed_at
		FROM transactions
		WHERE account_id = $1 AND created_at >= $2
		ORDER BY created_at DESC
	`

	rows, err := r.db.Pool.Query(ctx, query, accountID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	transactions, _, err := r.scanTransactions(rows, 0)
	return transactions, err
}

// GetTransactionStats retrieves transaction statistics for an account
func (r *TransactionRepository) GetTransactionStats(ctx context.Context, accountID uuid.UUID, days int) (map[string]interface{}, error) {
	query := `
		SELECT 
			COUNT(*) as total_count,
			COALESCE(SUM(amount), 0) as total_amount,
			COALESCE(AVG(amount), 0) as avg_amount,
			COALESCE(STDDEV(amount), 0) as stddev_amount,
			COUNT(DISTINCT location) as unique_locations,
			COUNT(DISTINCT merchant) as unique_merchants
		FROM transactions
		WHERE account_id = $1 AND created_at >= NOW() - ($2 || ' days')::interval
	`

	var totalCount int
	var totalAmount, avgAmount, stddevAmount float64
	var uniqueLocations, uniqueMerchants int

	// Convert days to string to avoid pgx encoding issues
	daysStr := fmt.Sprintf("%d", days)

	err := r.db.Pool.QueryRow(ctx, query, accountID, daysStr).Scan(
		&totalCount,
		&totalAmount,
		&avgAmount,
		&stddevAmount,
		&uniqueLocations,
		&uniqueMerchants,
	)

	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"total_count":      totalCount,
		"total_amount":     totalAmount,
		"avg_amount":       avgAmount,
		"stddev_amount":    stddevAmount,
		"unique_locations": uniqueLocations,
		"unique_merchants": uniqueMerchants,
	}, nil
}

func (r *TransactionRepository) scanTransactions(rows pgx.Rows, total int) ([]*models.Transaction, int, error) {
	var transactions []*models.Transaction
	for rows.Next() {
		tx := &models.Transaction{}
		var metadataBytes []byte

		if err := rows.Scan(
			&tx.ID,
			&tx.AccountID,
			&tx.Amount,
			&tx.Currency,
			&tx.Merchant,
			&tx.MerchantCategory,
			&tx.Location,
			&tx.Country,
			&tx.Channel,
			&tx.Status,
			&tx.IdempotencyKey,
			&metadataBytes,
			&tx.CreatedAt,
			&tx.ProcessedAt,
		); err != nil {
			return nil, 0, err
		}

		tx.Metadata.Scan(metadataBytes)
		transactions = append(transactions, tx)
	}

	return transactions, total, nil
}
