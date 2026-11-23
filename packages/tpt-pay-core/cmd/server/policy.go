package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/config"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/payment"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/policy"
)

// loadPolicy reads and validates the merchant rule file, if one is configured, and installs it on
// the payment service. A file that does not parse stops startup, so a broken rule never runs on
// the first payment.
func loadPolicy(cfg config.PolicyConfig, svc *payment.Service) error {
	if cfg.File == "" {
		return nil
	}
	data, err := os.ReadFile(cfg.File)
	if err != nil {
		return fmt.Errorf("POLICY_FILE: %w", err)
	}
	engine, err := policy.Parse(data)
	if err != nil {
		return fmt.Errorf("POLICY_FILE %s: %w", cfg.File, err)
	}
	svc.SetPolicy(engine)
	slog.Info("merchant rules loaded", "file", cfg.File, "rules", len(engine.Rules()))
	return nil
}
