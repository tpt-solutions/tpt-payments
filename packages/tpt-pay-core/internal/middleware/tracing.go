package middleware

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// Tracing starts a server span for every request. It continues a trace sent by the caller in
// a W3C traceparent header, so a request can be followed from the integrator's service into
// this one.
//
// The span is named by method and route pattern (for example "POST /v1/refunds"), not by the
// raw path, so IDs in the URL do not multiply span names. Only the method, route and status are
// recorded as attributes: no headers, bodies or query strings.
func Tracing(tp trace.TracerProvider) func(http.Handler) http.Handler {
	tracer := tp.Tracer("tpt-pay-core/http")
	prop := propagation.TraceContext{}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := prop.Extract(r.Context(), propagation.HeaderCarrier(r.Header))
			ctx, span := tracer.Start(ctx, r.Method,
				trace.WithSpanKind(trace.SpanKindServer),
				trace.WithAttributes(attribute.String("http.request.method", r.Method)),
			)
			defer span.End()

			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r.WithContext(ctx))

			// chi fills the route context while routing, which happens inside next.
			route := ""
			if rc := chi.RouteContext(ctx); rc != nil {
				route = rc.RoutePattern()
			}
			if route == "" {
				route = "unmatched"
			}
			span.SetName(r.Method + " " + route)
			span.SetAttributes(
				attribute.String("http.route", route),
				attribute.Int("http.response.status_code", rec.status),
			)
			if rec.status >= http.StatusInternalServerError {
				span.SetStatus(codes.Error, http.StatusText(rec.status))
			}
		})
	}
}
