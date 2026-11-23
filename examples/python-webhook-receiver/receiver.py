"""TPT Pay webhook receiver, standard library HTTP server plus tpt_payments.

The signature is checked over the raw request bytes before anything is parsed. A failed check
returns 400 with no reason in the body. A handler error returns 500, so the sender retries.
"""

from __future__ import annotations

import json
import os
import sys
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from typing import Callable, Dict, Tuple

from tpt_payments import SignatureVerificationError, construct_event

MAX_BODY_BYTES = 1024 * 1024  # matches the core service's request limit


def process_delivery(
    raw_body: bytes,
    signature_header: str,
    secret: str,
    on_event: Callable[[Dict], None],
) -> Tuple[int, Dict]:
    """Verify one delivery and hand the event to on_event. Returns (status, body)."""
    try:
        event = construct_event(raw_body, signature_header or "", secret)
    except SignatureVerificationError:
        return 400, {"error": "signature verification failed"}
    except ValueError:
        return 400, {"error": "body is not a JSON object"}

    try:
        on_event(event)
    except Exception as exc:  # the sender retries on any non-2xx
        print(f"handler failed for {event.get('id')}: {exc!r}", file=sys.stderr)
        return 500, {"error": "handler failed"}
    return 200, {"received": True}


def make_handler(secret: str, on_event: Callable[[Dict], None]):
    class WebhookHandler(BaseHTTPRequestHandler):
        def do_POST(self) -> None:  # noqa: N802 (http.server naming)
            if self.path != "/webhook":
                self._send(404, {"error": "not found"})
                return
            length = int(self.headers.get("Content-Length") or 0)
            if length > MAX_BODY_BYTES:
                self._send(413, {"error": "body too large"})
                return
            raw = self.rfile.read(length)
            status, body = process_delivery(raw, self.headers.get("TPT-Signature", ""), secret, on_event)
            self._send(status, body)

        def _send(self, status: int, body: Dict) -> None:
            data = json.dumps(body).encode("utf-8")
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(data)))
            self.end_headers()
            self.wfile.write(data)

        def log_message(self, fmt: str, *args) -> None:  # keep test output quiet
            return

    return WebhookHandler


def main() -> None:
    secret = os.environ.get("WEBHOOK_SECRET")
    if not secret:
        sys.exit("WEBHOOK_SECRET is required")
    port = int(os.environ.get("PORT", "4242"))
    handler = make_handler(secret, lambda event: print("received", event.get("type"), event.get("id")))
    print(f"listening on http://localhost:{port}/webhook")
    ThreadingHTTPServer(("", port), handler).serve_forever()


if __name__ == "__main__":
    main()
