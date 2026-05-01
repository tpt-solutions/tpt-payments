// tpt-payments/backend/fraud/rules_test.go
package fraud

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

func setupTestEngine(t *testing.T) (*RuleEngine, *miniredis.Miniredis, func()) {
	s, err := miniredis.Run()
	if err != nil {
		t.Fatalf("could not start miniredis: %v", err)
	}

	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})

	engine := NewRuleEngine(client)
	cleanup := func() {
		client.Close()
		s.Close()
	}

	return engine, s, cleanup
}

func TestCheckCardVelocity(t *testing.T) {
	engine, s, cleanup := setupTestEngine(t)
	defer cleanup()

	ctx := context.Background()
	req := AuthorizationRequest{
		CardFingerprint: "test_fingerprint_123",
	}

	// Test under threshold
	for i := 0; i < 9; i++ {
		result := engine.checkCardVelocity(ctx, req)
		assert.False(t, result.Blocked)
		assert.Equal(t, i+1*5, result.Score)
	}

	// Test at threshold (10th attempt)
	result := engine.checkCardVelocity(ctx, req)
	assert.True(t, result.Blocked)
	assert.Equal(t, 100, result.Score)
	assert.Equal(t, "card_velocity_exceeded", result.Reason)
	assert.Equal(t, "card_velocity_limiter", result.RuleName)

	// Verify expiry
	s.FastForward(61 * time.Minute)
	result = engine.checkCardVelocity(ctx, req)
	assert.False(t, result.Blocked)
	assert.Equal(t, 5, result.Score)
}

func TestCheckCardTestingPattern(t *testing.T) {
	engine, s, cleanup := setupTestEngine(t)
	defer cleanup()

	ctx := context.Background()

	// Test normal amount (not card testing)
	reqNormal := AuthorizationRequest{
		Amount:   500, // $5.00
		ClientIP: "192.168.1.1",
	}
	result := engine.checkCardTestingPattern(ctx, reqNormal)
	assert.False(t, result.Flagged)
	assert.Equal(t, 0, result.Score)

	// Test low amount pattern
	reqLow := AuthorizationRequest{
		Amount:   100, // $1.00
		ClientIP: "192.168.1.2",
	}

	// First 4 attempts should not flag
	for i := 0; i < 4; i++ {
		result = engine.checkCardTestingPattern(ctx, reqLow)
		assert.False(t, result.Flagged)
	}

	// 5th attempt should flag
	result = engine.checkCardTestingPattern(ctx, reqLow)
	assert.True(t, result.Flagged)
	assert.Equal(t, 75, result.Score)
	assert.Equal(t, "card_testing_detected", result.Reason)
	assert.Equal(t, "card_testing_detector", result.RuleName)

	// Verify expiry
	s.FastForward(16 * time.Minute)
	result = engine.checkCardTestingPattern(ctx, reqLow)
	assert.False(t, result.Flagged)
}

func TestCheckBINBlocklist(t *testing.T) {
	engine, s, cleanup := setupTestEngine(t)
	defer cleanup()

	ctx := context.Background()

	// Add blocked BIN
	s.SAdd("fraud:blocked_bins", "411111")

	reqBlocked := AuthorizationRequest{
		CardBIN: "411111",
	}
	result := engine.checkBINBlocklist(ctx, reqBlocked)
	assert.True(t, result.Blocked)
	assert.Equal(t, 100, result.Score)
	assert.Equal(t, "bin_blocked", result.Reason)
	assert.Equal(t, "bin_blocklist", result.RuleName)

	reqAllowed := AuthorizationRequest{
		CardBIN: "555555",
	}
	result = engine.checkBINBlocklist(ctx, reqAllowed)
	assert.False(t, result.Blocked)
	assert.Equal(t, 0, result.Score)
}

func TestCheckIPBlocklist(t *testing.T) {
	engine, s, cleanup := setupTestEngine(t)
	defer cleanup()

	ctx := context.Background()

	// Add blocked IP
	s.SAdd("fraud:blocked_ips", "10.0.0.1")

	reqBlocked := AuthorizationRequest{
		ClientIP: "10.0.0.1",
	}
	result := engine.checkIPBlocklist(ctx, reqBlocked)
	assert.True(t, result.Blocked)
	assert.Equal(t, 100, result.Score)
	assert.Equal(t, "ip_blocked", result.Reason)
	assert.Equal(t, "ip_blocklist", result.RuleName)

	reqAllowed := AuthorizationRequest{
		ClientIP: "10.0.0.2",
	}
	result = engine.checkIPBlocklist(ctx, reqAllowed)
	assert.False(t, result.Blocked)
	assert.Equal(t, 0, result.Score)

	// Test invalid IP
	reqInvalid := AuthorizationRequest{
		ClientIP: "not_an_ip",
	}
	result = engine.checkIPBlocklist(ctx, reqInvalid)
	assert.False(t, result.Blocked)
	assert.Equal(t, 0, result.Score)
}

func TestEvaluateFullEngine(t *testing.T) {
	engine, s, cleanup := setupTestEngine(t)
	defer cleanup()

	ctx := context.Background()

	// Add blocked IP
	s.SAdd("fraud:blocked_ips", "192.168.1.100")

	// Test blocked request
	req := AuthorizationRequest{
		Amount:          1000,
		Currency:        "usd",
		CardFingerprint: "test_fp",
		CardBIN:         "424242",
		ClientIP:        "192.168.1.100",
		MerchantID:      "merch_123",
	}

	result := engine.Evaluate(ctx, req)
	assert.True(t, result.Blocked)
	assert.False(t, result.Allowed)
	assert.Equal(t, 100, result.Score)
	assert.Equal(t, "ip_blocked", result.Reason)

	// Test allowed request
	req.ClientIP = "192.168.1.101"
	result = engine.Evaluate(ctx, req)
	assert.False(t, result.Blocked)
	assert.True(t, result.Allowed)
	assert.Equal(t, 5, result.Score)
}

func TestRuleAggregation(t *testing.T) {
	engine, s, cleanup := setupTestEngine(t)
	defer cleanup()

	ctx := context.Background()

	// Test multiple rules contributing to score
	req := AuthorizationRequest{
		Amount:          150, // $1.50
		CardFingerprint: "test_fp_multi",
		ClientIP:        "10.0.0.5",
	}

	// Run multiple times to build velocity
	for i := 0; i < 3; i++ {
		engine.checkCardVelocity(ctx, req)
		engine.checkCardTestingPattern(ctx, req)
	}

	result := engine.Evaluate(ctx, req)
	assert.False(t, result.Blocked)
	assert.True(t, result.Flagged)
	assert.Equal(t, 20, result.Score) // 4 * 5 from velocity
}

func TestBoundaryConditions(t *testing.T) {
	engine, _, cleanup := setupTestEngine(t)
	defer cleanup()

	ctx := context.Background()

	// Exact amount threshold
	req := AuthorizationRequest{
		Amount:   200, // Exact $2.00 threshold
		ClientIP: "192.168.1.50",
	}

	result := engine.checkCardTestingPattern(ctx, req)
	assert.False(t, result.Flagged, "Amount equal to threshold should not trigger card testing rule")

	// Zero amount
	req.Amount = 0
	result = engine.checkCardTestingPattern(ctx, req)
	assert.False(t, result.Flagged)

	// Negative amount (should never happen in production but test gracefully)
	req.Amount = -100
	result = engine.checkCardTestingPattern(ctx, req)
	assert.False(t, result.Flagged)
}