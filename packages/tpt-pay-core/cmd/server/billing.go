package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/subscription"
)

// subscriptionBillingInterval is how often the billing run looks for due subscriptions. A due
// charge waits at most this long; the sandbox clock is what decides whether it is due.
const subscriptionBillingInterval = 2 * time.Second

// runSubscriptionBilling bills due subscriptions until ctx is cancelled.
func runSubscriptionBilling(ctx context.Context, svc *subscription.Service, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n, err := svc.RunDue(ctx, 100)
			if err != nil && ctx.Err() == nil {
				slog.Error("subscription billing run failed", "err", err)
				continue
			}
			if n > 0 {
				slog.Info("subscription billing run", "attempted", n)
			}
		}
	}
}
