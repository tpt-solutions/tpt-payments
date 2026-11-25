// Package tptpay is a Go client for the TPT Pay API. It uses only the standard library.
//
// Keys: a secret key (sk_...) may call every endpoint and belongs on the server only. A
// publishable key (pk_...) may retrieve and confirm payment intents, which is what a browser
// checkout needs; other calls are refused before any request is sent.
//
// Every write carries an Idempotency-Key. If the caller does not give one, a key is generated for
// the call and reused across the client's own retries, so a retried payment is not created twice.
package tptpay

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultBaseURL is the production API.
const DefaultBaseURL = "https://api.tpt-payments.com/v1"

// Version is the SDK version, sent in the User-Agent header.
const Version = "0.1.0"

const (
	defaultTimeout    = 30 * time.Second
	defaultMaxRetries = 2
	defaultRetryBase  = 500 * time.Millisecond
	maxBackoff        = 8 * time.Second
	maxRetryAfter     = 60 * time.Second
)

// retryableStatus are the responses worth another attempt. They match the other SDKs.
var retryableStatus = map[int]bool{409: true, 429: true, 500: true, 502: true, 503: true, 504: true}

// Client calls the TPT Pay API. It is safe for concurrent use.
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	maxRetries int
	retryBase  time.Duration

	PaymentIntents *PaymentIntents
	Refunds        *Refunds
	Subscriptions  *Subscriptions
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL points the client at another API, such as a self-hosted deployment or a local server.
func WithBaseURL(u string) Option {
	return func(c *Client) { c.baseURL = strings.TrimRight(u, "/") }
}

// WithHTTPClient replaces the HTTP client. Its timeout applies to each attempt.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.httpClient = h }
}

// WithMaxRetries sets how many times a failed attempt is retried (connection failures and
// retryable responses: 409, 429 and 5xx). The default is 2.
func WithMaxRetries(n int) Option {
	return func(c *Client) { c.maxRetries = n }
}

// WithRetryBase sets the first backoff delay. It doubles each attempt, with jitter, up to 8 seconds.
func WithRetryBase(d time.Duration) Option {
	return func(c *Client) { c.retryBase = d }
}

// New returns a client. It returns an error for a key that is not a TPT Pay key.
func New(apiKey string, opts ...Option) (*Client, error) {
	if !strings.HasPrefix(apiKey, "sk_") && !strings.HasPrefix(apiKey, "pk_") {
		return nil, errors.New("tptpay: apiKey must be a secret (sk_...) or publishable (pk_...) key")
	}
	c := &Client{
		apiKey:     apiKey,
		baseURL:    DefaultBaseURL,
		httpClient: &http.Client{Timeout: defaultTimeout},
		maxRetries: defaultMaxRetries,
		retryBase:  defaultRetryBase,
	}
	for _, opt := range opts {
		opt(c)
	}
	if c.maxRetries < 0 {
		return nil, errors.New("tptpay: max retries must be zero or more")
	}
	c.PaymentIntents = &PaymentIntents{c: c}
	c.Refunds = &Refunds{c: c}
	c.Subscriptions = &Subscriptions{c: c}
	return c, nil
}

// request describes one API call.
type request struct {
	method  string
	path    string
	query   url.Values
	body    any
	secret  bool // needs a secret key
	idemKey string
}

// do performs a request, retrying as the client is configured to, and decodes the JSON response
// into out (which may be nil).
func (c *Client) do(ctx context.Context, r request, out any) error {
	if r.secret && !strings.HasPrefix(c.apiKey, "sk_") {
		return &Error{
			Status:  http.StatusForbidden,
			Type:    "invalid_request_error",
			Code:    "permission_denied",
			Message: fmt.Sprintf("%s %s requires a secret key (sk_...). Publishable keys cannot perform this operation.", r.method, r.path),
		}
	}

	var payload []byte
	if r.body != nil {
		b, err := json.Marshal(r.body)
		if err != nil {
			return fmt.Errorf("tptpay: encode body: %w", err)
		}
		payload = b
	}
	idem := r.idemKey
	if r.method != http.MethodGet && idem == "" {
		idem = newIdempotencyKey()
	}

	target := c.baseURL + r.path
	if len(r.query) > 0 {
		target += "?" + r.query.Encode()
	}

	attempts := c.maxRetries + 1
	for attempt := 0; attempt < attempts; attempt++ {
		last := attempt == attempts-1
		status, body, header, err := c.send(ctx, r.method, target, payload, idem)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if last {
				return &Error{Type: "api_connection_error", Message: fmt.Sprintf("could not reach the TPT Pay API: %v", err)}
			}
			if !sleep(ctx, c.backoff(attempt, nil)) {
				return ctx.Err()
			}
			continue
		}

		if status >= 200 && status < 300 {
			if out == nil || len(body) == 0 {
				return nil
			}
			if err := json.Unmarshal(body, out); err != nil {
				return fmt.Errorf("tptpay: decode response: %w", err)
			}
			return nil
		}

		apiErr := decodeError(status, body, header.Get("X-Request-ID"))
		if retryableStatus[status] && !last {
			if !sleep(ctx, c.backoff(attempt, parseRetryAfter(header.Get("Retry-After"), time.Now()))) {
				return ctx.Err()
			}
			continue
		}
		return apiErr
	}
	return &Error{Type: "api_error", Message: "request failed"} // unreachable
}

func (c *Client) send(ctx context.Context, method, target string, payload []byte, idem string) (int, []byte, http.Header, error) {
	var reader io.Reader
	if payload != nil {
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return 0, nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "tpt-pay-go/"+Version)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if idem != "" {
		req.Header.Set("Idempotency-Key", idem)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return 0, nil, nil, err
	}
	return resp.StatusCode, body, resp.Header, nil
}

// backoff returns the wait before the next attempt. A server's Retry-After wins, capped at 60 s.
func (c *Client) backoff(attempt int, retryAfter *time.Duration) time.Duration {
	if retryAfter != nil {
		if *retryAfter > maxRetryAfter {
			return maxRetryAfter
		}
		return *retryAfter
	}
	base := float64(c.retryBase) * math.Pow(2, float64(attempt))
	if base > float64(maxBackoff) {
		base = float64(maxBackoff)
	}
	jitter := 0.5 + randomFraction()/2
	return time.Duration(base * jitter)
}

// parseRetryAfter reads delay-seconds or an HTTP date. It returns nil when absent or unusable.
func parseRetryAfter(value string, now time.Time) *time.Duration {
	v := strings.TrimSpace(value)
	if v == "" {
		return nil
	}
	if secs, err := strconv.ParseInt(v, 10, 64); err == nil && secs >= 0 {
		d := time.Duration(secs) * time.Second
		return &d
	}
	if when, err := http.ParseTime(v); err == nil {
		d := when.Sub(now)
		if d < 0 {
			d = 0
		}
		return &d
	}
	return nil
}

// sleep waits for d or until ctx is done. It reports whether the wait completed.
func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// newIdempotencyKey returns 128 random bits, hex encoded.
func newIdempotencyKey() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("tptpay: no randomness available: " + err.Error())
	}
	return hex.EncodeToString(b[:])
}

// randomFraction returns a value in [0, 1) from crypto/rand.
func randomFraction() float64 {
	n, err := rand.Int(rand.Reader, big.NewInt(1<<53))
	if err != nil {
		return 0.5
	}
	return float64(n.Int64()) / float64(int64(1)<<53)
}
