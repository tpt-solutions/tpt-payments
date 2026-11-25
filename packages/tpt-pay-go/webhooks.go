package tptpay

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// SignatureHeader is the header that carries a webhook signature.
const SignatureHeader = "TPT-Signature"

// DefaultTolerance is how old a signed timestamp may be before it is refused.
const DefaultTolerance = 5 * time.Minute

// maxClockSkew is how far in the future a timestamp may be.
const maxClockSkew = time.Minute

// ErrSignature is returned when a webhook does not verify. Do not process the event.
var ErrSignature = errors.New("tptpay: webhook signature verification failed")

// VerifyWebhook checks a TPT-Signature header against the raw request body and the endpoint's
// signing secret (whsec_...). Pass the body exactly as received, before any JSON decoding. A
// tolerance of 0 uses DefaultTolerance; a timestamp outside it, or in the future, is refused.
//
// It accepts the header when any v1 signature matches, so both secrets are valid during rotation.
func VerifyWebhook(payload []byte, header, secret string, tolerance time.Duration) error {
	ts, sigs, err := parseSignatureHeader(header)
	if err != nil {
		return err
	}
	matched := false
	expected := computeSignature(payload, ts, secret)
	for _, sig := range sigs {
		if hmac.Equal([]byte(sig), []byte(expected)) {
			matched = true
		}
	}
	if !matched {
		return ErrSignature
	}

	secs, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return fmt.Errorf("%w: bad timestamp", ErrSignature)
	}
	issued := time.Unix(secs, 0)
	now := time.Now()
	if issued.After(now.Add(maxClockSkew)) {
		return fmt.Errorf("%w: timestamp is in the future", ErrSignature)
	}
	if tolerance <= 0 {
		tolerance = DefaultTolerance
	}
	if age := now.Sub(issued); age > tolerance {
		return fmt.Errorf("%w: timestamp is too old (%s)", ErrSignature, age.Truncate(time.Second))
	}
	return nil
}

// SignWebhook returns a TPT-Signature header for a payload, for tests of a receiver.
func SignWebhook(payload []byte, secret string, at time.Time) string {
	ts := strconv.FormatInt(at.Unix(), 10)
	return "t=" + ts + ",v1=" + computeSignature(payload, ts, secret)
}

func computeSignature(payload []byte, ts, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "." + string(payload)))
	return hex.EncodeToString(mac.Sum(nil))
}

// parseSignatureHeader returns the timestamp and every v1 signature in the header.
func parseSignatureHeader(header string) (string, []string, error) {
	var ts string
	var sigs []string
	for _, part := range strings.Split(header, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		switch k {
		case "t":
			ts = v
		case "v1":
			sigs = append(sigs, v)
		}
	}
	if ts == "" || len(sigs) == 0 {
		return "", nil, fmt.Errorf("%w: malformed header", ErrSignature)
	}
	return ts, sigs, nil
}
