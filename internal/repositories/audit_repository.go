package repositories

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/enterprise/risk-engine/internal/models"
)

// AuditRepository handles audit log database operations
type AuditRepository struct {
	db *Database
}

// NewAuditRepository creates a new audit repository
func NewAuditRepository(db *Database) *AuditRepository {
	return &AuditRepository{db: db}
}

// Create creates a new audit log entry
func (r *AuditRepository) Create(ctx context.Context, log *models.AuditLog) error {
	query := `
		INSERT INTO audit_logs (
			id, event_type, entity_id, entity_type, user_id, action,
			payload, ip_address, user_agent, request_id, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8::inet, $9, $10, $11)
	`

	log.ID = uuid.New()
	log.CreatedAt = time.Now()

	payloadBytes, _ := log.Payload.Value()

	_, err := r.db.Pool.Exec(ctx, query,
		log.ID,
		log.EventType,
		log.EntityID,
		log.EntityType,
		log.UserID,
		log.Action,
		payloadBytes,
		log.IPAddress,
		log.UserAgent,
		log.RequestID,
		log.CreatedAt,
	)

	return err
}

// CreateBatch creates multiple audit log entries in a batch
func (r *AuditRepository) CreateBatch(ctx context.Context, logs []*models.AuditLog) error {
	if len(logs) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	query := `
		INSERT INTO audit_logs (
			id, event_type, entity_id, entity_type, user_id, action,
			payload, ip_address, user_agent, request_id, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8::inet, $9, $10, $11)
	`

	for _, log := range logs {
		log.ID = uuid.New()
		log.CreatedAt = time.Now()
		payloadBytes, _ := log.Payload.Value()

		batch.Queue(query,
			log.ID,
			log.EventType,
			log.EntityID,
			log.EntityType,
			log.UserID,
			log.Action,
			payloadBytes,
			log.IPAddress,
			log.UserAgent,
			log.RequestID,
			log.CreatedAt,
		)
	}

	br := r.db.Pool.SendBatch(ctx, batch)
	defer br.Close()

	for range logs {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}

	return nil
}

// GetByEntityID retrieves audit logs for an entity
func (r *AuditRepository) GetByEntityID(ctx context.Context, entityType string, entityID uuid.UUID, page, pageSize int) ([]*models.AuditLog, int, error) {
	offset := (page - 1) * pageSize

	countQuery := `SELECT COUNT(*) FROM audit_logs WHERE entity_type = $1 AND entity_id = $2`
	var total int
	if err := r.db.Pool.QueryRow(ctx, countQuery, entityType, entityID).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
		SELECT id, event_type, entity_id, entity_type, user_id, action,
			   payload, ip_address, user_agent, request_id, created_at
		FROM audit_logs
		WHERE entity_type = $1 AND entity_id = $2
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4
	`

	rows, err := r.db.Pool.Query(ctx, query, entityType, entityID, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	return r.scanAuditLogs(rows, total)
}

// GetByEventType retrieves audit logs by event type
func (r *AuditRepository) GetByEventType(ctx context.Context, eventType string, page, pageSize int) ([]*models.AuditLog, int, error) {
	offset := (page - 1) * pageSize

	countQuery := `SELECT COUNT(*) FROM audit_logs WHERE event_type = $1`
	var total int
	if err := r.db.Pool.QueryRow(ctx, countQuery, eventType).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
		SELECT id, event_type, entity_id, entity_type, user_id, action,
			   payload, ip_address, user_agent, request_id, created_at
		FROM audit_logs
		WHERE event_type = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.db.Pool.Query(ctx, query, eventType, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	return r.scanAuditLogs(rows, total)
}

// GetByRequestID retrieves audit logs by request ID
func (r *AuditRepository) GetByRequestID(ctx context.Context, requestID string) ([]*models.AuditLog, error) {
	query := `
		SELECT id, event_type, entity_id, entity_type, user_id, action,
			   payload, ip_address, user_agent, request_id, created_at
		FROM audit_logs
		WHERE request_id = $1
		ORDER BY created_at ASC
	`

	rows, err := r.db.Pool.Query(ctx, query, requestID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	logs, _, err := r.scanAuditLogs(rows, 0)
	return logs, err
}

// GetRecent retrieves recent audit logs
func (r *AuditRepository) GetRecent(ctx context.Context, limit int) ([]*models.AuditLog, error) {
	query := `
		SELECT id, event_type, entity_id, entity_type, user_id, action,
			   payload, ip_address, user_agent, request_id, created_at
		FROM audit_logs
		ORDER BY created_at DESC
		LIMIT $1
	`

	rows, err := r.db.Pool.Query(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	logs, _, err := r.scanAuditLogs(rows, 0)
	return logs, err
}

func (r *AuditRepository) scanAuditLogs(rows pgx.Rows, total int) ([]*models.AuditLog, int, error) {
	var logs []*models.AuditLog
	for rows.Next() {
		log := &models.AuditLog{}
		var payloadBytes []byte
		var ipAddress *string

		if err := rows.Scan(
			&log.ID,
			&log.EventType,
			&log.EntityID,
			&log.EntityType,
			&log.UserID,
			&log.Action,
			&payloadBytes,
			&ipAddress,
			&log.UserAgent,
			&log.RequestID,
			&log.CreatedAt,
		); err != nil {
			return nil, 0, err
		}

		if ipAddress != nil {
			log.IPAddress = *ipAddress
		}
		log.Payload.Scan(payloadBytes)
		logs = append(logs, log)
	}

	return logs, total, nil
}
