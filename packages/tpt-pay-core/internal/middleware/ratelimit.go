package middleware

import (
	"fmt"
	"log/slog"
	"math"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/apierrors"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/ratelimit"
)

// RateLimit limits requests per API key. It must run after RequireAuth, so the key is known.
// Requests without an authenticated key are limited by client address.
//
// If the limiter itself fails (for example Redis is unreachable), the request is allowed and the
// failure is logged. A payments API should not turn a cache outage into a full outage.
// Rejected requests get 429 with Retry-After and the X-RateLimit headers.
func RateLimit(limiter ratelimit.Limiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := rateLimitKey(r)
			d, err := limiter.Allow(r.Context(), key)
			if err != nil {
				slog.Warn("rate limiter unavailable; allowing request",
					"request_id", GetRequestID(r.Context()), "err", err)
				next.ServeHTTP(w, r)
				return
			}

			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(d.Limit))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(d.Remaining))
			if !d.Allowed {
				w.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds(d.RetryAfter)))
				writeError(w, r, &apierrors.APIError{
					StatusCode: http.StatusTooManyRequests,
					Type:       apierrors.ErrorTypeRateLimit,
					Code:       apierrors.CodeRateLimitExceeded,
					Message:    "Too many requests. Retry after the time in the Retry-After header.",
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// rateLimitKey identifies the caller. Authenticated calls are counted against the key ID, which
// is not the secret. Anonymous calls are counted against the client IP.
func rateLimitKey(r *http.Request) string {
	if k := AuthenticatedKey(r.Context()); k != nil {
		return "key:" + k.ID
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return fmt.Sprintf("ip:%s", host)
}

// retryAfterSeconds rounds up, so a client that waits the advertised time is never early.
func retryAfterSeconds(d time.Duration) int {
	s := int(math.Ceil(d.Seconds()))
	if s < 1 {
		s = 1
	}
	return s
}
