package tptpay

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// PaymentIntent is a payment. Amounts are in the currency's smallest unit (cents for USD).
type PaymentIntent struct {
	ID                 string            `json:"id"`
	Object             string            `json:"object"`
	Amount             int64             `json:"amount"`
	AmountCaptured     int64             `json:"amount_captured,omitempty"`
	Currency           string            `json:"currency"`
	Status             string            `json:"status"`
	CaptureMethod      string            `json:"capture_method"`
	ConfirmationMethod string            `json:"confirmation_method"`
	PaymentMethod      string            `json:"payment_method,omitempty"`
	Metadata           map[string]string `json:"metadata,omitempty"`
	RiskFlags          []string          `json:"risk_flags"`
	Livemode           bool              `json:"livemode"`
	Created            int64             `json:"created"`
}

// CreatePaymentIntentParams are the parameters for creating a payment intent.
type CreatePaymentIntentParams struct {
	Amount             int64             `json:"amount"`
	Currency           string            `json:"currency"`
	CaptureMethod      string            `json:"capture_method,omitempty"`      // automatic (default) or manual
	ConfirmationMethod string            `json:"confirmation_method,omitempty"` // automatic (default) or manual
	Metadata           map[string]string `json:"metadata,omitempty"`
}

// PaymentIntents groups the payment intent calls.
type PaymentIntents struct{ c *Client }

// Create creates a payment intent. Pass an idempotency key with WithIdempotencyKey to make a retry
// across process restarts safe; otherwise one is generated for this call.
func (p *PaymentIntents) Create(ctx context.Context, params CreatePaymentIntentParams, opts ...CallOption) (*PaymentIntent, error) {
	var out PaymentIntent
	err := p.c.do(ctx, request{method: http.MethodPost, path: "/payment_intents", body: params, secret: true, idemKey: callOptions(opts).idempotencyKey}, &out)
	return &out, err
}

// Retrieve fetches a payment intent. A publishable key may retrieve one.
func (p *PaymentIntents) Retrieve(ctx context.Context, id string) (*PaymentIntent, error) {
	var out PaymentIntent
	err := p.c.do(ctx, request{method: http.MethodGet, path: "/payment_intents/" + url.PathEscape(id)}, &out)
	return &out, err
}

// ConfirmParams are the parameters for confirming a payment intent with a payment method.
type ConfirmParams struct {
	PaymentMethod string `json:"payment_method"`
}

// Confirm attaches a payment method and charges it. A publishable key may confirm an intent.
func (p *PaymentIntents) Confirm(ctx context.Context, id string, params ConfirmParams, opts ...CallOption) (*PaymentIntent, error) {
	var out PaymentIntent
	err := p.c.do(ctx, request{method: http.MethodPost, path: "/payment_intents/" + url.PathEscape(id) + "/confirm", body: params, idemKey: callOptions(opts).idempotencyKey}, &out)
	return &out, err
}

// Capture captures an authorized payment. Pass amountToCapture of 0 to capture the full amount.
func (p *PaymentIntents) Capture(ctx context.Context, id string, amountToCapture int64, opts ...CallOption) (*PaymentIntent, error) {
	body := map[string]int64{}
	if amountToCapture > 0 {
		body["amount_to_capture"] = amountToCapture
	}
	var out PaymentIntent
	err := p.c.do(ctx, request{method: http.MethodPost, path: "/payment_intents/" + url.PathEscape(id) + "/capture", body: body, secret: true, idemKey: callOptions(opts).idempotencyKey}, &out)
	return &out, err
}

// Cancel cancels a payment intent that has not been captured.
func (p *PaymentIntents) Cancel(ctx context.Context, id string, opts ...CallOption) (*PaymentIntent, error) {
	var out PaymentIntent
	err := p.c.do(ctx, request{method: http.MethodPost, path: "/payment_intents/" + url.PathEscape(id) + "/cancel", body: map[string]string{}, secret: true, idemKey: callOptions(opts).idempotencyKey}, &out)
	return &out, err
}

// Refund is a refund of a captured payment.
type Refund struct {
	ID            string `json:"id"`
	Object        string `json:"object"`
	Amount        int64  `json:"amount"`
	Currency      string `json:"currency"`
	PaymentIntent string `json:"payment_intent"`
	Status        string `json:"status"`
	Reason        string `json:"reason,omitempty"`
	Livemode      bool   `json:"livemode"`
	Created       int64  `json:"created"`
}

// CreateRefundParams are the parameters for a refund. Amount 0 refunds the whole captured amount.
type CreateRefundParams struct {
	PaymentIntent string `json:"payment_intent"`
	Amount        int64  `json:"amount,omitempty"`
	Reason        string `json:"reason,omitempty"`
}

