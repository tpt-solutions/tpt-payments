package middleware

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Idempotency enforces idempotency for mutating API requests.
//
// Behaviour:
//   - Requests without an Idempotency-Key header pass through unchanged.
//   - On first request: lock the key, process, store response.
//   - On retry with same key + same body: return the stored response.
//   - On retry with same key + different body: return 422 conflict.
//   - On concurrent duplicate request: return 409 until first completes.
func Idempotency(pool *pgxpool.Pool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			idempKey := r.Header.Get("Idempotency-Key")
			if idempKey == "" {
				next.ServeHTTP(w, r)
				return
			}
			if len(idempKey) > 255 {
				writeError(w, r, &idempotencyError{"idempotency_key too long (max 255 characters)"})
				return
			}

			merchantID := MerchantID(r.Context())
			if merchantID == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Buffer the request body so we can hash it and re-read it.
			body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1 MB limit
			if err != nil {
				writeError(w, r, fmt.Errorf("read body: %w", err))
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))

			checksum := sha256Body(body)

			// Check for existing idempotency record.
			var (
				storedChecksum string
				responseStatus *int
				responseBody   *json.RawMessage
				lockedAt       *time.Time
				completedAt    *time.Time
			)
			err = pool.QueryRow(r.Context(), `
				SELECT request_checksum, response_status, response_body, locked_at, completed_at
				FROM idempotency_keys
				WHERE merchant_id = $1 AND idempotency_key = $2
			`, merchantID, idempKey).Scan(
				&storedChecksum, &responseStatus, &responseBody, &lockedAt, &completedAt,
			)

			if err == nil {
				// Record exists.
				if storedChecksum != checksum {
					writeError(w, r, &idempotencyError{
						"Idempotency-Key already used with a different request body.",
					})
					return
				}
				if completedAt != nil && responseStatus != nil && responseBody != nil {
					// Replay the stored response.
					w.Header().Set("Idempotent-Replayed", "true")
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(*responseStatus)
					_, _ = w.Write(*responseBody)
					return
				}
				if lockedAt != nil && completedAt == nil {
					http.Error(w, `{"error":{"type":"idempotency_error","message":"Request in flight — retry after a moment."}}`, http.StatusConflict)
					return
				}
			} else {
				// Insert a new record and lock it.
				_, err = pool.Exec(r.Context(), `
					INSERT INTO idempotency_keys
						(idempotency_key, merchant_id, request_path, request_checksum, locked_at)
					VALUES ($1, $2, $3, $4, NOW())
					ON CONFLICT (merchant_id, idempotency_key) DO NOTHING
				`, idempKey, merchantID, r.URL.Path, checksum)
				if err != nil {
					next.ServeHTTP(w, r)
					return
				}
			}

			// Capture the response so we can store it.
			rec := &responseRecorder{ResponseWriter: w, status: 200}
			next.ServeHTTP(rec, r)

			// Store the response (best-effort — don't fail the request if this fails).
			body2 := rec.body.Bytes()
			_, _ = pool.Exec(context.Background(), `
				UPDATE idempotency_keys
				SET response_status = $1, response_body = $2, completed_at = NOW()
				WHERE merchant_id = $3 AND idempotency_key = $4
			`, rec.status, body2, merchantID, idempKey)
		})
	}
}

type idempotencyError struct{ msg string }

func (e *idempotencyError) Error() string { return e.msg }

type responseRecorder struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
}

func (r *responseRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}

func sha256Body(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
