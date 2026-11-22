package api_test

// Audit log tests. Skipped unless TEST_DATABASE_URL is set (see internal/testdb).

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/internal/apikey"
)

// auditRow is the subset of audit_log these tests check.
type auditRow struct {
	Action       string
	ResourceType string
	ResourceID   string
	ActorType    string
	ActorID      string
	RequestID    string
}

func (s *stack) auditRows(t *testing.T, action string) []auditRow {
	t.Helper()
	rows, err := s.pool.Query(context.Background(), `
		SELECT action, resource_type, COALESCE(resource_id, ''), actor_type,
		       COALESCE(actor_id, ''), COALESCE(request_id, '')
		FROM audit_log
		WHERE merchant_id = $1 AND action = $2
		ORDER BY created_at
	`, s.merchantID, action)
	require.NoError(t, err)
	defer rows.Close()

	var out []auditRow
	for rows.Next() {
		var r auditRow
		require.NoError(t, rows.Scan(&r.Action, &r.ResourceType, &r.ResourceID, &r.ActorType,
			&r.ActorID, &r.RequestID))
		out = append(out, r)
	}
	require.NoError(t, rows.Err())
	return out
}

func TestAudit_SuccessfulCreateAndCaptureAreRecorded(t *testing.T) {
	s := newStack(t)
	secret := s.key(t, apikey.TypeSecret, apikey.ModeTest)
	id := createIntent(t, s, secret)

	created := s.auditRows(t, "payment_intent.create")
	require.Len(t, created, 1)
	assert.Equal(t, "payment_intent", created[0].ResourceType)
	assert.Equal(t, id, created[0].ResourceID, "creates record the new resource's ID")
	assert.Equal(t, "api_key", created[0].ActorType)
	assert.NotEmpty(t, created[0].ActorID, "the acting key is recorded")
	assert.NotEmpty(t, created[0].RequestID, "the request ID correlates the row with logs")

	// Confirm the intent: confirm is open to any key, and it is audited too.
	rec := s.do(http.MethodPost, "/v1/payment_intents/"+id+"/confirm", secret, `{"payment_method":"pm_card_visa"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	rows := s.auditRows(t, "payment_intent.confirm")
	require.Len(t, rows, 1)
	assert.Equal(t, id, rows[0].ResourceID)
}

func TestAudit_FailedRequestsAreNotRecorded(t *testing.T) {
	s := newStack(t)
	secret := s.key(t, apikey.TypeSecret, apikey.ModeTest)

	rec := s.do(http.MethodPost, "/v1/payment_intents/pi_missing/capture", secret, "")
	require.Equal(t, http.StatusNotFound, rec.Code)
	assert.Empty(t, s.auditRows(t, "payment_intent.capture"),
		"a request that changed nothing must not be audited as if it did")
}

func TestAudit_ReadRoutesAreNotRecorded(t *testing.T) {
	s := newStack(t)
	secret := s.key(t, apikey.TypeSecret, apikey.ModeTest)
	id := createIntent(t, s, secret)

	rec := s.do(http.MethodGet, "/v1/payment_intents/"+id, secret, "")
	require.Equal(t, http.StatusOK, rec.Code)
	var n int
	require.NoError(t, s.pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM audit_log WHERE merchant_id = $1 AND action LIKE 'payment_intent.%' AND action <> 'payment_intent.create'`,
		s.merchantID).Scan(&n))
	assert.Equal(t, 0, n, "reads are not state changes and are not audited")
}

func TestAudit_NeverStoresKeyMaterial(t *testing.T) {
	s := newStack(t)
	secret := s.key(t, apikey.TypeSecret, apikey.ModeTest)

	// Minting a key returns its secret once. The audit row must hold the key's ID only.
	rec := s.do(http.MethodPost, "/v1/api_keys", secret, `{"name":"ci","type":"secret","mode":"test"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var created map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	newID, _ := created["id"].(string)
	require.NotEmpty(t, newID)

	rows := s.auditRows(t, "api_key.create")
	require.Len(t, rows, 1)
	assert.Equal(t, newID, rows[0].ResourceID)

	var dump string
	require.NoError(t, s.pool.QueryRow(context.Background(),
		`SELECT COALESCE(string_agg(row_to_json(audit_log)::text, ' '), '') FROM audit_log WHERE merchant_id = $1`,
		s.merchantID).Scan(&dump))
	assert.False(t, strings.Contains(dump, secret), "the raw API key must never be written to the audit log")
	assert.False(t, strings.Contains(dump, "sk_test_"), "no secret key may appear in the audit log")
}
