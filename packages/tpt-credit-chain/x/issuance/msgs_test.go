package issuance_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tpt-platform/tpt-platform/packages/tpt-credit-chain/x/issuance"
)

// validUUID is a valid 64-char SHA-256 hex string used in tests.
const validUUID = "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0aab1"

func TestMsgRegisterHuman_ValidateBasic_Valid(t *testing.T) {
	msg := issuance.NewMsgRegisterHuman(
		"cosmos1qyqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqf0d5g9",
		validUUID,
		"kyc",
		issuance.FrequencyMonthly,
	)
	err := msg.ValidateBasic()
	assert.NoError(t, err)
}

func TestMsgRegisterHuman_ValidateBasic_AllValidFrequencies(t *testing.T) {
	freqs := []issuance.PayoutFrequency{
		issuance.FrequencyWeekly,
		issuance.FrequencyBiweekly,
		issuance.FrequencyMonthly,
		issuance.FrequencyQuarterly,
		issuance.FrequencyAnnual,
	}
	for _, freq := range freqs {
		t.Run(string(freq), func(t *testing.T) {
			msg := issuance.NewMsgRegisterHuman(
				"cosmos1qyqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqf0d5g9",
				validUUID,
				"kyc",
				freq,
			)
			err := msg.ValidateBasic()
			assert.NoError(t, err)
		})
	}
}

func TestMsgRegisterHuman_ValidateBasic_AllValidTiers(t *testing.T) {
	tiers := []string{"kyc", "biometric", "dna"}
	for _, tier := range tiers {
		t.Run(tier, func(t *testing.T) {
			msg := issuance.NewMsgRegisterHuman(
				"cosmos1qyqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqf0d5g9",
				validUUID,
				tier,
				issuance.FrequencyMonthly,
			)
			err := msg.ValidateBasic()
			assert.NoError(t, err)
		})
	}
}

// ── Address validation ─────────────────────────────────────────────────────────

func TestMsgRegisterHuman_ValidateBasic_EmptyAddress(t *testing.T) {
	msg := issuance.NewMsgRegisterHuman("", validUUID, "kyc", issuance.FrequencyMonthly)
	err := msg.ValidateBasic()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "address cannot be empty")
}

func TestMsgRegisterHuman_ValidateBasic_InvalidAddress(t *testing.T) {
	msg := issuance.NewMsgRegisterHuman("not-a-bech32-address", validUUID, "kyc", issuance.FrequencyMonthly)
	err := msg.ValidateBasic()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not a valid bech32 address")
}

func TestMsgRegisterHuman_ValidateBasic_TooLongAddress(t *testing.T) {
	longAddr := "cosmos1" + string(make([]byte, 100))
	msg := issuance.NewMsgRegisterHuman(longAddr, validUUID, "kyc", issuance.FrequencyMonthly)
	err := msg.ValidateBasic()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds maximum")
}

// ── UUID validation ───────────────────────────────────────────────────────────

func TestMsgRegisterHuman_ValidateBasic_EmptyUUID(t *testing.T) {
	msg := issuance.NewMsgRegisterHuman(
		"cosmos1qyqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqf0d5g9",
		"",
		"kyc",
		issuance.FrequencyMonthly,
	)
	err := msg.ValidateBasic()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "identity_uuid cannot be empty")
}

func TestMsgRegisterHuman_ValidateBasic_ShortUUID(t *testing.T) {
	msg := issuance.NewMsgRegisterHuman(
		"cosmos1qyqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqf0d5g9",
		"too-short",
		"kyc",
		issuance.FrequencyMonthly,
	)
	err := msg.ValidateBasic()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not match SHA-256 hex length")
}

func TestMsgRegisterHuman_ValidateBasic_NonHexUUID(t *testing.T) {
	msg := issuance.NewMsgRegisterHuman(
		"cosmos1qyqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqf0d5g9",
		"zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz",
		"kyc",
		issuance.FrequencyMonthly,
	)
	err := msg.ValidateBasic()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not a valid 64-character hex string")
}

// ── Tier validation ───────────────────────────────────────────────────────────

