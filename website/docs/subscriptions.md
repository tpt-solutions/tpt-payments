---
sidebar_position: 3
---

# Subscriptions

> Test mode only. Live keys get `403` until a live billing policy is decided.

A subscription charges a fixed amount to a payment method at the start of each period.

```bash
curl https://api.tpt-payments.com/v1/subscriptions \
  -u sk_test_...: \
  -d amount=1500 -d currency=usd -d interval=month -d payment_method=pm_card_visa
```

`interval` is `week`, `month` or `year`. Month and year periods follow the calendar: a period that starts on 31
January ends on the last day of February, or on 28 February in a non-leap year.

## What happens

- **Creation charges the first period** before the response. The response's `status` shows the outcome:
  `active` if it succeeded, `past_due` if it was declined.
- **Each later period is charged automatically** when it starts. The subscription's
  `current_period_start` is the period the next charge pays for.
- **A declined charge is retried** after 1 day, then 3 days, then 5 days from the first failure. The
  subscription is `past_due` while it is being retried. If the last retry is declined, the subscription is
  `canceled` with `cancel_reason: payment_failed`.
- **An unknown outcome is held, not retried.** If the card network does not answer, the charge may or may not
  have happened. The subscription is `past_due` and is checked again daily; it is never charged twice for the
  same period and is not canceled by a network failure.
- **Every charge is a payment intent** tagged with the subscription and period, so it appears in the payments list.

## Cancelling

- `POST /v1/subscriptions/{id}/cancel` cancels now (`cancel_reason: requested`).
- `POST /v1/subscriptions/{id}/cancel` with `{"at_period_end": true}` keeps it running until the paid period
  ends (`cancel_reason: canceled_at_period_end`).

A canceled subscription cannot be canceled again, and no later period is charged. A charge that was already
in progress when you cancel can still complete; cancelling does not refund it.

## Events

| Event | When |
|---|---|
| `subscription.created` | The subscription is created. |
| `subscription.past_due` | The first charge of a period fails, or an unknown outcome holds it. |
| `subscription.canceled` | Canceled by the merchant, or the retries run out. |

A renewal that succeeds is reported by the `payment_intent.succeeded` event of its payment intent.

## Testing time

In the sandbox, subscriptions follow the sandbox clock. Advance it with `POST /v1/test_clocks/advance` to renew
a subscription without waiting. The billing run looks for due subscriptions every two seconds.
