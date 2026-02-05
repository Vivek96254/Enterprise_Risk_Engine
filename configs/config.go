package configs

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Redis    RedisConfig
	JWT      JWTConfig
	Worker   WorkerConfig
}

type ServerConfig struct {
	Port         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	Environment  string
}

type DatabaseConfig struct {
	URL             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

type RedisConfig struct {
	URL          string
	StreamName   string
	ConsumerGroup string
	MaxRetries   int
}

type JWTConfig struct {
	Secret     string
	Expiration time.Duration
}

type WorkerConfig struct {
	Concurrency    int
	BatchSize      int
	PollInterval   time.Duration
	RetryAttempts  int
	DeadLetterStream string
}

func Load() *Config {
	return &Config{
		Server: ServerConfig{
			Port:         getEnv("PORT", "8080"),
			ReadTimeout:  getDurationEnv("SERVER_READ_TIMEOUT", 30*time.Second),
			WriteTimeout: getDurationEnv("SERVER_WRITE_TIMEOUT", 30*time.Second),
			Environment:  getEnv("ENVIRONMENT", "development"),
		},
		Database: DatabaseConfig{
			URL:             getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/risk_engine?sslmode=disable"),
			MaxOpenConns:    getIntEnv("DB_MAX_OPEN_CONNS", 25),
			MaxIdleConns:    getIntEnv("DB_MAX_IDLE_CONNS", 5),
			ConnMaxLifetime: getDurationEnv("DB_CONN_MAX_LIFETIME", 5*time.Minute),
		},
		Redis: RedisConfig{
			URL:           getEnv("REDIS_URL", "redis://localhost:6379"),
			StreamName:    getEnv("REDIS_STREAM_NAME", "transactions"),
			ConsumerGroup: getEnv("REDIS_CONSUMER_GROUP", "scoring-workers"),
			MaxRetries:    getIntEnv("REDIS_MAX_RETRIES", 3),
		},
		JWT: JWTConfig{
			Secret:     getEnv("JWT_SECRET", "your-super-secret-key-change-in-production"),
			Expiration: getDurationEnv("JWT_EXPIRATION", 24*time.Hour),
		},
		Worker: WorkerConfig{
			Concurrency:      getIntEnv("WORKER_CONCURRENCY", 5),
			BatchSize:        getIntEnv("WORKER_BATCH_SIZE", 100),
			PollInterval:     getDurationEnv("WORKER_POLL_INTERVAL", 100*time.Millisecond),
			RetryAttempts:    getIntEnv("WORKER_RETRY_ATTEMPTS", 3),
			DeadLetterStream: getEnv("DEAD_LETTER_STREAM", "transactions-dlq"),
		},
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getIntEnv(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getDurationEnv(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}
