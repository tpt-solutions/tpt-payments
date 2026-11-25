// Package app — Prometheus metrics for the TPT Credit chain.
//
// This file exposes a /metrics HTTP endpoint for Prometheus scraping.
// It collects chain-level metrics: block height, human count, supply,
// balances, events, burn statistics, and validator info.
package app

import (
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

// MetricsCollector holds Prometheus-style metrics for the chain.
// Metrics are updated on each block and served via HTTP GET /metrics.
type MetricsCollector struct {
	mu sync.RWMutex

	// Chain metadata
	ChainID       string `json:"-"`
	AppVersion    string `json:"-"`

	// Gauge metrics (values that go up and down)
	BlockHeight       int64   `json:"block_height"`
	HumanCount        int64   `json:"human_count"`
	SupplyUcredit     float64 `json:"supply_ucredit"`
	TotalBalanceUcredit float64 `json:"total_balance_ucredit"`
	SupplyPerHuman    float64 `json:"supply_per_human"`
	AvgBalanceUcredit float64 `json:"avg_balance_ucredit"`

	// Event counters (monotonically increasing)
	TotalHumanRegistered int64 `json:"total_human_registered"`
	TotalCreditsIssued   int64 `json:"total_credits_issued"`
	TotalTransfers       int64 `json:"total_transfers"`
	TotalBatchSends      int64 `json:"total_batch_sends"`

	// Burn metrics
	LastBurnYearHeight     int64   `json:"last_burn_year_height"`
	LastBurnTotalBurned    float64 `json:"last_burn_total_burned"`
	LastBurnPerHumanAmount float64 `json:"last_burn_per_human_amount"`
	LastBurnActiveHumans   int64   `json:"last_burn_active_humans"`
	LastBurnRemainder      float64 `json:"last_burn_remainder"`

	// Chain health
	UptimeSeconds  int64   `json:"uptime_seconds"`
	BlocksPerMinute float64 `json:"blocks_per_minute"`

	// Internal tracking
	startTime       time.Time
	lastBlockHeight int64
	lastBlockTime   time.Time
}

// NewMetricsCollector creates a new metrics collector.
func NewMetricsCollector(chainID, version string) *MetricsCollector {
	return &MetricsCollector{
		ChainID:       chainID,
		AppVersion:    version,
		startTime:     time.Now(),
		lastBlockTime: time.Now(),
	}
}

// UpdateBlockMetrics updates metrics from the chain state snapshot.
// Call this once per block.
func (mc *MetricsCollector) UpdateBlockMetrics(snapshot map[string]interface{}) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	now := time.Now()
	mc.UptimeSeconds = int64(now.Sub(mc.startTime).Seconds())

	if blockHeight, ok := snapshot["block_height"].(int64); ok {
		oldHeight := mc.BlockHeight
		mc.BlockHeight = blockHeight

		// Calculate blocks per minute (moving average over the last minute of blocks).
		elapsed := now.Sub(mc.lastBlockTime).Seconds()
		if elapsed > 0 {
			diff := float64(blockHeight - oldHeight)
			mc.BlocksPerMinute = (diff / elapsed) * 60.0
		}
		mc.lastBlockTime = now
		mc.lastBlockHeight = blockHeight
	}

	if humanCount, ok := snapshot["human_count"].(int64); ok {
		mc.HumanCount = humanCount
	}

	if supply, ok := snapshot["supply"].(string); ok {
		mc.SupplyUcredit = parseMetricFloat(supply)
	}

	if totalBalance, ok := snapshot["total_balance"].(string); ok {
		mc.TotalBalanceUcredit = parseMetricFloat(totalBalance)
	}

	// Derived metrics.
	if mc.HumanCount > 0 {
		mc.SupplyPerHuman = mc.SupplyUcredit / float64(mc.HumanCount)
		mc.AvgBalanceUcredit = mc.TotalBalanceUcredit / float64(mc.HumanCount)
	}

	if eventCount, ok := snapshot["event_count"].(map[string]int); ok {
		mc.TotalHumanRegistered = int64(eventCount["human_registered"])
		mc.TotalCreditsIssued = int64(eventCount["credits_issued"])
		mc.TotalTransfers = int64(eventCount["credits_transferred"] + eventCount["batch_send"])
		mc.TotalBatchSends = int64(eventCount["batch_send"])
	}
}

// UpdateBurnMetrics updates burn-specific metrics from the chain state.
func (mc *MetricsCollector) UpdateBurnMetrics(summary map[string]interface{}) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if yearHeight, ok := summary["year_height"].(int64); ok {
		mc.LastBurnYearHeight = yearHeight
	}
	if totalBurned, ok := summary["total_burned"].(int64); ok {
		mc.LastBurnTotalBurned = float64(totalBurned)
	}
	if perHuman, ok := summary["per_human_amount"].(int64); ok {
		mc.LastBurnPerHumanAmount = float64(perHuman)
	}
	if activeHumans, ok := summary["active_humans"].(int64); ok {
		mc.LastBurnActiveHumans = activeHumans
	}
	if remainder, ok := summary["remainder"].(int64); ok {
		mc.LastBurnRemainder = float64(remainder)
	}
}

