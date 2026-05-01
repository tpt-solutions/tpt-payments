package webhook_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/internal/webhook"
)

func TestSignAndVerify_RoundTrip(t *testing.T) {
	secret := "whsec_testsecret1234567890abcdef"
	payload := []byte(`{"id":"evt_test","type":"payment_intent.succeeded"}`)
	ts := time.Now()

	header := webhook.Sign(payload, secret, ts)
	err := webhook.Verify(payload, header, secret, 5*time.Minute)
	require.NoError(t, err)
}

func TestVerify_WrongSecret(t *testing.T) {
	payload := []byte(`{"test":"payload"}`)
	ts := time.Now()
	header := webhook.Sign(payload, "secret-a", ts)

	err := webhook.Verify(payload, header, "secret-b", 5*time.Minute)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "signature mismatch")
}

func TestVerify_TamperedPayload(t *testing.T) {
	secret := "whsec_testsecret"
	original := []byte(`{"amount":1000}`)
	ts := time.Now()
	header := webhook.Sign(original, secret, ts)

	tampered := []byte(`{"amount":9999}`)
	err := webhook.Verify(tampered, header, secret, 5*time.Minute)
	require.Error(t, err)
}

func TestVerify_ExpiredTimestamp(t *testing.T) {
	secret := "whsec_test"
	payload := []byte(`{}`)
	old := time.Now().Add(-10 * time.Minute)
	header := webhook.Sign(payload, secret, old)

	err := webhook.Verify(payload, header, secret, 5*time.Minute)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too old")
}

func TestVerify_ZeroTolerance_AlwaysPasses(t *testing.T) {
	// tolerance=0 means skip timestamp check.
	secret := "whsec_test"
	payload := []byte(`{}`)
	ancient := time.Now().Add(-365 * 24 * time.Hour)
	header := webhook.Sign(payload, secret, ancient)

	err := webhook.Verify(payload, header, secret, 0)
	require.NoError(t, err)
}

func TestRetrySchedule(t *testing.T) {
	// Attempt 1 should be immediate.
	assert.Equal(t, time.Duration(0), webhook.RetrySchedule(1))
	// Later attempts should increase.
	assert.Greater(t, int64(webhook.RetrySchedule(3)), int64(webhook.RetrySchedule(2)))
	// After max defined delays, cap at 12 hours.
	assert.Equal(t, 12*time.Hour, webhook.RetrySchedule(100))
}

func TestGenerateSecret(t *testing.T) {
	s1 := webhook.GenerateSecret()
	s2 := webhook.GenerateSecret()
	assert.True(t, len(s1) > 10)
	assert.NotEqual(t, s1, s2, "secrets should be unique")
	assert.Contains(t, s1, "whsec_")
}
