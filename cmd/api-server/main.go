package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/enterprise/risk-engine/configs"
	"github.com/enterprise/risk-engine/internal/analytics"
	"github.com/enterprise/risk-engine/internal/auth"
	"github.com/enterprise/risk-engine/internal/ingestion"
	"github.com/enterprise/risk-engine/internal/models"
	"github.com/enterprise/risk-engine/internal/queue"
	"github.com/enterprise/risk-engine/internal/repositories"
	"github.com/enterprise/risk-engine/internal/scoring"
	"github.com/enterprise/risk-engine/internal/services"
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
		Str("port", cfg.Server.Port).
		Msg("Starting Enterprise Risk Engine API Server")

	// Initialize database
	db, err := repositories.NewDatabase(cfg.Database)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to database")
	}
	defer db.Close()

	// Initialize Redis
	streamClient, err := queue.NewRedisStreamClient(cfg.Redis)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to Redis Stream")
	}
	defer streamClient.Close()

	cacheClient, err := queue.NewCacheClient(cfg.Redis)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to Redis Cache")
	}
	defer cacheClient.Close()

	// Initialize repositories
	userRepo := repositories.NewUserRepository(db)
	accountRepo := repositories.NewAccountRepository(db)
	txRepo := repositories.NewTransactionRepository(db)
	riskScoreRepo := repositories.NewRiskScoreRepository(db)
	auditRepo := repositories.NewAuditRepository(db)

	// Initialize services
	jwtManager := auth.NewJWTManager(cfg.JWT.Secret, cfg.JWT.Expiration)
	authService := services.NewAuthService(userRepo, jwtManager)
	ingestionService := ingestion.NewIngestionService(txRepo, accountRepo, auditRepo, streamClient, cacheClient)
	scoringEngine := scoring.NewScoringEngine(txRepo, accountRepo, riskScoreRepo, cacheClient)
	analyticsService := analytics.NewAnalyticsService(txRepo, riskScoreRepo, accountRepo, db, cacheClient)

	// Setup Gin router
	if cfg.Server.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(requestIDMiddleware())
	router.Use(loggingMiddleware())
	router.Use(corsMiddleware())
	
	// Rate limiting: 100 requests per minute per IP
	rateLimiter := NewRateLimiter(100, time.Minute)
	router.Use(rateLimitMiddleware(rateLimiter))

	// Setup routes
	setupRoutes(router, jwtManager, authService, ingestionService, scoringEngine, analyticsService, streamClient, db, txRepo)

	// Create HTTP server
	srv := &http.Server{
		Addr:         ":" + cfg.Server.Port,
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	// Start server in goroutine
	go func() {
		log.Info().Str("port", cfg.Server.Port).Msg("Server listening")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("Server failed")
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("Shutting down server...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("Server forced to shutdown")
	}

	log.Info().Msg("Server exited")
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

func setupRoutes(
	router *gin.Engine,
	jwtManager *auth.JWTManager,
	authService *services.AuthService,
	ingestionService *ingestion.IngestionService,
	scoringEngine *scoring.ScoringEngine,
	analyticsService *analytics.AnalyticsService,
	streamClient *queue.RedisStreamClient,
	db *repositories.Database,
	txRepo *repositories.TransactionRepository,
) {
	// Health check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "healthy",
			"timestamp": time.Now().Format(time.RFC3339),
		})
	})

	// API v1 routes
	v1 := router.Group("/api/v1")

	// Auth routes (public)
	authRoutes := v1.Group("/auth")
	{
		authRoutes.POST("/register", registerHandler(authService))
		authRoutes.POST("/login", loginHandler(authService))
		authRoutes.POST("/refresh", auth.AuthMiddleware(jwtManager), refreshTokenHandler(authService))
	}

	// Protected routes
	protected := v1.Group("")
	protected.Use(auth.AuthMiddleware(jwtManager))

	// Transaction routes
	txRoutes := protected.Group("/transactions")
	{
		txRoutes.POST("", ingestTransactionHandler(ingestionService))
		txRoutes.POST("/batch", ingestBatchHandler(ingestionService))
		txRoutes.GET("/recent", getRecentTransactionsHandler(txRepo))
		txRoutes.GET("/:id", getTransactionHandler(ingestionService))
		txRoutes.GET("/account/:account_id", getAccountTransactionsHandler(ingestionService))
		txRoutes.GET("/flagged", getFlaggedTransactionsHandler(analyticsService))
	}

	// Risk routes
	riskRoutes := protected.Group("/risk")
	{
		riskRoutes.GET("/summary", getRiskSummaryHandler(analyticsService))
		riskRoutes.GET("/account/:account_id", getAccountRiskHandler(analyticsService))
		riskRoutes.GET("/distribution", getRiskDistributionHandler(analyticsService))
		riskRoutes.GET("/rules/top", getTopRulesHandler(analyticsService))
	}

	// Backtest routes (admin only)
	backtestRoutes := protected.Group("/backtest")
	backtestRoutes.Use(auth.RoleMiddleware("admin", "analyst"))
	{
		backtestService := scoring.NewBacktestService(scoringEngine, txRepo)
		backtestRoutes.POST("/run", runBacktestHandler(backtestService))
	}

	// A/B Testing routes (admin only)
	abTestRoutes := protected.Group("/experiments")
	abTestRoutes.Use(auth.RoleMiddleware("admin"))
	{
		abManager := scoringEngine.GetABTestManager()
		abTestRoutes.POST("", createExperimentHandler(abManager))
		abTestRoutes.GET("", listExperimentsHandler(abManager))
		abTestRoutes.GET("/:id", getExperimentHandler(abManager))
		abTestRoutes.POST("/:id/start", startExperimentHandler(abManager))
		abTestRoutes.POST("/:id/stop", stopExperimentHandler(abManager))
		abTestRoutes.POST("/:id/pause", pauseExperimentHandler(abManager))
		abTestRoutes.GET("/:id/results", getExperimentResultsHandler(abManager))
		abTestRoutes.GET("/:id/significance", getExperimentSignificanceHandler(abManager))
		abTestRoutes.DELETE("/:id", deleteExperimentHandler(abManager))
	}

	// Analytics routes
	analyticsRoutes := protected.Group("/analytics")
	{
		analyticsRoutes.GET("/volume/hourly", getHourlyVolumeHandler(analyticsService))
	}

	// Metrics routes (admin only)
	metricsRoutes := protected.Group("/metrics")
	metricsRoutes.Use(auth.RoleMiddleware("admin", "analyst"))
	{
		metricsRoutes.GET("/system", getSystemMetricsHandler(analyticsService, streamClient, db))
	}

	// Account routes
	accountRoutes := protected.Group("/accounts")
	{
		accountRoutes.GET("", listAccountsHandler(db))
		accountRoutes.POST("", createAccountHandler(db))
		accountRoutes.GET("/:id", getAccountHandler(db))
	}
}

