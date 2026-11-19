// Package apikey handles API key generation, storage, and authentication.
//
// Keys are 192-bit random values. They are never stored in plain text: the store
// holds a SHA-256 digest of the full key, used both for storage and for lookup
// (a unique index hit per request). The first 12 characters are kept for display only.
//
// Permissions are carried on the key record:
//   - Type: secret keys may perform server-side operations; publishable keys are
//     limited to what a browser checkout needs (enforced in the router).
//   - Mode: test or live. A key can only see data and mint keys in its own mode.
package apikey

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// KeyType is either publishable or secret.
type KeyType string

const (
	TypePublishable KeyType = "publishable"
	TypeSecret      KeyType = "secret"
)

// Mode is test or live.
type Mode string

const (
	ModeTest Mode = "test"
	ModeLive Mode = "live"
)

// Key is a fully resolved API key record (with the merchant association).
type Key struct {
	ID         string
	MerchantID string
	Type       KeyType
	Mode       Mode
	Name       string
	Prefix     string
	LastUsedAt *time.Time
	RevokedAt  *time.Time
	CreatedAt  time.Time
}

// Service manages API key lifecycle.
type Service struct {
	pool *pgxpool.Pool
}

// NewService creates a Service.
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// GenerateResult holds the newly created key and its raw value.
// The raw value is only available at creation time — it cannot be recovered.
type GenerateResult struct {
	Key    *Key
	RawKey string // shown to the caller once, never stored
}

// ErrInvalidKey is returned when a key does not match an active stored record.
var ErrInvalidKey = errors.New("invalid API key")

// Generate creates a new API key for the given merchant.
func (s *Service) Generate(ctx context.Context, merchantID, name string, keyType KeyType, mode Mode) (*GenerateResult, error) {
	rawKey, err := buildRawKey(keyType, mode)
	if err != nil {
		return nil, err
	}
	digest := Digest(rawKey)
	prefix := rawKey[:12]
	id := "key_" + strings.ReplaceAll(uuid.New().String(), "-", "")

	var key Key
	err = s.pool.QueryRow(ctx, `
		INSERT INTO api_keys (id, merchant_id, key_digest, key_prefix, type, mode, name)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, merchant_id, key_prefix, type, mode, name, created_at
	`, id, merchantID, digest, prefix, string(keyType), string(mode), name,
	).Scan(&key.ID, &key.MerchantID, &key.Prefix, &key.Type, &key.Mode, &key.Name, &key.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert api key: %w", err)
	}

	return &GenerateResult{Key: &key, RawKey: rawKey}, nil
}

// Authenticate resolves a raw key to its active record. It returns ErrInvalidKey for
// unknown or revoked keys, and a wrapped error for infrastructure failures, so callers
// can distinguish "reject" (401) from "try again later" (500).
func (s *Service) Authenticate(ctx context.Context, rawKey string) (*Key, error) {
	if !looksLikeKey(rawKey) {
		return nil, ErrInvalidKey
	}

	var key Key
	err := s.pool.QueryRow(ctx, `
		SELECT id, merchant_id, key_prefix, type, mode, name, last_used_at, revoked_at, created_at
		FROM api_keys
		WHERE key_digest = $1 AND revoked_at IS NULL
	`, Digest(rawKey)).Scan(
		&key.ID, &key.MerchantID, &key.Prefix, &key.Type, &key.Mode,
		&key.Name, &key.LastUsedAt, &key.RevokedAt, &key.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInvalidKey
	}
	if err != nil {
		return nil, fmt.Errorf("lookup api key: %w", err)
	}

	s.touchLastUsed(key.ID)
	return &key, nil
}

// touchLastUsed records usage without blocking the request. It is bounded by a timeout
// and detached from the request context, so it cannot outlive the server indefinitely.
func (s *Service) touchLastUsed(id string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if _, err := s.pool.Exec(ctx, `UPDATE api_keys SET last_used_at = NOW() WHERE id = $1`, id); err != nil {
			slog.Warn("record api key usage failed", "key_id", id, "err", err)
		}
	}()
}

// Revoke marks a key as revoked. It returns pgx.ErrNoRows if no active key matches.
func (s *Service) Revoke(ctx context.Context, id, merchantID string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE api_keys SET revoked_at = NOW() WHERE id = $1 AND merchant_id = $2 AND revoked_at IS NULL`,
		id, merchantID,
	)
	if err != nil {
		return fmt.Errorf("revoke key: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// Digest returns the storage digest of a raw key (hex SHA-256).
func Digest(rawKey string) string {
	sum := sha256.Sum256([]byte(rawKey))
	return hex.EncodeToString(sum[:])
}

// looksLikeKey rejects values that cannot be keys before touching the database.
func looksLikeKey(raw string) bool {
	if len(raw) < 12 || len(raw) > 200 {
		return false
	}
	return strings.HasPrefix(raw, "sk_") || strings.HasPrefix(raw, "pk_")
}

func buildRawKey(keyType KeyType, mode Mode) (string, error) {
	var prefix string
	switch keyType {
	case TypePublishable:
		prefix = "pk_"
	default:
		prefix = "sk_"
	}
	switch mode {
	case ModeLive:
		prefix += "live_"
	default:
		prefix += "test_"
	}

	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate api key: %w", err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(b), nil
}
