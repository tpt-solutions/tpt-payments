package issuance

import (
	"fmt"
	"regexp"
)

// Module-wide constants for message validation.
const (
	// MaxAddressLen is the maximum length of a Cosmos bech32 address.
	MaxAddressLen = 64

	// UUIDHexLen is the expected length of a SHA-256 hex string (64 hex chars).
	UUIDHexLen = 64

	// MaxTierLen is the maximum length of an identity tier string.
	MaxTierLen = 20

	// MaxMemoLen is the maximum length of a transaction memo.
	MaxMemoLen = 256

	// MaxBatchRecipients is the maximum number of recipients in a batch send.
	MaxBatchRecipients = 100

	// MinTransferAmount is the minimum transfer amount (1 ucredit).
	MinTransferAmount int64 = 1
)

// ValidTiers contains the only allowed identity tiers.
var ValidTiers = map[string]bool{
	"kyc":       true,
	"biometric": true,
	"dna":       true,
}

// ValidFrequencies contains the only allowed payout frequencies.
var ValidFrequencies = map[PayoutFrequency]bool{
	FrequencyWeekly:    true,
	FrequencyBiweekly:  true,
	FrequencyMonthly:   true,
	FrequencyQuarterly: true,
	FrequencyAnnual:    true,
}

// bech32Regex validates a Cosmos bech32 address (prefix1 + 38 hex chars).
var bech32Regex = regexp.MustCompile(`^[a-z]{2,10}1[0-9a-z]{38}$`)

// hex64Regex validates a 64-character lowercase hex string (SHA-256).
var hex64Regex = regexp.MustCompile(`^[0-9a-f]{64}$`)

// ValidAddress returns true if the given string is a valid bech32 address.
func ValidAddress(addr string) bool {
	return bech32Regex.MatchString(addr)
}

// MsgRegisterHuman is the message to register a new verified human identity.
// It is submitted by the identity bridge on behalf of a verified user.
type MsgRegisterHuman struct {
	// Address is the Cosmos bech32 account address to register.
	Address string `json:"address"`

	// IdentityUUID is the SHA-256 hash of the KYC provider's unique person ID.
	// This serves as the uniqueness constraint on-chain.
	IdentityUUID string `json:"identity_uuid"`

	// IdentityTier is the verification level: "kyc" | "biometric" | "dna"
	IdentityTier string `json:"identity_tier"`

	// PayoutFrequency is the desired payout schedule.
	PayoutFrequency PayoutFrequency `json:"payout_frequency"`
}

// NewMsgRegisterHuman creates a new MsgRegisterHuman with validation.
func NewMsgRegisterHuman(address, identityUUID, tier string, freq PayoutFrequency) MsgRegisterHuman {
	return MsgRegisterHuman{
		Address:         address,
		IdentityUUID:    identityUUID,
		IdentityTier:    tier,
		PayoutFrequency: freq,
	}
}

// ValidateBasic performs stateless validation on MsgRegisterHuman.
// Stateful validation (duplicate UUID, duplicate address) is done in the keeper.
func (msg MsgRegisterHuman) ValidateBasic() error {
	// Validate address format.
	if msg.Address == "" {
		return fmt.Errorf("address cannot be empty")
	}
	if len(msg.Address) > MaxAddressLen {
		return fmt.Errorf("address length %d exceeds maximum %d", len(msg.Address), MaxAddressLen)
	}
	if !bech32Regex.MatchString(msg.Address) {
		return fmt.Errorf("address %q is not a valid bech32 address", msg.Address)
	}

	// Validate IdentityUUID format (must be 64-char hex SHA-256).
	if msg.IdentityUUID == "" {
		return fmt.Errorf("identity_uuid cannot be empty")
	}
	if len(msg.IdentityUUID) != UUIDHexLen {
		return fmt.Errorf("identity_uuid length %d does not match SHA-256 hex length %d",
			len(msg.IdentityUUID), UUIDHexLen)
	}
	if !hex64Regex.MatchString(msg.IdentityUUID) {
		return fmt.Errorf("identity_uuid %q is not a valid 64-character hex string", msg.IdentityUUID)
	}

	// Validate identity tier.
	if msg.IdentityTier == "" {
		return fmt.Errorf("identity_tier cannot be empty")
	}
	if len(msg.IdentityTier) > MaxTierLen {
		return fmt.Errorf("identity_tier length %d exceeds maximum %d", len(msg.IdentityTier), MaxTierLen)
	}
	if !ValidTiers[msg.IdentityTier] {
		return fmt.Errorf("identity_tier %q is not valid; must be one of: kyc, biometric, dna",
			msg.IdentityTier)
	}

	// Validate payout frequency.
	if msg.PayoutFrequency == "" {
		return fmt.Errorf("payout_frequency cannot be empty")
	}
	if !ValidFrequencies[msg.PayoutFrequency] {
		return fmt.Errorf(
			"payout_frequency %q is not valid; must be one of: weekly, biweekly, monthly, quarterly, annual",
			msg.PayoutFrequency,
		)
	}

	return nil
}

