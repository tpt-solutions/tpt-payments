package issuance_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-credit-chain/x/issuance"
)

// The keeper rejects malformed addresses and identity UUIDs even when the caller skipped
// ValidateBasic.
func TestKeeper_RegisterHuman_RejectsMalformedFields(t *testing.T) {
	k := issuance.NewKeeper()
	ctx := context.Background()

	_, err := k.RegisterHuman(ctx, "not-an-address", testUUID1, "kyc", issuance.FrequencyMonthly)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bech32")

	_, err = k.RegisterHuman(ctx, testAddr1, "short-uuid", "kyc", issuance.FrequencyMonthly)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SHA-256")

	assert.Empty(t, k.GetAccounts(), "a rejected registration stores nothing")
}

// Genesis with a duplicate address or identity is refused as a whole.
func TestInitializeGenesis_RejectsDuplicateAccounts(t *testing.T) {
	k := issuance.NewKeeper()
	state := &issuance.GenesisState{HumanAccounts: []issuance.HumanAccount{
		{Address: testAddr1, IdentityUUID: testUUID1, PayoutFrequency: issuance.FrequencyMonthly},
		{Address: testAddr1, IdentityUUID: testUUID2, PayoutFrequency: issuance.FrequencyMonthly},
	}}
	err := k.InitializeGenesis(context.Background(), state)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate address")
	assert.Empty(t, k.GetAccounts())
}
