---
sidebar_position: 4
---

# Sandbox

The sandbox card adapter never talks to a real network. It approves anything unless you ask for a specific
outcome. Use these payment methods in test mode.

| Payment method | Result |
|---|---|
| `pm_card_visa` (or any other value) | Approved |
| `pm_card_declined` (`pm_test_declined`) | Declined, `card_declined` |
| `pm_card_insufficient_funds` (`pm_test_insufficient_funds`) | Declined, `insufficient_funds` |
| `pm_card_expired` (`pm_test_expired`) | Declined, `expired_card` |
| `pm_card_cvc_incorrect` (`pm_test_cvc_incorrect`) | Declined, `incorrect_cvc` |
| `pm_card_network_error` (`pm_test_network_error`) | Network error. The outcome is unknown and the intent is left for reconciliation. |
| `pm_card_slow` (`pm_test_slow`) | Approved after two seconds, which exercises client timeouts. |

Each `pm_test_*` name behaves the same as its `pm_card_*` name.

## Chaos

For resilience tests, the server can fail a fraction of adapter calls with a network error:

- `ADAPTER_CHAOS_RATE`: a number from 0 to 1. The default is 0 (off).
- `ADAPTER_CHAOS_SEED`: a fixed seed makes the same calls fail on every run.

A chaos failure is an unknown outcome, as a real timeout is. Chaos applies only to the sandbox adapter.
