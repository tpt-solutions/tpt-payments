package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"strconv"
	"strings"
	"time"
)

// SignatureHeader is the request header that carries the signature.
const SignatureHeader = "TPT-Signature"

// DefaultTolerance is the maximum age accepted for a signed timestamp when the caller
// does not specify one. A zero or negative tolerance means "use the default"; it never
// disables the replay check.
const DefaultTolerance = 5 * time.Minute

// maxClockSkew is how far in the future a timestamp may be before it is rejected.
const maxClockSkew = time.Minute

// Verify checks a TPT-Signature header against the payload and secret.
//
// It accepts the header when any v1 signature matches, which supports secret rotation
// (several v1 values may be present while both secrets are valid). It rejects
// timestamps that are too old, or too far in the future.
func Verify(payload []byte, header, secret string, tolerance time.Duration) error {
	ts, sigs, err := parseSignatureHeader(header)
	if err != nil {
		return err
	}

	expected := computeSignature(payload, ts, secret)
	matched := false
	for _, sig := range sigs {
		if hmac.Equal([]byte(sig), []byte(expected)) {
			matched = true
		}
	}
	if !matched {
		return errors.New("webhook: signature mismatch")
	}

	tsUnix, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return errors.New("webhook: malformed timestamp")
	}
	issued := time.Unix(tsUnix, 0)
	if issued.Sub(time.Now()) > maxClockSkew {
		return errors.New("webhook: timestamp is in the future")
	}
	if tolerance <= 0 {
		tolerance = DefaultTolerance
	}
	if age := time.Since(issued); age > tolerance {
		return fmt.Errorf("webhook: timestamp too old (%s > %s)", age.Truncate(time.Second), tolerance)
	}
	return nil
}

func computeSignature(payload []byte, ts, secret string) string {
	mac := hmacSHA256([]byte(secret))
	mac.Write([]byte(ts + "." + string(payload)))
	return hex.EncodeToString(mac.Sum(nil))
}

// parseSignatureHeader extracts the timestamp and every v1 signature.
func parseSignatureHeader(header string) (string, []string, error) {
	var ts string
	var sigs []string
	for _, part := range strings.Split(header, ",") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "t":
			if ts != "" {
				return "", nil, errors.New("webhook: duplicate timestamp in signature header")
			}
			ts = kv[1]
		case "v1":
			sigs = append(sigs, kv[1])
		}
	}
	if ts == "" {
		return "", nil, errors.New("webhook: missing timestamp in signature header")
	}
	if len(sigs) == 0 {
		return "", nil, errors.New("webhook: missing v1 signature")
	}
	return ts, sigs, nil
}

func hmacSHA256(key []byte) hash.Hash {
	return hmac.New(sha256.New, key)
}
