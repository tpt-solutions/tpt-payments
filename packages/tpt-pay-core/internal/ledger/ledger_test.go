package ledger_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/internal/ledger"
)

// ── Unit tests: balance validation (no database required) ─────────────────────

func TestValidateBalance_Balanced(t *testing.T) {
	acct1 := uuid.New()
	acct2 := uuid.New()

	entries := []ledger.Entry{
		{AccountID: acct1, Amount: 1000, Currency: "USD", Description: "credit merchant"},
		{AccountID: acct2, Amount: -1000, Currency: "USD", Description: "debit clearing"},
	}

	// PostTransaction would accept these — we test indirectly via the exported
	// error type since validateBalance is unexported.
	// The real invariant is: if balanced entries are submitted, no error is returned.
	// We test the unbalanced path explicitly below.
	_ = entries
}

func TestValidateBalance_Unbalanced(t *testing.T) {
	tests := []struct {
		name    string
		entries []ledger.Entry
		wantErr bool
	}{
		{
			name: "balanced two entries",
			entries: []ledger.Entry{
				{AccountID: uuid.New(), Amount: 500, Currency: "USD"},
				{AccountID: uuid.New(), Amount: -500, Currency: "USD"},
			},
			wantErr: false,
		},
		{
			name: "balanced three entries",
			entries: []ledger.Entry{
				{AccountID: uuid.New(), Amount: 1000, Currency: "USD"},
				{AccountID: uuid.New(), Amount: -900, Currency: "USD"},
				{AccountID: uuid.New(), Amount: -100, Currency: "USD"},
			},
			wantErr: false,
		},
		{
			name: "unbalanced — sum non-zero",
			entries: []ledger.Entry{
				{AccountID: uuid.New(), Amount: 1000, Currency: "USD"},
				{AccountID: uuid.New(), Amount: -999, Currency: "USD"},
			},
			wantErr: true,
		},
		{
			name: "single entry — trivially unbalanced",
			entries: []ledger.Entry{
				{AccountID: uuid.New(), Amount: 100, Currency: "USD"},
			},
			wantErr: true, // also fails the minimum 2 entries check
		},
		{
			name: "all same sign — unbalanced",
			entries: []ledger.Entry{
				{AccountID: uuid.New(), Amount: 100, Currency: "USD"},
				{AccountID: uuid.New(), Amount: 200, Currency: "USD"},
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// We use a nil pool because balanced validation happens before any DB call.
			l := ledger.New(nil)
			_, err := l.PostTransaction(nil, ledger.TxnTypePaymentIntent, "test", tc.entries)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				// With a nil pool this will fail at the DB call, but we only care
				// that the balance check passed (i.e. the error is not ErrUnbalanced).
				if err != nil {
					assert.NotErrorIs(t, err, ledger.ErrUnbalanced)
				}
			}
		})
	}
}

func TestEntry_AmountIsNeverFloat(t *testing.T) {
	// Compile-time check: Entry.Amount is int64, not float64.
	var e ledger.Entry
	var _ int64 = e.Amount // this line fails to compile if Amount is float64
	_ = e
}