func TestMsgRegisterHuman_ValidateBasic_EmptyTier(t *testing.T) {
	msg := issuance.NewMsgRegisterHuman(
		"cosmos1qyqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqf0d5g9",
		validUUID,
		"",
		issuance.FrequencyMonthly,
	)
	err := msg.ValidateBasic()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "identity_tier cannot be empty")
}

func TestMsgRegisterHuman_ValidateBasic_InvalidTier(t *testing.T) {
	msg := issuance.NewMsgRegisterHuman(
		"cosmos1qyqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqf0d5g9",
		validUUID,
		"superadmin",
		issuance.FrequencyMonthly,
	)
	err := msg.ValidateBasic()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not valid")
	assert.Contains(t, err.Error(), "kyc, biometric, dna")
}

// ── Frequency validation ──────────────────────────────────────────────────────

func TestMsgRegisterHuman_ValidateBasic_EmptyFrequency(t *testing.T) {
	msg := issuance.NewMsgRegisterHuman(
		"cosmos1qyqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqf0d5g9",
		validUUID,
		"kyc",
		"",
	)
	err := msg.ValidateBasic()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "payout_frequency cannot be empty")
}

func TestMsgRegisterHuman_ValidateBasic_InvalidFrequency(t *testing.T) {
	msg := issuance.NewMsgRegisterHuman(
		"cosmos1qyqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqf0d5g9",
		validUUID,
		"kyc",
		"daily",
	)
	err := msg.ValidateBasic()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not valid")
	assert.Contains(t, err.Error(), "weekly, biweekly, monthly, quarterly, annual")
}

// ── NEW: MsgUpdatePayoutFrequency tests ────────────────────────────────────────

func TestMsgUpdatePayoutFrequency_ValidateBasic_Valid(t *testing.T) {
	msg := issuance.NewMsgUpdatePayoutFrequency(
		"cosmos1qyqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqf0d5g9",
		issuance.FrequencyMonthly,
	)
	err := msg.ValidateBasic()
	assert.NoError(t, err)
}

func TestMsgUpdatePayoutFrequency_ValidateBasic_AllFrequencies(t *testing.T) {
	freqs := []issuance.PayoutFrequency{
		issuance.FrequencyWeekly,
		issuance.FrequencyBiweekly,
		issuance.FrequencyMonthly,
		issuance.FrequencyQuarterly,
		issuance.FrequencyAnnual,
	}
	for _, freq := range freqs {
		t.Run(string(freq), func(t *testing.T) {
			msg := issuance.NewMsgUpdatePayoutFrequency(
				"cosmos1qyqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqf0d5g9",
				freq,
			)
			err := msg.ValidateBasic()
			assert.NoError(t, err)
		})
	}
}

func TestMsgUpdatePayoutFrequency_ValidateBasic_EmptyAddress(t *testing.T) {
	msg := issuance.NewMsgUpdatePayoutFrequency("", issuance.FrequencyMonthly)
	err := msg.ValidateBasic()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "address cannot be empty")
}

func TestMsgUpdatePayoutFrequency_ValidateBasic_InvalidAddress(t *testing.T) {
	msg := issuance.NewMsgUpdatePayoutFrequency("not-a-bech32-address", issuance.FrequencyMonthly)
	err := msg.ValidateBasic()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not a valid bech32 address")
}

func TestMsgUpdatePayoutFrequency_ValidateBasic_EmptyFrequency(t *testing.T) {
	msg := issuance.NewMsgUpdatePayoutFrequency(
		"cosmos1qyqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqf0d5g9",
		"",
	)
	err := msg.ValidateBasic()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "new_frequency cannot be empty")
}

func TestMsgUpdatePayoutFrequency_ValidateBasic_InvalidFrequency(t *testing.T) {
	msg := issuance.NewMsgUpdatePayoutFrequency(
		"cosmos1qyqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqf0d5g9",
		"daily",
	)
	err := msg.ValidateBasic()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not valid")
}

// ── NEW: MsgSendCredits tests ─────────────────────────────────────────────────

func TestMsgSendCredits_ValidateBasic_Valid(t *testing.T) {
	msg := issuance.NewMsgSendCredits(
		"cosmos1qyqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqf0d5g9",
		"cosmos1qyqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqg7f8d8",
		"1000000",
		"test memo",
	)
	err := msg.ValidateBasic()
	assert.NoError(t, err)
}

