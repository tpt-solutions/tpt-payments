package ledger

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// AuditPackVersion is the format version of AuditPack. It changes only when the committed
// fields or the hashing rules change.
const AuditPackVersion = 1

// SettlementGrace is how long a period must have ended before BuildAuditPack will export it.
// A transaction's created_at is its start time, so a transaction that started inside the
// period can commit after the period ends. Waiting this long makes the export, and so the
// Merkle root, stable for a given period.
const SettlementGrace = time.Minute

// ErrPeriodNotSettled is returned when the requested period has not ended long enough ago.
var ErrPeriodNotSettled = errors.New("audit period has not settled yet; wait before exporting it")

// AuditPack is a self-contained, exportable record of the ledger for [PeriodStart, PeriodEnd).
// Everything an auditor needs to recompute the Merkle root is in the pack; Verify does so.
// Entry descriptions are left out on purpose: they are free text and not committed.
type AuditPack struct {
	Version          int               `json:"version"`
	GeneratedAt      time.Time         `json:"generated_at"`
	PeriodStart      time.Time         `json:"period_start"`
	PeriodEnd        time.Time         `json:"period_end"`
	TransactionCount int               `json:"transaction_count"`
	MerkleRoot       string            `json:"merkle_root"` // hex SHA-256 over transaction leaves in order
	Transactions     []PackTransaction `json:"transactions"`
}

// PackTransaction is one transaction in an audit pack, with its leaf hash.
type PackTransaction struct {
	ID          string      `json:"id"`
	Type        string      `json:"type"`
	Status      string      `json:"status"`
	ReferenceID string      `json:"reference_id"`
	CreatedAt   time.Time   `json:"created_at"`
	LeafHash    string      `json:"leaf_hash"`
	Entries     []PackEntry `json:"entries"`
}

// PackEntry is one ledger entry in an audit pack.
type PackEntry struct {
	AccountID string `json:"account_id"`
	Amount    int64  `json:"amount"`
	Currency  string `json:"currency"`
}

// NewAuditPack builds a pack from records already read from the ledger, in the order given.
// Records must be sorted by (CreatedAt, ID), the order BuildAuditPack reads them in, so that
// the root is reproducible. It does not touch the database.
func NewAuditPack(records []Record, from, to, generatedAt time.Time) AuditPack {
	leaves := make([]Hash, 0, len(records))
	txns := make([]PackTransaction, 0, len(records))
	for _, r := range records {
		leaf := LeafHash(r)
		leaves = append(leaves, leaf)
		pt := PackTransaction{
			ID:          r.ID.String(),
			Type:        r.Type,
			Status:      r.Status,
			ReferenceID: r.ReferenceID,
			CreatedAt:   r.CreatedAt.UTC(),
			LeafHash:    hex.EncodeToString(leaf[:]),
			Entries:     make([]PackEntry, 0, len(r.Entries)),
		}
		for _, e := range r.Entries {
			pt.Entries = append(pt.Entries, PackEntry{
				AccountID: e.AccountID.String(),
				Amount:    e.Amount,
				Currency:  e.Currency,
			})
		}
		txns = append(txns, pt)
	}
	root := MerkleRoot(leaves)
	return AuditPack{
		Version:          AuditPackVersion,
		GeneratedAt:      generatedAt.UTC(),
		PeriodStart:      from.UTC(),
		PeriodEnd:        to.UTC(),
		TransactionCount: len(txns),
		MerkleRoot:       hex.EncodeToString(root[:]),
		Transactions:     txns,
	}
}

