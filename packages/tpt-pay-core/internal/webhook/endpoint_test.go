package webhook_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/webhook"
)

func TestValidateEndpointURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		live bool
		ok   bool
	}{
		{"live https public", "https://merchant.example.com/hooks", true, true},
		{"live http rejected", "http://merchant.example.com/hooks", true, false},
		{"test http localhost allowed", "http://localhost:3000/hook", false, true},
		{"test http public rejected", "http://merchant.example.com/hook", false, false},
		{"test https loopback allowed", "https://127.0.0.1/hook", false, true},
		{"live loopback rejected", "https://127.0.0.1/hook", true, false},
		{"live localhost rejected", "https://localhost/hook", true, false},
		{"live private rejected", "https://10.0.0.5/hook", true, false},
		{"test private allowed", "https://10.0.0.5/hook", false, true},
		{"link-local metadata rejected in test", "https://169.254.169.254/latest", false, false},
		{"link-local metadata rejected in live", "https://169.254.169.254/latest", true, false},
		{"credentials rejected", "https://user:pass@merchant.example.com/hook", true, false},
		{"ftp rejected", "ftp://merchant.example.com/hook", true, false},
		{"no host rejected", "https:///hook", true, false},
		{"empty rejected", "", true, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := webhook.ValidateEndpointURL(tc.url, tc.live)
			if tc.ok {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestValidateEvents(t *testing.T) {
	assert.NoError(t, webhook.ValidateEvents([]string{"*"}))
	assert.NoError(t, webhook.ValidateEvents([]string{"payment_intent.succeeded"}))
	assert.Error(t, webhook.ValidateEvents([]string{"payment_intent.typo"}))
	assert.Error(t, webhook.ValidateEvents(nil))
}
