package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/enterprise/risk-engine/configs"
	"github.com/enterprise/risk-engine/internal/queue"
	"github.com/enterprise/risk-engine/internal/repositories"
	"github.com/enterprise/risk-engine/internal/scoring"
)

func main() {
	// Load .env file if exists
	_ = godotenv.Load()

	// Load configuration
	cfg := configs.Load()

	// Setup logging
	setupLogging(cfg.Server.Environment)

	log.Info().
		Str("environment", cfg.Server.Environment).
		Int("concurrency", cfg.Worker.Concurrency).
		Msg("Starting Enterprise Risk Engine Worker")

	// Initialize database
	db, err := repositories.NewDatabase(cfg.Database)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to database")
	}
	defer db.Close()

	// Initialize Redis Stream client
	streamClient, err := queue.NewRedisStreamClient(cfg.Redis)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to Redis Stream")
	}
	defer streamClient.Close()

	// Initialize Redis Cache client
	cacheClient, err := queue.NewCacheClient(cfg.Redis)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to Redis Cache")
	}
	defer cacheClient.Close()

	// Initialize repositories
	txRepo := repositories.NewTransactionRepository(db)
	accountRepo := repositories.NewAccountRepository(db)
	riskScoreRepo := repositories.NewRiskScoreRepository(db)

	// Initialize scoring engine
	scoringEngine := scoring.NewScoringEngine(txRepo, accountRepo, riskScoreRepo, cacheClient)

	// Create worker pool
	workerPool := scoring.NewWorkerPool(
		cfg.Worker.Concurrency,
		scoringEngine,
		streamClient,
		cfg.Worker,
	)

	// Setup context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Start worker pool in goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- workerPool.Start(ctx)
	}()

	// Wait for shutdown signal or error
	select {
	case sig := <-sigCh:
		log.Info().Str("signal", sig.String()).Msg("Received shutdown signal")
		cancel()
	case err := <-errCh:
		if err != nil && err != context.Canceled {
			log.Error().Err(err).Msg("Worker pool error")
		}
	}

	// Stop worker pool
	if err := workerPool.Stop(); err != nil {
		log.Error().Err(err).Msg("Failed to stop worker pool")
	}

	log.Info().Msg("Worker shutdown complete")
}

func setupLogging(env string) {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	if env == "development" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}
}
