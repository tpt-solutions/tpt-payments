package connect

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// =============================================================================
// Connected Account Dashboard Access
// =============================================================================

// DashboardService provides connected accounts with access to their own data
type DashboardService struct {
	db *sql.DB
}

// NewDashboardService creates a new dashboard service for connected accounts
func NewDashboardService(db *sql.DB) *DashboardService {
	return &DashboardService{db: db}
}

// DashboardAccount represents a connected account's own view of their data
type DashboardAccount struct {
	ID                string          `json:"id"`
	Email             string          `json:"email"`
	DisplayName       string          `json:"display_name"`
	PlatformID        string          `json:"platform_id"`
	Status            AccountStatus   `json:"status"`
	CreatedAt         int64           `json:"created_at"`
	ChargesEnabled    bool            `json:"charges_enabled"`
	PayoutsEnabled    bool            `json:"payouts_enabled"`
	CurrentBalance    decimal.Decimal `json:"current_balance"`
	PendingBalance    decimal.Decimal `json:"pending_balance"`
	AvailableBalance  decimal.Decimal `json:"available_balance"`
	Currency          string          `json:"currency"`
	Country           string          `json:"country"`
	DefaultCurrency   string          `json:"default_currency"`
	BusinessType      string          `json:"business_type,omitempty"`
	VerificationStatus string         `json:"verification_status"`
	Requirements      []Requirement   `json:"requirements"`
}

// Requirement represents an onboarding requirement
type Requirement struct {
	Field        string `json:"field"`
	CurrentValue string `json:"current_value,omitempty"`
	Required     bool   `json:"required"`
	Provided     bool   `json:"provided"`
}

// DashboardCharge represents a charge visible to the connected account
type DashboardCharge struct {
	ID              string          `json:"id"`
	Amount          decimal.Decimal `json:"amount"`
	Currency        string          `json:"currency"`
	Status          string          `json:"status"`
	CreatedAt       int64           `json:"created_at"`
	CustomerEmail   string          `json:"customer_email,omitempty"`
	Description     string          `json:"description,omitempty"`
	PlatformFee     decimal.Decimal `json:"platform_fee"`
	NetAmount       decimal.Decimal `json:"net_amount"`
	PaymentMethod   string          `json:"payment_method"`
	RefundedAmount  decimal.Decimal `json:"refunded_amount"`
	Metadata        map[string]string `json:"metadata,omitempty"`
}

// DashboardPayout represents a payout visible to the connected account
type DashboardPayout struct {
	ID            string          `json:"id"`
	Amount        decimal.Decimal `json:"amount"`
	Currency      string          `json:"currency"`
	Status        string          `json:"status"`
	CreatedAt     int64           `json:"created_at"`
	ArrivalDate   *int64          `json:"arrival_date,omitempty"`
	Method        string          `json:"method"`
	BankAccountLast4 string       `json:"bank_account_last4,omitempty"`
	Description   string          `json:"description,omitempty"`
	FailureReason *string         `json:"failure_reason,omitempty"`
}

// DashboardTransfer represents a transfer to/from the connected account
type DashboardTransfer struct {
	ID              string          `json:"id"`
	Amount          decimal.Decimal `json:"amount"`
	Currency        string          `json:"currency"`
	Status          string          `json:"status"`
	CreatedAt       int64           `json:"created_at"`
	Direction       string          `json:"direction"` // "inbound" or "outbound"
	SourceType      string          `json:"source_type"`
	SourceID        string          `json:"source_id"`
	Description     string          `json:"description,omitempty"`
}

// BalanceTransaction represents a single balance-affecting event
type BalanceTransaction struct {
	ID          string          `json:"id"`
	Amount      decimal.Decimal `json:"amount"`
	Currency    string          `json:"currency"`
	NetAmount   decimal.Decimal `json:"net_amount"`
	FeeAmount   decimal.Decimal `json:"fee_amount"`
	Status      string          `json:"status"`
	Type        string          `json:"type"`
	CreatedAt   int64           `json:"created_at"`
	AvailableOn int64           `json:"available_on"`
	Description string          `json:"description,omitempty"`
}

