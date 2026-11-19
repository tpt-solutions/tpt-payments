package payment

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeCurrency(t *testing.T) {
	code, err := NormalizeCurrency(" usd ")
	require.NoError(t, err)
	assert.Equal(t, "USD", code)

	for _, bad := range []string{"", "US", "USDX", "XXX1", "ZZZ"} {
		_, err := NormalizeCurrency(bad)
		assert.Error(t, err, "currency %q must be rejected", bad)
	}
}

func TestMinorUnitScale(t *testing.T) {
	assert.Equal(t, 2, MinorUnitScale("USD"))
	assert.Equal(t, 0, MinorUnitScale("JPY"), "yen has no minor unit")
	assert.Equal(t, 0, MinorUnitScale("KRW"))
}

func TestValidateAmount(t *testing.T) {
	assert.NoError(t, ValidateAmount(1, "USD"))
	assert.NoError(t, ValidateAmount(MaxAmount, "USD"))
	assert.Error(t, ValidateAmount(0, "USD"))
	assert.Error(t, ValidateAmount(-5, "USD"))
	assert.Error(t, ValidateAmount(MaxAmount+1, "USD"))
}

func TestValidateMetadata(t *testing.T) {
	assert.NoError(t, ValidateMetadata(map[string]string{"order": "42"}))
	assert.Error(t, ValidateMetadata(map[string]string{"": "x"}))
	assert.Error(t, ValidateMetadata(map[string]string{strings.Repeat("k", 41): "x"}))
	assert.Error(t, ValidateMetadata(map[string]string{"big": strings.Repeat("v", 501)}))

	many := map[string]string{}
	for i := 0; i < 51; i++ {
		many[strings.Repeat("k", 1)+string(rune('a'+i%26))+string(rune('a'+i/26))] = "v"
	}
	assert.Error(t, ValidateMetadata(many))
}

func TestMethodValidation(t *testing.T) {
	assert.NoError(t, ValidCaptureMethod(""))
	assert.NoError(t, ValidCaptureMethod("manual"))
	assert.Error(t, ValidCaptureMethod("delayed"), "unknown capture methods must not silently default")
	assert.Error(t, ValidConfirmationMethod("sometimes"))
}
