package dashauth

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The RFC 6238 SHA-1 test vectors. The secret is the ASCII string "12345678901234567890"; the
// published 8-digit codes end in the 6-digit codes below.
func TestTOTP_MatchesTheRFC6238Vectors(t *testing.T) {
	secret := []byte("12345678901234567890")
	vectors := map[int64]string{
		59:         "287082",
		1111111109: "081804",
		1111111111: "050471",
		1234567890: "005924",
		2000000000: "279037",
	}
	for unix, want := range vectors {
		step := unix / int64(totpPeriod/time.Second)
		assert.Equal(t, want, totpCode(secret, step), "time %d", unix)
	}
}

func TestTOTP_AcceptsAdjacentStepsOnly(t *testing.T) {
	secret := []byte("12345678901234567890")
	now := time.Unix(1111111109, 0)
	current := now.Unix() / 30

	step, ok := matchTOTP(secret, totpCode(secret, current), now)
	require.True(t, ok)
	assert.Equal(t, current, step)

	step, ok = matchTOTP(secret, totpCode(secret, current-1), now)
	require.True(t, ok, "the previous step is accepted for clock skew")
	assert.Equal(t, current-1, step)

	_, ok = matchTOTP(secret, totpCode(secret, current-2), now)
	assert.False(t, ok, "two steps away is refused")

	_, ok = matchTOTP(secret, "12345", now)
	assert.False(t, ok, "a short code is refused")
}

func TestTOTP_OTPAuthURICarriesTheParameters(t *testing.T) {
	uri := OTPAuthURI("TPT Pay", "owner@example.com", []byte("12345678901234567890"))
	assert.True(t, strings.HasPrefix(uri, "otpauth://totp/TPT%20Pay:owner%40example.com?"), uri)
	assert.Contains(t, uri, "secret=GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ")
	assert.Contains(t, uri, "digits=6")
	assert.Contains(t, uri, "period=30")
	assert.Contains(t, uri, "algorithm=SHA1")
}

func TestPassword_RoundTripAndRefusals(t *testing.T) {
	hash, err := HashPassword("correct horse battery", minIterations)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(hash, "pbkdf2-sha256$100000$"))

	ok, err := VerifyPassword("correct horse battery", hash)
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = VerifyPassword("wrong", hash)
	require.NoError(t, err)
	assert.False(t, ok)

	// Two hashes of one password differ, because each has its own salt.
	other, err := HashPassword("correct horse battery", minIterations)
	require.NoError(t, err)
	assert.NotEqual(t, hash, other)

	_, err = VerifyPassword("x", "scrypt$abc$def")
	assert.Error(t, err, "a hash from another scheme is not a match")
	_, err = VerifyPassword("x", "pbkdf2-sha256$10$AAAA$AAAA")
	assert.Error(t, err, "a work factor below the minimum is refused")
	_, err = HashPassword("x", 1000)
	assert.Error(t, err, "new hashes below the minimum are refused")
}

func TestCounter_FifthFailureLocksAndStartsAgain(t *testing.T) {
	now := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)
	var c counter
	for i := 0; i < maxFailures-1; i++ {
		c = c.afterFailure(now)
		assert.False(t, c.locked(now), "failure %d must not lock", i+1)
	}
	c = c.afterFailure(now)
	assert.True(t, c.locked(now), "the fifth failure locks")
	assert.False(t, c.locked(now.Add(lockDuration)), "the lock ends after its duration")
	assert.Equal(t, 0, c.failures, "the count starts again after a lock")
}

func TestCounter_OldFailuresExpire(t *testing.T) {
	now := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)
	var c counter
	for i := 0; i < maxFailures-1; i++ {
		c = c.afterFailure(now)
	}
	// A failure long after the others starts a new count, so four old failures do not lock.
	c = c.afterFailure(now.Add(failureWindow + time.Minute))
	assert.False(t, c.locked(now.Add(failureWindow+time.Minute)))
	assert.Equal(t, 1, c.failures)
}
