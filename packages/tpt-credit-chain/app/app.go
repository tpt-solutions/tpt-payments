// Package app implements the TPT Credit chain application.
//
// This is a standalone chain app that runs the issuance and burn modules
// with an in-memory block simulator. It does NOT use the full Cosmos SDK
// IAVL/CometBFT consensus layer — block production is tick-based for
// devnet/simulator use.
//
// A future version will integrate with Cosmos SDK's store/consensus layer
// for production mainnet deployment.
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"cosmossdk.io/math"

	"github.com/tpt-platform/tpt-platform/packages/tpt-credit-chain/x/burn"
	"github.com/tpt-platform/tpt-platform/packages/tpt-credit-chain/x/issuance"
)

const (
	// AppName is the name of the application.
	AppName = "tpt-credit-chain"

	// DefaultNodeHome is the default home directory for the chain.
	DefaultNodeHome = "~/.tptcredit"

	// BlockInterval is the target block time (2 seconds).
	BlockInterval = 2 * time.Second

	// BlocksPerYear is the number of blocks in one year at 2-second block time.
	// 365.25 days x 24 h x 60 min x 30 blocks/min = 15,778,800 blocks/year.
	BlocksPerYear = int64(15_778_800)

	// MetricsAddr is the default address for Prometheus metrics HTTP endpoint.
	MetricsAddr = ":26660"

	// AppVersion is the current application version.
	AppVersion = "v0.1.0"
)

// DefaultReserveAddress is the protocol reserve address for burn remainder. It is the bech32
// encoding of sha256("tpt-credit/protocol-reserve")[:20], so no one holds its private key. The
// checksum is verified by TestReserveAddressIsValid.
const DefaultReserveAddress = "cosmos1nwert8dw9eyfxheva5gvkcmge0stra5mavdmsj"

// IntString is an int64 that Cosmos-style genesis files write as a JSON string ("1"). A bare
// JSON number is accepted too, so hand-written files work.
type IntString int64

// MarshalJSON writes the value as a JSON string, matching Cosmos genesis files.
func (i IntString) MarshalJSON() ([]byte, error) {
	return json.Marshal(strconv.FormatInt(int64(i), 10))
}

// UnmarshalJSON accepts a JSON string or number holding a base-10 integer.
func (i *IntString) UnmarshalJSON(b []byte) error {
	text := strings.Trim(strings.TrimSpace(string(b)), `"`)
	n, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid integer %s: %w", string(b), err)
	}
	*i = IntString(n)
	return nil
}

// GenesisDoc is the genesis document format for the TPT Credit chain.
type GenesisDoc struct {
	GenesisTime     string          `json:"genesis_time"`
	ChainID         string          `json:"chain_id"`
	Moniker         string          `json:"moniker"`
	InitialHeight   IntString       `json:"initial_height"`
	AppState        json.RawMessage `json:"app_state"`
	ConsensusParams json.RawMessage `json:"consensus_params,omitempty"`
}

// AppState contains the initial state for all modules.
type AppState struct {
	Issuance issuance.GenesisState `json:"issuance"`
	Burn     burn.GenesisState     `json:"burn"`
}

// Write writes the genesis document to the chain home directory.
func (gd *GenesisDoc) Write(home string) error {
	// Expand ~ to home directory.
	if strings.HasPrefix(home, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("cannot determine home directory: %w", err)
		}
		home = filepath.Join(homeDir, home[2:])
	}

	// Create config directory.
	configDir := filepath.Join(home, "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Create data directory.
	dataDir := filepath.Join(home, "data")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	// Write genesis file.
	genesisBytes, err := json.MarshalIndent(gd, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal genesis: %w", err)
	}

	genesisPath := filepath.Join(configDir, "genesis.json")
	if err := os.WriteFile(genesisPath, genesisBytes, 0644); err != nil {
		return fmt.Errorf("failed to write genesis file: %w", err)
	}

	fmt.Printf("Genesis written to %s\n", genesisPath)
	fmt.Printf("Chain ID: %s\n", gd.ChainID)
	fmt.Printf("Moniker: %s\n", gd.Moniker)
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  1. Start the chain:    tptcreditd start")

	return nil
}