// Middleware

func requestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = fmt.Sprintf("%d", time.Now().UnixNano())
		}
		c.Set("request_id", requestID)
		c.Header("X-Request-ID", requestID)
		c.Next()
	}
}

func loggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		log.Info().
			Str("method", c.Request.Method).
			Str("path", path).
			Int("status", status).
			Dur("latency", latency).
			Str("request_id", c.GetString("request_id")).
			Str("client_ip", c.ClientIP()).
			Msg("Request completed")
	}
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, X-Request-ID")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// RateLimiter implements a simple in-memory rate limiter using token bucket algorithm
type RateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	rate     int           // requests per window
	window   time.Duration // time window
}

type visitor struct {
	tokens    int
	lastSeen  time.Time
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(rate int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		visitors: make(map[string]*visitor),
		rate:     rate,
		window:   window,
	}
	// Clean up old visitors periodically
	go rl.cleanup()
	return rl
}

func (rl *RateLimiter) cleanup() {
	for {
		time.Sleep(time.Minute)
		rl.mu.Lock()
		for ip, v := range rl.visitors {
			if time.Since(v.lastSeen) > rl.window*2 {
				delete(rl.visitors, ip)
			}
		}
		rl.mu.Unlock()
	}
}

func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	v, exists := rl.visitors[ip]
	now := time.Now()

	if !exists {
		rl.visitors[ip] = &visitor{tokens: rl.rate - 1, lastSeen: now}
		return true
	}

	// Refill tokens based on time elapsed
	elapsed := now.Sub(v.lastSeen)
	refill := int(elapsed / (rl.window / time.Duration(rl.rate)))
	v.tokens += refill
	if v.tokens > rl.rate {
		v.tokens = rl.rate
	}
	v.lastSeen = now

	if v.tokens > 0 {
		v.tokens--
		return true
	}

	return false
}

