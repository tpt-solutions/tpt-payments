// tpt-payments/backend/multicurrency/providers/ecb.go
package providers

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"time"

	"github.com/shopspring/decimal"
)

const ECB_URL = "https://www.ecb.europa.eu/stats/eurofxref/eurofxref-daily.xml"

// ECBProvider implements ExchangeRateProvider using European Central Bank daily rates
type ECBProvider struct {
	httpClient *http.Client
}

// NewECBProvider creates a new ECB exchange rate provider
func NewECBProvider() *ECBProvider {
	return &ECBProvider{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

type Envelope struct {
	XMLName xml.Name `xml:"Envelope"`
	Subject string   `xml:"subject"`
	Sender  Sender   `xml:"Sender"`
	Cube    Cube     `xml:"Cube"`
}

type Sender struct {
	Name string `xml:"name"`
}

type Cube struct {
	Time string     `xml:"time,attr"`
	Cube []CubeItem `xml:"Cube"`
}

type CubeItem struct {
	Currency string  `xml:"currency,attr"`
	Rate     float64 `xml:"rate,attr"`
}

// GetRate retrieves exchange rate from ECB
func (e *ECBProvider) GetRate(ctx context.Context, baseCurrency string, targetCurrency string) (decimal.Decimal, error) {
	rates, err := e.GetRates(ctx, baseCurrency, []string{targetCurrency})
	if err != nil {
		return decimal.Zero, err
	}

	rate, ok := rates[targetCurrency]
	if !ok {
		return decimal.Zero, fmt.Errorf("rate not found for currency: %s", targetCurrency)
	}

	return rate, nil
}

// GetRates retrieves multiple exchange rates from ECB
func (e *ECBProvider) GetRates(ctx context.Context, baseCurrency string, targetCurrencies []string) (map[string]decimal.Decimal, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", ECB_URL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var envelope Envelope
	err = xml.NewDecoder(resp.Body).Decode(&envelope)
	if err != nil {
		return nil, err
	}

	// ECB rates are all relative to EUR
	eurRates := make(map[string]decimal.Decimal)
	eurRates["EUR"] = decimal.NewFromFloat(1.0)

	for _, cube := range envelope.Cube.Cube {
		eurRates[cube.Currency] = decimal.NewFromFloat(cube.Rate)
	}

	result := make(map[string]decimal.Decimal)

	for _, target := range targetCurrencies {
		if baseCurrency == target {
			result[target] = decimal.NewFromFloat(1.0)
			continue
		}

		// Convert via EUR base
		baseRate, ok := eurRates[baseCurrency]
		if !ok {
			return nil, fmt.Errorf("base currency not supported: %s", baseCurrency)
		}

		targetRate, ok := eurRates[target]
		if !ok {
			return nil, fmt.Errorf("target currency not supported: %s", target)
		}

		// Calculate rate: target = (targetRate / baseRate) * amount
		result[target] = targetRate.Div(baseRate)
	}

	return result, nil
}