// LoadGenesis loads the genesis document from the chain home directory.
func LoadGenesis(home string) (*GenesisDoc, error) {
	// Expand ~ to home directory.
	if strings.HasPrefix(home, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("cannot determine home directory: %w", err)
		}
		home = filepath.Join(homeDir, home[2:])
	}

	genesisPath := filepath.Join(home, "config", "genesis.json")
	genesisBytes, err := os.ReadFile(genesisPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read genesis file at %s: %w", genesisPath, err)
	}

	var genesisDoc GenesisDoc
	if err := json.Unmarshal(genesisBytes, &genesisDoc); err != nil {
		return nil, fmt.Errorf("failed to parse genesis file: %w", err)
	}

	return &genesisDoc, nil
}

// App is the TPT Credit chain application.
type App struct {
	mu sync.RWMutex

	// Chain identity.
	ChainID       string
	NodeHome      string
	GenesisHeight int64
	BlockHeight   int64

	// Modules.
	IssuanceKeeper *issuance.Keeper
	BurnKeeper     *burn.Keeper

	// Event bus.
	events *EventBus

	// Block timing.
	blockInterval time.Duration
	started       bool
	stopCh        chan struct{}

	// Tx log.
	TxLog []TxLogEntry

	// Metrics collector (Prometheus).
	Metrics       *MetricsCollector
	metricsServer *http.Server
}

// TxLogEntry records a processed transaction.
type TxLogEntry struct {
	Height    int64  `json:"height"`
	Hash      string `json:"hash"`
	Messages  string `json:"messages"`
	Timestamp int64  `json:"timestamp"`
}

// EventBus collects and stores events for querying.
type EventBus struct {
	HumanRegisteredEvents    []issuance.HumanRegisteredEvent    `json:"human_registered_events"`
	CreditsIssuedEvents      []issuance.CreditsIssuedEvent      `json:"credits_issued_events"`
	FrequencyUpdatedEvents   []issuance.FrequencyUpdatedEvent   `json:"frequency_updated_events"`
	CreditsTransferredEvents []issuance.CreditsTransferredEvent `json:"credits_transferred_events"`
	BatchSendEvents          []issuance.BatchSendEvent          `json:"batch_send_events"`
	AnnualBurnEvents         []burn.EventAnnualBurn             `json:"annual_burn_events"`
	AnnualBurnSummaryEvents  []burn.EventAnnualBurnSummary      `json:"annual_burn_summary_events"`
}

func NewEventBus() *EventBus {
	return &EventBus{
		HumanRegisteredEvents:    make([]issuance.HumanRegisteredEvent, 0),
		CreditsIssuedEvents:      make([]issuance.CreditsIssuedEvent, 0),
		FrequencyUpdatedEvents:   make([]issuance.FrequencyUpdatedEvent, 0),
		CreditsTransferredEvents: make([]issuance.CreditsTransferredEvent, 0),
		BatchSendEvents:          make([]issuance.BatchSendEvent, 0),
		AnnualBurnEvents:         make([]burn.EventAnnualBurn, 0),
		AnnualBurnSummaryEvents:  make([]burn.EventAnnualBurnSummary, 0),
	}
}

// EmitHumanRegistered implements issuance.EventBus.
func (eb *EventBus) EmitHumanRegistered(event issuance.HumanRegisteredEvent) {
	eb.HumanRegisteredEvents = append(eb.HumanRegisteredEvents, event)
}

// EmitCreditsIssued implements issuance.EventBus.
func (eb *EventBus) EmitCreditsIssued(event issuance.CreditsIssuedEvent) {
	eb.CreditsIssuedEvents = append(eb.CreditsIssuedEvents, event)
}

// EmitFrequencyUpdated implements issuance.EventBus.
func (eb *EventBus) EmitFrequencyUpdated(event issuance.FrequencyUpdatedEvent) {
	eb.FrequencyUpdatedEvents = append(eb.FrequencyUpdatedEvents, event)
}

// EmitCreditsTransferred implements issuance.EventBus.
func (eb *EventBus) EmitCreditsTransferred(event issuance.CreditsTransferredEvent) {
	eb.CreditsTransferredEvents = append(eb.CreditsTransferredEvents, event)
}

// EmitBatchSend implements issuance.EventBus.
func (eb *EventBus) EmitBatchSend(event issuance.BatchSendEvent) {
	eb.BatchSendEvents = append(eb.BatchSendEvents, event)
}