// Verify recomputes every leaf and the Merkle root from the pack's contents and reports the
// first mismatch. A pack that passes has not been altered since it was built, provided the
// root was published or stored elsewhere when it was produced.
func (p AuditPack) Verify() error {
	if p.Version != AuditPackVersion {
		return fmt.Errorf("unsupported audit pack version %d", p.Version)
	}
	if p.TransactionCount != len(p.Transactions) {
		return fmt.Errorf("transaction_count %d does not match %d transactions",
			p.TransactionCount, len(p.Transactions))
	}
	leaves := make([]Hash, 0, len(p.Transactions))
	for i, t := range p.Transactions {
		r, err := t.record()
		if err != nil {
			return fmt.Errorf("transaction %d: %w", i, err)
		}
		leaf := LeafHash(r)
		if hex.EncodeToString(leaf[:]) != t.LeafHash {
			return fmt.Errorf("transaction %d (%s): leaf hash mismatch", i, t.ID)
		}
		leaves = append(leaves, leaf)
	}
	root := MerkleRoot(leaves)
	if hex.EncodeToString(root[:]) != p.MerkleRoot {
		return errors.New("merkle root mismatch")
	}
	return nil
}

func (t PackTransaction) record() (Record, error) {
	id, err := uuid.Parse(t.ID)
	if err != nil {
		return Record{}, fmt.Errorf("invalid id: %w", err)
	}
	r := Record{
		ID:          id,
		Type:        t.Type,
		Status:      t.Status,
		ReferenceID: t.ReferenceID,
		CreatedAt:   t.CreatedAt,
		Entries:     make([]RecordEntry, 0, len(t.Entries)),
	}
	for _, e := range t.Entries {
		acct, err := uuid.Parse(e.AccountID)
		if err != nil {
			return Record{}, fmt.Errorf("invalid account id: %w", err)
		}
		r.Entries = append(r.Entries, RecordEntry{AccountID: acct, Amount: e.Amount, Currency: e.Currency})
	}
	return r, nil
}

// BuildAuditPack reads every transaction created in [from, to) and returns its audit pack.
// The read is one repeatable-read snapshot, so the transactions and their entries agree.
// The period must have ended at least SettlementGrace before now.
func (l *Ledger) BuildAuditPack(ctx context.Context, from, to time.Time) (AuditPack, error) {
	now := time.Now().UTC()
	if !from.Before(to) {
		return AuditPack{}, errors.New("period start must be before period end")
	}
	if now.Sub(to) < SettlementGrace {
		return AuditPack{}, ErrPeriodNotSettled
	}

	var records []Record
	err := pgx.BeginTxFunc(ctx, l.pool, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	}, func(tx pgx.Tx) error {
		records = records[:0]
		rows, err := tx.Query(ctx, `
			SELECT id, type, status, COALESCE(reference_id, ''), created_at
			FROM ledger_transactions
			WHERE created_at >= $1 AND created_at < $2
			ORDER BY created_at, id
		`, from, to)
		if err != nil {
			return fmt.Errorf("query transactions: %w", err)
		}
		index := map[uuid.UUID]int{}
		for rows.Next() {
			var r Record
			if err := rows.Scan(&r.ID, &r.Type, &r.Status, &r.ReferenceID, &r.CreatedAt); err != nil {
				rows.Close()
				return fmt.Errorf("scan transaction: %w", err)
			}
			index[r.ID] = len(records)
			records = append(records, r)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return fmt.Errorf("read transactions: %w", err)
		}

		erows, err := tx.Query(ctx, `
			SELECT le.transaction_id, le.account_id, le.amount, le.currency
			FROM ledger_entries le
			JOIN ledger_transactions lt ON lt.id = le.transaction_id
			WHERE lt.created_at >= $1 AND lt.created_at < $2
			ORDER BY le.transaction_id, le.id
		`, from, to)
		if err != nil {
			return fmt.Errorf("query entries: %w", err)
		}
		defer erows.Close()
		for erows.Next() {
			var (
				txnID uuid.UUID
				e     RecordEntry
			)
			if err := erows.Scan(&txnID, &e.AccountID, &e.Amount, &e.Currency); err != nil {
				return fmt.Errorf("scan entry: %w", err)
			}
			i, ok := index[txnID]
			if !ok {
				return fmt.Errorf("entry for transaction %s outside the snapshot", txnID)
			}
			records[i].Entries = append(records[i].Entries, e)
		}
		return erows.Err()
	})
	if err != nil {
		return AuditPack{}, err
	}
	return NewAuditPack(records, from, to, now), nil
}
