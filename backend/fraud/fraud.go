// tpt-payments/backend/fraud/fraud.go
package fraud

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// VelocityLimiter implements per-card fingerprint rate limiting
type VelocityLimiter struct {
	redis *redis.Client
}

// NewVelocityLimiter creates a new velocity limiter
func NewVelocityLimiter(redisClient *redis.Client) *VelocityLimiter {
	return &VelocityLimiter{
		redis: redisClient,
	}
}

// CheckCardVelocity checks if a card has exceeded the allowed number of attempts
func (v *VelocityLimiter) CheckCardVelocity(ctx context.Context, cardFingerprint string, maxAttempts int, window time.Duration) (bool, int, error) {
	key := "fraud:velocity:card:" + cardFingerprint
	
	count, err := v.redis.Incr(ctx, key).Result()
	if err != nil {
		return true, 0, err // Fail open
	}

	if count == 1 {
		v.redis.Expire(ctx, key, window)
	}

	return int(count) <= maxAttempts, int(count), nil
}

// CardTestingDetector detects card testing behavior
type CardTestingDetector struct {
	redis *redis.Client
}

// NewCardTestingDetector creates a new card testing detector
func NewCardTestingDetector(redisClient *redis.Client) *CardTestingDetector {
	return &CardTestingDetector{
		redis: redisClient,
	}
}

// CheckIPActivity checks an IP address for low-value authorization patterns
func (d *CardTestingDetector) CheckIPActivity(ctx context.Context, ipAddress string, threshold int, window time.Duration) (bool, int, error) {
	key := "fraud:cardtesting:ip:" + ipAddress
	
	count, err := v.redis.Incr(ctx, key).Result()
	if err != nil {
		return true, 0, err
	}

	if count == 1 {
		v.redis.Expire(ctx, key, window)
	}

	return int(count) <= threshold, int(count), nil
}

// RiskScore represents a calculated fraud risk score
type RiskScore struct {
	Score      int               `json:"score"` // 0-100
	Action     string            `json:"action"` // allow, flag, block
	Reasons    []string          `json:"reasons"`
	Metadata   map[string]int    `json:"metadata"`
}

// CalculateRiskScore calculates overall fraud risk for a payment attempt
func CalculateRiskScore(velocityCount int, cardTestingCount int) RiskScore {
	score := 0
	reasons := []string{}

	if velocityCount > 5 {
		score += 30
		reasons = append(reasons, "high_card_velocity")
	}

	if cardTestingCount > 3 {
		score += 50
		reasons = append(reasons, "card_testing_pattern")
	}

	action := "allow"
	switch {
	case score >= 75:
		action = "block"
	case score >= 30:
		action = "flag"
	}

	return RiskScore{
		Score:    score,
		Action:   action,
		Reasons:  reasons,
		Metadata: map[string]int{
			"velocity_count": velocityCount,
			"card_testing_count": cardTestingCount,
		},
	}
}