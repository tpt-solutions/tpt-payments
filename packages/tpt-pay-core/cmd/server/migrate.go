package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/config"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/migrate"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/migrations"
)

// runMigrate is `tpt-pay-server migrate`: it applies the embedded schema migrations to the database
// in DATABASE_URL and exits. The Helm chart runs it as a Job before the Deployment is upgraded. It
// needs only the database configuration, so it works without the other services' settings.
func runMigrate() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	setupLogger(cfg.App.LogLevel)

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.Database.URL)
	if err != nil {
		return fmt.Errorf("create db pool: %w", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}

	applied, err := migrate.Up(ctx, pool, migrations.FS)
	if err != nil {
		return err
	}
	slog.Info("migrations complete", "applied", len(applied), "versions", applied)
	return nil
}
