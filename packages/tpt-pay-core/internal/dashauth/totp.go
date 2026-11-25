package dashauth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// The TOTP parameters every authenticator app supports: SHA-1, 6 digits, 30-second steps (RFC 6238).
const (
	totpDigits = 6
	totpPeriod = 30 * time.Second
	// totpWindow accepts the code for the step before and after the current one, to allow for clock skew.
	totpWindow = 1
	secretSize = 20
)

var base32NoPad = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewTOTPSecret returns 160 random bits, the size RFC 4226 recommends for HMAC-SHA1.
func NewTOTPSecret() ([]byte, error) {
	secret := make([]byte, secretSize)
	if _, err := rand.Read(secret); err != nil {
		return nil, fmt.Errorf("dashauth: no randomness for a TOTP secret: %w", err)
	}
	return secret, nil
}

// OTPAuthURI is the otpauth:// URI an authenticator app reads (usually shown as a QR code).
func OTPAuthURI(issuer, account string, secret []byte) string {
	// "@" is escaped too: some authenticator apps read the label more reliably that way.
	label := url.PathEscape(issuer) + ":" + strings.ReplaceAll(url.PathEscape(account), "@", "%40")
	q := url.Values{}
	q.Set("secret", base32NoPad.EncodeToString(secret))
	q.Set("issuer", issuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", strconv.Itoa(totpDigits))
	q.Set("period", strconv.Itoa(int(totpPeriod/time.Second)))
	return "otpauth://totp/" + label + "?" + q.Encode()
}

// totpCode returns the code for one time step (RFC 4226 dynamic truncation).
func totpCode(secret []byte, step int64) string {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(step))
	mac := hmac.New(sha1.New, secret)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	code := value % uint32(pow10(totpDigits))
	return fmt.Sprintf("%0*d", totpDigits, code)
}

// matchTOTP returns the time step a code belongs to, when the code is valid within the window
// around now. It returns false when no step matches. The comparison is constant time.
func matchTOTP(secret []byte, code string, now time.Time) (int64, bool) {
	if len(code) != totpDigits {
		return 0, false
	}
	current := now.Unix() / int64(totpPeriod/time.Second)
	matched := int64(0)
	found := false
	for delta := int64(-totpWindow); delta <= totpWindow; delta++ {
		step := current + delta
		if subtle.ConstantTimeCompare([]byte(totpCode(secret, step)), []byte(code)) == 1 {
			matched = step
			found = true
		}
	}
	return matched, found
}

func pow10(n int) int {
	p := 1
	for i := 0; i < n; i++ {
		p *= 10
	}
	return p
}