// MsgUpdatePayoutFrequency is the message to change an account's payout frequency.
// The change applies starting from the next payout cycle (not retroactive).
type MsgUpdatePayoutFrequency struct {
	// Address is the account to update.
	Address string `json:"address"`

	// NewFrequency is the desired payout frequency.
	NewFrequency PayoutFrequency `json:"new_frequency"`
}

// NewMsgUpdatePayoutFrequency creates a new MsgUpdatePayoutFrequency.
func NewMsgUpdatePayoutFrequency(address string, freq PayoutFrequency) MsgUpdatePayoutFrequency {
	return MsgUpdatePayoutFrequency{
		Address:      address,
		NewFrequency: freq,
	}
}

// ValidateBasic performs stateless validation on MsgUpdatePayoutFrequency.
func (msg MsgUpdatePayoutFrequency) ValidateBasic() error {
	if msg.Address == "" {
		return fmt.Errorf("address cannot be empty")
	}
	if len(msg.Address) > MaxAddressLen {
		return fmt.Errorf("address length %d exceeds maximum %d", len(msg.Address), MaxAddressLen)
	}
	if !bech32Regex.MatchString(msg.Address) {
		return fmt.Errorf("address %q is not a valid bech32 address", msg.Address)
	}
	if msg.NewFrequency == "" {
		return fmt.Errorf("new_frequency cannot be empty")
	}
	if !ValidFrequencies[msg.NewFrequency] {
		return fmt.Errorf(
			"new_frequency %q is not valid; must be one of: weekly, biweekly, monthly, quarterly, annual",
			msg.NewFrequency,
		)
	}
	return nil
}

// MsgSendCredits is the message to transfer credits between accounts.
type MsgSendCredits struct {
	// From is the sender address.
	From string `json:"from"`

	// To is the recipient address.
	To string `json:"to"`

	// Amount is the ucredit amount to transfer.
	Amount string `json:"amount"`

	// Memo is an optional note for the transfer.
	Memo string `json:"memo,omitempty"`
}

// NewMsgSendCredits creates a new MsgSendCredits.
func NewMsgSendCredits(from, to, amount, memo string) MsgSendCredits {
	return MsgSendCredits{
		From:   from,
		To:     to,
		Amount: amount,
		Memo:   memo,
	}
}

// ValidateBasic performs stateless validation on MsgSendCredits.
func (msg MsgSendCredits) ValidateBasic() error {
	if msg.From == "" {
		return fmt.Errorf("from address cannot be empty")
	}
	if len(msg.From) > MaxAddressLen {
		return fmt.Errorf("from address length %d exceeds maximum %d", len(msg.From), MaxAddressLen)
	}
	if !bech32Regex.MatchString(msg.From) {
		return fmt.Errorf("from address %q is not a valid bech32 address", msg.From)
	}
	if msg.To == "" {
		return fmt.Errorf("to address cannot be empty")
	}
	if len(msg.To) > MaxAddressLen {
		return fmt.Errorf("to address length %d exceeds maximum %d", len(msg.To), MaxAddressLen)
	}
	if !bech32Regex.MatchString(msg.To) {
		return fmt.Errorf("to address %q is not a valid bech32 address", msg.To)
	}
	if msg.From == msg.To {
		return fmt.Errorf("cannot send credits to self")
	}
	amount, ok := newIntFromString(msg.Amount)
	if !ok {
		return fmt.Errorf("amount %q is not a valid integer", msg.Amount)
	}
	if amount < MinTransferAmount {
		return fmt.Errorf("amount %d is below minimum %d", amount, MinTransferAmount)
	}
	if len(msg.Memo) > MaxMemoLen {
		return fmt.Errorf("memo length %d exceeds maximum %d", len(msg.Memo), MaxMemoLen)
	}
	return nil
}

