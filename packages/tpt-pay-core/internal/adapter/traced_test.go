package adapter_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/adapter"
)

// fakeAdapter returns a fixed error from every call.
type fakeAdapter struct{ err error }

func (f fakeAdapter) Authorize(context.Context, adapter.AuthRequest) (*adapter.AuthResponse, error) {
	return &adapter.AuthResponse{}, f.err
}
func (f fakeAdapter) Capture(context.Context, adapter.CaptureRequest) (*adapter.CaptureResponse, error) {
	return &adapter.CaptureResponse{}, f.err
}
func (f fakeAdapter) Void(context.Context, adapter.VoidRequest) (*adapter.VoidResponse, error) {
	return &adapter.VoidResponse{}, f.err
}
func (f fakeAdapter) Refund(context.Context, adapter.RefundRequest) (*adapter.RefundResponse, error) {
	return &adapter.RefundResponse{}, f.err
}
func (f fakeAdapter) Tokenize(context.Context, adapter.TokenizeRequest) (*adapter.TokenResponse, error) {
	return &adapter.TokenResponse{}, f.err
}

func tracedWith(inner adapter.CardNetworkAdapter) (adapter.Traced, *tracetest.SpanRecorder) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	return adapter.Traced{Inner: inner, Tracer: tp.Tracer("test")}, rec
}

func TestTraced_SuccessIsASpanWithOperationAndNoError(t *testing.T) {
	tr, rec := tracedWith(fakeAdapter{})
	_, err := tr.Capture(context.Background(), adapter.CaptureRequest{})
	require.NoError(t, err)

	spans := rec.Ended()
	require.Len(t, spans, 1)
	assert.Equal(t, "adapter.capture", spans[0].Name())
	assert.Equal(t, codes.Unset, spans[0].Status().Code)
}

func TestTraced_CardDeclineIsNotAnErrorOnTheSpan(t *testing.T) {
	decline := &adapter.CardError{Code: "insufficient_funds"}
	tr, rec := tracedWith(fakeAdapter{err: fmt.Errorf("wrapped: %w", decline)})

	_, err := tr.Authorize(context.Background(), adapter.AuthRequest{})
	assert.ErrorIs(t, err, decline, "the error passes through unchanged")

	spans := rec.Ended()
	require.Len(t, spans, 1)
	assert.Equal(t, codes.Unset, spans[0].Status().Code, "a decline is a normal outcome")
}

func TestTraced_NetworkFailureIsAnErrorOnTheSpan(t *testing.T) {
	failure := errors.New("connection reset")
	tr, rec := tracedWith(fakeAdapter{err: failure})

	_, err := tr.Refund(context.Background(), adapter.RefundRequest{})
	assert.ErrorIs(t, err, failure)

	spans := rec.Ended()
	require.Len(t, spans, 1)
	assert.Equal(t, codes.Error, spans[0].Status().Code)
	assert.Equal(t, "adapter.refund", spans[0].Name())
}