// EmitAnnualBurn implements burn.EventBus.
func (eb *EventBus) EmitAnnualBurn(event burn.EventAnnualBurn) {
	eb.AnnualBurnEvents = append(eb.AnnualBurnEvents, event)
}

// EmitAnnualBurnSummary implements burn.EventBus.
func (eb *EventBus) EmitAnnualBurnSummary(event burn.EventAnnualBurnSummary) {
	eb.AnnualBurnSummaryEvents = append(eb.AnnualBurnSummaryEvents, event)
}

// NewDefaultGenesis creates a default genesis document for devnet.
func NewDefaultGenesis(chainID, moniker string) *GenesisDoc {
	return &GenesisDoc{
		GenesisTime:   time.Now().UTC().Format(time.RFC3339),
		ChainID:       chainID,
		Moniker:       moniker,
		InitialHeight: 1,
		AppState:      mustMarshalJSON(NewDefaultAppState()),
	}
}

// NewDefaultAppState creates the default app state for a fresh chain.
func NewDefaultAppState() *AppState {
	return &AppState{
		Issuance: *issuance.DefaultGenesis(),
		Burn:     *burn.DefaultGenesis(),
	}
}

func mustMarshalJSON(v interface{}) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal: %v", err))
	}
	return b
}

// IssuanceKeeperAdapter wraps issuance.Keeper to implement burn.IssuanceKeeper.
type IssuanceKeeperAdapter struct {
	inner *issuance.Keeper
}

func NewIssuanceKeeperAdapter(inner *issuance.Keeper) *IssuanceKeeperAdapter {
	return &IssuanceKeeperAdapter{inner: inner}
}

func (a *IssuanceKeeperAdapter) GetAccounts() []burn.IssuanceAccount {
	accounts := a.inner.GetAccounts()
	result := make([]burn.IssuanceAccount, len(accounts))
	for i, acct := range accounts {
		result[i] = burn.IssuanceAccount{
			Address:             acct.Address,
			Balance:             acct.Balance,
			YearCreditsReceived: acct.YearCreditsReceived,
			YearPayoutCount:     acct.YearPayoutCount,
			LastPaidAt:          acct.LastPaidAt,
			RegisteredAt:        acct.RegisteredAt,
		}
	}
	return result
}

func (a *IssuanceKeeperAdapter) GetHumanCount() int64 {
	return a.inner.GetHumanCount()
}

func (a *IssuanceKeeperAdapter) ResetYearCredits() {
	a.inner.ResetYearCredits()
}

func (a *IssuanceKeeperAdapter) GetGenesisHeight() int64 {
	return a.inner.GetGenesisHeight()
}

func (a *IssuanceKeeperAdapter) SetAccountBalance(address string, balance math.Int) {
	a.inner.SetAccountBalance(address, balance)
}

func (a *IssuanceKeeperAdapter) GetAccountBalance(address string) math.Int {
	return a.inner.GetAccountBalance(address)
}

// NewAppFromGenesis creates a new App from a genesis document.
func NewAppFromGenesis(home string, genesis *GenesisDoc) (*App, error) {
	// Parse app state from genesis.
	var appState AppState
	if err := json.Unmarshal(genesis.AppState, &appState); err != nil {
		return nil, fmt.Errorf("failed to parse app state: %w", err)
	}

	genesisHeight := int64(genesis.InitialHeight)
	if genesisHeight == 0 {
		genesisHeight = 1
	}

	// Create event bus.
	events := NewEventBus()

	// Create issuance keeper with events.
	issuanceKeeper := issuance.NewKeeperWithEvents(events, genesisHeight)

	// Create adapter and burn keeper with events.
	reserveAddress := DefaultReserveAddress
	issuanceAdapter := NewIssuanceKeeperAdapter(issuanceKeeper)
	burnKeeper := burn.NewKeeperWithEvents(issuanceAdapter, events, genesisHeight, reserveAddress)

	// Initialize from genesis state.
	ctx := context.Background()
	if err := issuanceKeeper.InitializeGenesis(ctx, &appState.Issuance); err != nil {
		return nil, fmt.Errorf("failed to init issuance genesis: %w", err)
	}
	if err := burnKeeper.InitializeGenesis(ctx, &appState.Burn); err != nil {
		return nil, fmt.Errorf("failed to init burn genesis: %w", err)
	}

	// Create metrics collector.
	metrics := NewMetricsCollector(genesis.ChainID, AppVersion)

	chainApp := &App{
		ChainID:        genesis.ChainID,
		NodeHome:       home,
		GenesisHeight:  genesisHeight,
		BlockHeight:    genesisHeight,
		IssuanceKeeper: issuanceKeeper,
		BurnKeeper:     burnKeeper,
		events:         events,
		blockInterval:  BlockInterval,
		stopCh:         make(chan struct{}),
		TxLog:          make([]TxLogEntry, 0),
		Metrics:        metrics,
	}

	return chainApp, nil
}

