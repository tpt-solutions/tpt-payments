package config

import (
	"strings"
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

func TestLoad_TrustedProxies(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://u:p@localhost:5432/db?sslmode=disable")

	t.Setenv("TRUSTED_PROXIES", "10.0.0.0/8, 203.0.113.7")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load with valid TRUSTED_PROXIES: %v", err)
	}
	if len(cfg.Server.TrustedProxies) != 2 {
		t.Fatalf("TrustedProxies = %v, want two entries", cfg.Server.TrustedProxies)
	}

	t.Setenv("TRUSTED_PROXIES", "10.0.0.0/99")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "TRUSTED_PROXIES") {
		t.Fatalf("Load with invalid TRUSTED_PROXIES: err = %v, want a TRUSTED_PROXIES error", err)
	}
}

func TestLoad_ChaosRateMustBeAFraction(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://u:p@localhost:5432/db?sslmode=disable")

	t.Setenv("ADAPTER_CHAOS_RATE", "0.25")
	cfg, err := Load()
	require.NoError(t, err)
	assert.InDelta(t, 0.25, cfg.Adapter.ChaosRate, 1e-9)

	for _, bad := range []string{"1.5", "-0.1", "lots"} {
		t.Setenv("ADAPTER_CHAOS_RATE", bad)
		_, err := Load()
		require.Error(t, err, bad)
		assert.Contains(t, err.Error(), "ADAPTER_CHAOS_RATE", bad)
	}
}