func rateLimitMiddleware(limiter *RateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		if !limiter.Allow(ip) {
			c.Header("Retry-After", "60")
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": "rate limit exceeded",
				"retry_after": 60,
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

// Handlers

func registerHandler(authService *services.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req services.RegisterRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		resp, err := authService.Register(c.Request.Context(), &req)
		if err != nil {
			status := http.StatusInternalServerError
			if err == services.ErrWeakPassword || err == repositories.ErrUserAlreadyExists {
				status = http.StatusBadRequest
			}
			c.JSON(status, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusCreated, resp)
	}
}

func loginHandler(authService *services.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req services.LoginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		resp, err := authService.Login(c.Request.Context(), &req)
		if err != nil {
			status := http.StatusInternalServerError
			if err == services.ErrInvalidCredentials {
				status = http.StatusUnauthorized
			}
			c.JSON(status, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, resp)
	}
}

func refreshTokenHandler(authService *services.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.GetHeader("Authorization")
		if len(token) > 7 {
			token = token[7:] // Remove "Bearer "
		}

		resp, err := authService.RefreshToken(c.Request.Context(), token)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, resp)
	}
}

func ingestTransactionHandler(ingestionService *ingestion.IngestionService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req ingestion.TransactionRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		requestID := c.GetString("request_id")
		resp, err := ingestionService.IngestTransaction(c.Request.Context(), &req, requestID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusCreated, resp)
	}
}

func ingestBatchHandler(ingestionService *ingestion.IngestionService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req ingestion.BatchTransactionRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		requestID := c.GetString("request_id")
		resp, err := ingestionService.IngestBatch(c.Request.Context(), &req, requestID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, resp)
	}
}

func getTransactionHandler(ingestionService *ingestion.IngestionService) gin.HandlerFunc {
	return func(c *gin.Context) {
		txID := c.Param("id")

		tx, err := ingestionService.GetTransaction(c.Request.Context(), txID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, tx)
	}
}

func getAccountTransactionsHandler(ingestionService *ingestion.IngestionService) gin.HandlerFunc {
	return func(c *gin.Context) {
		accountID := c.Param("account_id")
		page := getIntParam(c, "page", 1)
		pageSize := getIntParam(c, "page_size", 20)

		transactions, total, err := ingestionService.GetTransactionsByAccount(c.Request.Context(), accountID, page, pageSize, nil, nil)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"transactions": transactions,
			"pagination": gin.H{
				"page":      page,
				"page_size": pageSize,
				"total":     total,
			},
		})
	}
}

func getFlaggedTransactionsHandler(analyticsService *analytics.AnalyticsService) gin.HandlerFunc {
	return func(c *gin.Context) {
		page := getIntParam(c, "page", 1)
		pageSize := getIntParam(c, "page_size", 20)

		resp, err := analyticsService.GetFlaggedTransactions(c.Request.Context(), page, pageSize)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, resp)
	}
}