// GetBlockHeight returns the current block height.
func (app *App) GetBlockHeight() int64 {
	app.mu.RLock()
	defer app.mu.RUnlock()
	return app.BlockHeight
}

// GetEvents returns the event bus (for querying).
func (app *App) GetEvents() *EventBus {
	return app.events
}

// Start begins block production.
func (app *App) Start() error {
	app.mu.Lock()
	if app.started {
		app.mu.Unlock()
		return fmt.Errorf("chain is already running")
	}
	app.started = true
	app.mu.Unlock()

	fmt.Printf("Chain started at block height %d (chain ID: %s)\n", app.BlockHeight, app.ChainID)
	fmt.Printf("Block interval: %s\n", app.blockInterval)
	fmt.Printf("Reserve address: %s\n", DefaultReserveAddress)
	fmt.Printf("Metrics endpoint: http://localhost%s/metrics\n", MetricsAddr)
	fmt.Println("Press Ctrl+C to stop.")
	fmt.Println()

	// Start Prometheus metrics server.
	app.metricsServer = StartMetricsServer(MetricsAddr, app.Metrics)

	ticker := time.NewTicker(app.blockInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			app.mu.Lock()
			app.BlockHeight++
			height := app.BlockHeight

			// Print block progress every 100 blocks.
			if height%100 == 0 {
				fmt.Printf("Block %d\n", height)
			}

			// Run issuance BeginBlock (process payouts).
			issuedEvents := app.IssuanceKeeper.BeginBlock(height, app.BurnKeeper.IsBurnBlock)

			// Run burn BeginBlock (check if annual burn).
			burnSummary := app.BurnKeeper.BeginBlock(height)

			// Check invariants every block.
			if broken, msg := app.CheckInvariants(); broken {
				fmt.Printf("INVARIANT BROKEN at block %d: %s\n", height, msg)
			}

			// Update Prometheus metrics.
			app.Metrics.UpdateBlockMetrics(app.metricsSnapshot())

			// Update burn metrics if a burn occurred.
			if burnSummary != nil {
				app.Metrics.UpdateBurnMetrics(map[string]interface{}{
					"year_height":      burnSummary.YearHeight,
					"total_burned":     burnSummary.TotalBurned,
					"per_human_amount": burnSummary.PerHumanAmount,
					"active_humans":    burnSummary.ActiveHumans,
					"remainder":        burnSummary.Remainder,
				})
			}

			app.mu.Unlock()

			// Print payout info.
			if len(issuedEvents) > 0 {
				for _, e := range issuedEvents {
					fmt.Printf("Payout: %s received %s ucredit (payout #%d)\n",
						e.Address, e.Amount, e.PayoutNumber)
				}
			}

			// Print burn info.
			if burnSummary != nil {
				fmt.Printf("ANNUAL BURN at block %d:\n", height)
				fmt.Printf("  Total burned: %d ucredit\n", burnSummary.TotalBurned)
				fmt.Printf("  Active humans: %d\n", burnSummary.ActiveHumans)
				fmt.Printf("  Per human redistribution: %d ucredit\n", burnSummary.PerHumanAmount)
				fmt.Printf("  Remainder to reserve: %d ucredit\n", burnSummary.Remainder)
			}

		case <-app.stopCh:
			fmt.Println("Chain stopped.")
			return nil
		}
	}
}

