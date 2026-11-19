import hashlib
import hmac
import json
import time

import pytest

from tpt_payments import SignatureVerificationError, construct_event, verify_signature

SECRET = "whsec_test_secret"


def sign(payload: bytes, secret: str = SECRET, t: int | None = None) -> str:
    ts = str(int(time.time()) if t is None else t)
    mac = hmac.new(secret.encode(), ts.encode() + b"." + payload, hashlib.sha256).hexdigest()
    return f"t={ts},v1={mac}"


def test_valid_delivery_verifies():
    payload = b'{"id":"evt_1","type":"payment_intent.succeeded"}'
    verify_signature(payload, sign(payload), SECRET)


def test_tampered_body_is_rejected():
    header = sign(b'{"amount":1000}')
    with pytest.raises(SignatureVerificationError, match="no signature matches"):
        verify_signature(b'{"amount":9999}', header, SECRET)


def test_wrong_secret_is_rejected():
    payload = b"{}"
    with pytest.raises(SignatureVerificationError):
        verify_signature(payload, sign(payload, secret="whsec_other"), SECRET)


def test_non_utf8_body_verifies_over_raw_bytes():
    # Signing is over raw bytes; a body that is not valid UTF-8 must still verify.
    payload = b"\xff\xfe raw bytes"
    verify_signature(payload, sign(payload), SECRET)


def test_stale_timestamp_is_rejected():
    payload = b"{}"
    old = int(time.time()) - 3600
    with pytest.raises(SignatureVerificationError, match="too old"):
        verify_signature(payload, sign(payload, t=old), SECRET, tolerance=300)


def test_future_timestamp_is_rejected():
    payload = b"{}"
    future = int(time.time()) + 3600
    with pytest.raises(SignatureVerificationError, match="future"):
        verify_signature(payload, sign(payload, t=future), SECRET)


def test_secret_rotation_accepts_any_matching_v1():
    payload = b'{"id":"evt_rot"}'
    ts = str(int(time.time()))
    new = hmac.new(b"whsec_new", ts.encode() + b"." + payload, hashlib.sha256).hexdigest()
    old = hmac.new(b"whsec_old", ts.encode() + b"." + payload, hashlib.sha256).hexdigest()
    header = f"t={ts},v1={new},v1={old}"
    verify_signature(payload, header, "whsec_new")
    verify_signature(payload, header, "whsec_old")


@pytest.mark.parametrize(
    "header",
    ["", "v1=abc", "t=123", "t=1,t=2,v1=abc", "t=notanumber,v1=abc"],
)
def test_malformed_headers_raise_signature_error(header):
    with pytest.raises(SignatureVerificationError):
        verify_signature(b"{}", header, SECRET)


def test_construct_event_returns_parsed_object():
    payload = json.dumps({"id": "evt_1", "type": "payment_intent.succeeded"}).encode()
    event = construct_event(payload, sign(payload), SECRET)
    assert event["id"] == "evt_1"


def test_construct_event_rejects_unverified_body():
    payload = b'{"id":"evt_1"}'
    with pytest.raises(SignatureVerificationError):
        construct_event(payload, sign(b'{"id":"other"}'), SECRET)


def test_signature_error_is_a_value_error():
    # Compatible with callers that catch ValueError.
    with pytest.raises(ValueError):
        verify_signature(b"{}", "", SECRET)
