package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_RefusesBothPlaintextAndWrappedWebhookKey(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("WEBHOOK_SECRET_KEY", "AAAA")
	t.Setenv("VAULT_WEBHOOK_KEY_CIPHERTEXT", "vault:v1:AAAA")
	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not both")
}

func TestLoad_ReadsVaultTransitSettings(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("VAULT_TRANSIT_KEY", "tpt-webhooks")
	t.Setenv("VAULT_TRANSIT_MOUNT", "")
	t.Setenv("VAULT_WEBHOOK_KEY_CIPHERTEXT", "vault:v1:AAAA")
	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "tpt-webhooks", cfg.Vault.TransitKey)
	assert.Equal(t, "transit", cfg.Vault.TransitMount, "an empty mount falls back to the default")
	assert.Equal(t, "vault:v1:AAAA", cfg.Vault.WrappedWebhookKey)
}

func TestValidateProduction_RequiresWrappedWebhookKey(t *testing.T) {
	cfg := &Config{App: AppConfig{Env: "production"}, Adapter: AdapterConfig{Type: "bank_sponsor"}}
	cfg.Vault.Token = "real-token"
	err := ValidateProduction(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "VAULT_WEBHOOK_KEY_CIPHERTEXT")

	cfg.Vault.WrappedWebhookKey = "vault:v1:AAAA"
	assert.NoError(t, ValidateProduction(cfg))
}
