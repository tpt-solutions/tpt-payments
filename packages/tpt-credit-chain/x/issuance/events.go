// Package issuance implements the TPT Credit issuance module.
//
// Event types for issuance operations.
package issuance

import (
	"fmt"
)

// EventTypeHumanRegistered is emitted when a new human is registered.
const EventTypeHumanRegistered = "human_registered"

// EventTypeCreditsIssued is emitted when credits are minted to an account.
const EventTypeCreditsIssued = "credits_issued"

// EventTypeFrequencyUpdated is emitted when a payout frequency is changed.
const EventTypeFrequencyUpdated = "frequency_updated"

// EventTypeCreditsTransferred is emitted when credits are transferred between accounts.
const EventTypeCreditsTransferred = "credits_transferred"

// EventTypeBatchSend is emitted when a batch send occurs.
const EventTypeBatchSend = "credits_batch_sent"

// HumanRegisteredEvent is emitted after a successful MsgRegisterHuman.
type HumanRegisteredEvent struct {
	// Address is the Cosmos bech32 account address.
	Address string `json:"address"`

	// IdentityUUID is the SHA-256 hash of the KYC provider's unique person ID.
	IdentityUUID string `json:"identity_uuid"`

	// IdentityTier is the verification level: "kyc" | "biometric" | "dna"
	IdentityTier string `json:"identity_tier"`

	// PayoutFrequency is the selected payout schedule.
	PayoutFrequency PayoutFrequency `json:"payout_frequency"`

	// BlockHeight is the block at which registration occurred.
	BlockHeight int64 `json:"block_height"`
}

// Attributes returns the event attributes as key-value pairs.
func (e HumanRegisteredEvent) Attributes() []string {
	return []string{
		"address", e.Address,
		"identity_uuid", e.IdentityUUID,
		"identity_tier", e.IdentityTier,
		"payout_frequency", string(e.PayoutFrequency),
		"block_height", fmt.Sprintf("%d", e.BlockHeight),
	}
}

// CreditsIssuedEvent is emitted each time credits are minted to an account.
type CreditsIssuedEvent struct {
	// Address is the recipient account.
	Address string `json:"address"`

	// Amount is the ucredit amount minted in this payout.
	Amount string `json:"amount"`

	// PayoutNumber is the nth payout for this account in the current year.
	PayoutNumber int64 `json:"payout_number"`

	// Year is the issuance year (1-based, starting from genesis).
	Year int64 `json:"year"`

	// BlockHeight is the block at which the payout occurred.
	BlockHeight int64 `json:"block_height"`
}

// Attributes returns the event attributes as key-value pairs.
func (e CreditsIssuedEvent) Attributes() []string {
	return []string{
		"address", e.Address,
		"amount", e.Amount,
		"payout_number", fmt.Sprintf("%d", e.PayoutNumber),
		"year", fmt.Sprintf("%d", e.Year),
		"block_height", fmt.Sprintf("%d", e.BlockHeight),
	}
}

// FrequencyUpdatedEvent is emitted when an account's payout frequency changes.
type FrequencyUpdatedEvent struct {
	// Address is the account that changed frequency.
	Address string `json:"address"`

	// OldFrequency is the previous payout frequency.
	OldFrequency PayoutFrequency `json:"old_frequency"`

	// NewFrequency is the new payout frequency.
	NewFrequency PayoutFrequency `json:"new_frequency"`

	// BlockHeight is the block at which the change was applied.
	BlockHeight int64 `json:"block_height"`

	// EffectiveAt is the block height when the new frequency takes effect
	// (next payout cycle, not retroactive).
	EffectiveAt int64 `json:"effective_at"`
}

// Attributes returns the event attributes as key-value pairs.
func (e FrequencyUpdatedEvent) Attributes() []string {
	return []string{
		"address", e.Address,
		"old_frequency", string(e.OldFrequency),
		"new_frequency", string(e.NewFrequency),
		"block_height", fmt.Sprintf("%d", e.BlockHeight),
		"effective_at", fmt.Sprintf("%d", e.EffectiveAt),
	}
}

// CreditsTransferredEvent is emitted when credits are transferred between accounts.
type CreditsTransferredEvent struct {
	// From is the sender address.
	From string `json:"from"`

	// To is the recipient address.
	To string `json:"to"`

	// Amount is the ucredit amount transferred.
	Amount string `json:"amount"`

	// BlockHeight is the block at which the transfer occurred.
	BlockHeight int64 `json:"block_height"`

	// SenderBalanceBefore is the sender's balance before the transfer.
	SenderBalanceBefore string `json:"sender_balance_before"`

	// RecipientBalanceBefore is the recipient's balance before the transfer.
	RecipientBalanceBefore string `json:"recipient_balance_before"`
}

// Attributes returns the event attributes as key-value pairs.
func (e CreditsTransferredEvent) Attributes() []string {
	return []string{
		"from", e.From,
		"to", e.To,
		"amount", e.Amount,
		"block_height", fmt.Sprintf("%d", e.BlockHeight),
		"sender_balance_before", e.SenderBalanceBefore,
		"recipient_balance_before", e.RecipientBalanceBefore,
	}
}

// BatchSendEvent is emitted when a batch send operation occurs.
type BatchSendEvent struct {
	// From is the sender address.
	From string `json:"from"`

	// Entries is the list of (recipient, amount) pairs.
	Entries []BatchSendEntry `json:"entries"`

	// TotalAmount is the sum of all amounts in the batch.
	TotalAmount string `json:"total_amount"`

	// BlockHeight is the block at which the batch send occurred.
	BlockHeight int64 `json:"block_height"`
}

// Attributes returns the event attributes as key-value pairs.
func (e BatchSendEvent) Attributes() []string {
	attrs := []string{
		"from", e.From,
		"total_amount", e.TotalAmount,
		"block_height", fmt.Sprintf("%d", e.BlockHeight),
		"num_recipients", fmt.Sprintf("%d", len(e.Entries)),
	}
	for _, entry := range e.Entries {
		attrs = append(attrs, "to", entry.To)
		attrs = append(attrs, "amount", entry.Amount.String())
	}
	return attrs
}