func getRiskSummaryHandler(analyticsService *analytics.AnalyticsService) gin.HandlerFunc {
	return func(c *gin.Context) {
		dateStr := c.Query("date")
		var date time.Time
		var err error

		if dateStr != "" {
			date, err = time.Parse("2006-01-02", dateStr)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid date format, use YYYY-MM-DD"})
				return
			}
		} else {
			date = time.Now()
		}

		summary, err := analyticsService.GetRiskSummary(c.Request.Context(), date)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, summary)
	}
}

func getAccountRiskHandler(analyticsService *analytics.AnalyticsService) gin.HandlerFunc {
	return func(c *gin.Context) {
		accountID := c.Param("account_id")

		profile, err := analyticsService.GetAccountRiskProfile(c.Request.Context(), accountID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, profile)
	}
}

func getRecentTransactionsHandler(txRepo *repositories.TransactionRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		page := getIntParam(c, "page", 1)
		pageSize := getIntParam(c, "page_size", 20)

		transactions, total, err := txRepo.GetRecent(c.Request.Context(), page, pageSize)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"transactions": transactions,
			"pagination": gin.H{
				"page":      page,
				"page_size": pageSize,
				"total":     total,
			},
		})
	}
}

func getRiskDistributionHandler(analyticsService *analytics.AnalyticsService) gin.HandlerFunc {
	return func(c *gin.Context) {
		days := getIntParam(c, "days", 7)

		distribution, err := analyticsService.GetRiskDistribution(c.Request.Context(), days)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, distribution)
	}
}

func getTopRulesHandler(analyticsService *analytics.AnalyticsService) gin.HandlerFunc {
	return func(c *gin.Context) {
		days := getIntParam(c, "days", 7)
		limit := getIntParam(c, "limit", 10)

		rules, err := analyticsService.GetTopTriggeredRules(c.Request.Context(), days, limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"rules": rules})
	}
}

func getHourlyVolumeHandler(analyticsService *analytics.AnalyticsService) gin.HandlerFunc {
	return func(c *gin.Context) {
		dateStr := c.Query("date")
		var date time.Time
		var err error

		if dateStr != "" {
			date, err = time.Parse("2006-01-02", dateStr)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid date format"})
				return
			}
		} else {
			date = time.Now()
		}

		volumes, err := analyticsService.GetHourlyTransactionVolume(c.Request.Context(), date)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"volumes": volumes})
	}
}

func getSystemMetricsHandler(analyticsService *analytics.AnalyticsService, streamClient *queue.RedisStreamClient, db *repositories.Database) gin.HandlerFunc {
	return func(c *gin.Context) {
		metrics, err := analyticsService.GetSystemMetrics(c.Request.Context(), streamClient)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, metrics)
	}
}

