// Command bootstrap creates a merchant and its first API keys. It is the supported way to
// obtain the first key on a fresh installation, because the API itself requires a key.
//
// Usage:
//
//	DATABASE_URL=postgres://... go run ./cmd/bootstrap --email you@example.com --name "My Shop"
//
// Keys are printed once. Store them in a secrets manager; they cannot be shown again.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/apikey"
)

func main() {
	if err := run(); err != nil {
		slog.Error("bootstrap failed", "err", err)
		os.Exit(1)
	}
}

func run() error {
	email := flag.String("email", "", "merchant email (required; identifies the merchant on re-runs)")
	name := flag.String("name", "", "merchant display name (required)")
	live := flag.Bool("live", false, "also create a live-mode secret key")
	flag.Parse()

	if *email == "" || *name == "" {
		return fmt.Errorf("--email and --name are required")
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return fmt.Errorf("DATABASE_URL must be set")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer pool.Close()

	// Idempotent by email: re-running updates the name and adds keys.
	var merchantID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO merchants (name, email, status, kyc_status)
		VALUES ($1, $2, 'active', 'not_started')
		ON CONFLICT (email) DO UPDATE SET name = EXCLUDED.name
		RETURNING id
	`, *name, strings.ToLower(strings.TrimSpace(*email))).Scan(&merchantID); err != nil {
		return fmt.Errorf("create merchant: %w", err)
	}

	keys := apikey.NewService(pool)
	type want struct {
		typ  apikey.KeyType
		mode apikey.Mode
	}
	wanted := []want{
		{apikey.TypeSecret, apikey.ModeTest},
		{apikey.TypePublishable, apikey.ModeTest},
	}
	if *live {
		wanted = append(wanted,
			want{apikey.TypeSecret, apikey.ModeLive},
			want{apikey.TypePublishable, apikey.ModeLive},
		)
	}

	fmt.Printf("merchant_id: %s\n", merchantID)
	for _, w := range wanted {
		res, err := keys.Generate(ctx, merchantID, "bootstrap", w.typ, w.mode)
		if err != nil {
			return fmt.Errorf("create %s %s key: %w", w.mode, w.typ, err)
		}
		fmt.Printf("%s %s key (shown once): %s\n", w.mode, w.typ, res.RawKey)
	}
	return nil
}
