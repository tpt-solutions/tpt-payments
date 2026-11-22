// tpt-payments/backend/multicurrency/currency.go
package multicurrency

import (
	"context"
	"database/sql"
	"time"

	"github.com/shopspring/decimal"
)

// Currency represents a supported currency
type Currency struct {
	Code           string          `json:"code"`
	Name           string          `json:"name"`
	Symbol         string          `json:"symbol"`
	DecimalPlaces  int             `json:"decimal_places"`
	MinimumAmount  decimal.Decimal `json:"minimum_amount"`
	Enabled        bool            `json:"enabled"`
	CreatedAt      int64           `json:"created_at"`
	UpdatedAt      int64           `json:"updated_at"`
}

// ExchangeRate represents an exchange rate between two currencies
type ExchangeRate struct {
	BaseCurrency   string          `json:"base_currency"`
	TargetCurrency string        `json:"target_currency"`
	Rate        decimal.Decimal `json:"rate"`
	RateTime    int64           `json:"rate_time"`
	Provider    string          `json:"provider"`
}

// ExchangeRateProvider interface for external rate sources
type ExchangeRateProvider interface {
	GetRate(ctx context.Context, baseCurrency string, targetCurrency string) (decimal.Decimal, error)
	GetRates(ctx context.Context, baseCurrency string, targetCurrencies []string) (map[string]decimal.Decimal, error)
}

// Service handles currency operations
type Service struct {
	db         *sql.DB
	rateProvider ExchangeRateProvider
}

// NewService creates multicurrency service
func NewService(db *sql.DB, rateProvider ExchangeRateProvider) *Service {
	return &Service{
		db:         db,
		rateProvider: rateProvider,
	}
}

// Convert converts an amount from one currency to another
func (s *Service) Convert(ctx context.Context, amount decimal.Decimal, fromCurrency string, toCurrency string) (decimal.Decimal, error) {
	if fromCurrency == toCurrency {
		return amount, nil
	}

	rate, err := s.rateProvider.GetRate(ctx, fromCurrency, toCurrency)
	if err != nil {
		return decimal.Zero, err
	}

	return amount.Mul(rate).RoundBank(2), nil
}

// ConvertPayment converts payment amount to merchant home currency
// Returns converted amount, exchange rate used, and error
func (s *Service) ConvertPayment(ctx context.Context, merchantID string, amount decimal.Decimal, paymentCurrency string) (decimal.Decimal, decimal.Decimal, error) {
	// Check if merchant has multi-currency enabled
	var enabled bool
	err := s.db.QueryRowContext(ctx, `
		SELECT multi_currency_enabled FROM merchants WHERE id = $1`,
		merchantID,
	).Scan(&enabled)
	if err != nil {
		return decimal.Zero, decimal.Zero, err
	}

	if !enabled {
		// Multi-currency disabled, return original amount
		return amount, decimal.NewFromInt(1), nil
	}

	// Get merchant home currency
	var homeCurrency string
	err = s.db.QueryRowContext(ctx, `
		SELECT home_currency FROM merchants WHERE id = $1`,
		merchantID,
	).Scan(&homeCurrency)
	if err != nil {
		return decimal.Zero, decimal.Zero, err
	}

	if paymentCurrency == homeCurrency {
		return amount, decimal.NewFromInt(1), nil
	}

	rate, err := s.rateProvider.GetRate(ctx, paymentCurrency, homeCurrency)
	if err != nil {
		return decimal.Zero, decimal.Zero, err
	}

	converted := amount.Mul(rate).RoundBank(2)
	return converted, rate, nil
}

// GetSupportedCurrencies returns all enabled currencies
func (s *Service) GetSupportedCurrencies(ctx context.Context) ([]Currency, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT code, name, symbol, decimal_places, minimum_amount, enabled, created_at, updated_at
		FROM currencies WHERE enabled = true
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var currencies []Currency
	for rows.Next() {
		var c Currency
		err := rows.Scan(
			&c.Code, &c.Name, &c.Symbol, &c.DecimalPlaces,
			&c.MinimumAmount, &c.Enabled, &c.CreatedAt, &c.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		currencies = append(currencies, c)
	}

	return currencies, nil
}

// GetMerchantCurrencies returns currencies enabled for a merchant
func (s *Service) GetMerchantCurrencies(ctx context.Context, merchantID string) ([]Currency, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.code, c.name, c.symbol, c.decimal_places, c.minimum_amount, c.enabled, c.created_at, c.updated_at
		FROM currencies c
		JOIN merchant_currencies mc ON c.code = mc.currency_code
		WHERE mc.merchant_id = $1 AND c.enabled = true AND mc.enabled = true`,
		merchantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var currencies []Currency
	for rows.Next() {
		var c Currency
		err := rows.Scan(
			&c.Code, &c.Name, &c.Symbol, &c.DecimalPlaces,
			&c.MinimumAmount, &c.Enabled, &c.CreatedAt, &c.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		currencies = append(currencies, c)
	}

	return currencies, nil
}

// SetMerchantCurrency enables/disables a currency for a merchant
func (s *Service) SetMerchantCurrency(ctx context.Context, merchantID string, currencyCode string, enabled bool) error {
	now := time.Now().Unix()

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO merchant_currencies (merchant_id, currency_code, enabled, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $4)
		ON CONFLICT (merchant_id, currency_code) DO UPDATE SET enabled = $3, updated_at = $4`,
		merchantID, currencyCode, enabled, now,
	)

	return err
}