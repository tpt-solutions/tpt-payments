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
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/api"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/adapter"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/adapter/sandbox"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/apikey"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/clock"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/config"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/ledger"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/middleware"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/payment"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/ratelimit"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/secretbox"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/tracing"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/vault"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/webhook"
)

var (
	version = "dev"
	commit  = "none"
)

func main() {
	var err error
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		err = runMigrate()
	} else {
		err = run()
	}
	if err != nil {
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
	// The sandbox runs on a test clock, which test keys can move forward (see /v1/test_clocks).
	// It starts at the wall-clock time and is not persisted.
	sandboxClock := clock.NewTest(time.Now().UTC())
	adpt := sandbox.New(sandbox.Config{
		ArtificialLatency: 50 * time.Millisecond,
		ChaosRate:         cfg.Adapter.ChaosRate,
		ChaosSeed:         cfg.Adapter.ChaosSeed,
		Clock:             sandboxClock,
	})
	if cfg.Adapter.Type != "sandbox" {
		return fmt.Errorf("adapter type %q not yet implemented — use 'sandbox'", cfg.Adapter.Type)
	}
	slog.Info("adapter loaded", "type", cfg.Adapter.Type)

	// Tracing is exported only when an OTLP endpoint is configured; spans are created either way.
	tp, err := tracing.New(ctx, "tpt-pay-core", version)
	if err != nil {
		return fmt.Errorf("init tracing: %w", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := tp.Shutdown(shutdownCtx); err != nil {
			slog.Warn("tracing shutdown incomplete", "err", err)
		}
	}()
	otel.SetTracerProvider(tp)
	if tp.Exporting() {
		slog.Info("tracing enabled: exporting spans over OTLP")
	}
	tracedAdpt := adapter.Traced{Inner: adpt, Tracer: tp.Tracer("tpt-pay-core/adapter")}

	paymentSvc := payment.NewService(pool, tracedAdpt, lgr)

	// Compare the ledger with payment intents and refunds. Discrepancies are reported, never
	// repaired automatically: a person decides what the correct record is.
	go lgr.RunReconcileJob(ctx, cfg.Ledger.ReconcileInterval, func(r ledger.ReconcileReport, err error) {
		switch {
		case err != nil:
			slog.Error("ledger reconciliation could not run", "err", err)
		case !r.Healthy():
			slog.Error("LEDGER RECONCILIATION FAILURE", "discrepancies", len(r.Discrepancies), "first", r.Discrepancies[0])
		default:
			slog.Debug("ledger reconciliation passed")
		}
	})

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
	secrets, err := loadSecretBox(ctx, cfg)
	if err != nil {
		return err
	}

	if err := loadPolicy(cfg.Policy, paymentSvc); err != nil {
		return err
	}

	limiter, err := buildRateLimiter(cfg.RateLimit, cfg.Redis.URL)
	if err != nil {
		return err
	}

	// Outside the router so that every request, including the operational endpoints, sees the
	// client address the trusted-proxy rules produce.
	router := middleware.TrustedProxyRealIP(cfg.Server.TrustedProxies)(
		api.NewRouter(pool, paymentSvc, apiKeySvc, secrets, metricsRegistry, limiter,
			api.WithTestClocks(sandboxClock, adpt)))

	// Webhook delivery worker: drains the outbox written by payment state changes.
	deliveries := webhook.NewWorker(pool, secrets, webhook.WorkerConfig{
		MaxAttempts: cfg.Webhook.MaxAttempts,
		Window:      cfg.Webhook.RetryWindow,
		BatchSize:   cfg.Webhook.WorkerConcurrency * 2,
	})
	if cfg.NATS.URL != "" {
		if err := startJetStreamDelivery(ctx, cfg, deliveries); err != nil {
			return err
		}
	} else {
		slog.Warn("NATS_URL is not set; webhook deliveries are polled from the database")
		go deliveries.Run(ctx, 2*time.Second)
	}

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

// startJetStreamDelivery connects to NATS, makes sure the webhook stream exists, and starts the
// relay (outbox to stream) and the consumer (stream to HTTP). Both stop when ctx is cancelled.
func startJetStreamDelivery(ctx context.Context, cfg *config.Config, deliveries *webhook.Worker) error {
	opts := []nats.Option{nats.Name("tpt-pay-core"), nats.MaxReconnects(-1)}
	if cfg.NATS.CredsFile != "" {
		opts = append(opts, nats.UserCredentials(cfg.NATS.CredsFile))
	}
	nc, err := nats.Connect(cfg.NATS.URL, opts...)
	if err != nil {
		return fmt.Errorf("connect to NATS: %w", err)
	}
	go func() {
		<-ctx.Done()
		nc.Drain() //nolint:errcheck // shutting down
	}()

	js, err := jetstream.New(nc)
	if err != nil {
		return fmt.Errorf("create JetStream context: %w", err)
	}
	if err := webhook.EnsureStream(ctx, js, cfg.Webhook.MaxAttempts); err != nil {
		return err
	}

	relay := webhook.NewRelay(deliveries, js, 100)
	go relay.Run(ctx, time.Second)

	consumer, err := webhook.NewConsumer(ctx, deliveries, js, cfg.Webhook.WorkerConcurrency*2)
	if err != nil {
		return err
	}
	go consumer.Run(ctx)
	slog.Info("webhook delivery uses JetStream", "stream", webhook.StreamName)
	return nil
}

// loadSecretBox builds the box that encrypts webhook signing secrets. With a Vault Transit
// ciphertext configured, the base64 key is unwrapped by Vault at startup and never read from the
// environment in plaintext. Otherwise WEBHOOK_SECRET_KEY is used (development only).
func loadSecretBox(ctx context.Context, cfg *config.Config) (*secretbox.Box, error) {
	if cfg.Vault.WrappedWebhookKey == "" {
		box, err := secretbox.New(cfg.Webhook.SecretKey)
		if err != nil {
			return nil, fmt.Errorf("WEBHOOK_SECRET_KEY: %w (generate with: openssl rand -base64 32)", err)
		}
		return box, nil
	}
	client, err := vault.NewTransit(cfg.Vault.Addr, cfg.Vault.Token, cfg.Vault.TransitMount, cfg.Vault.TransitKey)
	if err != nil {
		return nil, fmt.Errorf("VAULT settings: %w", err)
	}
	keyB64, err := client.Decrypt(ctx, cfg.Vault.WrappedWebhookKey)
	if err != nil {
		return nil, fmt.Errorf("unwrap webhook key: %w", err)
	}
	box, err := secretbox.New(string(keyB64))
	if err != nil {
		return nil, fmt.Errorf("unwrapped webhook key is invalid: %w", err)
	}
	slog.Info("webhook secret key unwrapped by Vault Transit", "key", cfg.Vault.TransitKey)
	return box, nil
}
