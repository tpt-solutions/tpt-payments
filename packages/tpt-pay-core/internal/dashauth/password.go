// Package dashauth stores and checks the users who sign in to the merchant dashboard: their
// passwords (PBKDF2-SHA256), their second factor (TOTP, RFC 6238) and the lockout that protects
// both from guessing. The frontend calls it over an internal route; no end user reaches it directly.
package dashauth

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// DefaultIterations is the PBKDF2-SHA256 work factor for new passwords (OWASP guidance, 2023).
// The count is stored with each hash, so it can be raised later without invalidating old hashes.
const DefaultIterations = 600_000

// minIterations refuses a stored hash that claims a work factor too low to be safe.
const minIterations = 100_000

const passwordScheme = "pbkdf2-sha256"

// HashPassword returns a self-describing hash: pbkdf2-sha256$<iterations>$<salt>$<key>.
func HashPassword(password string, iterations int) (string, error) {
	if iterations < minIterations {
		return "", fmt.Errorf("dashauth: iterations must be at least %d", minIterations)
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, iterations, 32)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s$%d$%s$%s", passwordScheme, iterations,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

// VerifyPassword reports whether password matches a stored hash. The comparison is constant time.
// A malformed hash is an error, not a match.
func VerifyPassword(password, stored string) (bool, error) {
	parts := strings.Split(stored, "$")
	if len(parts) != 4 || parts[0] != passwordScheme {
		return false, errors.New("dashauth: unrecognized password hash")
	}
	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations < minIterations {
		return false, errors.New("dashauth: password hash has an unsafe work factor")
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return false, errors.New("dashauth: password hash salt is malformed")
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || len(want) == 0 {
		return false, errors.New("dashauth: password hash key is malformed")
	}
	got, err := pbkdf2.Key(sha256.New, password, salt, iterations, len(want))
	if err != nil {
		return false, err
	}
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}
