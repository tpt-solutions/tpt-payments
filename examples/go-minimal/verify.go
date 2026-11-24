package main

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

// DefaultTolerance is how old a signed timestamp may be. Matches the core's default.
const DefaultTolerance = 5 * time.Minute

// maxClockSkew is how far in the future a timestamp may be.
const maxClockSkew = time.Minute

// verifySignature checks a TPT-Signature header against the raw request body.
//
// The signature is HMAC-SHA256 of "<t>.<raw body>", keyed with the endpoint's full secret string
// (including the whsec_ prefix). Any v1 value that matches is accepted, which supports secret rotation.
// The body must be the exact bytes received: parsing and re-encoding it changes the signature.
func verifySignature(body []byte, header, secret string, now time.Time, tolerance time.Duration) error {
	ts, sigs, err := parseSignatureHeader(header)
	if err != nil {
		return err
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "." + string(body)))
	expected := hex.EncodeToString(mac.Sum(nil))

	matched := false
	for _, sig := range sigs {
		if hmac.Equal([]byte(sig), []byte(expected)) {
			matched = true
		}
	}
	if !matched {
		return errors.New("signature does not match")
	}

	unix, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return errors.New("malformed timestamp")
	}
	issued := time.Unix(unix, 0)
	if issued.After(now.Add(maxClockSkew)) {
		return errors.New("timestamp is in the future")
	}
	if tolerance <= 0 {
		tolerance = DefaultTolerance
	}
	if age := now.Sub(issued); age > tolerance {
		return fmt.Errorf("timestamp too old (%s)", age.Truncate(time.Second))
	}
	return nil
}

// parseSignatureHeader returns the timestamp and every v1 signature in the header.
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
			ts = kv[1]
		case "v1":
			sigs = append(sigs, kv[1])
		}
	}
	if ts == "" || len(sigs) == 0 {
		return "", nil, errors.New("missing timestamp or signature")
	}
	return ts, sigs, nil
}