// Refunds groups the refund calls.
type Refunds struct{ c *Client }

// Create refunds all or part of a captured payment.
func (r *Refunds) Create(ctx context.Context, params CreateRefundParams, opts ...CallOption) (*Refund, error) {
	var out Refund
	err := r.c.do(ctx, request{method: http.MethodPost, path: "/refunds", body: params, secret: true, idemKey: callOptions(opts).idempotencyKey}, &out)
	return &out, err
}

// Retrieve fetches a refund.
func (r *Refunds) Retrieve(ctx context.Context, id string) (*Refund, error) {
	var out Refund
	err := r.c.do(ctx, request{method: http.MethodGet, path: "/refunds/" + url.PathEscape(id), secret: true}, &out)
	return &out, err
}

// Subscription is a recurring charge. Test mode only until a live billing policy is decided.
type Subscription struct {
	ID                 string            `json:"id"`
	Object             string            `json:"object"`
	Status             string            `json:"status"` // active, past_due or canceled
	Amount             int64             `json:"amount"`
	Currency           string            `json:"currency"`
	Interval           string            `json:"interval"` // week, month or year
	PaymentMethod      string            `json:"payment_method"`
	CurrentPeriodStart int64             `json:"current_period_start"`
	CurrentPeriodEnd   int64             `json:"current_period_end"`
	CancelAtPeriodEnd  bool              `json:"cancel_at_period_end"`
	CancelReason       string            `json:"cancel_reason,omitempty"`
	CanceledAt         int64             `json:"canceled_at,omitempty"`
	Metadata           map[string]string `json:"metadata,omitempty"`
	Livemode           bool              `json:"livemode"`
	Created            int64             `json:"created"`
}

// CreateSubscriptionParams are the parameters for a subscription. The first period is charged
// before Create returns.
type CreateSubscriptionParams struct {
	Amount        int64             `json:"amount"`
	Currency      string            `json:"currency"`
	Interval      string            `json:"interval"` // week, month or year
	PaymentMethod string            `json:"payment_method"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

// SubscriptionList is one page of subscriptions.
type SubscriptionList struct {
	Object  string         `json:"object"`
	URL     string         `json:"url"`
	HasMore bool           `json:"has_more"`
	Data    []Subscription `json:"data"`
}

// Subscriptions groups the subscription calls.
type Subscriptions struct{ c *Client }

// Create starts a subscription and charges its first period.
func (s *Subscriptions) Create(ctx context.Context, params CreateSubscriptionParams, opts ...CallOption) (*Subscription, error) {
	var out Subscription
	err := s.c.do(ctx, request{method: http.MethodPost, path: "/subscriptions", body: params, secret: true, idemKey: callOptions(opts).idempotencyKey}, &out)
	return &out, err
}

// Retrieve fetches a subscription.
func (s *Subscriptions) Retrieve(ctx context.Context, id string) (*Subscription, error) {
	var out Subscription
	err := s.c.do(ctx, request{method: http.MethodGet, path: "/subscriptions/" + url.PathEscape(id), secret: true}, &out)
	return &out, err
}

// List returns one page of subscriptions, newest first. Pass a limit of 0 for the default (10).
// Pass startingAfter with the last ID of a page to get the next one.
func (s *Subscriptions) List(ctx context.Context, limit int, startingAfter string) (*SubscriptionList, error) {
	q := url.Values{}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if startingAfter != "" {
		q.Set("starting_after", startingAfter)
	}
	var out SubscriptionList
	err := s.c.do(ctx, request{method: http.MethodGet, path: "/subscriptions", query: q, secret: true}, &out)
	return &out, err
}

// Cancel cancels a subscription now, or at the end of the paid period when atPeriodEnd is true.
func (s *Subscriptions) Cancel(ctx context.Context, id string, atPeriodEnd bool, opts ...CallOption) (*Subscription, error) {
	var out Subscription
	err := s.c.do(ctx, request{method: http.MethodPost, path: "/subscriptions/" + url.PathEscape(id) + "/cancel", body: map[string]bool{"at_period_end": atPeriodEnd}, secret: true, idemKey: callOptions(opts).idempotencyKey}, &out)
	return &out, err
}

// CallOption adjusts one call.
type CallOption func(*callConfig)

type callConfig struct {
	idempotencyKey string
}

// WithIdempotencyKey sets the Idempotency-Key for one write. Use a key that identifies the business
// operation (for example the order ID), so a retry from another process is still safe.
func WithIdempotencyKey(key string) CallOption {
	return func(c *callConfig) { c.idempotencyKey = key }
}

func callOptions(opts []CallOption) callConfig {
	var c callConfig
	for _, o := range opts {
		o(&c)
	}
	return c
}