func createAccountHandler(db *repositories.Database) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			UserID      string `json:"user_id" binding:"required"`
			AccountType string `json:"account_type"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		accountRepo := repositories.NewAccountRepository(db)
		
		userID, err := parseUUID(req.UserID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user_id"})
			return
		}

		accountType := req.AccountType
		if accountType == "" {
			accountType = "standard"
		}

		account := &models.Account{
			UserID:      userID,
			AccountType: accountType,
			RiskProfile: models.RiskProfileLow,
			Status:      models.AccountStatusActive,
		}

		if err := accountRepo.Create(c.Request.Context(), account); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusCreated, account)
	}
}

func listAccountsHandler(db *repositories.Database) gin.HandlerFunc {
	return func(c *gin.Context) {
		page := getIntParam(c, "page", 1)
		pageSize := getIntParam(c, "page_size", 50)

		accountRepo := repositories.NewAccountRepository(db)
		accounts, total, err := accountRepo.List(c.Request.Context(), page, pageSize)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"accounts": accounts,
			"pagination": gin.H{
				"page":      page,
				"page_size": pageSize,
				"total":     total,
			},
		})
	}
}

func getAccountHandler(db *repositories.Database) gin.HandlerFunc {
	return func(c *gin.Context) {
		accountID := c.Param("id")
		
		id, err := parseUUID(accountID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid account_id"})
			return
		}

		accountRepo := repositories.NewAccountRepository(db)
		account, err := accountRepo.GetByID(c.Request.Context(), id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, account)
	}
}

// Helper functions

func getIntParam(c *gin.Context, key string, defaultValue int) int {
	if val := c.Query(key); val != "" {
		var result int
		if _, err := fmt.Sscanf(val, "%d", &result); err == nil && result > 0 {
			return result
		}
	}
	return defaultValue
}

func parseUUID(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}

func runBacktestHandler(backtestService *scoring.BacktestService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req scoring.BacktestRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Set defaults
		if req.SampleSize == 0 {
			req.SampleSize = 100
		}
		if req.StartDate.IsZero() {
			req.StartDate = time.Now().AddDate(0, 0, -30) // Last 30 days
		}
		if req.EndDate.IsZero() {
			req.EndDate = time.Now()
		}

		result, err := backtestService.RunBacktest(c.Request.Context(), &req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, result)
	}
}

// A/B Testing Handlers

func createExperimentHandler(abManager *scoring.ABTestManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Name         string   `json:"name" binding:"required"`
			Description  string   `json:"description"`
			ControlRules []string `json:"control_rules"`
			TestRules    []string `json:"test_rules"`
			TrafficSplit float64  `json:"traffic_split" binding:"required,min=0,max=1"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		exp := &scoring.Experiment{
			Name:         req.Name,
			Description:  req.Description,
			ControlRules: req.ControlRules,
			TestRules:    req.TestRules,
			TrafficSplit: req.TrafficSplit,
		}

		if err := abManager.CreateExperiment(exp); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusCreated, exp)
	}
}

func listExperimentsHandler(abManager *scoring.ABTestManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		experiments := abManager.GetAllExperiments()
		c.JSON(http.StatusOK, gin.H{"experiments": experiments})
	}
}

func getExperimentHandler(abManager *scoring.ABTestManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		experimentID := c.Param("id")

		exp, err := abManager.GetExperiment(experimentID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, exp)
	}
}

func startExperimentHandler(abManager *scoring.ABTestManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		experimentID := c.Param("id")

		if err := abManager.StartExperiment(experimentID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		exp, _ := abManager.GetExperiment(experimentID)
		c.JSON(http.StatusOK, gin.H{
			"message":    "Experiment started",
			"experiment": exp,
		})
	}
}

func stopExperimentHandler(abManager *scoring.ABTestManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		experimentID := c.Param("id")

		if err := abManager.StopExperiment(experimentID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		exp, _ := abManager.GetExperiment(experimentID)
		c.JSON(http.StatusOK, gin.H{
			"message":    "Experiment stopped",
			"experiment": exp,
		})
	}
}

func pauseExperimentHandler(abManager *scoring.ABTestManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		experimentID := c.Param("id")

		if err := abManager.PauseExperiment(experimentID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		exp, _ := abManager.GetExperiment(experimentID)
		c.JSON(http.StatusOK, gin.H{
			"message":    "Experiment paused",
			"experiment": exp,
		})
	}
}

func getExperimentResultsHandler(abManager *scoring.ABTestManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		experimentID := c.Param("id")

		results, err := abManager.GetResults(experimentID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, results)
	}
}

func getExperimentSignificanceHandler(abManager *scoring.ABTestManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		experimentID := c.Param("id")

		significance, err := abManager.GetStatisticalSignificance(experimentID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, significance)
	}
}

func deleteExperimentHandler(abManager *scoring.ABTestManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		experimentID := c.Param("id")

		if err := abManager.DeleteExperiment(experimentID); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Experiment deleted"})
	}
}
