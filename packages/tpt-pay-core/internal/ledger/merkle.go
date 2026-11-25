package ledger

import (
	"crypto/sha256"
	"encoding/binary"
	"sort"
	"strconv"
	"time"

	"github.com/google/uuid"
)

// Merkle commitments over ledger transactions.
//
// Each transaction is hashed to a leaf over a canonical encoding (every field is
// length-prefixed, entries are sorted), so the same logical transaction always gives the
// same leaf. Leaves and inner nodes use different one-byte prefixes, so a leaf can never be
// read as a node. An odd node at the end of a level is promoted unchanged, not duplicated,
// so two different leaf lists cannot share a root by repeating the last leaf.
//
// The root commits to the ordered list of transactions in an audit pack. Publishing that
// root somewhere the operator does not control is what makes it an anchor.

const (
	leafPrefix = 0x00
	nodePrefix = 0x01
)

// Hash is a SHA-256 digest.
type Hash [sha256.Size]byte

// RecordEntry is the committed form of one ledger entry.
type RecordEntry struct {
	AccountID uuid.UUID
	Amount    int64
	Currency  string
}

// Record is the committed form of one ledger transaction, as it appears in an audit pack.
type Record struct {
	ID          uuid.UUID
	Type        string
	Status      string
	ReferenceID string
	CreatedAt   time.Time
	Entries     []RecordEntry
}

// LeafHash returns the leaf for a record. Entry order does not affect the result.
func LeafHash(r Record) Hash {
	entries := make([]RecordEntry, len(r.Entries))
	copy(entries, r.Entries)
	sort.Slice(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if c := compareBytes(a.AccountID[:], b.AccountID[:]); c != 0 {
			return c < 0
		}
		if a.Amount != b.Amount {
			return a.Amount < b.Amount
		}
		return a.Currency < b.Currency
	})

	h := sha256.New()
	h.Write([]byte{leafPrefix})
	writeField(h, r.ID[:])
	writeField(h, []byte(r.Type))
	writeField(h, []byte(r.Status))
	writeField(h, []byte(r.ReferenceID))
	writeField(h, []byte(r.CreatedAt.UTC().Format(time.RFC3339Nano)))
	writeField(h, []byte(strconv.Itoa(len(entries))))
	for _, e := range entries {
		writeField(h, e.AccountID[:])
		writeField(h, []byte(strconv.FormatInt(e.Amount, 10)))
		writeField(h, []byte(e.Currency))
	}
	var out Hash
	copy(out[:], h.Sum(nil))
	return out
}

// MerkleRoot returns the root over the given leaves, in order. An empty list has the root
// of the empty string, so an empty period still has a well-defined commitment.
func MerkleRoot(leaves []Hash) Hash {
	if len(leaves) == 0 {
		return sha256.Sum256(nil)
	}
	level := leaves
	for len(level) > 1 {
		next := make([]Hash, 0, (len(level)+1)/2)
		for i := 0; i < len(level); i += 2 {
			if i+1 == len(level) {
				next = append(next, level[i]) // promote the unpaired node unchanged
				break
			}
			next = append(next, nodeHash(level[i], level[i+1]))
		}
		level = next
	}
	return level[0]
}

func nodeHash(left, right Hash) Hash {
	h := sha256.New()
	h.Write([]byte{nodePrefix})
	h.Write(left[:])
	h.Write(right[:])
	var out Hash
	copy(out[:], h.Sum(nil))
	return out
}

// writeField writes a length-prefixed field, so adjacent fields cannot be confused.
func writeField(h interface{ Write([]byte) (int, error) }, b []byte) {
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], uint64(len(b)))
	_, _ = h.Write(n[:])
	_, _ = h.Write(b)
}

func compareBytes(a, b []byte) int {
	for i := range a {
		if a[i] != b[i] {
			if a[i] < b[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}
