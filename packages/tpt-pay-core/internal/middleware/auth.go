package middleware

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/apierrors"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/apikey"
)

type authKey struct{}

// AuthenticatedKey retrieves the verified API key from the request context.
// Returns nil if the request was not authenticated (should not happen after
// the RequireAuth middleware has run).
func AuthenticatedKey(ctx context.Context) *apikey.Key {
	k, _ := ctx.Value(authKey{}).(*apikey.Key)
	return k
}

// WithAuthenticatedKey returns a context carrying key as the verified key. Only tests that call
// handlers directly, without RequireAuth, need it.
func WithAuthenticatedKey(ctx context.Context, key *apikey.Key) context.Context {
	return context.WithValue(ctx, authKey{}, key)
}

// MerchantID is a convenience helper that returns the merchant ID from the
// authenticated key in the context.
func MerchantID(ctx context.Context) string {
	if k := AuthenticatedKey(ctx); k != nil {
		return k.MerchantID
	}
	return ""
}

// RequireAuth is middleware that enforces API key authentication.
// It expects a Bearer token in the Authorization header:
//
//	Authorization: Bearer sk_test_...
//
// On success it injects the verified key into the request context.
// Unknown or revoked keys get 401. Infrastructure failures get 500, so a database
// outage is never reported to clients as "your key is invalid".
func RequireAuth(keySvc *apikey.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := extractBearerToken(r)
			if raw == "" {
				writeError(w, r, apierrors.Unauthorized("No API key provided. "+
					"Use the Authorization: Bearer sk_... header."))
				return
			}

			key, err := keySvc.Authenticate(r.Context(), raw)
			if errors.Is(err, apikey.ErrInvalidKey) {
				writeError(w, r, apierrors.Unauthorized("Invalid API key."))
				return
			}
			if err != nil {
				slog.Error("api key authentication failed", "request_id", GetRequestID(r.Context()), "err", err)
				writeError(w, r, apierrors.Internal("Authentication is temporarily unavailable."))
				return
			}

			ctx := context.WithValue(r.Context(), authKey{}, key)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireSecretKey rejects requests authenticated with a publishable key.
// Server-side operations (creating payments, capturing, managing keys and webhooks)
// must use a secret key; publishable keys may be embedded in browsers.
func RequireSecretKey(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := AuthenticatedKey(r.Context())
		if key == nil || key.Type != apikey.TypeSecret {
			writeError(w, r, apierrors.PermissionDenied(
				"This operation requires a secret key (sk_...). Publishable keys (pk_...) cannot perform it."))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}
