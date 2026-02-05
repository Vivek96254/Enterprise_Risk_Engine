package main

import (
	"context"
	"encoding/json"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/IBM/sarama"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/enterprise/risk-engine/configs"
	"github.com/enterprise/risk-engine/internal/queue"
	"github.com/enterprise/risk-engine/internal/repositories"
)

// =============================================================================
// HYBRID ARCHITECTURE: Kafka CDC Analytics Pipeline
// =============================================================================
// This worker does NOT score transactions (Redis worker handles that).
// Instead, it captures ALL database changes for:
//   - Audit trail / compliance logging
//   - Real-time analytics aggregation
//   - ML model training data collection
//   - Event replay capabilities
//   - Data warehouse sync
// =============================================================================

// DebeziumMessage represents a CDC event from Debezium
type DebeziumMessage struct {
	Before      json.RawMessage `json:"before"`
	After       json.RawMessage `json:"after"`
	Source      DebeziumSource  `json:"source"`
	Op          string          `json:"op"` // c=create, u=update, d=delete, r=read (snapshot)
	TsMs        int64           `json:"ts_ms"`
	Transaction json.RawMessage `json:"transaction"`
}

// DebeziumSource contains metadata about the change
type DebeziumSource struct {
	Version   string `json:"version"`
	Connector string `json:"connector"`
	Name      string `json:"name"`
	TsMs      int64  `json:"ts_ms"`
	Snapshot  string `json:"snapshot"`
	DB        string `json:"db"`
	Schema    string `json:"schema"`
	Table     string `json:"table"`
	TxID      int64  `json:"txId"`
	LSN       int64  `json:"lsn"`
}

// TransactionCDC represents a transaction from CDC
type TransactionCDC struct {
	ID               string      `json:"id"`
	AccountID        string      `json:"account_id"`
	Amount           interface{} `json:"amount"`
	Currency         string      `json:"currency"`
	Merchant         string      `json:"merchant"`
	MerchantCategory string      `json:"merchant_category"`
	Location         string      `json:"location"`
	Country          string      `json:"country"`
	Channel          string      `json:"channel"`
	Status           string      `json:"status"`
	IdempotencyKey   string      `json:"idempotency_key"`
	CreatedAt        string      `json:"created_at"`
	ProcessedAt      *string     `json:"processed_at"`
}

