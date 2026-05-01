// Package apikey handles API key generation, hashing, and authentication.
//
// Keys are never stored in plain text. We store a bcrypt hash and the first
// 12 characters (prefix) for lookup. On authentication, we find candidate
// rows by prefix, then verify the full key against each hash.
package apikey

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
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
	Key      *Key
	RawKey   string // shown to user once, never stored
}

// Generate creates a new API key for the given merchant.
func (s *Service) Generate(ctx context.Context, merchantID, name string, keyType KeyType, mode Mode) (*GenerateResult, error) {
	rawKey := buildRawKey(keyType, mode)
	hash, err := bcrypt.GenerateFromPassword([]byte(rawKey), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash key: %w", err)
	}

	prefix := rawKey[:12] // e.g. "sk_test_abc1"
	id := "key_" + strings.ReplaceAll(uuid.New().String(), "-", "")

	var key Key
	err = s.pool.QueryRow(ctx, `
		INSERT INTO api_keys (id, merchant_id, key_hash, key_prefix, type, mode, name)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, merchant_id, key_prefix, type, mode, name, created_at
	`, id, merchantID, string(hash), prefix, string(keyType), string(mode), name,
	).Scan(&key.ID, &key.MerchantID, &key.Prefix, &key.Type, &key.Mode, &key.Name, &key.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert api key: %w", err)
	}

	return &GenerateResult{Key: &key, RawKey: rawKey}, nil
}

// Authenticate verifies a raw key and returns the associated Key record.
// Updates last_used_at on success.
func (s *Service) Authenticate(ctx context.Context, rawKey string) (*Key, error) {
	if len(rawKey) < 12 {
		return nil, ErrInvalidKey
	}

	prefix := rawKey[:12]

	rows, err := s.pool.Query(ctx, `
		SELECT id, merchant_id, key_hash, key_prefix, type, mode, name, last_used_at, created_at
		FROM api_keys
		WHERE key_prefix = $1 AND revoked_at IS NULL
	`, prefix)
	if err != nil {
		return nil, fmt.Errorf("query api keys: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var key Key
		var hash string
		err := rows.Scan(
			&key.ID, &key.MerchantID, &hash, &key.Prefix,
			&key.Type, &key.Mode, &key.Name, &key.LastUsedAt, &key.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan api key: %w", err)
		}

		if bcrypt.CompareHashAndPassword([]byte(hash), []byte(rawKey)) == nil {
			// Match found — update last_used_at asynchronously (non-blocking).
			go func(id string) {
				_, _ = s.pool.Exec(context.Background(),
					`UPDATE api_keys SET last_used_at = NOW() WHERE id = $1`, id)
			}(key.ID)
			return &key, nil
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return nil, ErrInvalidKey
}

// Revoke marks a key as revoked.
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

// ErrInvalidKey is returned when a key does not match any stored record.
var ErrInvalidKey = errors.New("invalid API key")

func buildRawKey(keyType KeyType, mode Mode) string {
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
		panic("apikey: crypto/rand failed: " + err.Error())
	}
	return prefix + base64.RawURLEncoding.EncodeToString(b)
}
