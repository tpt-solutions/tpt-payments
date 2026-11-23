package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Server    ServerConfig
	Database  DatabaseConfig
	NATS      NATSConfig
	Vault     VaultConfig
	Redis     RedisConfig
	RateLimit RateLimitConfig
	Adapter   AdapterConfig
	App       AppConfig
	Webhook   WebhookConfig
	Ledger    LedgerConfig
}

// LedgerConfig controls the scheduled ledger integrity check.
type LedgerConfig struct {
	// IntegrityInterval is how often every transaction is re-checked for balance.
	IntegrityInterval time.Duration
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

// NATSConfig is provisioned for the planned NATS JetStream transport. Nothing uses it yet,
// so it is optional.
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

// RateLimitConfig controls per-key request limits. See internal/middleware/ratelimit.go.
type RateLimitConfig struct {
	// Backend is "redis" (shared by all replicas; the default) or "memory" (one process only).
	Backend string
	// PerMinute is how many requests each API key may make per minute. 0 turns limiting off.
	PerMinute int
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
	// SecretKey encrypts endpoint signing secrets at rest: 32 random bytes, base64.
	SecretKey string
}

// devDatabasePassword is the credential shipped in .env.example and docker-compose.
const devDatabasePassword = "tptpay:tptpay@"

// devVaultToken is the Vault dev-mode root token shipped in docker-compose.
const devVaultToken = "dev-root-token"

// Load reads configuration from the environment. Every invalid value is reported, not just
// the first, so one run shows everything to fix.
func Load() (*Config, error) {
	var errs []error
	p := parser{errs: &errs}

	cfg := &Config{}
	cfg.Server = ServerConfig{
		Addr:         getEnv("SERVER_ADDR", ":8080"),
		ReadTimeout:  p.duration("SERVER_READ_TIMEOUT", "30s"),
		WriteTimeout: p.duration("SERVER_WRITE_TIMEOUT", "30s"),
		IdleTimeout:  p.duration("SERVER_IDLE_TIMEOUT", "120s"),
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}
	cfg.Database = DatabaseConfig{
		URL:      dbURL,
		MaxConns: int32(p.integer("DATABASE_MAX_CONNS", "25")),
		MinConns: int32(p.integer("DATABASE_MIN_CONNS", "5")),
	}
	if cfg.Database.MinConns < 0 || cfg.Database.MaxConns <= 0 || cfg.Database.MinConns > cfg.Database.MaxConns {
		errs = append(errs, fmt.Errorf("DATABASE_MIN_CONNS (%d) must be between 0 and DATABASE_MAX_CONNS (%d)",
			cfg.Database.MinConns, cfg.Database.MaxConns))
	}

	cfg.NATS = NATSConfig{URL: os.Getenv("NATS_URL"), CredsFile: os.Getenv("NATS_CREDS_FILE")}

	cfg.Vault = VaultConfig{
		Addr:     getEnv("VAULT_ADDR", "http://localhost:8200"),
		Token:    os.Getenv("VAULT_TOKEN"),
		RoleID:   os.Getenv("VAULT_ROLE_ID"),
		SecretID: os.Getenv("VAULT_SECRET_ID"),
	}

	cfg.Redis = RedisConfig{URL: getEnv("REDIS_URL", "redis://localhost:6379/0")}
	cfg.RateLimit = RateLimitConfig{
		Backend:   getEnv("RATE_LIMIT_BACKEND", "redis"),
		PerMinute: p.integer("API_RATE_LIMIT", "100"),
	}
	if cfg.RateLimit.Backend != "redis" && cfg.RateLimit.Backend != "memory" {
		errs = append(errs, errors.New("RATE_LIMIT_BACKEND must be redis or memory"))
	}
	if cfg.RateLimit.PerMinute < 0 {
		errs = append(errs, errors.New("API_RATE_LIMIT must be 0 or more"))
	}
	cfg.Adapter = AdapterConfig{Type: getEnv("ADAPTER_TYPE", "sandbox")}
	cfg.App = AppConfig{
		Env:      getEnv("APP_ENV", "development"),
		LogLevel: getEnv("LOG_LEVEL", "info"),
	}

	cfg.Ledger = LedgerConfig{
		IntegrityInterval: p.duration("LEDGER_INTEGRITY_INTERVAL", "1h"),
	}
	if cfg.Ledger.IntegrityInterval <= 0 {
		errs = append(errs, errors.New("LEDGER_INTEGRITY_INTERVAL must be positive"))
	}

	cfg.Webhook = WebhookConfig{
		WorkerConcurrency: p.integer("WEBHOOK_WORKER_CONCURRENCY", "10"),
		MaxAttempts:       p.integer("WEBHOOK_MAX_ATTEMPTS", "15"),
		RetryWindow:       p.duration("WEBHOOK_RETRY_WINDOW", "72h"),
		SecretKey:         os.Getenv("WEBHOOK_SECRET_KEY"),
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return cfg, nil
}

// ValidateProduction refuses configurations that are unsafe in production: development
// credentials, or the sandbox card adapter. It returns nil outside production.
func ValidateProduction(cfg *Config) error {
	if cfg.App.Env != "production" {
		return nil
	}
	var problems []string
	if cfg.Adapter.Type == "sandbox" {
		problems = append(problems, "ADAPTER_TYPE=sandbox is for testing only; production needs a real card adapter")
	}
	if strings.Contains(cfg.Database.URL, devDatabasePassword) {
		problems = append(problems, "DATABASE_URL uses the development credentials tptpay:tptpay")
	}
	if cfg.RateLimit.PerMinute > 0 && cfg.RateLimit.Backend == "memory" {
		problems = append(problems, "RATE_LIMIT_BACKEND=memory does not share limits between replicas; production needs redis")
	}
	if cfg.Vault.Token == devVaultToken {
		problems = append(problems, "VAULT_TOKEN is the development root token")
	}
	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("refusing to start in production: %s", strings.Join(problems, "; "))
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// parser collects conversion errors so Load can report them all at once.
type parser struct {
	errs *[]error
}

func (p parser) duration(key, fallback string) time.Duration {
	v := getEnv(key, fallback)
	d, err := time.ParseDuration(v)
	if err != nil {
		*p.errs = append(*p.errs, fmt.Errorf("invalid duration for %s=%q: %w", key, v, err))
	}
	return d
}

func (p parser) integer(key, fallback string) int {
	v := getEnv(key, fallback)
	n, err := strconv.Atoi(v)
	if err != nil {
		*p.errs = append(*p.errs, fmt.Errorf("invalid integer for %s=%q: %w", key, v, err))
	}
	return n
}
