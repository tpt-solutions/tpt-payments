package tptpay

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// recorder is an API stand-in: responses are served in order, and every request is recorded.
type recorder struct {
	mu        sync.Mutex
	responses []func(w http.ResponseWriter)
	requests  []*http.Request
	bodies    []string
}

func (rec *recorder) handler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		rec.mu.Lock()
		rec.requests = append(rec.requests, r)
		rec.bodies = append(rec.bodies, string(body))
		n := len(rec.requests) - 1
		var respond func(http.ResponseWriter)
		if n < len(rec.responses) {
			respond = rec.responses[n]
		} else {
			respond = func(w http.ResponseWriter) { w.WriteHeader(http.StatusInternalServerError) }
		}
		rec.mu.Unlock()
		respond(w)
	})
}

func jsonResponse(status int, body string, headers ...string) func(http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json")
		for i := 0; i+1 < len(headers); i += 2 {
			w.Header().Set(headers[i], headers[i+1])
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}
}

func newTestClient(t *testing.T, rec *recorder, key string, opts ...Option) *Client {
	t.Helper()
	srv := httptest.NewServer(rec.handler(t))
	t.Cleanup(srv.Close)
	base := []Option{WithBaseURL(srv.URL), WithRetryBase(time.Millisecond)}
	c, err := New(key, append(base, opts...)...)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

const intentJSON = `{"id":"pi_1","object":"payment_intent","amount":1000,"currency":"usd","status":"requires_payment_method","capture_method":"automatic","confirmation_method":"automatic","risk_flags":[],"livemode":false,"created":1}`

func TestNew_RejectsOtherKeys(t *testing.T) {
	if _, err := New("rk_test_abc"); err == nil {
		t.Fatal("a key that is neither secret nor publishable must be refused")
	}
	if _, err := New("pk_test_abc"); err != nil {
		t.Fatalf("a publishable key is accepted: %v", err)
	}
}

func TestPublishableKeyRefusedForServerCallsWithoutSendingAnything(t *testing.T) {
	rec := &recorder{}
	c := newTestClient(t, rec, "pk_test_abc")
	_, err := c.PaymentIntents.Create(context.Background(), CreatePaymentIntentParams{Amount: 1000, Currency: "usd"})
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusForbidden {
		t.Fatalf("want a 403 error, got %v", err)
	}
	if len(rec.requests) != 0 {
		t.Fatal("no request may be sent with a publishable key for a server call")
	}
}

func TestCreateSendsIdempotencyKeyAndDecodesTheIntent(t *testing.T) {
	rec := &recorder{responses: []func(http.ResponseWriter){jsonResponse(200, intentJSON)}}
	c := newTestClient(t, rec, "sk_test_abc")
	pi, err := c.PaymentIntents.Create(context.Background(), CreatePaymentIntentParams{Amount: 1000, Currency: "usd"})
	if err != nil {
		t.Fatal(err)
	}
	if pi.ID != "pi_1" || pi.Status != "requires_payment_method" {
		t.Fatalf("decoded %+v", pi)
	}
	req := rec.requests[0]
	if req.Header.Get("Idempotency-Key") == "" {
		t.Fatal("a write must carry an Idempotency-Key")
	}
	if req.Header.Get("Authorization") != "Bearer sk_test_abc" {
		t.Fatalf("authorization header = %q", req.Header.Get("Authorization"))
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(rec.bodies[0]), &sent); err != nil {
		t.Fatal(err)
	}
	if sent["amount"] != float64(1000) || sent["currency"] != "usd" {
		t.Fatalf("body = %v", sent)
	}
}

func TestRetriesReuseTheIdempotencyKey(t *testing.T) {
	rec := &recorder{responses: []func(http.ResponseWriter){
		jsonResponse(503, `{"error":{"type":"api_error","message":"busy"}}`),
		jsonResponse(503, `{"error":{"type":"api_error","message":"busy"}}`),
		jsonResponse(200, intentJSON),
	}}
	c := newTestClient(t, rec, "sk_test_abc", WithMaxRetries(2))
	if _, err := c.PaymentIntents.Create(context.Background(), CreatePaymentIntentParams{Amount: 1000, Currency: "usd"}); err != nil {
		t.Fatal(err)
	}
	if len(rec.requests) != 3 {
		t.Fatalf("want 3 attempts, got %d", len(rec.requests))
	}
	first := rec.requests[0].Header.Get("Idempotency-Key")
	for i, r := range rec.requests {
		if r.Header.Get("Idempotency-Key") != first {
			t.Fatalf("attempt %d used a different key", i)
		}
	}
}

func TestCallerIdempotencyKeyIsUsedAcrossProcesses(t *testing.T) {
	rec := &recorder{responses: []func(http.ResponseWriter){jsonResponse(200, intentJSON)}}
	c := newTestClient(t, rec, "sk_test_abc")
	if _, err := c.PaymentIntents.Create(context.Background(), CreatePaymentIntentParams{Amount: 1000, Currency: "usd"},
		WithIdempotencyKey("order-42")); err != nil {
		t.Fatal(err)
	}
	if got := rec.requests[0].Header.Get("Idempotency-Key"); got != "order-42" {
		t.Fatalf("Idempotency-Key = %q, want the caller's key", got)
	}
}

func TestReadsCarryNoIdempotencyKey(t *testing.T) {
	rec := &recorder{responses: []func(http.ResponseWriter){jsonResponse(200, intentJSON)}}
	c := newTestClient(t, rec, "pk_test_abc")
	if _, err := c.PaymentIntents.Retrieve(context.Background(), "pi_1"); err != nil {
		t.Fatal(err)
	}
	if rec.requests[0].Header.Get("Idempotency-Key") != "" {
		t.Fatal("a read must not carry an Idempotency-Key")
	}
}

func TestCardDeclineIsACardError(t *testing.T) {
	rec := &recorder{responses: []func(http.ResponseWriter){
		jsonResponse(402, `{"error":{"type":"card_error","code":"card_declined","message":"Your card was declined.","param":""}}`, "X-Request-ID", "req_9"),
	}}
	c := newTestClient(t, rec, "sk_test_abc")
	_, err := c.PaymentIntents.Confirm(context.Background(), "pi_1", ConfirmParams{PaymentMethod: "pm_card_declined"})
	if !IsCardError(err) {
		t.Fatalf("want a card error, got %v", err)
	}
	var apiErr *Error
	errors.As(err, &apiErr)
	if apiErr.Code != "card_declined" || apiErr.RequestID != "req_9" {
		t.Fatalf("error = %+v", apiErr)
	}
	if len(rec.requests) != 1 {
		t.Fatal("a card decline is not retried")
	}
}

func TestNotFoundIsReported(t *testing.T) {
	rec := &recorder{responses: []func(http.ResponseWriter){
		jsonResponse(404, `{"error":{"type":"invalid_request_error","code":"resource_missing","message":"No such refund"}}`),
	}}
	c := newTestClient(t, rec, "sk_test_abc")
	if _, err := c.Refunds.Retrieve(context.Background(), "re_missing"); !IsNotFound(err) {
		t.Fatalf("want not found, got %v", err)
	}
}

func TestRetryAfterIsHonored(t *testing.T) {
	rec := &recorder{responses: []func(http.ResponseWriter){
		jsonResponse(429, `{"error":{"type":"api_error","message":"slow down"}}`, "Retry-After", "1"),
		jsonResponse(200, intentJSON),
	}}
	c := newTestClient(t, rec, "sk_test_abc", WithMaxRetries(1))
	start := time.Now()
	if _, err := c.PaymentIntents.Create(context.Background(), CreatePaymentIntentParams{Amount: 1000, Currency: "usd"}); err != nil {
		t.Fatal(err)
	}
	if waited := time.Since(start); waited < 900*time.Millisecond {
		t.Fatalf("waited %s, want about 1s from Retry-After", waited)
	}
}

func TestConnectionFailureAfterRetries(t *testing.T) {
	c, err := New("sk_test_abc", WithBaseURL("http://127.0.0.1:1"), WithMaxRetries(1), WithRetryBase(time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.PaymentIntents.Retrieve(context.Background(), "pi_1")
	if !IsConnectionError(err) {
		t.Fatalf("want a connection error, got %v", err)
	}
}

func TestSubscriptionsListSendsPagingAndCancelSendsTheFlag(t *testing.T) {
	rec := &recorder{responses: []func(http.ResponseWriter){
		jsonResponse(200, `{"object":"list","has_more":false,"data":[]}`),
		jsonResponse(200, `{"id":"sub_1","object":"subscription","status":"active","cancel_at_period_end":true}`),
	}}
	c := newTestClient(t, rec, "sk_test_abc")
	if _, err := c.Subscriptions.List(context.Background(), 5, "sub_0"); err != nil {
		t.Fatal(err)
	}
	if q := rec.requests[0].URL.Query(); q.Get("limit") != "5" || q.Get("starting_after") != "sub_0" {
		t.Fatalf("query = %v", q)
	}
	sub, err := c.Subscriptions.Cancel(context.Background(), "sub_1", true)
	if err != nil {
		t.Fatal(err)
	}
	if !sub.CancelAtPeriodEnd || !strings.Contains(rec.bodies[1], `"at_period_end":true`) {
		t.Fatalf("cancel body %s, decoded %+v", rec.bodies[1], sub)
	}
}

func TestWebhookSignatureVerification(t *testing.T) {
	secret := "whsec_test_secret"
	payload := []byte(`{"id":"evt_1","type":"payment_intent.succeeded"}`)
	// Vector computed independently (HMAC-SHA256 of "1700000000." + payload, key = secret).
	const vector = "22d8dc182f5c588077683bd378368e29765bfe5df2560112b4f28fef120f0e31"
	at := time.Unix(1700000000, 0)
	header := SignWebhook(payload, secret, at)
	if !strings.HasSuffix(header, vector) {
		t.Fatalf("signature %q does not match the independent vector", header)
	}

	// Verification checks the age against the current time, so use a timestamp that is recent.
	now := time.Now()
	fresh := SignWebhook(payload, secret, now)
	if err := VerifyWebhook(payload, fresh, secret, 0); err != nil {
		t.Fatalf("a fresh, correctly signed event must verify: %v", err)
	}
	if err := VerifyWebhook([]byte(`{"id":"evt_2"}`), fresh, secret, 0); !errors.Is(err, ErrSignature) {
		t.Fatal("a changed body must not verify")
	}
	if err := VerifyWebhook(payload, fresh, "whsec_other", 0); !errors.Is(err, ErrSignature) {
		t.Fatal("the wrong secret must not verify")
	}
	old := SignWebhook(payload, secret, now.Add(-10*time.Minute))
	if err := VerifyWebhook(payload, old, secret, 0); !errors.Is(err, ErrSignature) {
		t.Fatal("a timestamp older than the default tolerance must be refused")
	}
	future := SignWebhook(payload, secret, now.Add(10*time.Minute))
	if err := VerifyWebhook(payload, future, secret, 0); !errors.Is(err, ErrSignature) {
		t.Fatal("a timestamp in the future must be refused")
	}
	if err := VerifyWebhook(payload, "garbage", secret, 0); !errors.Is(err, ErrSignature) {
		t.Fatal("a malformed header must be refused")
	}
}

func TestWebhookSecretRotationAcceptsEitherSignature(t *testing.T) {
	payload := []byte(`{"id":"evt_3"}`)
	now := time.Now()
	oldSig := SignWebhook(payload, "whsec_old", now)
	newSig := SignWebhook(payload, "whsec_new", now)
	both := strings.Split(oldSig, ",")[0] + "," + strings.Split(oldSig, ",")[1] + "," + strings.Split(newSig, ",")[1]
	if err := VerifyWebhook(payload, both, "whsec_new", 0); err != nil {
		t.Fatalf("during rotation the new secret must verify: %v", err)
	}
}
