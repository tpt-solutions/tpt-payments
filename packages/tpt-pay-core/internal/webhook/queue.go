package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Enqueue writes one delivery per enabled endpoint that subscribes to eventType, for the
// merchant and mode of the change. It must run in the same transaction as the state change,
// so the event exists if and only if the change committed. It returns the number of
// deliveries created.
//
// object is the resource snapshot, already in its wire representation.
func Enqueue(ctx context.Context, tx pgx.Tx, merchantID string, livemode bool, eventType EventType, object map[string]any) (int, error) {
	eventID := "evt_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	event := map[string]any{
		"id":       eventID,
		"object":   "event",
		"type":     string(eventType),
		"created":  time.Now().Unix(),
		"livemode": livemode,
		"data":     map[string]any{"object": object},
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return 0, fmt.Errorf("marshal event: %w", err)
	}

	tag, err := tx.Exec(ctx, `
		INSERT INTO webhook_deliveries
			(endpoint_id, event_type, event_id, payload, next_attempt_at, merchant_id, livemode)
		SELECT we.id, $1, $2, $3, NOW(), we.merchant_id, we.livemode
		FROM webhook_endpoints we
		WHERE we.merchant_id = $4
		  AND we.livemode = $5
		  AND we.status = 'enabled'
		  AND ('*' = ANY(we.enabled_events) OR $1 = ANY(we.enabled_events))
		ON CONFLICT (endpoint_id, event_id) DO NOTHING
	`, string(eventType), eventID, string(payload), merchantID, livemode)
	if err != nil {
		return 0, fmt.Errorf("enqueue webhook deliveries: %w", err)
	}
	return int(tag.RowsAffected()), nil
}
