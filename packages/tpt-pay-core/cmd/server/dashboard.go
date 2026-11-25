package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/clock"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/dashauth"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/secretbox"
)

// withDashboardAuth adds the internal dashboard-login routes in front of the API. They are served
// only when DASHBOARD_SERVICE_TOKEN is set (at least 32 characters) and DASHBOARD_TOTP_KEY holds a
// base64 32-byte key for encrypting second-factor secrets. Without the token the routes do not exist.
func withDashboardAuth(pool *pgxpool.Pool, api http.Handler) (http.Handler, error) {
	token := os.Getenv("DASHBOARD_SERVICE_TOKEN")
	if token == "" {
		return api, nil
	}
	if !dashauth.ValidServiceToken(token) {
		return nil, fmt.Errorf("DASHBOARD_SERVICE_TOKEN must be at least %d characters", dashauth.MinServiceTokenLength)
	}
	box, err := secretbox.New(os.Getenv("DASHBOARD_TOTP_KEY"))
	if err != nil {
		return nil, fmt.Errorf("DASHBOARD_TOTP_KEY: %w (generate with: openssl rand -base64 32)", err)
	}
	svc, err := dashauth.NewService(pool, box, clock.System{}, dashauth.DefaultIterations)
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.Handle("/internal/dashboard/", svc.Handler(token))
	mux.Handle("/", api)
	return mux, nil
}
