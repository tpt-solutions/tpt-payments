package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/internal/apikey"
	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/internal/apierrors"
)

type authKey struct{}

// AuthenticatedKey retrieves the verified API key from the request context.
// Returns nil if the request was not authenticated (should not happen after
// the RequireAuth middleware has run).
func AuthenticatedKey(ctx context.Context) *apikey.Key {
	k, _ := ctx.Value(authKey{}).(*apikey.Key)
	return k
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
			if err != nil {
				writeError(w, r, apierrors.Unauthorized("Invalid API key."))
				return
			}

			ctx := context.WithValue(r.Context(), authKey{}, key)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
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
