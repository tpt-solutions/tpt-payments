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
	"github.com/prometheus/client_golang/prometheus"

	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/api"
	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/internal/adapter/sandbox"
	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/internal/apikey"
	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/internal/config"
	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/internal/ledger"
	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/internal/middleware"
	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/internal/payment"
	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/internal/secretbox"
	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/internal/webhook"
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

	router := api.NewRouter(pool, paymentSvc, apiKeySvc, secrets, metricsRegistry)

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