// AnalyticsEvent represents a processed event for analytics
type AnalyticsEvent struct {
	EventType     string                 `json:"event_type"`
	TransactionID string                 `json:"transaction_id"`
	AccountID     string                 `json:"account_id"`
	Merchant      string                 `json:"merchant"`
	Category      string                 `json:"category"`
	Country       string                 `json:"country"`
	Channel       string                 `json:"channel"`
	Status        string                 `json:"status"`
	PrevStatus    string                 `json:"prev_status,omitempty"`
	Timestamp     time.Time              `json:"timestamp"`
	CDCTimestamp  int64                  `json:"cdc_timestamp_ms"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// RealTimeMetrics tracks live metrics
type RealTimeMetrics struct {
	mu                    sync.RWMutex
	TransactionsCreated   int64
	TransactionsProcessed int64
	TransactionsFlagged   int64
	TransactionsBlocked   int64
	CountryDistribution   map[string]int64
	ChannelDistribution   map[string]int64
	StatusTransitions     map[string]int64
	LastEventTime         time.Time
	EventsPerSecond       float64
	windowStart           time.Time
	windowCount           int64
}

func NewRealTimeMetrics() *RealTimeMetrics {
	return &RealTimeMetrics{
		CountryDistribution: make(map[string]int64),
		ChannelDistribution: make(map[string]int64),
		StatusTransitions:   make(map[string]int64),
		windowStart:         time.Now(),
	}
}

func (m *RealTimeMetrics) RecordEvent(event *AnalyticsEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.LastEventTime = time.Now()
	m.windowCount++

	// Calculate events per second
	elapsed := time.Since(m.windowStart).Seconds()
	if elapsed > 0 {
		m.EventsPerSecond = float64(m.windowCount) / elapsed
	}

	// Reset window every minute
	if elapsed > 60 {
		m.windowStart = time.Now()
		m.windowCount = 0
	}

	switch event.EventType {
	case "transaction_created":
		m.TransactionsCreated++
		m.CountryDistribution[event.Country]++
		m.ChannelDistribution[event.Channel]++
	case "transaction_updated":
		transition := event.PrevStatus + "->" + event.Status
		m.StatusTransitions[transition]++

		switch event.Status {
		case "processed":
			m.TransactionsProcessed++
		case "flagged":
			m.TransactionsFlagged++
		case "blocked":
			m.TransactionsBlocked++
		}
	}
}

func (m *RealTimeMetrics) GetSnapshot() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return map[string]interface{}{
		"transactions_created":   m.TransactionsCreated,
		"transactions_processed": m.TransactionsProcessed,
		"transactions_flagged":   m.TransactionsFlagged,
		"transactions_blocked":   m.TransactionsBlocked,
		"events_per_second":      m.EventsPerSecond,
		"country_distribution":   m.CountryDistribution,
		"channel_distribution":   m.ChannelDistribution,
		"status_transitions":     m.StatusTransitions,
		"last_event_time":        m.LastEventTime,
	}
}

func main() {
	// Setup logging
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	if os.Getenv("ENVIRONMENT") == "development" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}

	log.Info().Msg("🔄 Starting Kafka CDC Analytics Pipeline")
	log.Info().Msg("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Info().Msg("This pipeline captures CDC events for analytics & audit.")
	log.Info().Msg("Scoring is handled by Redis Stream workers (fast path).")
	log.Info().Msg("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Load configuration
	cfg := configs.Load()

	// Get Kafka configuration from environment
	kafkaBrokers := os.Getenv("KAFKA_BROKERS")
	if kafkaBrokers == "" {
		kafkaBrokers = "localhost:9092"
	}
	brokers := strings.Split(kafkaBrokers, ",")

	kafkaGroupID := os.Getenv("KAFKA_GROUP_ID")
	if kafkaGroupID == "" {
		kafkaGroupID = "analytics-pipeline"
	}

	kafkaTopics := os.Getenv("KAFKA_TOPICS")
	if kafkaTopics == "" {
		kafkaTopics = "risk-engine.public.transactions"
	}
	topics := strings.Split(kafkaTopics, ",")

	// Connect to database (for enrichment queries)
	db, err := repositories.NewDatabase(cfg.Database)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to database")
	}
	defer db.Close()

	// Connect to Redis (for caching and publishing analytics)
	cacheClient, err := queue.NewCacheClient(cfg.Redis)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to Redis")
	}
	defer cacheClient.Close()

	// Initialize repositories
	riskScoreRepo := repositories.NewRiskScoreRepository(db)

	// Initialize real-time metrics
	metrics := NewRealTimeMetrics()

	// Create Kafka consumer
	config := sarama.NewConfig()
	config.Consumer.Group.Rebalance.GroupStrategies = []sarama.BalanceStrategy{sarama.NewBalanceStrategyRoundRobin()}
	config.Consumer.Offsets.Initial = sarama.OffsetNewest
	config.Consumer.Return.Errors = true
	config.Version = sarama.V3_0_0_0

	// Retry connecting to Kafka
	var consumerGroup sarama.ConsumerGroup
	for i := 0; i < 30; i++ {
		consumerGroup, err = sarama.NewConsumerGroup(brokers, kafkaGroupID, config)
		if err == nil {
			break
		}
		log.Warn().Err(err).Int("attempt", i+1).Msg("Failed to connect to Kafka, retrying...")
		time.Sleep(5 * time.Second)
	}
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create Kafka consumer group after retries")
	}
	defer consumerGroup.Close()

	// Create consumer handler
	handler := &AnalyticsPipelineHandler{
		metrics:       metrics,
		riskScoreRepo: riskScoreRepo,
		cacheClient:   cacheClient,
	}

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Info().Msg("Shutdown signal received, stopping analytics pipeline...")
		cancel()
	}()

	// Start metrics reporter (logs every 30 seconds)
	go handler.startMetricsReporter(ctx)

	// Start consuming
	log.Info().
		Strs("brokers", brokers).
		Strs("topics", topics).
		Str("group_id", kafkaGroupID).
		Msg("📊 Analytics pipeline started - consuming CDC events")

	for {
		if err := consumerGroup.Consume(ctx, topics, handler); err != nil {
			log.Error().Err(err).Msg("Error from consumer")
		}

		if ctx.Err() != nil {
			log.Info().Msg("Context cancelled, shutting down analytics pipeline")
			return
		}
	}
}

// AnalyticsPipelineHandler processes CDC events for analytics
type AnalyticsPipelineHandler struct {
	metrics       *RealTimeMetrics
	riskScoreRepo *repositories.RiskScoreRepository
	cacheClient   *queue.CacheClient
}

func (h *AnalyticsPipelineHandler) Setup(sarama.ConsumerGroupSession) error {
	log.Info().Msg("Analytics pipeline session started")
	return nil
}

func (h *AnalyticsPipelineHandler) Cleanup(sarama.ConsumerGroupSession) error {
	log.Info().Msg("Analytics pipeline session ended")
	return nil
}

func (h *AnalyticsPipelineHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for {
		select {
		case message, ok := <-claim.Messages():
			if !ok {
				return nil
			}

			h.processMessage(session.Context(), message)
			session.MarkMessage(message, "")

		case <-session.Context().Done():
			return nil
		}
	}
}

func (h *AnalyticsPipelineHandler) processMessage(ctx context.Context, message *sarama.ConsumerMessage) {
	// Parse Debezium message
	var debeziumMsg DebeziumMessage
	if err := json.Unmarshal(message.Value, &debeziumMsg); err != nil {
		log.Error().Err(err).Msg("Failed to parse Debezium message")
		return
	}

	// Parse the transaction
	var tx TransactionCDC
	var prevTx *TransactionCDC

	if debeziumMsg.After != nil {
		if err := json.Unmarshal(debeziumMsg.After, &tx); err != nil {
			log.Error().Err(err).Msg("Failed to parse transaction from CDC payload")
			return
		}
	}

	if debeziumMsg.Before != nil {
		prevTx = &TransactionCDC{}
		if err := json.Unmarshal(debeziumMsg.Before, prevTx); err != nil {
			prevTx = nil // Ignore parse errors for 'before'
		}
	}

	// Create analytics event
	event := h.createAnalyticsEvent(&debeziumMsg, &tx, prevTx)

	// Record in real-time metrics
	h.metrics.RecordEvent(event)

	// Log the event with appropriate level
	h.logEvent(event, &debeziumMsg)

	// Store in audit trail (could be sent to data lake, S3, etc.)
	h.storeAuditEvent(ctx, event)
}

func (h *AnalyticsPipelineHandler) createAnalyticsEvent(msg *DebeziumMessage, tx *TransactionCDC, prevTx *TransactionCDC) *AnalyticsEvent {
	eventType := "unknown"
	switch msg.Op {
	case "c":
		eventType = "transaction_created"
	case "u":
		eventType = "transaction_updated"
	case "d":
		eventType = "transaction_deleted"
	case "r":
		eventType = "transaction_snapshot"
	}

	event := &AnalyticsEvent{
		EventType:     eventType,
		TransactionID: tx.ID,
		AccountID:     tx.AccountID,
		Merchant:      tx.Merchant,
		Category:      tx.MerchantCategory,
		Country:       tx.Country,
		Channel:       tx.Channel,
		Status:        tx.Status,
		Timestamp:     time.Now(),
		CDCTimestamp:  msg.TsMs,
		Metadata: map[string]interface{}{
			"table":     msg.Source.Table,
			"lsn":       msg.Source.LSN,
			"txId":      msg.Source.TxID,
			"connector": msg.Source.Connector,
		},
	}

	if prevTx != nil {
		event.PrevStatus = prevTx.Status
	}

	return event
}

func (h *AnalyticsPipelineHandler) logEvent(event *AnalyticsEvent, msg *DebeziumMessage) {
	switch event.EventType {
	case "transaction_created":
		log.Info().
			Str("event", "📥 NEW").
			Str("tx_id", event.TransactionID[:8]+"...").
			Str("merchant", event.Merchant).
			Str("country", event.Country).
			Str("channel", event.Channel).
			Msg("Transaction captured")

	case "transaction_updated":
		icon := "📝"
		if event.Status == "flagged" {
			icon = "🚨"
		} else if event.Status == "blocked" {
			icon = "🛑"
		} else if event.Status == "processed" {
			icon = "✅"
		}

		log.Info().
			Str("event", icon+" UPDATE").
			Str("tx_id", event.TransactionID[:8]+"...").
			Str("status", event.PrevStatus+"→"+event.Status).
			Msg("Transaction status changed")

	case "transaction_deleted":
		log.Warn().
			Str("event", "🗑️ DELETE").
			Str("tx_id", event.TransactionID[:8]+"...").
			Msg("Transaction deleted")
	}
}

func (h *AnalyticsPipelineHandler) storeAuditEvent(ctx context.Context, event *AnalyticsEvent) {
	// In production, this would:
	// 1. Write to audit log table
	// 2. Send to data lake (S3, GCS, etc.)
	// 3. Forward to SIEM system
	// 4. Update ML training dataset

	// For now, we'll cache the latest events in Redis for dashboard access
	eventJSON, err := json.Marshal(event)
	if err != nil {
		return
	}

	// Store in Redis list (recent events)
	key := "analytics:recent_events"
	h.cacheClient.LPush(ctx, key, string(eventJSON))
	h.cacheClient.LTrim(ctx, key, 0, 999) // Keep last 1000 events
}

func (h *AnalyticsPipelineHandler) startMetricsReporter(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			snapshot := h.metrics.GetSnapshot()
			log.Info().
				Int64("created", snapshot["transactions_created"].(int64)).
				Int64("processed", snapshot["transactions_processed"].(int64)).
				Int64("flagged", snapshot["transactions_flagged"].(int64)).
				Int64("blocked", snapshot["transactions_blocked"].(int64)).
				Float64("events_per_sec", snapshot["events_per_second"].(float64)).
				Msg("📊 Analytics Pipeline Metrics")

		case <-ctx.Done():
			return
		}
	}
}
