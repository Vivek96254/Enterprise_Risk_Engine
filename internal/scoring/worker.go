package scoring

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/enterprise/risk-engine/configs"
	"github.com/enterprise/risk-engine/internal/models"
	"github.com/enterprise/risk-engine/internal/queue"
)

// Worker processes transaction events from the queue
type Worker struct {
	id            string
	engine        *ScoringEngine
	streamClient  *queue.RedisStreamClient
	config        configs.WorkerConfig
	wg            sync.WaitGroup
	stopCh        chan struct{}
	metrics       *WorkerMetrics
}

// WorkerMetrics tracks worker performance
type WorkerMetrics struct {
	mu                sync.RWMutex
	ProcessedCount    int64
	FailedCount       int64
	TotalProcessingMs int64
	LastProcessedAt   time.Time
}

// NewWorker creates a new scoring worker
func NewWorker(id string, engine *ScoringEngine, streamClient *queue.RedisStreamClient, config configs.WorkerConfig) *Worker {
	return &Worker{
		id:           id,
		engine:       engine,
		streamClient: streamClient,
		config:       config,
		stopCh:       make(chan struct{}),
		metrics:      &WorkerMetrics{},
	}
}

// Start starts the worker
func (w *Worker) Start(ctx context.Context) error {
	log.Info().
		Str("worker_id", w.id).
		Int("concurrency", w.config.Concurrency).
		Msg("Starting scoring worker")

	// Start worker goroutines
	for i := 0; i < w.config.Concurrency; i++ {
		w.wg.Add(1)
		go w.processLoop(ctx, fmt.Sprintf("%s-%d", w.id, i))
	}

	// Handle graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-sigCh:
		log.Info().Msg("Received shutdown signal")
	case <-ctx.Done():
		log.Info().Msg("Context cancelled")
	}

	return w.Stop()
}

// Stop stops the worker gracefully
func (w *Worker) Stop() error {
	log.Info().Str("worker_id", w.id).Msg("Stopping worker...")
	close(w.stopCh)
	w.wg.Wait()
	log.Info().Str("worker_id", w.id).Msg("Worker stopped")
	return nil
}

// processLoop is the main processing loop for a worker goroutine
func (w *Worker) processLoop(ctx context.Context, consumerName string) {
	defer w.wg.Done()

	log.Info().Str("consumer", consumerName).Msg("Worker goroutine started")

	for {
		select {
		case <-w.stopCh:
			log.Info().Str("consumer", consumerName).Msg("Worker goroutine stopping")
			return
		case <-ctx.Done():
			return
		default:
			w.processBatch(ctx, consumerName)
		}
	}
}

// processBatch processes a batch of messages from the queue
func (w *Worker) processBatch(ctx context.Context, consumerName string) {
	// Read messages from stream
	messages, err := w.streamClient.Consume(ctx, consumerName, int64(w.config.BatchSize), w.config.PollInterval)
	if err != nil {
		log.Error().Err(err).Str("consumer", consumerName).Msg("Failed to consume messages")
		time.Sleep(time.Second) // Back off on error
		return
	}

	if len(messages) == 0 {
		return
	}

	log.Debug().
		Str("consumer", consumerName).
		Int("count", len(messages)).
		Msg("Processing batch")

	var ackIDs []string

	for _, msg := range messages {
		if err := w.processMessage(ctx, msg); err != nil {
			log.Error().
				Err(err).
				Str("message_id", msg.ID).
				Str("transaction_id", msg.Event.TransactionID).
				Msg("Failed to process message")

			// Handle retry logic
			if msg.Event.RetryCount < w.config.RetryAttempts {
				msg.Event.RetryCount++
				if _, err := w.streamClient.Publish(ctx, msg.Event); err != nil {
					log.Error().Err(err).Msg("Failed to requeue message")
				}
			} else {
				// Send to dead letter queue
				if err := w.streamClient.SendToDeadLetter(ctx, msg.Event, err); err != nil {
					log.Error().Err(err).Msg("Failed to send to dead letter queue")
				}
			}

			w.metrics.mu.Lock()
			w.metrics.FailedCount++
			w.metrics.mu.Unlock()
		}

		ackIDs = append(ackIDs, msg.ID)
	}

	// Acknowledge processed messages
	if len(ackIDs) > 0 {
		if err := w.streamClient.AcknowledgeBatch(ctx, ackIDs); err != nil {
			log.Error().Err(err).Msg("Failed to acknowledge messages")
		}
	}
}

