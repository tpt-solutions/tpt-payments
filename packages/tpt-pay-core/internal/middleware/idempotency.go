package middleware

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/apierrors"
)

// maxIdempotentBody bounds the request body that is hashed and stored. Larger bodies
// are rejected rather than truncated, so the hash always covers the whole request.
const maxIdempotentBody = 1 << 20 // 1 MB

// Idempotency makes mutating requests safe to retry.
//
// Behaviour, for a request carrying an Idempotency-Key header:
//   - The key is claimed with a single atomic INSERT. Only one concurrent request can
//     claim it; others see the claim and wait or replay.
//   - The fingerprint covers method, path and body. Reusing a key for a different
//     request returns 422.
//   - Once the handler has produced a response below 500, that response is stored and
//     replayed to every retry (marked with Idempotent-Replayed: true).
//   - A request that is still running returns 409 to concurrent duplicates.
//   - If the handler returns 5xx or panics, the claim is released so the client can
//     retry. Nothing is stored for a failed attempt.
//
// Requests without the header, and read-only methods, pass through unchanged.
func Idempotency(pool *pgxpool.Pool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			idempKey := r.Header.Get("Idempotency-Key")
			if idempKey == "" || !isMutating(r.Method) {
				next.ServeHTTP(w, r)
				return
			}
			if len(idempKey) > 255 {
				writeError(w, r, apierrors.InvalidParam("Idempotency-Key",
					"Idempotency-Key must be at most 255 characters."))
				return
			}

			ctx := r.Context()
			merchantID := MerchantID(ctx)
			if merchantID == "" {
				next.ServeHTTP(w, r)
				return
			}

			body, err := io.ReadAll(io.LimitReader(r.Body, maxIdempotentBody+1))
			if err != nil {
				if isBodyTooLarge(err) {
					writeError(w, r, bodyTooLarge())
					return
				}
				writeError(w, r, apierrors.InvalidParam("body", "The request body could not be read."))
				return
			}
			if len(body) > maxIdempotentBody {
				writeError(w, r, bodyTooLarge())
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))

			fingerprint := requestFingerprint(r.Method, r.URL.Path, body)

			claim, err := claimKey(ctx, pool, merchantID, idempKey, r.URL.Path, fingerprint)
			if err != nil {
				slog.Error("idempotency claim failed", "request_id", GetRequestID(ctx), "err", err)
				writeError(w, r, apierrors.Internal("Idempotency storage is temporarily unavailable."))
				return
			}

			if !claim.claimed {
				switch {
				case claim.fingerprint != fingerprint:
					writeError(w, r, &apierrors.APIError{
						StatusCode: http.StatusUnprocessableEntity,
						Type:       apierrors.ErrorTypeIdempotency,
						Code:       apierrors.CodeIdempotencyKeyInUse,
						Message:    "This Idempotency-Key was already used with a different request.",
					})
				case claim.completed:
					replay(w, claim)
				default:
					writeError(w, r, &apierrors.APIError{
						StatusCode: http.StatusConflict,
						Type:       apierrors.ErrorTypeIdempotency,
						Code:       apierrors.CodeIdempotencyKeyInUse,
						Message:    "A request with this Idempotency-Key is still in progress. Retry shortly.",
					})
				}
				return
			}

			// We own the key. Release it unless the handler produced a storable response.
			// The deferred release also runs when the handler panics.
			keep := false
			defer func() {
				if !keep {
					releaseKey(context.WithoutCancel(ctx), pool, merchantID, idempKey)
				}
			}()

			rec := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)

			if rec.status >= 500 {
				return // not stored: the client may retry
			}
			keep = true
			if err := completeKey(context.WithoutCancel(ctx), pool, merchantID, idempKey, rec.status, rec.body.Bytes()); err != nil {
				// The response has already been sent. Leave the key claimed so duplicates get
				// 409 rather than running the operation again; an operator can resolve it.
				slog.Error("idempotency response store failed; key left claimed",
					"request_id", GetRequestID(ctx), "err", err)
			}
		})
	}
}