// DashboardOverview provides a summary for the connected account dashboard
type DashboardOverview struct {
	Account           DashboardAccount     `json:"account"`
	RecentCharges     []DashboardCharge    `json:"recent_charges"`
	RecentPayouts     []DashboardPayout    `json:"recent_payouts"`
	RecentTransfers   []DashboardTransfer  `json:"recent_transfers"`
	TotalVolume30d    decimal.Decimal      `json:"total_volume_30d"`
	TotalCharges30d   int64                `json:"total_charges_30d"`
	TotalPayouts30d   int64                `json:"total_payouts_30d"`
	PendingPayouts    []DashboardPayout    `json:"pending_payouts"`
}

// GetAccount retrieves the connected account's own account information
func (s *DashboardService) GetAccount(ctx context.Context, accountID string) (*DashboardAccount, error) {
	account, err := s.getAccountByID(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve account: %w", err)
	}

	// Get current balances
	account.CurrentBalance, account.PendingBalance, account.AvailableBalance, err = s.getBalances(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve balances: %w", err)
	}

	// Get requirements
	account.Requirements, err = s.getRequirements(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve requirements: %w", err)
	}

	return account, nil
}

// ListCharges retrieves charges for a connected account
func (s *DashboardService) ListCharges(ctx context.Context, accountID string, limit int, offset int) ([]DashboardCharge, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	query := `
		SELECT id, amount, currency, status, created_at, customer_email, 
		       description, platform_fee, net_amount, payment_method, 
		       refunded_amount, metadata
		FROM direct_charges
		WHERE connected_account_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := s.db.QueryContext(ctx, query, accountID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query charges: %w", err)
	}
	defer rows.Close()

	var charges []DashboardCharge
	for rows.Next() {
		var c DashboardCharge
		var metadataStr string
		err := rows.Scan(
			&c.ID, &c.Amount, &c.Currency, &c.Status, &c.CreatedAt,
			&c.CustomerEmail, &c.Description, &c.PlatformFee, &c.NetAmount,
			&c.PaymentMethod, &c.RefundedAmount, &metadataStr,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan charge: %w", err)
		}

		// Parse metadata JSON
		if metadataStr != "" {
			c.Metadata = make(map[string]string)
			// Simple parsing for demo; use json.Unmarshal in production
		}

		charges = append(charges, c)
	}

	return charges, rows.Err()
}

// ListPayouts retrieves payouts for a connected account
func (s *DashboardService) ListPayouts(ctx context.Context, accountID string, limit int, offset int) ([]DashboardPayout, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	query := `
		SELECT id, amount, currency, status, created_at, arrival_date,
		       method, bank_account_last4, description, failure_reason
		FROM payouts
		WHERE merchant_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := s.db.QueryContext(ctx, query, accountID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query payouts: %w", err)
	}
	defer rows.Close()

	var payouts []DashboardPayout
	for rows.Next() {
		var p DashboardPayout
		err := rows.Scan(
			&p.ID, &p.Amount, &p.Currency, &p.Status, &p.CreatedAt,
			&p.ArrivalDate, &p.Method, &p.BankAccountLast4,
			&p.Description, &p.FailureReason,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan payout: %w", err)
		}
		payouts = append(payouts, p)
	}

	return payouts, rows.Err()
}

