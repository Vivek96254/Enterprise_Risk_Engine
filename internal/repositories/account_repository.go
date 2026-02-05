package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/enterprise/risk-engine/internal/models"
)

var (
	ErrAccountNotFound = errors.New("account not found")
)

// AccountRepository handles account database operations
type AccountRepository struct {
	db *Database
}

// NewAccountRepository creates a new account repository
func NewAccountRepository(db *Database) *AccountRepository {
	return &AccountRepository{db: db}
}

// Create creates a new account
func (r *AccountRepository) Create(ctx context.Context, account *models.Account) error {
	query := `
		INSERT INTO accounts (id, user_id, account_type, risk_profile, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	account.ID = uuid.New()
	account.CreatedAt = time.Now()
	account.UpdatedAt = time.Now()

	_, err := r.db.Pool.Exec(ctx, query,
		account.ID,
		account.UserID,
		account.AccountType,
		account.RiskProfile,
		account.Status,
		account.CreatedAt,
		account.UpdatedAt,
	)

	return err
}

// GetByID retrieves an account by ID
func (r *AccountRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Account, error) {
	query := `
		SELECT id, user_id, account_type, risk_profile, status, created_at, updated_at
		FROM accounts
		WHERE id = $1
	`

	account := &models.Account{}
	err := r.db.Pool.QueryRow(ctx, query, id).Scan(
		&account.ID,
		&account.UserID,
		&account.AccountType,
		&account.RiskProfile,
		&account.Status,
		&account.CreatedAt,
		&account.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAccountNotFound
		}
		return nil, err
	}

	return account, nil
}

// GetByUserID retrieves all accounts for a user
func (r *AccountRepository) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*models.Account, error) {
	query := `
		SELECT id, user_id, account_type, risk_profile, status, created_at, updated_at
		FROM accounts
		WHERE user_id = $1
		ORDER BY created_at DESC
	`

	rows, err := r.db.Pool.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accounts []*models.Account
	for rows.Next() {
		account := &models.Account{}
		if err := rows.Scan(
			&account.ID,
			&account.UserID,
			&account.AccountType,
			&account.RiskProfile,
			&account.Status,
			&account.CreatedAt,
			&account.UpdatedAt,
		); err != nil {
			return nil, err
		}
		accounts = append(accounts, account)
	}

	return accounts, nil
}

// Update updates an account
func (r *AccountRepository) Update(ctx context.Context, account *models.Account) error {
	query := `
		UPDATE accounts
		SET account_type = $2, risk_profile = $3, status = $4, updated_at = $5
		WHERE id = $1
	`

	account.UpdatedAt = time.Now()

	result, err := r.db.Pool.Exec(ctx, query,
		account.ID,
		account.AccountType,
		account.RiskProfile,
		account.Status,
		account.UpdatedAt,
	)

	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrAccountNotFound
	}

	return nil
}

// UpdateRiskProfile updates an account's risk profile
func (r *AccountRepository) UpdateRiskProfile(ctx context.Context, id uuid.UUID, riskProfile string) error {
	query := `
		UPDATE accounts
		SET risk_profile = $2, updated_at = $3
		WHERE id = $1
	`

	result, err := r.db.Pool.Exec(ctx, query, id, riskProfile, time.Now())
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrAccountNotFound
	}

	return nil
}

// List retrieves all accounts with pagination
func (r *AccountRepository) List(ctx context.Context, page, pageSize int) ([]*models.Account, int, error) {
	offset := (page - 1) * pageSize

	countQuery := `SELECT COUNT(*) FROM accounts`
	var total int
	if err := r.db.Pool.QueryRow(ctx, countQuery).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
		SELECT id, user_id, account_type, risk_profile, status, created_at, updated_at
		FROM accounts
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := r.db.Pool.Query(ctx, query, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var accounts []*models.Account
	for rows.Next() {
		account := &models.Account{}
		if err := rows.Scan(
			&account.ID,
			&account.UserID,
			&account.AccountType,
			&account.RiskProfile,
			&account.Status,
			&account.CreatedAt,
			&account.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		accounts = append(accounts, account)
	}

	return accounts, total, nil
}

// GetByRiskProfile retrieves accounts by risk profile
func (r *AccountRepository) GetByRiskProfile(ctx context.Context, riskProfile string, page, pageSize int) ([]*models.Account, int, error) {
	offset := (page - 1) * pageSize

	countQuery := `SELECT COUNT(*) FROM accounts WHERE risk_profile = $1`
	var total int
	if err := r.db.Pool.QueryRow(ctx, countQuery, riskProfile).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
		SELECT id, user_id, account_type, risk_profile, status, created_at, updated_at
		FROM accounts
		WHERE risk_profile = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.db.Pool.Query(ctx, query, riskProfile, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var accounts []*models.Account
	for rows.Next() {
		account := &models.Account{}
		if err := rows.Scan(
			&account.ID,
			&account.UserID,
			&account.AccountType,
			&account.RiskProfile,
			&account.Status,
			&account.CreatedAt,
			&account.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		accounts = append(accounts, account)
	}

	return accounts, total, nil
}
