// Command auditpack exports a ledger audit pack for a period as JSON.
//
// The pack lists every ledger transaction created in [--from, --to), with its entries and leaf
// hash, and the Merkle root over them. The root is printed to stderr. Store or publish that root
// somewhere the operator does not control to anchor the period; a pack can then be checked
// offline with `auditpack -verify <file>`.
//
// Usage:
//
//	DATABASE_URL=... auditpack --from 2026-01-01T00:00:00Z --to 2026-02-01T00:00:00Z --out pack.json
//	auditpack --verify pack.json
//
// With --record, the period's root is also stored in ledger_checkpoints. A later export of the
// same period must give the same root; a different one fails, because the ledger has changed.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/ledger"
)

func main() {
	if err := run(); err != nil {
		slog.Error("auditpack failed", "err", err)
		os.Exit(1)
	}
}

func run() error {
	from := flag.String("from", "", "period start, RFC 3339 (inclusive)")
	to := flag.String("to", "", "period end, RFC 3339 (exclusive)")
	out := flag.String("out", "", "output file (default stdout)")
	verify := flag.String("verify", "", "verify an existing pack file instead of exporting")
	record := flag.Bool("record", false, "also store the period's root in ledger_checkpoints (refuses a changed root)")
	flag.Parse()

	if *verify != "" {
		return verifyFile(*verify)
	}

	start, err := time.Parse(time.RFC3339, *from)
	if err != nil {
		return fmt.Errorf("--from: %w", err)
	}
	end, err := time.Parse(time.RFC3339, *to)
	if err != nil {
		return fmt.Errorf("--to: %w", err)
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

	lgr := ledger.New(pool)
	pack, err := lgr.BuildAuditPack(ctx, start, end)
	if err != nil {
		return err
	}
	if *record {
		if err := lgr.RecordCheckpoint(ctx, pack); err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "checkpoint recorded")
	}

	w := io.Writer(os.Stdout)
	if *out != "" {
		f, err := os.OpenFile(*out, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			return fmt.Errorf("open output: %w", err)
		}
		defer func() { _ = f.Close() }()
		w = f
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(pack); err != nil {
		return fmt.Errorf("write pack: %w", err)
	}
	fmt.Fprintf(os.Stderr, "transactions=%d merkle_root=%s\n", pack.TransactionCount, pack.MerkleRoot)
	return nil
}

func verifyFile(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var pack ledger.AuditPack
	if err := json.Unmarshal(raw, &pack); err != nil {
		return fmt.Errorf("parse pack: %w", err)
	}
	if err := pack.Verify(); err != nil {
		return fmt.Errorf("pack does not verify: %w", err)
	}
	fmt.Printf("ok: %d transactions, merkle_root=%s\n", pack.TransactionCount, pack.MerkleRoot)
	return nil
}
