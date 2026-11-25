package middleware

import (
	"context"

	"go.opentelemetry.io/otel/trace"
)

// LogAttrs returns the correlation attributes for a log line written while handling a request:
// the request ID, and the trace and span IDs when the request is being traced. A log entry with
// a trace ID can be matched to the span that the collector received for the same request.
func LogAttrs(ctx context.Context) []any {
	attrs := []any{"request_id", GetRequestID(ctx)}
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		attrs = append(attrs, "trace_id", sc.TraceID().String(), "span_id", sc.SpanID().String())
	}
	return attrs
}
