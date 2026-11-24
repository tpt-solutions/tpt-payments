package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"
)

const testSecret = "whsec_test_secret_for_examples"

// sign produces the header the core would send for body at t, using the documented algorithm.
func sign(body string, t time.Time, secrets ...string) string {
	ts := fmt.Sprintf("%d", t.Unix())
	header := "t=" + ts
	for _, s := range secrets {
		mac := hmac.New(sha256.New, []byte(s))
		mac.Write([]byte(ts + "." + body))
		header += ",v1=" + hex.EncodeToString(mac.Sum(nil))
	}
	return header
}

func TestVerify_AcceptsAFreshSignedBody(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	body := `{"id":"evt_1","type":"payment_intent.succeeded"}`
	if err := verifySignature([]byte(body), sign(body, now, testSecret), testSecret, now, 0); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
}

func TestVerify_RejectsAWrongSecret(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	body := `{"id":"evt_1"}`
	err := verifySignature([]byte(body), sign(body, now, "whsec_other"), testSecret, now, 0)
	if err == nil {
		t.Fatal("signature made with another secret was accepted")
	}
}

func TestVerify_RejectsATamperedBody(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	header := sign(`{"amount":1000}`, now, testSecret)
	if err := verifySignature([]byte(`{"amount":9999}`), header, testSecret, now, 0); err == nil {
		t.Fatal("tampered body was accepted")
	}
}

func TestVerify_AcceptsAnyMatchingV1DuringRotation(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	body := `{"id":"evt_2"}`
	header := sign(body, now, "whsec_retired", testSecret)
	if err := verifySignature([]byte(body), header, testSecret, now, 0); err != nil {
		t.Fatalf("rotation with two v1 values rejected: %v", err)
	}
}

func TestVerify_RejectsStaleAndFutureTimestamps(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	body := `{"id":"evt_3"}`

	old := sign(body, now.Add(-6*time.Minute), testSecret)
	if err := verifySignature([]byte(body), old, testSecret, now, 0); err == nil || !strings.Contains(err.Error(), "too old") {
		t.Fatalf("stale timestamp: err = %v", err)
	}

	future := sign(body, now.Add(2*time.Minute), testSecret)
	if err := verifySignature([]byte(body), future, testSecret, now, 0); err == nil || !strings.Contains(err.Error(), "future") {
		t.Fatalf("future timestamp: err = %v", err)
	}
}

func TestVerify_ZeroToleranceUsesTheDefaultNotNoCheck(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	body := `{"id":"evt_4"}`
	old := sign(body, now.Add(-time.Hour), testSecret)
	if err := verifySignature([]byte(body), old, testSecret, now, 0); err == nil {
		t.Fatal("zero tolerance disabled the replay check")
	}
}

func TestVerify_MalformedHeadersAreRefused(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	for _, header := range []string{"", "t=1700000000", "v1=abcd", "t=notanumber,v1=abcd"} {
		if err := verifySignature([]byte("{}"), header, testSecret, now, 0); err == nil {
			t.Errorf("header %q was accepted", header)
		}
	}
}
