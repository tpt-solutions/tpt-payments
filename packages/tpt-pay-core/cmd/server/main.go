package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/api"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/adapter/sandbox"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/apikey"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/config"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/ledger"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/middleware"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/payment"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/ratelimit"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/secretbox"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/webhook"
)

var (
	version = "dev"
	commit  = "none"
)

func main() {
	if err := run(); err != nil {
		slog.Error("server exited with error", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if err := config.ValidateProduction(cfg); err != nil {
		return err
	}

	setupLogger(cfg.App.LogLevel)
	slog.Info("starting tpt-pay-core", "version", version, "commit", commit, "env", cfg.App.Env)

	// ── Database ──────────────────────────────────────────────────────────────
	ctx := context.Background()
	poolCfg, err := pgxpool.ParseConfig(cfg.Database.URL)
	if err != nil {
		return fmt.Errorf("parse database url: %w", err)
	}
	poolCfg.MaxConns = cfg.Database.MaxConns
	poolCfg.MinConns = cfg.Database.MinConns

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return fmt.Errorf("create db pool: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}
	slog.Info("database connected")

	// ── Services ──────────────────────────────────────────────────────────────
	metricsRegistry := prometheus.NewRegistry()
	lgr := ledger.New(pool)
	apiKeySvc := apikey.NewService(pool)

	// Adapter selection: in production, swap sandbox for BankSponsorAdapter.
	var adpt = sandbox.Default()
	if cfg.Adapter.Type != "sandbox" {
		return fmt.Errorf("adapter type %q not yet implemented — use 'sandbox'", cfg.Adapter.Type)
	}
	slog.Info("adapter loaded", "type", cfg.Adapter.Type)

	paymentSvc := payment.NewService(pool, adpt, lgr)

	// Re-verify every ledger transaction on a schedule. A non-zero count is an incident.
	go lgr.RunIntegrityJob(ctx, cfg.Ledger.IntegrityInterval, func(unbalanced int, err error) {
		switch {
		case err != nil:
			slog.Error("ledger integrity check could not run", "err", err)
		case unbalanced > 0:
			slog.Error("LEDGER INTEGRITY FAILURE: unbalanced transactions found", "count", unbalanced)
		default:
			slog.Debug("ledger integrity check passed")
		}
	})

	// Idempotency records expire after 24 hours; purge them hourly.
	middleware.StartIdempotencyCleanup(ctx, pool, time.Hour, 24*time.Hour)

	// ── HTTP server ───────────────────────────────────────────────────────────
	secrets, err := secretbox.New(cfg.Webhook.SecretKey)
	if err != nil {
		return fmt.Errorf("WEBHOOK_SECRET_KEY: %w (generate with: openssl rand -base64 32)", err)
	}

	limiter, err := buildRateLimiter(cfg.RateLimit, cfg.Redis.URL)
	if err != nil {
		return err
	}

	router := api.NewRouter(pool, paymentSvc, apiKeySvc, secrets, metricsRegistry, limiter)

	// Webhook delivery worker: drains the outbox written by payment state changes.
	deliveries := webhook.NewWorker(pool, secrets, webhook.WorkerConfig{
		MaxAttempts: cfg.Webhook.MaxAttempts,
		Window:      cfg.Webhook.RetryWindow,
		BatchSize:   cfg.Webhook.WorkerConcurrency * 2,
	})
	go deliveries.Run(ctx, 2*time.Second)

	srv := &http.Server{
		Addr:         cfg.Server.Addr,
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	// Start server in background.
	serverErr := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", cfg.Server.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	// Wait for shutdown signal or server error.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		return fmt.Errorf("server error: %w", err)
	case sig := <-quit:
		slog.Info("shutting down", "signal", sig.String())
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}

	slog.Info("server stopped cleanly")
	return nil
}

func setupLogger(level string) {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})))
}

// buildRateLimiter returns the per-key limiter, or nil when limiting is off. The redis backend
// fails startup if Redis is unreachable, so a replica never runs without shared limits by accident.
func buildRateLimiter(cfg config.RateLimitConfig, redisURL string) (ratelimit.Limiter, error) {
	const window = time.Minute
	if cfg.PerMinute == 0 {
		slog.Warn("API rate limiting is off (API_RATE_LIMIT=0)")
		return nil, nil
	}
	if cfg.Backend == "memory" {
		slog.Warn("rate limiting uses memory; limits are not shared between replicas", "per_minute", cfg.PerMinute)
		l, err := ratelimit.NewMemory(cfg.PerMinute, window)
		if err != nil {
			return nil, err
		}
		return l, nil
	}

	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("REDIS_URL: %w", err)
	}
	client := redis.NewClient(opts)
	pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis is unreachable for rate limiting: %w (for single-node development set RATE_LIMIT_BACKEND=memory)", err)
	}
	l, err := ratelimit.NewRedis(client, cfg.PerMinute, window)
	if err != nil {
		_ = client.Close()
		return nil, err
	}
	slog.Info("rate limiting on", "backend", "redis", "per_minute", cfg.PerMinute)
	return l, nil
}
