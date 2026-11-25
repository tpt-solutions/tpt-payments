package issuance

import (
	"crypto/ed25519"
	"encoding/binary"
	"errors"
	"fmt"

	"cosmossdk.io/math"
)

// Authorized transfers. A transfer that comes from outside the chain's own modules must be signed
// by the sender's key. Each account binds one Ed25519 public key, once, and every signed transfer
// carries a nonce that must be exactly one more than the account's last accepted nonce. That makes
// a signed message usable once, and in order.
//
// The unauthenticated SendCredits and BatchSend stay for the chain's own modules and tests. The
// transaction path (the tptcreditd tx commands) uses only the authorized functions below.

const (
	sendDomain  = "tpt-credit/send/v1"
	batchDomain = "tpt-credit/batch-send/v1"
)

// ErrNoKey is returned when a signed transfer comes from an account that has not bound a key.
var ErrNoKey = errors.New("account has no bound public key; bind one before sending")

// ErrBadSignature is returned when a signature does not verify for the sender's key.
var ErrBadSignature = errors.New("signature does not verify for the sender's key")

// ErrBadNonce is returned when a signed transfer's nonce is not the next one for its account.
var ErrBadNonce = errors.New("nonce must be exactly one more than the account's last accepted nonce")

// SignedSend is a transfer authorized by the sender's key.
type SignedSend struct {
	From      string
	To        string
	Amount    math.Int
	Nonce     uint64
	Signature []byte
}

// SignedBatch is a batch transfer authorized by the sender's key.
type SignedBatch struct {
	From      string
	Entries   []BatchSendEntry
	Nonce     uint64
	Signature []byte
}

// SendSigningBytes is the message a sender signs for a transfer. The fields are length-prefixed, so
// two different transfers can never produce the same bytes.
func SendSigningBytes(from, to string, amount math.Int, nonce uint64) []byte {
	var b []byte
	b = appendField(b, sendDomain)
	b = appendField(b, from)
	b = appendField(b, to)
	b = appendField(b, amount.String())
	b = appendUint(b, nonce)
	return b
}

// BatchSigningBytes is the message a sender signs for a batch transfer.
func BatchSigningBytes(from string, entries []BatchSendEntry, nonce uint64) []byte {
	var b []byte
	b = appendField(b, batchDomain)
	b = appendField(b, from)
	b = appendUint(b, uint64(len(entries)))
	for _, e := range entries {
		b = appendField(b, e.To)
		b = appendField(b, e.Amount.String())
	}
	b = appendUint(b, nonce)
	return b
}

// BindKey stores an account's public key. It can be done once: a later attempt is refused, so a
// key cannot be taken over by whoever can reach the chain's administrative commands.
func (k *Keeper) BindKey(address string, pub ed25519.PublicKey) error {
	if len(pub) != ed25519.PublicKeySize {
		return fmt.Errorf("public key must be %d bytes", ed25519.PublicKeySize)
	}
	idx := k.findAccountIndex(address)
	if idx == -1 {
		return fmt.Errorf("account %q not found", address)
	}
	if len(k.accounts[idx].PublicKey) != 0 {
		return errors.New("a public key is already bound to this account")
	}
	k.accounts[idx].PublicKey = append([]byte(nil), pub...)
	return nil
}

// SendCreditsSigned performs a transfer that the sender authorized with its key.
func (k *Keeper) SendCreditsSigned(s SignedSend) (CreditsTransferredEvent, error) {
	_, err := k.authorize(s.From, s.Nonce, s.Signature, func() []byte {
		return SendSigningBytes(s.From, s.To, s.Amount, s.Nonce)
	})
	if err != nil {
		return CreditsTransferredEvent{}, err
	}
	event, err := k.SendCredits(s.From, s.To, s.Amount)
	if err != nil {
		return CreditsTransferredEvent{}, err
	}
	k.accounts[k.findAccountIndex(s.From)].Nonce = s.Nonce
	return event, nil
}

// BatchSendSigned performs a batch transfer that the sender authorized with its key.
func (k *Keeper) BatchSendSigned(s SignedBatch) (BatchSendEvent, error) {
	_, err := k.authorize(s.From, s.Nonce, s.Signature, func() []byte {
		return BatchSigningBytes(s.From, s.Entries, s.Nonce)
	})
	if err != nil {
		return BatchSendEvent{}, err
	}
	event, err := k.BatchSend(s.From, s.Entries)
	if err != nil {
		return BatchSendEvent{}, err
	}
	k.accounts[k.findAccountIndex(s.From)].Nonce = s.Nonce
	return event, nil
}

// authorize checks the sender, the nonce and the signature. It changes nothing: the nonce is
// recorded only after the transfer itself has succeeded.
func (k *Keeper) authorize(from string, nonce uint64, sig []byte, message func() []byte) (int, error) {
	idx := k.findAccountIndex(from)
	if idx == -1 {
		return -1, fmt.Errorf("sender account %q not found", from)
	}
	acct := k.accounts[idx]
	if len(acct.PublicKey) != ed25519.PublicKeySize {
		return -1, ErrNoKey
	}
	if nonce != acct.Nonce+1 {
		return -1, ErrBadNonce
	}
	if !ed25519.Verify(ed25519.PublicKey(acct.PublicKey), message(), sig) {
		return -1, ErrBadSignature
	}
	return idx, nil
}

func appendField(b []byte, s string) []byte {
	b = appendUint(b, uint64(len(s)))
	return append(b, s...)
}

func appendUint(b []byte, v uint64) []byte {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], v)
	return append(b, buf[:]...)
}