// ListTransfers retrieves transfers for a connected account
func (s *DashboardService) ListTransfers(ctx context.Context, accountID string, limit int, offset int) ([]DashboardTransfer, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	query := `
		SELECT id, amount, currency, status, created_at, direction,
		       source_type, source_id, description
		FROM transfers
		WHERE connected_account_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := s.db.QueryContext(ctx, query, accountID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query transfers: %w", err)
	}
	defer rows.Close()

	var transfers []DashboardTransfer
	for rows.Next() {
		var t DashboardTransfer
		err := rows.Scan(
			&t.ID, &t.Amount, &t.Currency, &t.Status, &t.CreatedAt,
			&t.Direction, &t.SourceType, &t.SourceID, &t.Description,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan transfer: %w", err)
		}
		transfers = append(transfers, t)
	}

	return transfers, rows.Err()
}

// ListBalanceTransactions retrieves balance transactions for a connected account
func (s *DashboardService) ListBalanceTransactions(ctx context.Context, accountID string, limit int, offset int) ([]BalanceTransaction, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	query := `
		SELECT id, amount, currency, net_amount, fee_amount, status,
		       type, created_at, available_on, description
		FROM balance_transactions
		WHERE account_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := s.db.QueryContext(ctx, query, accountID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query balance transactions: %w", err)
	}
	defer rows.Close()

	var transactions []BalanceTransaction
	for rows.Next() {
		var bt BalanceTransaction
		err := rows.Scan(
			&bt.ID, &bt.Amount, &bt.Currency, &bt.NetAmount, &bt.FeeAmount,
			&bt.Status, &bt.Type, &bt.CreatedAt, &bt.AvailableOn, &bt.Description,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan balance transaction: %w", err)
		}
		transactions = append(transactions, bt)
	}

	return transactions, rows.Err()
}

// GetOverview provides a comprehensive dashboard overview for a connected account
func (s *DashboardService) GetOverview(ctx context.Context, accountID string) (*DashboardOverview, error) {
	overview := &DashboardOverview{}

	// Get account info
	account, err := s.GetAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	overview.Account = *account

	// Get recent charges
	overview.RecentCharges, err = s.ListCharges(ctx, accountID, 5, 0)
	if err != nil {
		return nil, err
	}

	// Get recent payouts
	overview.RecentPayouts, err = s.ListPayouts(ctx, accountID, 5, 0)
	if err != nil {
		return nil, err
	}

	// Get recent transfers
	overview.RecentTransfers, err = s.ListTransfers(ctx, accountID, 5, 0)
	if err != nil {
		return nil, err
	}

	// Get 30-day metrics
	overview.TotalVolume30d, overview.TotalCharges30d, overview.TotalPayouts30d, err = s.get30DayMetrics(ctx, accountID)
	if err != nil {
		return nil, err
	}

	// Get pending payouts
	overview.PendingPayouts, err = s.getPendingPayouts(ctx, accountID)
	if err != nil {
		return nil, err
	}

	return overview, nil
}

// Helper methods

func (s *DashboardService) getAccountByID(ctx context.Context, accountID string) (*DashboardAccount, error) {
	query := `
		SELECT id, email, display_name, platform_id, status, created_at,
		       charges_enabled, payouts_enabled, currency, country,
		       default_currency, business_type, verification_status
		FROM connected_accounts
		WHERE id = $1
	`

	var a DashboardAccount
	err := s.db.QueryRowContext(ctx, query, accountID).Scan(
		&a.ID, &a.Email, &a.DisplayName, &a.PlatformID, &a.Status, &a.CreatedAt,
		&a.ChargesEnabled, &a.PayoutsEnabled, &a.Currency, &a.Country,
		&a.DefaultCurrency, &a.BusinessType, &a.VerificationStatus,
	)
	if err != nil {
		return nil, err
	}

	return &a, nil
}

func (s *DashboardService) getBalances(ctx context.Context, accountID string) (current, pending, available decimal.Decimal, err error) {
	query := `
		SELECT 
			COALESCE(SUM(CASE WHEN status = 'available' THEN amount ELSE 0 END), 0) as available,
			COALESCE(SUM(CASE WHEN status = 'pending' THEN amount ELSE 0 END), 0) as pending,
			COALESCE(SUM(amount), 0) as current
		FROM balance_transactions
		WHERE account_id = $1
	`

	err = s.db.QueryRowContext(ctx, query, accountID).Scan(&available, &pending, &current)
	return current, pending, available, err
}

