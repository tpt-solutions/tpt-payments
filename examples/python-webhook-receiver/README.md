# Python webhook receiver

Verifies and handles TPT Pay webhook deliveries with the `tpt-payments` SDK and the standard library HTTP server.

- The signature is checked over the raw body before parsing (`construct_event`).
- A failed signature returns 400 with no reason in the body. A handler error returns 500, so the sender retries.
- Bodies over 1 MB are refused with 413.

## Run

```bash
pip install -e ../../packages/python
WEBHOOK_SECRET=whsec_... PORT=4242 python receiver.py
```

## Test

```bash
pip install -e ../../packages/python pytest
python -m pytest -q
```
