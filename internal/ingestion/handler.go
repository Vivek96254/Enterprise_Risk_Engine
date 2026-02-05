package ingestion

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

// TransactionRequest represents an incoming transaction request
type TransactionRequest struct {
	AccountID        string                 `json:"account_id" binding:"required"`
	Amount           float64                `json:"amount" binding:"required,gt=0"`
	Currency         string                 `json:"currency" binding:"required,len=3"`
	Merchant         string                 `json:"merchant"`
	MerchantCategory string                 `json:"merchant_category"`
	Location         string                 `json:"location"`
	Country          string                 `json:"country"`
	Channel          string                 `json:"channel" binding:"required,oneof=online pos atm"`
	IdempotencyKey   string                 `json:"idempotency_key" binding:"required"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
}

// BatchTransactionRequest represents a batch of transactions
type BatchTransactionRequest struct {
	Transactions []TransactionRequest `json:"transactions" binding:"required,min=1,max=1000"`
}

// TransactionResponse represents the response after ingesting a transaction
type TransactionResponse struct {
	TransactionID  string    `json:"transaction_id"`
	Status         string    `json:"status"`
	IdempotencyKey string    `json:"idempotency_key"`
	CreatedAt      time.Time `json:"created_at"`
	Message        string    `json:"message,omitempty"`
}

// BatchTransactionResponse represents the response for batch ingestion
type BatchTransactionResponse struct {
	Successful int                   `json:"successful"`
	Failed     int                   `json:"failed"`
	Results    []TransactionResponse `json:"results"`
}

// IngestionService handles transaction ingestion
type IngestionService struct {
	txRepo      *repositories.TransactionRepository
	accountRepo *repositories.AccountRepository
	auditRepo   *repositories.AuditRepository
	streamClient *queue.RedisStreamClient
	cacheClient  *queue.CacheClient
}

// NewIngestionService creates a new ingestion service
func NewIngestionService(
	txRepo *repositories.TransactionRepository,
	accountRepo *repositories.AccountRepository,
	auditRepo *repositories.AuditRepository,
	streamClient *queue.RedisStreamClient,
	cacheClient *queue.CacheClient,
) *IngestionService {
	return &IngestionService{
		txRepo:       txRepo,
		accountRepo:  accountRepo,
		auditRepo:    auditRepo,
		streamClient: streamClient,
		cacheClient:  cacheClient,
	}
}

// IngestTransaction ingests a single transaction
func (s *IngestionService) IngestTransaction(ctx context.Context, req *TransactionRequest, requestID string) (*TransactionResponse, error) {
	startTime := time.Now()

	// Parse account ID
	accountID, err := uuid.Parse(req.AccountID)
	if err != nil {
		return nil, fmt.Errorf("invalid account_id format: %w", err)
	}

	// Check for duplicate (idempotency)
	existing, err := s.txRepo.GetByIdempotencyKey(ctx, req.IdempotencyKey)
	if err == nil && existing != nil {
		log.Debug().
			Str("idempotency_key", req.IdempotencyKey).
			Str("transaction_id", existing.ID.String()).
			Msg("Duplicate transaction detected")
		
		return &TransactionResponse{
			TransactionID:  existing.ID.String(),
			Status:         existing.Status,
			IdempotencyKey: existing.IdempotencyKey,
			CreatedAt:      existing.CreatedAt,
			Message:        "Transaction already exists (idempotent)",
		}, nil
	}

	// Verify account exists and is active
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("account not found: %w", err)
	}

	if account.Status != models.AccountStatusActive {
		return nil, fmt.Errorf("account is not active: %s", account.Status)
	}

	// Create transaction
	tx := &models.Transaction{
		AccountID:        accountID,
		Amount:           req.Amount,
		Currency:         req.Currency,
		Merchant:         req.Merchant,
		MerchantCategory: req.MerchantCategory,
		Location:         req.Location,
		Country:          req.Country,
		Channel:          req.Channel,
		IdempotencyKey:   req.IdempotencyKey,
		Metadata:         models.JSONB(req.Metadata),
	}

	if err := s.txRepo.Create(ctx, tx); err != nil {
		return nil, fmt.Errorf("failed to create transaction: %w", err)
	}

	// Publish event to Redis Stream for async processing
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
		RetryCount:    0,
	}

	if _, err := s.streamClient.Publish(ctx, event); err != nil {
		log.Error().Err(err).
			Str("transaction_id", tx.ID.String()).
			Msg("Failed to publish event to stream")
		// Don't fail the request - transaction is saved, will be processed later
	}

	// Create audit log
	s.createAuditLog(ctx, tx, requestID, "create")

	processingTime := time.Since(startTime)
	log.Info().
		Str("transaction_id", tx.ID.String()).
		Str("account_id", tx.AccountID.String()).
		Float64("amount", tx.Amount).
		Dur("processing_time", processingTime).
		Msg("Transaction ingested")

	return &TransactionResponse{
		TransactionID:  tx.ID.String(),
		Status:         tx.Status,
		IdempotencyKey: tx.IdempotencyKey,
		CreatedAt:      tx.CreatedAt,
	}, nil
}

// IngestBatch ingests multiple transactions
func (s *IngestionService) IngestBatch(ctx context.Context, req *BatchTransactionRequest, requestID string) (*BatchTransactionResponse, error) {
	startTime := time.Now()
	
	response := &BatchTransactionResponse{
		Results: make([]TransactionResponse, 0, len(req.Transactions)),
	}

	// Process transactions
	var transactions []*models.Transaction
	var events []*models.TransactionEvent

	for _, txReq := range req.Transactions {
		accountID, err := uuid.Parse(txReq.AccountID)
		if err != nil {
			response.Failed++
			response.Results = append(response.Results, TransactionResponse{
				IdempotencyKey: txReq.IdempotencyKey,
				Status:         "failed",
				Message:        fmt.Sprintf("invalid account_id: %v", err),
			})
			continue
		}

		tx := &models.Transaction{
			AccountID:        accountID,
			Amount:           txReq.Amount,
			Currency:         txReq.Currency,
			Merchant:         txReq.Merchant,
			MerchantCategory: txReq.MerchantCategory,
			Location:         txReq.Location,
			Country:          txReq.Country,
			Channel:          txReq.Channel,
			IdempotencyKey:   txReq.IdempotencyKey,
			Metadata:         models.JSONB(txReq.Metadata),
		}

		transactions = append(transactions, tx)
	}

	// Batch insert transactions
	if len(transactions) > 0 {
		if err := s.txRepo.CreateBatch(ctx, transactions); err != nil {
			log.Error().Err(err).Msg("Failed to batch insert transactions")
			// Mark all as failed
			for _, tx := range transactions {
				response.Failed++
				response.Results = append(response.Results, TransactionResponse{
					IdempotencyKey: tx.IdempotencyKey,
					Status:         "failed",
					Message:        fmt.Sprintf("batch insert failed: %v", err),
				})
			}
		} else {
			// Create events for successful transactions
			for _, tx := range transactions {
				events = append(events, &models.TransactionEvent{
					TransactionID: tx.ID.String(),
					AccountID:     tx.AccountID.String(),
					Amount:        tx.Amount,
					Currency:      tx.Currency,
					Merchant:      tx.Merchant,
					Location:      tx.Location,
					Country:       tx.Country,
					Channel:       tx.Channel,
					Timestamp:     tx.CreatedAt,
					RetryCount:    0,
				})

				response.Successful++
				response.Results = append(response.Results, TransactionResponse{
					TransactionID:  tx.ID.String(),
					Status:         tx.Status,
					IdempotencyKey: tx.IdempotencyKey,
					CreatedAt:      tx.CreatedAt,
				})
			}

			// Batch publish events
			if _, err := s.streamClient.PublishBatch(ctx, events); err != nil {
				log.Error().Err(err).Msg("Failed to batch publish events")
			}
		}
	}

	processingTime := time.Since(startTime)
	log.Info().
		Int("total", len(req.Transactions)).
		Int("successful", response.Successful).
		Int("failed", response.Failed).
		Dur("processing_time", processingTime).
		Msg("Batch ingestion completed")

	return response, nil
}

// createAuditLog creates an audit log entry for a transaction
func (s *IngestionService) createAuditLog(ctx context.Context, tx *models.Transaction, requestID, action string) {
	auditLog := &models.AuditLog{
		EventType:  models.AuditEventTransaction,
		EntityID:   tx.ID,
		EntityType: "transaction",
		Action:     action,
		RequestID:  requestID,
		Payload: models.JSONB{
			"amount":     tx.Amount,
			"currency":   tx.Currency,
			"merchant":   tx.Merchant,
			"location":   tx.Location,
			"account_id": tx.AccountID.String(),
		},
	}

	if err := s.auditRepo.Create(ctx, auditLog); err != nil {
		log.Error().Err(err).
			Str("transaction_id", tx.ID.String()).
			Msg("Failed to create audit log")
	}
}

// GetTransaction retrieves a transaction by ID
func (s *IngestionService) GetTransaction(ctx context.Context, transactionID string) (*models.Transaction, error) {
	id, err := uuid.Parse(transactionID)
	if err != nil {
		return nil, fmt.Errorf("invalid transaction_id format: %w", err)
	}

	return s.txRepo.GetByID(ctx, id)
}

// GetTransactionsByAccount retrieves transactions for an account
func (s *IngestionService) GetTransactionsByAccount(ctx context.Context, accountID string, page, pageSize int, startDate, endDate *time.Time) ([]*models.Transaction, int, error) {
	id, err := uuid.Parse(accountID)
	if err != nil {
		return nil, 0, fmt.Errorf("invalid account_id format: %w", err)
	}

	return s.txRepo.GetByAccountID(ctx, id, page, pageSize, startDate, endDate)
}
