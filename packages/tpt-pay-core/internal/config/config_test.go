package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_ReportsEveryInvalidValue(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("SERVER_READ_TIMEOUT", "forever")
	t.Setenv("WEBHOOK_MAX_ATTEMPTS", "many")

	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SERVER_READ_TIMEOUT")
	assert.Contains(t, err.Error(), "WEBHOOK_MAX_ATTEMPTS")
}

func TestLoad_RequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DATABASE_URL is required")
}

func TestLoad_NATSIsOptional(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("NATS_URL", "")
	_, err := Load()
	assert.NoError(t, err)
}

func TestLoad_RejectsInvertedPoolBounds(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("DATABASE_MIN_CONNS", "50")
	t.Setenv("DATABASE_MAX_CONNS", "10")
	_, err := Load()
	require.Error(t, err)
}

func TestValidateProduction_RefusesDevelopmentSettings(t *testing.T) {
	cfg := &Config{
		App:      AppConfig{Env: "production"},
		Adapter:  AdapterConfig{Type: "sandbox"},
		Database: DatabaseConfig{URL: "postgres://tptpay:tptpay@db:5432/tptpay"},
		Vault:    VaultConfig{Token: "dev-root-token"},
	}
	err := ValidateProduction(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sandbox")
	assert.Contains(t, err.Error(), "tptpay:tptpay")
	assert.Contains(t, err.Error(), "development root token")
}

func TestValidateProduction_AllowsDevelopmentEnvironment(t *testing.T) {
	cfg := &Config{
		App:      AppConfig{Env: "development"},
		Adapter:  AdapterConfig{Type: "sandbox"},
		Database: DatabaseConfig{URL: "postgres://tptpay:tptpay@db:5432/tptpay"},
	}
	assert.NoError(t, ValidateProduction(cfg))
}

func TestLoad_LedgerIntegrityIntervalDefaultsAndValidates(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "1h0m0s", cfg.Ledger.IntegrityInterval.String())

	t.Setenv("LEDGER_INTEGRITY_INTERVAL", "0s")
	_, err = Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "LEDGER_INTEGRITY_INTERVAL")
}