// metricsSnapshot returns a state snapshot for metrics collection.
func (app *App) metricsSnapshot() map[string]interface{} {
	accounts := app.IssuanceKeeper.GetAccounts()
	accountSummaries := make([]map[string]interface{}, len(accounts))
	for i, acct := range accounts {
		accountSummaries[i] = map[string]interface{}{
			"address":               acct.Address,
			"identity_uuid":         acct.IdentityUUID,
			"identity_tier":         acct.IdentityTier,
			"payout_frequency":      string(acct.PayoutFrequency),
			"balance":               acct.Balance.String(),
			"year_credits_received": acct.YearCreditsReceived.String(),
			"year_payout_count":     acct.YearPayoutCount,
			"last_paid_at":          acct.LastPaidAt,
			"next_due_at":           acct.NextDueAt,
			"registered_at":         acct.RegisteredAt,
		}
	}

	return map[string]interface{}{
		"chain_id":       app.ChainID,
		"block_height":   app.BlockHeight,
		"genesis_height": app.GenesisHeight,
		"human_count":    app.IssuanceKeeper.GetHumanCount(),
		"supply":         app.IssuanceKeeper.GetSupply().String(),
		"total_balance":  app.IssuanceKeeper.GetTotalBalance().String(),
		"accounts":       accountSummaries,
		"event_count": map[string]int{
			"human_registered":    len(app.events.HumanRegisteredEvents),
			"credits_issued":      len(app.events.CreditsIssuedEvents),
			"frequency_updated":   len(app.events.FrequencyUpdatedEvents),
			"credits_transferred": len(app.events.CreditsTransferredEvents),
			"batch_send":          len(app.events.BatchSendEvents),
			"annual_burn":         len(app.events.AnnualBurnEvents),
			"annual_burn_summary": len(app.events.AnnualBurnSummaryEvents),
		},
	}
}

// Stop gracefully stops block production.
func (app *App) Stop() {
	if app.metricsServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := app.metricsServer.Shutdown(ctx); err != nil {
			fmt.Printf("Metrics server shutdown error: %v\n", err)
		}
	}
	close(app.stopCh)
}

// CheckInvariants runs all module invariants and returns broken status.
func (app *App) CheckInvariants() (broken bool, msg string) {
	if desc, broken := app.IssuanceKeeper.CheckInvariant(context.Background()); broken {
		return true, desc
	}
	if desc, broken := app.IssuanceKeeper.CheckBalanceInvariant(context.Background()); broken {
		return true, desc
	}
	return false, ""
}

// LogTx records a processed transaction.
func (app *App) LogTx(hash, messages string) {
	app.TxLog = append(app.TxLog, TxLogEntry{
		Height:    app.BlockHeight,
		Hash:      hash,
		Messages:  messages,
		Timestamp: time.Now().Unix(),
	})
}

// GetStateSnapshot returns a JSON-serializable snapshot of the chain state.
func (app *App) GetStateSnapshot() map[string]interface{} {
	app.mu.RLock()
	defer app.mu.RUnlock()

	accounts := app.IssuanceKeeper.GetAccounts()
	accountSummaries := make([]map[string]interface{}, len(accounts))
	for i, acct := range accounts {
		accountSummaries[i] = map[string]interface{}{
			"address":               acct.Address,
			"identity_uuid":         acct.IdentityUUID,
			"identity_tier":         acct.IdentityTier,
			"payout_frequency":      string(acct.PayoutFrequency),
			"balance":               acct.Balance.String(),
			"year_credits_received": acct.YearCreditsReceived.String(),
			"year_payout_count":     acct.YearPayoutCount,
			"last_paid_at":          acct.LastPaidAt,
			"next_due_at":           acct.NextDueAt,
			"registered_at":         acct.RegisteredAt,
		}
	}

	return map[string]interface{}{
		"chain_id":       app.ChainID,
		"block_height":   app.BlockHeight,
		"genesis_height": app.GenesisHeight,
		"human_count":    app.IssuanceKeeper.GetHumanCount(),
		"supply":         app.IssuanceKeeper.GetSupply().String(),
		"total_balance":  app.IssuanceKeeper.GetTotalBalance().String(),
		"accounts":       accountSummaries,
		"event_count": map[string]int{
			"human_registered":    len(app.events.HumanRegisteredEvents),
			"credits_issued":      len(app.events.CreditsIssuedEvents),
			"frequency_updated":   len(app.events.FrequencyUpdatedEvents),
			"credits_transferred": len(app.events.CreditsTransferredEvents),
			"batch_send":          len(app.events.BatchSendEvents),
			"annual_burn":         len(app.events.AnnualBurnEvents),
			"annual_burn_summary": len(app.events.AnnualBurnSummaryEvents),
		},
	}
}