func (s *DashboardService) getRequirements(ctx context.Context, accountID string) ([]Requirement, error) {
	query := `
		SELECT field, current_value, required, provided
		FROM account_requirements
		WHERE account_id = $1
	`

	rows, err := s.db.QueryContext(ctx, query, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var requirements []Requirement
	for rows.Next() {
		var r Requirement
		if err := rows.Scan(&r.Field, &r.CurrentValue, &r.Required, &r.Provided); err != nil {
			return nil, err
		}
		requirements = append(requirements, r)
	}

	return requirements, rows.Err()
}

func (s *DashboardService) get30DayMetrics(ctx context.Context, accountID string) (volume decimal.Decimal, charges, payouts int64, err error) {
	query := `
		SELECT 
			COALESCE(SUM(amount), 0),
			COUNT(DISTINCT CASE WHEN type = 'charge' THEN id END),
			COUNT(DISTINCT CASE WHEN type = 'payout' THEN id END)
		FROM balance_transactions
		WHERE account_id = $1 
		  AND created_at >= $2
	`

	thirtyDaysAgo := time.Now().Add(-30 * 24 * time.Hour).Unix()
	err = s.db.QueryRowContext(ctx, query, accountID, thirtyDaysAgo).Scan(&volume, &charges, &payouts)
	return volume, charges, payouts, err
}

func (s *DashboardService) getPendingPayouts(ctx context.Context, accountID string) ([]DashboardPayout, error) {
	query := `
		SELECT id, amount, currency, status, created_at, arrival_date,
		       method, bank_account_last4, description, failure_reason
		FROM payouts
		WHERE merchant_id = $1 AND status IN ('pending', 'in_transit')
		ORDER BY created_at DESC
	`

	rows, err := s.db.QueryContext(ctx, query, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var payouts []DashboardPayout
	for rows.Next() {
		var p DashboardPayout
		err := rows.Scan(
			&p.ID, &p.Amount, &p.Currency, &p.Status, &p.CreatedAt,
			&p.ArrivalDate, &p.Method, &p.BankAccountLast4,
			&p.Description, &p.FailureReason,
		)
		if err != nil {
			return nil, err
		}
		payouts = append(payouts, p)
	}

	return payouts, rows.Err()
}

// GenerateDashboardToken creates a secure token for embedded dashboard access
func (s *DashboardService) GenerateDashboardToken(ctx context.Context, accountID string, expiresIn time.Duration) (string, error) {
	token := uuid.New().String()

	// Store token in database with expiration
	query := `
		INSERT INTO dashboard_access_tokens (token, account_id, created_at, expires_at)
		VALUES ($1, $2, $3, $4)
	`

	now := time.Now().UTC()
	expiresAt := now.Add(expiresIn)
	_, err := s.db.ExecContext(ctx, query, token, accountID, now.Unix(), expiresAt.Unix())
	if err != nil {
		return "", fmt.Errorf("failed to create dashboard token: %w", err)
	}

	return token, nil
}

// ValidateDashboardToken validates a dashboard access token
func (s *DashboardService) ValidateDashboardToken(ctx context.Context, token string) (string, error) {
	query := `
		SELECT account_id FROM dashboard_access_tokens
		WHERE token = $1 AND expires_at > $2 AND revoked_at IS NULL
	`

	var accountID string
	now := time.Now().UTC().Unix()
	err := s.db.QueryRowContext(ctx, query, token, now).Scan(&accountID)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("invalid or expired token")
		}
		return "", fmt.Errorf("failed to validate token: %w", err)
	}

	return accountID, nil
}

// RevokeDashboardToken revokes a dashboard access token
func (s *DashboardService) RevokeDashboardToken(ctx context.Context, token string) error {
	query := `
		UPDATE dashboard_access_tokens
		SET revoked_at = $1
		WHERE token = $2
	`

	_, err := s.db.ExecContext(ctx, query, time.Now().UTC().Unix(), token)
	if err != nil {
		return fmt.Errorf("failed to revoke token: %w", err)
	}

	return nil
}

// AccountStatus represents the status of a connected account
type AccountStatus string

const (
	AccountStatusOnboarding AccountStatus = "onboarding"
	AccountStatusActive     AccountStatus = "active"
	AccountStatusRestricted AccountStatus = "restricted"
	AccountStatusRejected   AccountStatus = "rejected"
)