func TestMsgSendCredits_ValidateBasic_EmptyFrom(t *testing.T) {
	msg := issuance.NewMsgSendCredits("", testAddr2, "1000000", "")
	err := msg.ValidateBasic()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "from address cannot be empty")
}

func TestMsgSendCredits_ValidateBasic_InvalidFrom(t *testing.T) {
	msg := issuance.NewMsgSendCredits("invalid-addr", testAddr2, "1000000", "")
	err := msg.ValidateBasic()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "from address")
}

func TestMsgSendCredits_ValidateBasic_EmptyTo(t *testing.T) {
	msg := issuance.NewMsgSendCredits(testAddr1, "", "1000000", "")
	err := msg.ValidateBasic()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "to address cannot be empty")
}

func TestMsgSendCredits_ValidateBasic_InvalidTo(t *testing.T) {
	msg := issuance.NewMsgSendCredits(testAddr1, "invalid-addr", "1000000", "")
	err := msg.ValidateBasic()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "to address")
}

func TestMsgSendCredits_ValidateBasic_SelfTransfer(t *testing.T) {
	msg := issuance.NewMsgSendCredits(testAddr1, testAddr1, "1000000", "")
	err := msg.ValidateBasic()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot send credits to self")
}

func TestMsgSendCredits_ValidateBasic_EmptyAmount(t *testing.T) {
	msg := issuance.NewMsgSendCredits(testAddr1, testAddr2, "", "")
	err := msg.ValidateBasic()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not a valid integer")
}

func TestMsgSendCredits_ValidateBasic_ZeroAmount(t *testing.T) {
	msg := issuance.NewMsgSendCredits(testAddr1, testAddr2, "0", "")
	err := msg.ValidateBasic()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "below minimum")
}

func TestMsgSendCredits_ValidateBasic_NegativeAmount(t *testing.T) {
	msg := issuance.NewMsgSendCredits(testAddr1, testAddr2, "-100", "")
	err := msg.ValidateBasic()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "below minimum")
}

func TestMsgSendCredits_ValidateBasic_NonNumericAmount(t *testing.T) {
	msg := issuance.NewMsgSendCredits(testAddr1, testAddr2, "abc", "")
	err := msg.ValidateBasic()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not a valid integer")
}

func TestMsgSendCredits_ValidateBasic_MemoTooLong(t *testing.T) {
	longMemo := string(make([]byte, issuance.MaxMemoLen+1))
	msg := issuance.NewMsgSendCredits(testAddr1, testAddr2, "1000000", longMemo)
	err := msg.ValidateBasic()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "memo length")
}

// ── NEW: MsgBatchSend tests ───────────────────────────────────────────────────

func TestMsgBatchSend_ValidateBasic_Valid(t *testing.T) {
	entries := []issuance.BatchSendEntry{
		{To: testAddr2, Amount: issuance.PayoutAmount(issuance.FrequencyMonthly)},
		{To: testAddr3, Amount: issuance.PayoutAmount(issuance.FrequencyMonthly)},
	}
	msg := issuance.NewMsgBatchSend(testAddr1, entries, "batch memo")
	err := msg.ValidateBasic()
	assert.NoError(t, err)
}

func TestMsgBatchSend_ValidateBasic_EmptyFrom(t *testing.T) {
	entries := []issuance.BatchSendEntry{
		{To: testAddr2, Amount: issuance.PayoutAmount(issuance.FrequencyMonthly)},
	}
	msg := issuance.NewMsgBatchSend("", entries, "")
	err := msg.ValidateBasic()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "from address cannot be empty")
}

func TestMsgBatchSend_ValidateBasic_InvalidFrom(t *testing.T) {
	entries := []issuance.BatchSendEntry{
		{To: testAddr2, Amount: issuance.PayoutAmount(issuance.FrequencyMonthly)},
	}
	msg := issuance.NewMsgBatchSend("invalid-addr", entries, "")
	err := msg.ValidateBasic()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "from address")
}

func TestMsgBatchSend_ValidateBasic_EmptyEntries(t *testing.T) {
	msg := issuance.NewMsgBatchSend(testAddr1, []issuance.BatchSendEntry{}, "")
	err := msg.ValidateBasic()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least one recipient")
}

