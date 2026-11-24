package adapter

import (
	"context"
	"errors"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Traced wraps a CardNetworkAdapter so that every call to the network is a span. The wrapped
// adapter is unchanged: requests, responses and errors pass through as they are. Only the
// operation name is recorded, never card or request data.
type Traced struct {
	Inner  CardNetworkAdapter
	Tracer trace.Tracer
}

var _ CardNetworkAdapter = Traced{}

// start opens a client span for one adapter operation and returns it with its context.
func (t Traced) start(ctx context.Context, op string) (context.Context, trace.Span) {
	return t.Tracer.Start(ctx, "adapter."+op,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attribute.String("tpt.adapter.operation", op)),
	)
}

// finish records an error on the span. A card decline is a normal outcome, so it is not an
// error on the span; a network or system failure is.
func finish(span trace.Span, err error) {
	if err != nil {
		var decline *CardError
		if !errors.As(err, &decline) {
			span.RecordError(err)
			span.SetStatus(codes.Error, "adapter call failed")
		}
	}
	span.End()
}

func (t Traced) Authorize(ctx context.Context, req AuthRequest) (*AuthResponse, error) {
	ctx, span := t.start(ctx, "authorize")
	resp, err := t.Inner.Authorize(ctx, req)
	finish(span, err)
	return resp, err
}

func (t Traced) Capture(ctx context.Context, req CaptureRequest) (*CaptureResponse, error) {
	ctx, span := t.start(ctx, "capture")
	resp, err := t.Inner.Capture(ctx, req)
	finish(span, err)
	return resp, err
}

func (t Traced) Void(ctx context.Context, req VoidRequest) (*VoidResponse, error) {
	ctx, span := t.start(ctx, "void")
	resp, err := t.Inner.Void(ctx, req)
	finish(span, err)
	return resp, err
}

func (t Traced) Refund(ctx context.Context, req RefundRequest) (*RefundResponse, error) {
	ctx, span := t.start(ctx, "refund")
	resp, err := t.Inner.Refund(ctx, req)
	finish(span, err)
	return resp, err
}

func (t Traced) Tokenize(ctx context.Context, req TokenizeRequest) (*TokenResponse, error) {
	ctx, span := t.start(ctx, "tokenize")
	resp, err := t.Inner.Tokenize(ctx, req)
	finish(span, err)
	return resp, err
}
