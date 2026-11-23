package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/apikey"
)

// auditBodyLimit bounds how much of a response is kept to find the created resource's ID.
// Responses that matter here are small; the cap keeps memory bounded regardless.
const auditBodyLimit = 64 << 10

// Auditor writes one audit_log row per successful state-changing API call.
//
// Each row records who acted (the API key), which merchant and mode it acted in, what
// happened, and which resource it touched. It never records request or response bodies,
// so signing secrets and card-adjacent data stay out of the log.
//
// The row is written after the response is sent. If that write fails the failure is
// logged, not returned, because the state change has already happened and cannot be
// undone by an audit failure. A failed write is therefore a gap to investigate, not a
// silent loss.
type Auditor struct {
	pool *pgxpool.Pool
}

// NewAuditor creates an Auditor that writes to pool. A nil pool disables auditing, which is
// only used by route-table tests.
func NewAuditor(pool *pgxpool.Pool) *Auditor {
	return &Auditor{pool: pool}
}

// Record returns middleware that audits a route. action is the event name (for example
// "payment_intent.capture") and resourceType the kind of object it acts on.
func (a *Auditor) Record(action, resourceType string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if a.pool == nil {
				next.ServeHTTP(w, r)
				return
			}
			rec := &auditRecorder{ResponseWriter: w}
			next.ServeHTTP(rec, r)

			status := rec.status
			if status == 0 {
				status = http.StatusOK
			}
			if status < 200 || status >= 300 {
				return // failed requests change nothing, so there is nothing to audit
			}

			resourceID := chi.URLParam(r, "id")
			if resourceID == "" {
				resourceID = idFromJSON(rec.body.Bytes())
			}
			a.write(r, action, resourceType, resourceID)
		})
	}
}

func (a *Auditor) write(r *http.Request, action, resourceType, resourceID string) {
	// Detach from request cancellation so a client disconnect cannot skip the audit row.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
	defer cancel()

	key := AuthenticatedKey(r.Context())
	actorID := ""
	livemode := false
	if key != nil {
		actorID = key.ID
		livemode = key.Mode == apikey.ModeLive
	}

	metadata, _ := json.Marshal(map[string]any{
		"method":   r.Method,
		"livemode": livemode,
	})

	_, err := a.pool.Exec(ctx, `
		INSERT INTO audit_log
			(merchant_id, actor_type, actor_id, action, resource_type, resource_id,
			 request_id, ip_address, metadata)
		VALUES
			(NULLIF($1, '')::uuid, 'api_key', NULLIF($2, ''), $3, $4, NULLIF($5, ''),
			 NULLIF($6, ''), $7::inet, $8::jsonb)
	`,
		MerchantID(r.Context()), actorID, action, resourceType, resourceID,
		GetRequestID(r.Context()), clientIP(r), string(metadata),
	)
	if err != nil {
		slog.Error("audit write failed; the state change already happened",
			"action", action, "resource_id", resourceID, "request_id", GetRequestID(r.Context()), "err", err)
	}
}

// clientIP returns the connection's address, or nil when it cannot be parsed. Forwarded
// headers are not trusted here (see the router's note on RealIP).
func clientIP(r *http.Request) any {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.String()
	}
	return nil
}

// idFromJSON extracts a top-level "id" string from a JSON object, or "" if there is none.
func idFromJSON(body []byte) string {
	var v struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		return ""
	}
	return v.ID
}

// auditRecorder captures the status and the start of the body of a response.
type auditRecorder struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
}

func (a *auditRecorder) WriteHeader(code int) {
	if a.status == 0 {
		a.status = code
	}
	a.ResponseWriter.WriteHeader(code)
}

func (a *auditRecorder) Write(b []byte) (int, error) {
	if a.status == 0 {
		a.status = http.StatusOK
	}
	if room := auditBodyLimit - a.body.Len(); room > 0 {
		if len(b) > room {
			a.body.Write(b[:room])
		} else {
			a.body.Write(b)
		}
	}
	return a.ResponseWriter.Write(b)
}
