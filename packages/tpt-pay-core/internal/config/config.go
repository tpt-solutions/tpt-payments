package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	NATS     NATSConfig
	Vault    VaultConfig
	Redis    RedisConfig
	Adapter  AdapterConfig
	App      AppConfig
	Webhook  WebhookConfig
}

type ServerConfig struct {
	Addr         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}

type DatabaseConfig struct {
	URL      string
	MaxConns int32
	MinConns int32
}

type NATSConfig struct {
	URL       string
	CredsFile string
}

type VaultConfig struct {
	Addr     string
	Token    string
	RoleID   string
	SecretID string
}

type RedisConfig struct {
	URL string
}

type AdapterConfig struct {
	Type string // "sandbox" | "bank_sponsor" | "direct_acquiring"
}

type AppConfig struct {
	Env      string // "development" | "staging" | "production"
	LogLevel string // "debug" | "info" | "warn" | "error"
}

type WebhookConfig struct {
	WorkerConcurrency int
	MaxAttempts       int
	RetryWindow       time.Duration
}

func Load() (*Config, error) {
	cfg := &Config{}

	cfg.Server = ServerConfig{
		Addr:         getEnv("SERVER_ADDR", ":8080"),
		ReadTimeout:  mustParseDuration("SERVER_READ_TIMEOUT", "30s"),
		WriteTimeout: mustParseDuration("SERVER_WRITE_TIMEOUT", "30s"),
		IdleTimeout:  mustParseDuration("SERVER_IDLE_TIMEOUT", "120s"),
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	cfg.Database = DatabaseConfig{
		URL:      dbURL,
		MaxConns: int32(mustParseInt("DATABASE_MAX_CONNS", "25")),
		MinConns: int32(mustParseInt("DATABASE_MIN_CONNS", "5")),
	}

	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		return nil, fmt.Errorf("NATS_URL is required")
	}
	cfg.NATS = NATSConfig{
		URL:       natsURL,
		CredsFile: os.Getenv("NATS_CREDS_FILE"),
	}

	cfg.Vault = VaultConfig{
		Addr:     getEnv("VAULT_ADDR", "http://localhost:8200"),
		Token:    os.Getenv("VAULT_TOKEN"),
		RoleID:   os.Getenv("VAULT_ROLE_ID"),
		SecretID: os.Getenv("VAULT_SECRET_ID"),
	}

	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379/0"
	}
	cfg.Redis = RedisConfig{URL: redisURL}

	cfg.Adapter = AdapterConfig{
		Type: getEnv("ADAPTER_TYPE", "sandbox"),
	}

	cfg.App = AppConfig{
		Env:      getEnv("APP_ENV", "development"),
		LogLevel: getEnv("LOG_LEVEL", "info"),
	}

	cfg.Webhook = WebhookConfig{
		WorkerConcurrency: mustParseInt("WEBHOOK_WORKER_CONCURRENCY", "10"),
		MaxAttempts:       mustParseInt("WEBHOOK_MAX_ATTEMPTS", "15"),
		RetryWindow:       mustParseDuration("WEBHOOK_RETRY_WINDOW", "72h"),
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func mustParseDuration(key, fallback string) time.Duration {
	v := getEnv(key, fallback)
	d, err := time.ParseDuration(v)
	if err != nil {
		panic(fmt.Sprintf("invalid duration for %s=%q: %v", key, v, err))
	}
	return d
}

func mustParseInt(key, fallback string) int {
	v := getEnv(key, fallback)
	n, err := strconv.Atoi(v)
	if err != nil {
		panic(fmt.Sprintf("invalid int for %s=%q: %v", key, v, err))
	}
	return n
}
