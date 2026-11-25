package middleware

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/trace"
)

func TestLogAttrsIncludesTraceIDsWhenTraced(t *testing.T) {
	traceID, _ := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	spanID, _ := trace.SpanIDFromHex("00f067aa0ba902b7")
	sc := trace.NewSpanContext(trace.SpanContextConfig{TraceID: traceID, SpanID: spanID, TraceFlags: trace.FlagsSampled})
	ctx := trace.ContextWithSpanContext(context.Background(), sc)

	attrs := LogAttrs(ctx)
	assert.Contains(t, attrs, "trace_id")
	assert.Contains(t, attrs, "4bf92f3577b34da6a3ce929d0e0e4736")
	assert.Contains(t, attrs, "00f067aa0ba902b7")
}

func TestLogAttrsOmitsTraceIDsWithoutATrace(t *testing.T) {
	attrs := LogAttrs(context.Background())
	assert.Len(t, attrs, 2, "only request_id when there is no trace")
	assert.NotContains(t, attrs, "trace_id")
}