func TestMsgBatchSend_ValidateBasic_ExceedsMaxRecipients(t *testing.T) {
	entries := make([]issuance.BatchSendEntry, issuance.MaxBatchRecipients+1)
	for i := range entries {
		entries[i] = issuance.BatchSendEntry{To: testAddr2, Amount: issuance.PayoutAmount(issuance.FrequencyMonthly)}
	}
	msg := issuance.NewMsgBatchSend(testAddr1, entries, "")
	err := msg.ValidateBasic()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds maximum")
}

func TestMsgBatchSend_ValidateBasic_EmptyEntryTo(t *testing.T) {
	entries := []issuance.BatchSendEntry{
		{To: "", Amount: issuance.PayoutAmount(issuance.FrequencyMonthly)},
	}
	msg := issuance.NewMsgBatchSend(testAddr1, entries, "")
	err := msg.ValidateBasic()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "to address cannot be empty")
}

func TestMsgBatchSend_ValidateBasic_InvalidEntryTo(t *testing.T) {
	entries := []issuance.BatchSendEntry{
		{To: "invalid-addr", Amount: issuance.PayoutAmount(issuance.FrequencyMonthly)},
	}
	msg := issuance.NewMsgBatchSend(testAddr1, entries, "")
	err := msg.ValidateBasic()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not a valid bech32 address")
}

func TestMsgBatchSend_ValidateBasic_DuplicateRecipient(t *testing.T) {
	entries := []issuance.BatchSendEntry{
		{To: testAddr2, Amount: issuance.PayoutAmount(issuance.FrequencyMonthly)},
		{To: testAddr2, Amount: issuance.PayoutAmount(issuance.FrequencyMonthly)},
	}
	msg := issuance.NewMsgBatchSend(testAddr1, entries, "")
	err := msg.ValidateBasic()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate recipient")
}

func TestMsgBatchSend_ValidateBasic_SelfTransfer(t *testing.T) {
	entries := []issuance.BatchSendEntry{
		{To: testAddr1, Amount: issuance.PayoutAmount(issuance.FrequencyMonthly)},
	}
	msg := issuance.NewMsgBatchSend(testAddr1, entries, "")
	err := msg.ValidateBasic()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot send credits to self")
}

func TestMsgBatchSend_ValidateBasic_MemoTooLong(t *testing.T) {
	longMemo := string(make([]byte, issuance.MaxMemoLen+1))
	entries := []issuance.BatchSendEntry{
		{To: testAddr2, Amount: issuance.PayoutAmount(issuance.FrequencyMonthly)},
	}
	msg := issuance.NewMsgBatchSend(testAddr1, entries, longMemo)
	err := msg.ValidateBasic()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "memo length")
}

// ── Existing edge cases ────────────────────────────────────────────────────────

func TestMsgRegisterHuman_NewMsg_SetsFieldsCorrectly(t *testing.T) {
	msg := issuance.NewMsgRegisterHuman(
		"cosmos1qyqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqf0d5g9",
		validUUID,
		"biometric",
		issuance.FrequencyWeekly,
	)
	assert.Equal(t, "cosmos1qyqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqf0d5g9", msg.Address)
	assert.Equal(t, validUUID, msg.IdentityUUID)
	assert.Equal(t, "biometric", msg.IdentityTier)
	assert.Equal(t, issuance.FrequencyWeekly, msg.PayoutFrequency)
}

func TestMsgRegisterHuman_ValidateBasic_AllowsMultipleValidAddresses(t *testing.T) {
	addrs := []string{
		"cosmos1qyqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqf0d5g9",
		"cosmos1qyqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqg7f8d8",
		"cosmos1qyqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqp8n9e7",
	}
	for _, addr := range addrs {
		t.Run(addr, func(t *testing.T) {
			msg := issuance.NewMsgRegisterHuman(addr, validUUID, "kyc", issuance.FrequencyMonthly)
			err := msg.ValidateBasic()
			assert.NoError(t, err)
		})
	}
}

func TestMsgRegisterHuman_ValidateBasic_AddressPrefixValidation(t *testing.T) {
	msg := issuance.NewMsgRegisterHuman(
		"cosmosxqyqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqf0d5g9",
		validUUID,
		"kyc",
		issuance.FrequencyMonthly,
	)
	err := msg.ValidateBasic()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not a valid bech32 address")
}

