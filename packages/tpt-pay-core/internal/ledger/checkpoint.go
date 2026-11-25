package ledger

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ErrCheckpointMismatch is returned when a period already has a recorded checkpoint with a
// different Merkle root. The ledger for that period has changed since the first export, which
// must be investigated; the recorded root is never overwritten.
var ErrCheckpointMismatch = errors.New("ledger: a recorded checkpoint for this period has a different Merkle root; the ledger changed after the period was closed")

// RecordCheckpoint stores the pack's period and root. Recording the same root again is a no-op.
// A different root for an already-recorded period returns ErrCheckpointMismatch.
func (l *Ledger) RecordCheckpoint(ctx context.Context, pack AuditPack) error {
	if err := pack.Verify(); err != nil {
		return fmt.Errorf("refusing to record an invalid pack: %w", err)
	}
	if _, err := l.pool.Exec(ctx, `
		INSERT INTO ledger_checkpoints (period_start, period_end, merkle_root, transaction_count)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (period_start, period_end) DO NOTHING
	`, pack.PeriodStart, pack.PeriodEnd, pack.MerkleRoot, pack.TransactionCount); err != nil {
		return fmt.Errorf("record checkpoint: %w", err)
	}

	var recorded string
	err := l.pool.QueryRow(ctx, `
		SELECT merkle_root FROM ledger_checkpoints WHERE period_start = $1 AND period_end = $2
	`, pack.PeriodStart, pack.PeriodEnd).Scan(&recorded)
	if err != nil {
		return fmt.Errorf("read checkpoint: %w", err)
	}
	if recorded != pack.MerkleRoot {
		return ErrCheckpointMismatch
	}
	return nil
}

// Checkpoint is a recorded period root.
type Checkpoint struct {
	PeriodStart      string `json:"period_start"`
	PeriodEnd        string `json:"period_end"`
	MerkleRoot       string `json:"merkle_root"`
	TransactionCount int    `json:"transaction_count"`
}

// GetCheckpoint returns the recorded checkpoint for a period, or pgx.ErrNoRows wrapped in an error
// when none exists.
func (l *Ledger) GetCheckpoint(ctx context.Context, periodStart, periodEnd string) (Checkpoint, error) {
	var c Checkpoint
	err := l.pool.QueryRow(ctx, `
		SELECT to_char(period_start AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		       to_char(period_end AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		       merkle_root, transaction_count
		FROM ledger_checkpoints
		WHERE period_start = $1::timestamptz AND period_end = $2::timestamptz
	`, periodStart, periodEnd).Scan(&c.PeriodStart, &c.PeriodEnd, &c.MerkleRoot, &c.TransactionCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrNoCheckpoint
	}
	return c, err
}

// ErrNoCheckpoint is returned by GetCheckpoint when the period has not been recorded.
var ErrNoCheckpoint = errors.New("ledger: no checkpoint is recorded for that period")
