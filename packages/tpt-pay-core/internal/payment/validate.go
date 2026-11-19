package payment

import (
	"strings"

	"golang.org/x/text/currency"

	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/internal/apierrors"
)

// MaxAmount is the largest amount accepted, in minor units. It is a guard against
// fat-fingered values and integer misuse, not a card-network limit.
const MaxAmount int64 = 1_000_000_000

const (
	maxMetadataKeys     = 50
	maxMetadataKeyLen   = 40
	maxMetadataValueLen = 500
)

// NormalizeCurrency checks an ISO 4217 code and returns it upper-cased.
func NormalizeCurrency(code string) (string, error) {
	upper := strings.ToUpper(strings.TrimSpace(code))
	if len(upper) != 3 {
		return "", apierrors.InvalidParam("currency", "currency must be a 3-letter ISO 4217 code")
	}
	if _, err := currency.ParseISO(upper); err != nil {
		return "", apierrors.InvalidParam("currency", "unknown ISO 4217 currency code: "+upper)
	}
	return upper, nil
}

// ValidateAmount checks an amount in minor units against the platform bounds.
func ValidateAmount(amount int64, currencyCode string) error {
	if amount <= 0 {
		return apierrors.InvalidParam("amount", "amount must be greater than zero")
	}
	if amount > MaxAmount {
		return apierrors.InvalidParam("amount", "amount exceeds the maximum allowed")
	}
	return nil
}

// MinorUnitScale returns how many decimal places the currency has in its smallest unit
// (2 for USD, 0 for JPY). The code must already be validated.
func MinorUnitScale(code string) int {
	unit, err := currency.ParseISO(code)
	if err != nil {
		return 2
	}
	scale, _ := currency.Standard.Rounding(unit)
	return scale
}

// ValidateMetadata enforces key and value limits so metadata cannot grow without bound.
func ValidateMetadata(m map[string]string) error {
	if len(m) > maxMetadataKeys {
		return apierrors.InvalidParam("metadata", "metadata may have at most 50 keys")
	}
	for k, v := range m {
		if k == "" || len(k) > maxMetadataKeyLen {
			return apierrors.InvalidParam("metadata", "metadata keys must be 1 to 40 characters")
		}
		if len(v) > maxMetadataValueLen {
			return apierrors.InvalidParam("metadata", "metadata values must be at most 500 characters")
		}
	}
	return nil
}

// ValidCaptureMethod reports whether s is a supported capture method. Empty means default.
func ValidCaptureMethod(s string) error {
	switch s {
	case "", string(CaptureAutomatic), string(CaptureManual):
		return nil
	}
	return apierrors.InvalidParam("capture_method", "capture_method must be 'automatic' or 'manual'")
}

// ValidConfirmationMethod reports whether s is a supported confirmation method.
func ValidConfirmationMethod(s string) error {
	switch s {
	case "", "automatic", "manual":
		return nil
	}
	return apierrors.InvalidParam("confirmation_method", "confirmation_method must be 'automatic' or 'manual'")
}
