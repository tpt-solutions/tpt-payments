"""Verify and parse TPT Pay webhook deliveries.

The signature header is ``TPT-Signature: t=<unix>,v1=<hex>[,v1=<hex>...]``. Each ``v1``
is HMAC-SHA256 over ``"<t>.<raw body>"`` with the endpoint's signing secret.
Verification runs over the raw request bytes, exactly as received.
"""

from __future__ import annotations

import hashlib
import hmac
import json
import time
from typing import Any, Dict, List, Optional, Tuple

from .errors import SignatureVerificationError

SIGNATURE_HEADER = "TPT-Signature"
DEFAULT_TOLERANCE = 300  # seconds: maximum age of a signed timestamp
_MAX_CLOCK_SKEW = 60  # seconds: how far in the future a timestamp may be


def _parse_header(header: str) -> Tuple[str, List[str]]:
    timestamp: Optional[str] = None
    signatures: List[str] = []
    for part in header.split(","):
        key, sep, value = part.strip().partition("=")
        if not sep:
            continue
        if key == "t":
            if timestamp is not None:
                raise SignatureVerificationError("duplicate timestamp in signature header")
            timestamp = value
        elif key == "v1":
            signatures.append(value)
    if timestamp is None:
        raise SignatureVerificationError("missing timestamp in signature header")
    if not signatures:
        raise SignatureVerificationError("missing v1 signature in signature header")
    return timestamp, signatures


def verify_signature(
    payload: bytes,
    signature_header: str,
    secret: str,
    tolerance: int = DEFAULT_TOLERANCE,
    now: Optional[float] = None,
) -> None:
    """Verify a webhook delivery. Returns normally when the delivery is valid.

    Args:
        payload: The raw request body as bytes, before any JSON parsing.
        signature_header: The value of the ``TPT-Signature`` header.
        secret: The endpoint's signing secret (``whsec_...``).
        tolerance: Maximum age of the signed timestamp, in seconds.
        now: Current Unix time. Defaults to the system clock (useful in tests).

    Raises:
        SignatureVerificationError: if the header is malformed, no signature matches,
            or the timestamp is too old or too far in the future.
    """
    if not isinstance(payload, (bytes, bytearray)):
        raise TypeError("payload must be the raw request body as bytes")

    timestamp, signatures = _parse_header(signature_header)
    try:
        issued = int(timestamp)
    except ValueError:
        raise SignatureVerificationError("malformed timestamp in signature header") from None

    message = timestamp.encode("ascii") + b"." + bytes(payload)
    expected = hmac.new(secret.encode("utf-8"), message, hashlib.sha256).hexdigest()
    if not any(hmac.compare_digest(sig, expected) for sig in signatures):
        raise SignatureVerificationError("no signature matches the payload and secret")

    current = time.time() if now is None else now
    if issued - current > _MAX_CLOCK_SKEW:
        raise SignatureVerificationError("timestamp is in the future")
    if current - issued > tolerance:
        raise SignatureVerificationError(f"timestamp too old (tolerance {tolerance}s)")


def construct_event(
    payload: bytes,
    signature_header: str,
    secret: str,
    tolerance: int = DEFAULT_TOLERANCE,
) -> Dict[str, Any]:
    """Verify a delivery and return the parsed event object.

    Raises:
        SignatureVerificationError: if verification fails (see verify_signature).
        ValueError: if the verified body is not a JSON object.
    """
    verify_signature(payload, signature_header, secret, tolerance=tolerance)
    try:
        event = json.loads(bytes(payload).decode("utf-8"))
    except (UnicodeDecodeError, ValueError) as exc:
        raise ValueError(f"verified webhook body is not valid JSON: {exc}") from exc
    if not isinstance(event, dict):
        raise ValueError("verified webhook body is not a JSON object")
    return event