func TestMaxAddressLen_Constant(t *testing.T) {
	assert.Equal(t, 64, issuance.MaxAddressLen)
}

func TestUUIDHexLen_Constant(t *testing.T) {
	assert.Equal(t, 64, issuance.UUIDHexLen)
}

func TestMaxTierLen_Constant(t *testing.T) {
	assert.Equal(t, 20, issuance.MaxTierLen)
}

func TestValidTiers_ContainsAll(t *testing.T) {
	assert.True(t, issuance.ValidTiers["kyc"])
	assert.True(t, issuance.ValidTiers["biometric"])
	assert.True(t, issuance.ValidTiers["dna"])
	assert.False(t, issuance.ValidTiers["invalid"])
}

func TestValidFrequencies_ContainsAll(t *testing.T) {
	assert.True(t, issuance.ValidFrequencies[issuance.FrequencyWeekly])
	assert.True(t, issuance.ValidFrequencies[issuance.FrequencyBiweekly])
	assert.True(t, issuance.ValidFrequencies[issuance.FrequencyMonthly])
	assert.True(t, issuance.ValidFrequencies[issuance.FrequencyQuarterly])
	assert.True(t, issuance.ValidFrequencies[issuance.FrequencyAnnual])
	assert.False(t, issuance.ValidFrequencies["daily"])
}

func TestMsgRegisterHuman_ValidateBasic_NilReceiver(t *testing.T) {
	msg := issuance.MsgRegisterHuman{
		Address: "cosmos1qyqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqf0d5g9",
	}
	err := msg.ValidateBasic()
	assert.Error(t, err) // missing required fields
}

func TestMsgRegisterHuman_ValidateBasic_RejectsCapitalizedTier(t *testing.T) {
	msg := issuance.NewMsgRegisterHuman(
		"cosmos1qyqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqf0d5g9",
		validUUID,
		"KYC",
		issuance.FrequencyMonthly,
	)
	err := msg.ValidateBasic()
	assert.Error(t, err, "tier must be lowercase")
}

// ── NEW: ValidAddress utility ──────────────────────────────────────────────────

func TestValidAddress_Valid(t *testing.T) {
	assert.True(t, issuance.ValidAddress("cosmos1qyqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqf0d5g9"))
	assert.True(t, issuance.ValidAddress("cosmos1qyqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqg7f8d8"))
}

func TestValidAddress_Invalid(t *testing.T) {
	assert.False(t, issuance.ValidAddress(""))
	assert.False(t, issuance.ValidAddress("invalid"))
	assert.False(t, issuance.ValidAddress("cosmos1xx"))
}

func TestMsgUpdatePayoutFrequency_NewMsg_SetsFields(t *testing.T) {
	msg := issuance.NewMsgUpdatePayoutFrequency(testAddr1, issuance.FrequencyQuarterly)
	assert.Equal(t, testAddr1, msg.Address)
	assert.Equal(t, issuance.FrequencyQuarterly, msg.NewFrequency)
}

func TestMsgSendCredits_NewMsg_SetsFields(t *testing.T) {
	msg := issuance.NewMsgSendCredits(testAddr1, testAddr2, "5000000", "payment")
	assert.Equal(t, testAddr1, msg.From)
	assert.Equal(t, testAddr2, msg.To)
	assert.Equal(t, "5000000", msg.Amount)
	assert.Equal(t, "payment", msg.Memo)
}

func TestMsgSendCredits_ValidateBasic_NoMemo(t *testing.T) {
	msg := issuance.NewMsgSendCredits(testAddr1, testAddr2, "1000000", "")
	err := msg.ValidateBasic()
	assert.NoError(t, err, "empty memo should be valid")
}

func TestMsgBatchSend_NewMsg_SetsFields(t *testing.T) {
	entries := []issuance.BatchSendEntry{
		{To: testAddr2, Amount: issuance.PayoutAmount(issuance.FrequencyMonthly)},
	}
	msg := issuance.NewMsgBatchSend(testAddr1, entries, "batch")
	assert.Equal(t, testAddr1, msg.From)
	assert.Len(t, msg.Entries, 1)
	assert.Equal(t, "batch", msg.Memo)
}