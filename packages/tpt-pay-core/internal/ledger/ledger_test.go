package ledger_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/ledger"
)

// ── Unit tests: entry validation (no database required) ───────────────────────

func TestValidateEntries(t *testing.T) {
	tests := []struct {
		name    string
		entries []ledger.Entry
		wantErr error // nil = valid; sentinel matched with ErrorIs, otherwise any error
	}{
		{
			name: "balanced two entries",
			entries: []ledger.Entry{
				{AccountID: uuid.New(), Amount: 500, Currency: "USD"},
				{AccountID: uuid.New(), Amount: -500, Currency: "USD"},
			},
		},
		{
			name: "balanced three entries",
			entries: []ledger.Entry{
				{AccountID: uuid.New(), Amount: 1000, Currency: "USD"},
				{AccountID: uuid.New(), Amount: -900, Currency: "USD"},
				{AccountID: uuid.New(), Amount: -100, Currency: "USD"},
			},
		},
		{
			name: "unbalanced: sum non-zero",
			entries: []ledger.Entry{
				{AccountID: uuid.New(), Amount: 1000, Currency: "USD"},
				{AccountID: uuid.New(), Amount: -999, Currency: "USD"},
			},
			wantErr: ledger.ErrUnbalanced,
		},
		{
			name: "all same sign: unbalanced",
			entries: []ledger.Entry{
				{AccountID: uuid.New(), Amount: 100, Currency: "USD"},
				{AccountID: uuid.New(), Amount: 200, Currency: "USD"},
			},
			wantErr: ledger.ErrUnbalanced,
		},
		{
			name: "single entry",
			entries: []ledger.Entry{
				{AccountID: uuid.New(), Amount: 100, Currency: "USD"},
			},
			wantErr: errAny,
		},
		{
			name: "zero-amount entry rejected",
			entries: []ledger.Entry{
				{AccountID: uuid.New(), Amount: 0, Currency: "USD"},
				{AccountID: uuid.New(), Amount: 0, Currency: "USD"},
			},
			wantErr: errAny,
		},
		{
			name: "mixed currencies rejected",
			entries: []ledger.Entry{
				{AccountID: uuid.New(), Amount: 500, Currency: "USD"},
				{AccountID: uuid.New(), Amount: -500, Currency: "EUR"},
			},
			wantErr: ledger.ErrCurrencyMismatch,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ledger.ValidateEntries(tc.entries)
			switch {
			case tc.wantErr == nil:
				require.NoError(t, err)
			case errors.Is(tc.wantErr, errAny):
				require.Error(t, err)
			default:
				require.ErrorIs(t, err, tc.wantErr)
			}
		})
	}
}

// errAny is a sentinel meaning "any non-nil error" in the table above.
var errAny = assert.AnError

func TestEntry_AmountIsNeverFloat(t *testing.T) {
	// Compile-time check: Entry.Amount is int64, not float64.
	var e ledger.Entry
	var _ int64 = e.Amount //nolint:staticcheck // QF1011: explicit type is the point; fails to compile if Amount is float64
	_ = e
}
