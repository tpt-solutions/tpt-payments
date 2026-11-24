---
sidebar_position: 1
---

# API reference

All endpoints are under `/v1` and take `Authorization: Bearer <key>`. The OpenAPI 3 document for every route
is served by the server at `/openapi.json`, and is also in this site at
[`/openapi.json`](pathname:///openapi.json).

## Endpoints

| Method and path | Key | Purpose |
|---|---|---|
| `POST /v1/payment_intents` | secret | Create a payment intent. |
| `GET /v1/payment_intents` | secret | List intents, newest first. |
| `GET /v1/payment_intents/{id}` | any | Retrieve an intent. |
| `POST /v1/payment_intents/{id}/confirm` | any | Confirm with a payment method. |
| `POST /v1/payment_intents/{id}/capture` | secret | Capture, in full or in part. |
| `POST /v1/payment_intents/{id}/cancel` | secret | Cancel before capture. |
| `POST /v1/refunds` | secret | Refund a captured intent. |
| `GET /v1/refunds` | secret | List refunds. |
| `GET /v1/refunds/{id}` | any | Retrieve a refund. |
| `POST /v1/webhook_endpoints` | secret | Create an endpoint. The secret is returned once. |
| `GET /v1/webhook_endpoints/{id}` | secret | Retrieve an endpoint. |
| `DELETE /v1/webhook_endpoints/{id}` | secret | Delete an endpoint. |
| `GET /v1/webhook_deliveries` | secret | List recent deliveries. |
| `POST /v1/webhook_deliveries/{id}/replay` | secret | Send a delivery again. |
| `POST /v1/policy/evaluate` | secret | Dry-run the merchant rules. |
| `POST /v1/api_keys` | secret | Create an API key. |
| `DELETE /v1/api_keys/{id}` | secret | Revoke an API key. |

"Any" means a publishable or a secret key. Publishable keys (`pk_`) may only retrieve and confirm intents.

Operational endpoints without authentication: `GET /health`, `GET /readyz` and `GET /metrics`.