// MsgBatchSend is the message to send credits to multiple recipients in one transaction.
type MsgBatchSend struct {
	// From is the sender address.
	From string `json:"from"`

	// Entries is the list of (recipient, amount) pairs.
	Entries []BatchSendEntry `json:"entries"`

	// Memo is an optional note for the batch send.
	Memo string `json:"memo,omitempty"`
}

// NewMsgBatchSend creates a new MsgBatchSend.
func NewMsgBatchSend(from string, entries []BatchSendEntry, memo string) MsgBatchSend {
	return MsgBatchSend{
		From:    from,
		Entries: entries,
		Memo:    memo,
	}
}

// ValidateBasic performs stateless validation on MsgBatchSend.
func (msg MsgBatchSend) ValidateBasic() error {
	if msg.From == "" {
		return fmt.Errorf("from address cannot be empty")
	}
	if len(msg.From) > MaxAddressLen {
		return fmt.Errorf("from address length %d exceeds maximum %d", len(msg.From), MaxAddressLen)
	}
	if !bech32Regex.MatchString(msg.From) {
		return fmt.Errorf("from address %q is not a valid bech32 address", msg.From)
	}
	if len(msg.Entries) == 0 {
		return fmt.Errorf("batch send must have at least one recipient")
	}
	if len(msg.Entries) > MaxBatchRecipients {
		return fmt.Errorf("batch send recipients %d exceeds maximum %d", len(msg.Entries), MaxBatchRecipients)
	}
	seen := make(map[string]bool)
	for i, entry := range msg.Entries {
		if entry.To == "" {
			return fmt.Errorf("entry %d: to address cannot be empty", i)
		}
		if !bech32Regex.MatchString(entry.To) {
			return fmt.Errorf("entry %d: to address %q is not a valid bech32 address", i, entry.To)
		}
		if entry.To == msg.From {
			return fmt.Errorf("entry %d: cannot send credits to self", i)
		}
		amount, ok := newIntFromString(entry.Amount.String())
		if !ok {
			return fmt.Errorf("entry %d: amount is not a valid integer", i)
		}
		if amount < MinTransferAmount {
			return fmt.Errorf("entry %d: amount %d is below minimum %d", i, amount, MinTransferAmount)
		}
		if seen[entry.To] {
			return fmt.Errorf("entry %d: duplicate recipient %q", i, entry.To)
		}
		seen[entry.To] = true
	}
	if len(msg.Memo) > MaxMemoLen {
		return fmt.Errorf("memo length %d exceeds maximum %d", len(msg.Memo), MaxMemoLen)
	}
	return nil
}

// ParseAmount parses a string amount into an int64.
func ParseAmount(amountStr string) (int64, error) {
	amount, ok := newIntFromString(amountStr)
	if !ok {
		return 0, fmt.Errorf("amount %q is not a valid integer", amountStr)
	}
	if amount < 0 {
		return 0, fmt.Errorf("amount %d cannot be negative", amount)
	}
	return amount, nil
}

// newIntFromString is a helper to parse int64 from string.
func newIntFromString(s string) (int64, bool) {
	if s == "" {
		return 0, false
	}
	// Check for negative sign.
	negative := false
	start := 0
	if s[0] == '-' {
		negative = true
		start = 1
	}
	var n int64
	for i := start; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
		n = n*10 + int64(s[i]-'0')
	}
	if negative {
		n = -n
	}
	return n, true
}