package webhook

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/internal/secretbox"
)

// leaseDuration is how long a claimed delivery is hidden from other workers. If a worker
// dies mid-send, the delivery becomes due again after the lease, so delivery is
// at-least-once. Receivers de-duplicate using the TPT-Event-Id header.
const leaseDuration = 2 * time.Minute

// Worker sends queued webhook deliveries, signing each with its endpoint's secret and
// retrying failures on the RetrySchedule. Several workers may run concurrently.
type Worker struct {
	pool        *pgxpool.Pool
	box         *secretbox.Box
	liveClient  *http.Client
	testClient  *http.Client
	maxAttempts int
	window      time.Duration
	batchSize   int
	now         func() time.Time
}

// WorkerConfig controls retry policy and throughput.
type WorkerConfig struct {
	MaxAttempts int           // default 15
	Window      time.Duration // default 72h: deliveries older than this stop retrying
	BatchSize   int           // default 20
}

// NewWorker builds a Worker. Live endpoints are dialled with the strict address policy;
// test endpoints also reach loopback addresses so local receivers work.
func NewWorker(pool *pgxpool.Pool, box *secretbox.Box, cfg WorkerConfig) *Worker {
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 15
	}
	if cfg.Window <= 0 {
		cfg.Window = 72 * time.Hour
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 20
	}
	return &Worker{
		pool:        pool,
		box:         box,
		liveClient:  newClient(true),
		testClient:  newClient(false),
		maxAttempts: cfg.MaxAttempts,
		window:      cfg.Window,
		batchSize:   cfg.BatchSize,
		now:         time.Now,
	}
}

func newClient(live bool) *http.Client {
	dialer := &net.Dialer{
		Timeout: 5 * time.Second,
		// Control runs after DNS resolution, on the address that will actually be dialled.
		// Checking here (not on the hostname) defeats DNS rebinding.
		Control: func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			ip := net.ParseIP(host)
			if ip == nil {
				return errors.New("webhook: destination is not an IP address")
			}
			if blockedIP(ip, live) {
				return fmt.Errorf("webhook: destination %s is not allowed", ip)
			}
			return nil
		},
	}
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DialContext: dialer.DialContext,
			Proxy:       nil, // never route webhooks through an environment proxy
		},
		// Redirects are not followed: a redirect could point somewhere the URL check never saw.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// RunOnce claims up to BatchSize due deliveries and attempts each one. It returns how many
// were processed.
func (w *Worker) RunOnce(ctx context.Context) (int, error) {
	claimed, err := w.claim(ctx)
	if err != nil {
		return 0, err
	}
	for _, d := range claimed {
		w.attempt(ctx, d)
	}
	return len(claimed), nil
}

// Run calls RunOnce every interval until ctx is cancelled.
func (w *Worker) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := w.RunOnce(ctx); err != nil {
				slog.Error("webhook worker pass failed", "err", err)
			}
		}
	}
}

type claimedDelivery struct {
	id              string
	payload         string
	eventID         string
	attemptCount    int
	createdAt       time.Time
	url             string
	secretCiphertxt string
	livemode        bool
	endpointStatus  string
}

// claim leases due deliveries in one statement. FOR UPDATE SKIP LOCKED lets concurrent
// workers take disjoint rows without waiting on each other.
func (w *Worker) claim(ctx context.Context) ([]claimedDelivery, error) {
	// Due-ness and the lease are both judged by the database clock, so workers on
	// different machines agree on what is due.
	rows, err := w.pool.Query(ctx, `
		WITH due AS (
			SELECT id FROM webhook_deliveries
			WHERE delivered_at IS NULL AND failed_at IS NULL AND next_attempt_at <= NOW()
			ORDER BY next_attempt_at
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE webhook_deliveries d
		SET next_attempt_at = NOW() + $2::interval
		FROM due, webhook_endpoints we
		WHERE d.id = due.id AND we.id = d.endpoint_id
		RETURNING d.id::text, d.payload, d.event_id, d.attempt_count, d.created_at,
		          we.url, we.secret_ciphertext, d.livemode, we.status
	`, w.batchSize, fmt.Sprintf("%d seconds", int(leaseDuration.Seconds())))
	if err != nil {
		return nil, fmt.Errorf("claim deliveries: %w", err)
	}
	defer rows.Close()

	var out []claimedDelivery
	for rows.Next() {
		var d claimedDelivery
		if err := rows.Scan(&d.id, &d.payload, &d.eventID, &d.attemptCount, &d.createdAt,
			&d.url, &d.secretCiphertxt, &d.livemode, &d.endpointStatus); err != nil {
			return nil, fmt.Errorf("scan delivery: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (w *Worker) attempt(ctx context.Context, d claimedDelivery) {
	now := w.now()
	attempt := d.attemptCount + 1

	if d.endpointStatus != "enabled" {
		w.finish(ctx, d, attempt, now, 0, "endpoint is disabled", true)
		return
	}
	secretBytes, err := w.box.Open(d.secretCiphertxt)
	if err != nil {
		w.finish(ctx, d, attempt, now, 0, "signing secret cannot be decrypted; recreate the endpoint", true)
		return
	}

	status, sendErr := w.send(ctx, d, string(secretBytes), now)
	if sendErr == nil && status >= 200 && status < 300 {
		w.markDelivered(ctx, d.id, attempt, now, status)
		return
	}

	reason := fmt.Sprintf("HTTP %d", status)
	if sendErr != nil {
		reason = sendErr.Error()
	}
	exhausted := attempt >= w.maxAttempts || now.Sub(d.createdAt) > w.window
	w.finish(ctx, d, attempt, now, status, reason, exhausted)
}

func (w *Worker) send(ctx context.Context, d claimedDelivery, secret string, now time.Time) (int, error) {
	client := w.testClient
	if d.livemode {
		client = w.liveClient
	}
	body := []byte(d.payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.url, strings.NewReader(d.payload))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "tpt-pay-webhooks/1")
	req.Header.Set("TPT-Event-Id", d.eventID)
	req.Header.Set(SignatureHeader, Sign(body, secret, now))

	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	return resp.StatusCode, nil
}

func (w *Worker) markDelivered(ctx context.Context, id string, attempt int, now time.Time, status int) {
	if _, err := w.pool.Exec(ctx, `
		UPDATE webhook_deliveries
		SET delivered_at = $1, attempt_count = $2, last_http_status = $3, last_error = NULL
		WHERE id = $4
	`, now, attempt, status, id); err != nil {
		slog.Error("record webhook delivery failed", "delivery_id", id, "err", err)
	}
}

// finish records a failed attempt. Unless exhausted, it schedules the next attempt on the
// RetrySchedule. If exhausted, the delivery is marked failed and no longer retried.
func (w *Worker) finish(ctx context.Context, d claimedDelivery, attempt int, now time.Time, status int, reason string, exhausted bool) {
	nextAt := now.Add(RetrySchedule(attempt + 1))
	var failedAt *time.Time
	if exhausted {
		failedAt = &now
	}
	if _, err := w.pool.Exec(ctx, `
		UPDATE webhook_deliveries
		SET attempt_count = $1, last_http_status = NULLIF($2, 0), last_error = $3,
		    next_attempt_at = $4, failed_at = $5
		WHERE id = $6
	`, attempt, status, reason, nextAt, failedAt, d.id); err != nil {
		slog.Error("record webhook failure failed", "delivery_id", d.id, "err", err)
	}
}