// StartIdempotencyCleanup deletes idempotency records older than maxAge, every interval,
// until ctx is cancelled. It is the expiry job the schema refers to.
func StartIdempotencyCleanup(ctx context.Context, pool *pgxpool.Pool, every, maxAge time.Duration) {
	go func() {
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				tag, err := pool.Exec(ctx,
					`DELETE FROM idempotency_keys WHERE created_at < $1`, time.Now().Add(-maxAge))
				if err != nil {
					slog.Warn("idempotency cleanup failed", "err", err)
					continue
				}
				if n := tag.RowsAffected(); n > 0 {
					slog.Info("idempotency cleanup", "deleted", n)
				}
			}
		}
	}()
}

// claimResult describes the outcome of trying to claim an idempotency key.
type claimResult struct {
	claimed     bool
	fingerprint string
	completed   bool
	status      int
	body        []byte
}

// claimKey atomically claims the key for this request, or reads the existing record.
// Only one concurrent INSERT can succeed for a given (merchant, key).
func claimKey(ctx context.Context, pool *pgxpool.Pool, merchantID, key, path, fingerprint string) (claimResult, error) {
	for attempt := 0; attempt < 3; attempt++ {
		tag, err := pool.Exec(ctx, `
			INSERT INTO idempotency_keys
				(idempotency_key, merchant_id, request_path, request_checksum, locked_at)
			VALUES ($1, $2, $3, $4, NOW())
			ON CONFLICT (merchant_id, idempotency_key) DO NOTHING
		`, key, merchantID, path, fingerprint)
		if err != nil {
			return claimResult{}, err
		}
		if tag.RowsAffected() == 1 {
			return claimResult{claimed: true, fingerprint: fingerprint}, nil
		}

		var (
			stored      claimResult
			status      *int
			body        *string
			completedAt *time.Time
		)
		err = pool.QueryRow(ctx, `
			SELECT request_checksum, response_status, response_body, completed_at
			FROM idempotency_keys
			WHERE merchant_id = $1 AND idempotency_key = $2
		`, merchantID, key).Scan(&stored.fingerprint, &status, &body, &completedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			continue // released between our insert and read: try to claim again
		}
		if err != nil {
			return claimResult{}, err
		}
		if completedAt != nil && status != nil && body != nil {
			stored.completed = true
			stored.status = *status
			stored.body = []byte(*body)
		}
		return stored, nil
	}
	return claimResult{}, errors.New("idempotency key changed repeatedly during claim")
}

// completeKey stores the response exactly as sent, so replays are byte-identical.
func completeKey(ctx context.Context, pool *pgxpool.Pool, merchantID, key string, status int, body []byte) error {
	_, err := pool.Exec(ctx, `
		UPDATE idempotency_keys
		SET response_status = $1, response_body = $2, completed_at = NOW()
		WHERE merchant_id = $3 AND idempotency_key = $4
	`, status, string(body), merchantID, key)
	return err
}

func releaseKey(ctx context.Context, pool *pgxpool.Pool, merchantID, key string) {
	if _, err := pool.Exec(ctx, `
		DELETE FROM idempotency_keys
		WHERE merchant_id = $1 AND idempotency_key = $2 AND completed_at IS NULL
	`, merchantID, key); err != nil {
		slog.Error("idempotency release failed", "err", err)
	}
}

func replay(w http.ResponseWriter, c claimResult) {
	w.Header().Set("Idempotent-Replayed", "true")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(c.status)
	_, _ = w.Write(c.body)
}

// requestFingerprint binds a key to the exact method, path and body of the request.
func requestFingerprint(method, path string, body []byte) string {
	h := sha256.New()
	h.Write([]byte(method))
	h.Write([]byte{0})
	h.Write([]byte(path))
	h.Write([]byte{0})
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}

func isMutating(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}

type responseRecorder struct {
	http.ResponseWriter
	status      int
	body        bytes.Buffer
	wroteHeader bool
}

func (r *responseRecorder) WriteHeader(status int) {
	if !r.wroteHeader {
		r.status = status
		r.wroteHeader = true
	}
	r.ResponseWriter.WriteHeader(status)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}

// isBodyTooLarge reports whether err came from the request-size limit.
func isBodyTooLarge(err error) bool {
	var maxErr *http.MaxBytesError
	return errors.As(err, &maxErr)
}

// bodyTooLarge is the 413 response for a request over the size limit.
func bodyTooLarge() *apierrors.APIError {
	return &apierrors.APIError{
		StatusCode: http.StatusRequestEntityTooLarge,
		Type:       apierrors.ErrorTypeInvalidRequest,
		Message:    "Request body exceeds 1 MB.",
	}
}
