package apikey

import (
	"context"
	"errors"
	"fmt"
)

// ErrCursorNotFound is returned when the starting_after key is not a key of this merchant in
// this mode.
var ErrCursorNotFound = errors.New("starting_after key not found")

// List returns one page of a merchant's keys in one mode, newest first. Revoked keys are
// included, with RevokedAt set, so the list is a complete record. Key hashes are never read.
// hasMore reports whether another page exists after this one.
//
// afterID, when set, must be a key in the same merchant and mode; otherwise ErrCursorNotFound.
func (s *Service) List(ctx context.Context, merchantID string, mode Mode, afterID string, limit int) (keys []Key, hasMore bool, err error) {
	if afterID != "" {
		var exists bool
		if err := s.pool.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM api_keys WHERE id = $1 AND merchant_id = $2 AND mode = $3)
		`, afterID, merchantID, string(mode)).Scan(&exists); err != nil {
			return nil, false, fmt.Errorf("check cursor: %w", err)
		}
		if !exists {
			return nil, false, ErrCursorNotFound
		}
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, merchant_id, type, mode, COALESCE(name, ''), key_prefix, last_used_at, revoked_at, created_at
		FROM api_keys
		WHERE merchant_id = $1 AND mode = $2
		  AND ($3 = '' OR (created_at, id) < (
		      SELECT created_at, id FROM api_keys WHERE id = $3 AND merchant_id = $1 AND mode = $2))
		ORDER BY created_at DESC, id DESC
		LIMIT $4
	`, merchantID, string(mode), afterID, limit+1)
	if err != nil {
		return nil, false, fmt.Errorf("list keys: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var k Key
		var keyType, keyMode string
		if err := rows.Scan(&k.ID, &k.MerchantID, &keyType, &keyMode, &k.Name, &k.Prefix,
			&k.LastUsedAt, &k.RevokedAt, &k.CreatedAt); err != nil {
			return nil, false, fmt.Errorf("scan key: %w", err)
		}
		k.Type = KeyType(keyType)
		k.Mode = Mode(keyMode)
		keys = append(keys, k)
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("read keys: %w", err)
	}
	if len(keys) > limit {
		keys = keys[:limit]
		hasMore = true
	}
	return keys, hasMore, nil
}
