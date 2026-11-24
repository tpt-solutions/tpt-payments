package ledger_test

// Merkle commitments and audit packs. These need no database.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/ledger"
)

var t0 = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

func sampleRecord(ref string, amount int64) ledger.Record {
	return ledger.Record{
		ID:          uuid.New(),
		Type:        "payment_intent",
		Status:      "posted",
		ReferenceID: ref,
		CreatedAt:   t0,
		Entries: []ledger.RecordEntry{
			{AccountID: uuid.New(), Amount: amount, Currency: "USD"},
			{AccountID: uuid.New(), Amount: -amount, Currency: "USD"},
		},
	}
}

func TestLeafHash_IgnoresEntryOrder(t *testing.T) {
	r := sampleRecord("pi_1", 500)
	reversed := r
	reversed.Entries = []ledger.RecordEntry{r.Entries[1], r.Entries[0]}
	assert.Equal(t, ledger.LeafHash(r), ledger.LeafHash(reversed))
}

func TestLeafHash_ChangesWithAnyCommittedField(t *testing.T) {
	base := sampleRecord("pi_1", 500)
	leaf := ledger.LeafHash(base)

	changed := []func(r *ledger.Record){
		func(r *ledger.Record) { r.Type = "refund" },
		func(r *ledger.Record) { r.Status = "reversed" },
		func(r *ledger.Record) { r.ReferenceID = "pi_2" },
		func(r *ledger.Record) { r.CreatedAt = r.CreatedAt.Add(time.Nanosecond) },
		func(r *ledger.Record) { r.Entries[0].Amount++; r.Entries[1].Amount-- },
		func(r *ledger.Record) { r.Entries[0].Currency = "EUR" },
		func(r *ledger.Record) { r.Entries[0].AccountID = uuid.New() },
	}
	for i, change := range changed {
		r := base
		r.Entries = append([]ledger.RecordEntry(nil), base.Entries...)
		change(&r)
		assert.NotEqual(t, leaf, ledger.LeafHash(r), "field change %d must change the leaf", i)
	}
}

func TestLeafHash_LengthPrefixPreventsFieldConfusion(t *testing.T) {
	// "ab" + "c" and "a" + "bc" must not hash the same.
	a := sampleRecord("", 1)
	a.Type, a.ReferenceID = "ab", "c"
	b := a
	b.Type, b.ReferenceID = "a", "bc"
	assert.NotEqual(t, ledger.LeafHash(a), ledger.LeafHash(b))
}

func TestMerkleRoot_EmptyAndSingle(t *testing.T) {
	empty := ledger.MerkleRoot(nil)
	assert.Equal(t, ledger.MerkleRoot([]ledger.Hash{}), empty, "nil and empty agree")

	leaf := ledger.LeafHash(sampleRecord("pi_1", 1))
	assert.Equal(t, leaf, ledger.MerkleRoot([]ledger.Hash{leaf}), "a single leaf is its own root")
}

func TestMerkleRoot_OrderMatters(t *testing.T) {
	a := ledger.LeafHash(sampleRecord("pi_a", 1))
	b := ledger.LeafHash(sampleRecord("pi_b", 2))
	assert.NotEqual(t, ledger.MerkleRoot([]ledger.Hash{a, b}), ledger.MerkleRoot([]ledger.Hash{b, a}))
}

func TestMerkleRoot_OddCountDoesNotDuplicateLastLeaf(t *testing.T) {
	a := ledger.LeafHash(sampleRecord("pi_a", 1))
	b := ledger.LeafHash(sampleRecord("pi_b", 2))
	c := ledger.LeafHash(sampleRecord("pi_c", 3))
	// With duplication, [a,b,c] and [a,b,c,c] would share a root. They must not.
	assert.NotEqual(t, ledger.MerkleRoot([]ledger.Hash{a, b, c}),
		ledger.MerkleRoot([]ledger.Hash{a, b, c, c}))
}

func TestNewAuditPack_VerifiesAndDetectsTampering(t *testing.T) {
	records := []ledger.Record{sampleRecord("pi_1", 100), sampleRecord("pi_2", 250), sampleRecord("pi_3", 75)}
	pack := ledger.NewAuditPack(records, t0.Add(-time.Hour), t0.Add(time.Hour), t0.Add(2*time.Hour))

	require.NoError(t, pack.Verify())
	assert.Equal(t, 3, pack.TransactionCount)
	assert.Equal(t, ledger.AuditPackVersion, pack.Version)

	t.Run("amount changed", func(t *testing.T) {
		p := clonePack(t, pack)
		p.Transactions[1].Entries[0].Amount++
		assert.Error(t, p.Verify())
	})
	t.Run("transaction dropped", func(t *testing.T) {
		p := clonePack(t, pack)
		p.Transactions = p.Transactions[:2]
		p.TransactionCount = 2
		assert.Error(t, p.Verify())
	})
	t.Run("order swapped", func(t *testing.T) {
		p := clonePack(t, pack)
		p.Transactions[0], p.Transactions[1] = p.Transactions[1], p.Transactions[0]
		assert.Error(t, p.Verify())
	})
	t.Run("root replaced", func(t *testing.T) {
		p := clonePack(t, pack)
		p.MerkleRoot = "00" + p.MerkleRoot[2:]
		assert.Error(t, p.Verify())
	})
}

func TestNewAuditPack_EmptyPeriodVerifies(t *testing.T) {
	pack := ledger.NewAuditPack(nil, t0, t0.Add(time.Hour), t0.Add(2*time.Hour))
	require.NoError(t, pack.Verify())
	assert.Equal(t, 0, pack.TransactionCount)
	assert.Empty(t, pack.Transactions)
}

func TestAuditPack_JSONRoundTripStillVerifies(t *testing.T) {
	pack := ledger.NewAuditPack([]ledger.Record{sampleRecord("pi_1", 9)}, t0, t0.Add(time.Hour), t0.Add(2*time.Hour))
	raw, err := json.Marshal(pack)
	require.NoError(t, err)

	var back ledger.AuditPack
	require.NoError(t, json.Unmarshal(raw, &back))
	require.NoError(t, back.Verify(), "the exported form must verify after a round trip")
}

func TestBuildAuditPack_RefusesUnsettledAndBadPeriods(t *testing.T) {
	// BuildAuditPack returns before touching the pool in these cases, so a nil ledger is fine.
	lgr := ledger.New(nil)
	ctx := context.Background()

	_, err := lgr.BuildAuditPack(ctx, time.Now().Add(-time.Hour), time.Now().Add(-time.Second))
	assert.ErrorIs(t, err, ledger.ErrPeriodNotSettled, "a period ending seconds ago is not settled")

	_, err = lgr.BuildAuditPack(ctx, time.Now().Add(-time.Hour), time.Now().Add(-2*time.Hour))
	assert.Error(t, err, "start must be before end")
}

func clonePack(t *testing.T, p ledger.AuditPack) ledger.AuditPack {
	t.Helper()
	raw, err := json.Marshal(p)
	require.NoError(t, err)
	var out ledger.AuditPack
	require.NoError(t, json.Unmarshal(raw, &out))
	return out
}