// processMessage processes a single message
func (w *Worker) processMessage(ctx context.Context, msg queue.StreamMessage) error {
	startTime := time.Now()

	// Score the transaction
	_, err := w.engine.ScoreTransaction(ctx, msg.Event)
	if err != nil {
		return fmt.Errorf("scoring failed: %w", err)
	}

	processingTime := time.Since(startTime)

	// Update metrics
	w.metrics.mu.Lock()
	w.metrics.ProcessedCount++
	w.metrics.TotalProcessingMs += processingTime.Milliseconds()
	w.metrics.LastProcessedAt = time.Now()
	w.metrics.mu.Unlock()

	return nil
}

// GetMetrics returns the worker metrics
func (w *Worker) GetMetrics() WorkerMetrics {
	w.metrics.mu.RLock()
	defer w.metrics.mu.RUnlock()
	return WorkerMetrics{
		ProcessedCount:    w.metrics.ProcessedCount,
		FailedCount:       w.metrics.FailedCount,
		TotalProcessingMs: w.metrics.TotalProcessingMs,
		LastProcessedAt:   w.metrics.LastProcessedAt,
	}
}

// WorkerPool manages multiple workers
type WorkerPool struct {
	workers []*Worker
	wg      sync.WaitGroup
}

// NewWorkerPool creates a new worker pool
func NewWorkerPool(
	numWorkers int,
	engine *ScoringEngine,
	streamClient *queue.RedisStreamClient,
	config configs.WorkerConfig,
) *WorkerPool {
	pool := &WorkerPool{
		workers: make([]*Worker, numWorkers),
	}

	for i := 0; i < numWorkers; i++ {
		pool.workers[i] = NewWorker(
			fmt.Sprintf("worker-%d", i),
			engine,
			streamClient,
			config,
		)
	}

	return pool
}

// Start starts all workers in the pool
func (p *WorkerPool) Start(ctx context.Context) error {
	log.Info().Int("num_workers", len(p.workers)).Msg("Starting worker pool")

	errCh := make(chan error, len(p.workers))

	for _, worker := range p.workers {
		w := worker
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			if err := w.Start(ctx); err != nil {
				errCh <- err
			}
		}()
	}

	// Wait for first error or context cancellation
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Stop stops all workers in the pool
func (p *WorkerPool) Stop() error {
	log.Info().Msg("Stopping worker pool")

	for _, worker := range p.workers {
		if err := worker.Stop(); err != nil {
			log.Error().Err(err).Str("worker_id", worker.id).Msg("Failed to stop worker")
		}
	}

	p.wg.Wait()
	log.Info().Msg("Worker pool stopped")
	return nil
}

// GetAggregatedMetrics returns aggregated metrics from all workers
func (p *WorkerPool) GetAggregatedMetrics() map[string]interface{} {
	var totalProcessed, totalFailed, totalProcessingMs int64
	var lastProcessedAt time.Time

	for _, worker := range p.workers {
		metrics := worker.GetMetrics()
		totalProcessed += metrics.ProcessedCount
		totalFailed += metrics.FailedCount
		totalProcessingMs += metrics.TotalProcessingMs
		if metrics.LastProcessedAt.After(lastProcessedAt) {
			lastProcessedAt = metrics.LastProcessedAt
		}
	}

	avgProcessingMs := float64(0)
	if totalProcessed > 0 {
		avgProcessingMs = float64(totalProcessingMs) / float64(totalProcessed)
	}

	return map[string]interface{}{
		"total_processed":     totalProcessed,
		"total_failed":        totalFailed,
		"avg_processing_ms":   avgProcessingMs,
		"last_processed_at":   lastProcessedAt,
		"active_workers":      len(p.workers),
	}
}

// BacktestWorker processes historical transactions for backtesting
type BacktestWorker struct {
	engine *ScoringEngine
}

// NewBacktestWorker creates a new backtest worker
func NewBacktestWorker(engine *ScoringEngine) *BacktestWorker {
	return &BacktestWorker{engine: engine}
}

// BacktestTransaction scores a historical transaction without side effects
func (w *BacktestWorker) BacktestTransaction(ctx context.Context, tx *models.Transaction) (*models.RiskScore, error) {
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

	return w.engine.ScoreTransaction(ctx, event)
}
