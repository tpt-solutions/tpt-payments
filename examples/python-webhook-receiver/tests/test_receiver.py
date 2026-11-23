import hashlib
import hmac
import json
import threading
import time
import urllib.error
import urllib.request
from http.server import ThreadingHTTPServer

import pytest

from receiver import make_handler, process_delivery

SECRET = "whsec_test_secret"


def sign(body: bytes, t: int | None = None, secret: str = SECRET) -> str:
    t = int(time.time()) if t is None else t
    mac = hmac.new(secret.encode(), f"{t}.".encode() + body, hashlib.sha256).hexdigest()
    return f"t={t},v1={mac}"


def test_valid_delivery_reaches_handler():
    seen = []
    body = json.dumps({"id": "evt_1", "type": "payment_intent.succeeded"}).encode()
    status, out = process_delivery(body, sign(body), SECRET, seen.append)
    assert status == 200 and out == {"received": True}
    assert seen[0]["type"] == "payment_intent.succeeded"


def test_tampered_body_is_refused_and_not_handled():
    seen = []
    body = b'{"id":"evt_2","type":"charge.refunded"}'
    status, _ = process_delivery(body.replace(b"refunded", b"succeeded"), sign(body), SECRET, seen.append)
    assert status == 400
    assert seen == []


def test_wrong_secret_is_refused():
    body = b'{"id":"evt_3"}'
    status, _ = process_delivery(body, sign(body, secret="whsec_other"), SECRET, lambda e: None)
    assert status == 400


def test_missing_header_is_refused():
    status, _ = process_delivery(b'{"id":"evt_4"}', "", SECRET, lambda e: None)
    assert status == 400


def test_stale_timestamp_is_refused():
    body = b'{"id":"evt_5"}'
    status, _ = process_delivery(body, sign(body, t=int(time.time()) - 3600), SECRET, lambda e: None)
    assert status == 400


def test_handler_failure_returns_500_for_retry():
    def boom(event):
        raise RuntimeError("database down")

    body = b'{"id":"evt_6","type":"x"}'
    status, _ = process_delivery(body, sign(body), SECRET, boom)
    assert status == 500


def test_http_round_trip_and_get_refused():
    seen = []
    server = ThreadingHTTPServer(("127.0.0.1", 0), make_handler(SECRET, seen.append))
    threading.Thread(target=server.serve_forever, daemon=True).start()
    url = f"http://127.0.0.1:{server.server_address[1]}/webhook"
    try:
        body = b'{"id":"evt_7","type":"payment_intent.created"}'
        req = urllib.request.Request(url, data=body, method="POST",
                                     headers={"TPT-Signature": sign(body), "Content-Type": "application/json"})
        with urllib.request.urlopen(req) as resp:
            assert resp.status == 200
        assert seen[0]["id"] == "evt_7"

        with pytest.raises(urllib.error.HTTPError) as err:
            urllib.request.urlopen(urllib.request.Request(url, method="GET"))
        assert err.value.code == 501  # BaseHTTPRequestHandler has no do_GET
    finally:
        server.shutdown()
        server.server_close()