// MetricsHandler returns an http.HandlerFunc that serves Prometheus-formatted metrics.
func (mc *MetricsCollector) MetricsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mc.mu.RLock()
		defer mc.mu.RUnlock()

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		// Write errors are ignored: a failed write means the scraper has gone away.
		printf := func(format string, args ...any) { _, _ = fmt.Fprintf(w, format, args...) }
		newline := func() { _, _ = fmt.Fprintln(w) }

		// Write Prometheus-formatted metrics.
		printf("# HELP tpt_credit_chain_info Chain metadata\n")
		printf("# TYPE tpt_credit_chain_info gauge\n")
		printf("tpt_credit_chain_info{chain_id=%q,version=%q} 1\n", mc.ChainID, mc.AppVersion)
		newline()

		printf("# HELP tpt_credit_block_height Current block height\n")
		printf("# TYPE tpt_credit_block_height gauge\n")
		printf("tpt_credit_block_height %d\n", mc.BlockHeight)
		newline()

		printf("# HELP tpt_credit_human_count Number of registered verified humans\n")
		printf("# TYPE tpt_credit_human_count gauge\n")
		printf("tpt_credit_human_count %d\n", mc.HumanCount)
		newline()

		printf("# HELP tpt_credit_supply_ucredit Total yearly issuance supply in ucredit\n")
		printf("# TYPE tpt_credit_supply_ucredit gauge\n")
		printf("tpt_credit_supply_ucredit %.0f\n", mc.SupplyUcredit)
		newline()

		printf("# HELP tpt_credit_total_balance_ucredit Total spendable balance in ucredit\n")
		printf("# TYPE tpt_credit_total_balance_ucredit gauge\n")
		printf("tpt_credit_total_balance_ucredit %.0f\n", mc.TotalBalanceUcredit)
		newline()

		printf("# HELP tpt_credit_supply_per_human Average supply per human in ucredit\n")
		printf("# TYPE tpt_credit_supply_per_human gauge\n")
		printf("tpt_credit_supply_per_human %.2f\n", mc.SupplyPerHuman)
		newline()

		printf("# HELP tpt_credit_avg_balance_ucredit Average account balance in ucredit\n")
		printf("# TYPE tpt_credit_avg_balance_ucredit gauge\n")
		printf("tpt_credit_avg_balance_ucredit %.2f\n", mc.AvgBalanceUcredit)
		newline()

		printf("# HELP tpt_credit_events_total Total events by type\n")
		printf("# TYPE tpt_credit_events_total counter\n")
		printf("tpt_credit_events_total{type=\"human_registered\"} %d\n", mc.TotalHumanRegistered)
		printf("tpt_credit_events_total{type=\"credits_issued\"} %d\n", mc.TotalCreditsIssued)
		printf("tpt_credit_events_total{type=\"transfers\"} %d\n", mc.TotalTransfers)
		printf("tpt_credit_events_total{type=\"batch_sends\"} %d\n", mc.TotalBatchSends)
		newline()

		printf("# HELP tpt_credit_burn_info Last annual burn event info\n")
		printf("# TYPE tpt_credit_burn_info gauge\n")
		printf("tpt_credit_burn_info{field=\"year_height\"} %d\n", mc.LastBurnYearHeight)
		printf("tpt_credit_burn_info{field=\"total_burned\"} %.0f\n", mc.LastBurnTotalBurned)
		printf("tpt_credit_burn_info{field=\"per_human_amount\"} %.0f\n", mc.LastBurnPerHumanAmount)
		printf("tpt_credit_burn_info{field=\"active_humans\"} %d\n", mc.LastBurnActiveHumans)
		printf("tpt_credit_burn_info{field=\"remainder\"} %.0f\n", mc.LastBurnRemainder)
		newline()

		printf("# HELP tpt_credit_uptime_seconds Chain uptime in seconds\n")
		printf("# TYPE tpt_credit_uptime_seconds counter\n")
		printf("tpt_credit_uptime_seconds %d\n", mc.UptimeSeconds)
		newline()

		printf("# HELP tpt_credit_blocks_per_minute Block production rate (moving average)\n")
		printf("# TYPE tpt_credit_blocks_per_minute gauge\n")
		printf("tpt_credit_blocks_per_minute %.2f\n", mc.BlocksPerMinute)
		newline()
	}
}

// parseMetricFloat parses a string to a float64 for metric purposes.
func parseMetricFloat(s string) float64 {
	var f float64
	if _, err := fmt.Sscanf(s, "%f", &f); err != nil {
		return 0
	}
	return f
}

// StartMetricsServer starts the HTTP server for Prometheus metrics on the given address.
func StartMetricsServer(addr string, mc *MetricsCollector) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", mc.MetricsHandler())

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		log.Printf("Metrics server listening on %s/metrics", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Metrics server error: %v", err)
		}
	}()

	return server
}