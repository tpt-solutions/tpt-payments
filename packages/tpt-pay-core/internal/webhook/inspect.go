package webhook

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/apierrors"
)

// ErrDeliveryNotFound means no delivery with that ID exists in the caller's merchant and mode.
var ErrDeliveryNotFound = errors.New("webhook: delivery not found")

// DeliveryRecord is one delivery as shown to the merchant. The payload and signature are not
// included; the event is available from the event ID.
type DeliveryRecord struct {
	ID             string
	EndpointID     string
	EventType      string
	EventID        string
	Status         string // pending | delivered | failed
	Attempts       int
	LastHTTPStatus *int
	LastError      string
	CreatedAt      time.Time
	DeliveredAt    *time.Time
}

// DeliveryFilter scopes and pages a list of deliveries.
type DeliveryFilter struct {
	MerchantID string
	Livemode   bool
	EndpointID string // optional
	Limit      int    // 1-100
	// StartingAfter is the ID of the last delivery on the previous page.
	StartingAfter string
}

// ListDeliveries returns one page of deliveries, newest first, and whether another page exists.
func ListDeliveries(ctx context.Context, pool *pgxpool.Pool, f DeliveryFilter) ([]DeliveryRecord, bool, error) {
	if f.Limit < 1 || f.Limit > 100 {
		return nil, false, apierrors.InvalidParam("limit", "limit must be between 1 and 100")
	}
	if f.StartingAfter != "" && !validUUID(f.StartingAfter) {
		return nil, false, apierrors.InvalidParam("starting_after", "no delivery with that id")
	}

	var cursorAt any
	var cursorID any
	if f.StartingAfter != "" {
		var createdAt time.Time
		var id string
		err := pool.QueryRow(ctx, `
			SELECT created_at, id::text FROM webhook_deliveries
			WHERE id = $1::uuid AND merchant_id = $2 AND livemode = $3
		`, f.StartingAfter, f.MerchantID, f.Livemode).Scan(&createdAt, &id)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, apierrors.InvalidParam("starting_after", "no delivery with that id")
		}
		if err != nil {
			return nil, false, fmt.Errorf("resolve delivery cursor: %w", err)
		}
		cursorAt, cursorID = createdAt, id
	}

	rows, err := pool.Query(ctx, `
		SELECT d.id::text, d.endpoint_id::text, d.event_type, d.event_id,
		       CASE WHEN d.delivered_at IS NOT NULL THEN 'delivered'
		            WHEN d.failed_at IS NOT NULL THEN 'failed'
		            ELSE 'pending' END,
		       d.attempt_count, d.last_http_status, COALESCE(d.last_error, ''),
		       d.created_at, d.delivered_at
		FROM webhook_deliveries d
		WHERE d.merchant_id = $1
		  AND d.livemode = $2
		  AND ($3::text = '' OR d.endpoint_id::text = $3)
		  AND ($4::timestamptz IS NULL OR (d.created_at, d.id::text) < ($4::timestamptz, $5::text))
		ORDER BY d.created_at DESC, d.id DESC
		LIMIT $6
	`, f.MerchantID, f.Livemode, f.EndpointID, cursorAt, cursorID, f.Limit+1)
	if err != nil {
		return nil, false, fmt.Errorf("list deliveries: %w", err)
	}
	defer rows.Close()

	var out []DeliveryRecord
	for rows.Next() {
		var rec DeliveryRecord
		if err := rows.Scan(&rec.ID, &rec.EndpointID, &rec.EventType, &rec.EventID, &rec.Status,
			&rec.Attempts, &rec.LastHTTPStatus, &rec.LastError, &rec.CreatedAt, &rec.DeliveredAt); err != nil {
			return nil, false, fmt.Errorf("scan delivery: %w", err)
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(out) > f.Limit
	if hasMore {
		out = out[:f.Limit]
	}
	return out, hasMore, nil
}

// Replay sends a delivery again: the same event, with the same event ID, so receivers that
// de-duplicate by TPT-Event-Id see a repeat of an event they may already have. It works for
// delivered and failed deliveries alike. The row is reset to pending and un-published, so the
// poll worker or the JetStream relay picks it up.
func Replay(ctx context.Context, pool *pgxpool.Pool, merchantID string, livemode bool, deliveryID string) error {
	if !validUUID(deliveryID) {
		return ErrDeliveryNotFound
	}
	tag, err := pool.Exec(ctx, `
		UPDATE webhook_deliveries
		SET delivered_at = NULL,
		    failed_at = NULL,
		    attempt_count = 0,
		    last_error = NULL,
		    last_http_status = NULL,
		    next_attempt_at = NOW(),
		    published_at = NULL
		WHERE id = $1::uuid AND merchant_id = $2 AND livemode = $3
	`, deliveryID, merchantID, livemode)
	if err != nil {
		return fmt.Errorf("replay delivery: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrDeliveryNotFound
	}
	return nil
}

// validUUID checks an ID before it is cast in SQL, so a malformed ID is a 404 and not a database error.
func validUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}
