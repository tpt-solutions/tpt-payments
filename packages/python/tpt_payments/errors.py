"""Exception types raised by the TPT Pay Python client."""

from __future__ import annotations

import json
from typing import Any, Optional


class TPTError(Exception):
    """Base class for every error raised by this SDK.

    Attributes mirror the API error envelope ``{"error": {...}}`` where present.
    """

    def __init__(
        self,
        message: str,
        *,
        status: Optional[int] = None,
        type: Optional[str] = None,
        code: Optional[str] = None,
        param: Optional[str] = None,
        request_id: Optional[str] = None,
    ) -> None:
        super().__init__(message)
        self.message = message
        self.status = status
        self.type = type
        self.code = code
        self.param = param
        self.request_id = request_id

    def __str__(self) -> str:
        parts = [self.message]
        if self.code:
            parts.append(f"(code={self.code})")
        if self.request_id:
            parts.append(f"(request_id={self.request_id})")
        return " ".join(parts)


class APIConnectionError(TPTError):
    """The request never produced an HTTP response (network failure or timeout)."""


class AuthenticationError(TPTError):
    """The API key is missing, invalid or revoked (HTTP 401)."""


class PermissionDeniedError(TPTError):
    """The key is valid but may not perform this operation (HTTP 403)."""


class InvalidRequestError(TPTError):
    """The request was rejected as invalid (other HTTP 4xx)."""


class CardError(TPTError):
    """The card network declined the payment (HTTP 402). Safe to show to the customer."""


class APIError(TPTError):
    """The server failed (HTTP 5xx). The outcome may be unknown; see the API docs."""


class SignatureVerificationError(TPTError, ValueError):
    """A webhook signature is missing, malformed, mismatched, or outside the tolerance."""


def from_response(status: int, body: str, request_id: Optional[str]) -> TPTError:
    """Build the most specific exception for an error response."""
    message = body[:500] or f"HTTP {status}"
    error_type: Optional[str] = None
    code: Optional[str] = None
    param: Optional[str] = None
    try:
        envelope: Any = json.loads(body)
    except ValueError:
        envelope = None
    if isinstance(envelope, dict) and isinstance(envelope.get("error"), dict):
        err = envelope["error"]
        message = str(err.get("message") or message)
        error_type = err.get("type")
        code = err.get("code")
        param = err.get("param")

    kwargs = dict(status=status, type=error_type, code=code, param=param, request_id=request_id)
    if status == 401:
        return AuthenticationError(message, **kwargs)
    if status == 402:
        return CardError(message, **kwargs)
    if status == 403:
        return PermissionDeniedError(message, **kwargs)
    if status >= 500:
        return APIError(message, **kwargs)
    return InvalidRequestError(message, **kwargs)
