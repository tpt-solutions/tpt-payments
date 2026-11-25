"""TPT Pay Python client (async, built on aiohttp).

Every write request carries an ``Idempotency-Key``. If you do not pass one, a fresh key is
generated for that call, and reused across the SDK's own retries of the same call, so a
retried payment cannot be created twice. Pass your own key to make retries survive a
process restart.
"""

from __future__ import annotations

import asyncio
import json
import random
import uuid
from email.utils import parsedate_to_datetime
from datetime import datetime, timezone
from typing import Any, Dict, Optional

import aiohttp

from ._version import __version__
from .errors import APIConnectionError, TPTError, from_response

DEFAULT_API_BASE = "https://api.tpt-payments.com/v1"
DEFAULT_TIMEOUT = 30.0
DEFAULT_MAX_RETRIES = 2
_RETRYABLE_STATUS = frozenset({409, 429, 500, 502, 503, 504})
MAX_RETRY_AFTER = 60.0


class TPTPay:
    """Async client for the TPT Pay API.

    Args:
        api_key: A secret key (``sk_test_...`` or ``sk_live_...``). Publishable keys cannot
            perform server-side operations and are rejected here.
        api_base: Override the API base URL (for self-hosted deployments or local testing).
        timeout: Total request timeout in seconds.
        max_retries: Retries for connection failures and retryable responses (409, 429, 5xx).

    Use as an async context manager, or call :meth:`close` when finished.
    """

    def __init__(
        self,
        api_key: str,
        *,
        api_base: Optional[str] = None,
        timeout: float = DEFAULT_TIMEOUT,
        max_retries: int = DEFAULT_MAX_RETRIES,
    ) -> None:
        if not api_key or not api_key.startswith("sk_"):
            raise ValueError(
                "api_key must be a secret key (sk_test_... or sk_live_...). "
                "Publishable keys (pk_...) cannot perform server-side operations."
            )
        if max_retries < 0:
            raise ValueError("max_retries must be >= 0")
        self.api_key = api_key
        self.api_base = (api_base or DEFAULT_API_BASE).rstrip("/")
        self.timeout = aiohttp.ClientTimeout(total=timeout)
        self.max_retries = max_retries
        self._session: Optional[aiohttp.ClientSession] = None

        self.payment_intents = PaymentIntents(self)
        self.refunds = Refunds(self)
        self.subscriptions = Subscriptions(self)
        self.invoices = Invoices(self)

    async def _get_session(self) -> aiohttp.ClientSession:
        if self._session is None or self._session.closed:
            self._session = aiohttp.ClientSession(timeout=self.timeout)
        return self._session

    async def _request(
        self,
        method: str,
        path: str,
        *,
        params: Optional[Dict[str, Any]] = None,
        data: Optional[Dict[str, Any]] = None,
        idempotency_key: Optional[str] = None,
    ) -> Dict[str, Any]:
        """Send a request and return the decoded JSON body.

        Raises a subclass of :class:`TPTError` for every failure.
        """
        is_write = method != "GET"
        if is_write and idempotency_key is None:
            idempotency_key = str(uuid.uuid4())

        headers = {
            "Authorization": f"Bearer {self.api_key}",
            "Accept": "application/json",
            "User-Agent": f"tpt-payments-python/{__version__}",
        }
        if data is not None:
            headers["Content-Type"] = "application/json"
        if idempotency_key is not None:
            headers["Idempotency-Key"] = idempotency_key

        attempts = self.max_retries + 1
        url = f"{self.api_base}{path}"
        session = await self._get_session()

        for attempt in range(attempts):
            last_attempt = attempt == attempts - 1
            try:
                async with session.request(
                    method, url, headers=headers, params=params, json=data
                ) as response:
                    body = await response.text()
                    request_id = response.headers.get("X-Request-ID")
                    retry_after = response.headers.get("Retry-After")
                    status = response.status
            except (aiohttp.ClientError, asyncio.TimeoutError) as exc:
                if last_attempt:
                    raise APIConnectionError(f"could not reach the TPT Pay API: {exc}") from exc
                await _backoff(attempt)
                continue

            if status < 400:
                return json.loads(body) if body else {}

            error = from_response(status, body, request_id)
            if status in _RETRYABLE_STATUS and not last_attempt:
                await _backoff(attempt, parse_retry_after(retry_after))
                continue
            raise error

        raise TPTError("unreachable: retry loop exited without a result")  # pragma: no cover

    async def close(self) -> None:
        """Close the underlying HTTP session."""
        if self._session is not None and not self._session.closed:
            await self._session.close()
        self._session = None

    async def __aenter__(self) -> "TPTPay":
        return self

    async def __aexit__(self, exc_type, exc_val, exc_tb) -> None:
        await self.close()


def parse_retry_after(value: Optional[str], now: Optional[datetime] = None) -> Optional[float]:
    """Parse a Retry-After header (delay-seconds or an HTTP-date) into seconds.

    Returns None when the header is absent or unusable.
    """
    if value is None:
        return None
    text = value.strip()
    if text.isdigit():
        return float(text)
    try:
        when = parsedate_to_datetime(text)
    except (TypeError, ValueError):
        return None
    if when.tzinfo is None:
        when = when.replace(tzinfo=timezone.utc)
    current = now or datetime.now(timezone.utc)
    return max(0.0, (when - current).total_seconds())


async def _backoff(attempt: int, retry_after: Optional[float] = None) -> None:
    """Wait before a retry.

    A server's Retry-After (capped at MAX_RETRY_AFTER seconds) replaces the computed delay.
    Otherwise: exponential backoff with jitter, about 0.5s, 1s, 2s, capped at 8s.
    """
    if retry_after is not None:
        await _sleep(min(retry_after, MAX_RETRY_AFTER))
        return
    delay = min(0.5 * (2**attempt), 8.0)
    await _sleep(delay * random.uniform(0.5, 1.0))


async def _sleep(seconds: float) -> None:
    """The SDK's own wait, separate from asyncio.sleep so tests can observe retry delays."""
    await asyncio.sleep(seconds)


class _Resource:
    def __init__(self, client: TPTPay) -> None:
        self.client = client


class PaymentIntents(_Resource):
    """Payment intents. Creating, capturing and cancelling need a secret key."""

    async def create(self, *, idempotency_key: Optional[str] = None, **fields: Any) -> Dict[str, Any]:
        """Create a payment intent. Fields: amount (int, minor units), currency, capture_method, metadata."""
        return await self.client._request(
            "POST", "/payment_intents", data=fields, idempotency_key=idempotency_key
        )

    async def retrieve(self, payment_intent_id: str) -> Dict[str, Any]:
        """Retrieve a payment intent. Publishable keys may also call this."""
        return await self.client._request("GET", f"/payment_intents/{payment_intent_id}")

    async def confirm(
        self, payment_intent_id: str, *, idempotency_key: Optional[str] = None, **fields: Any
    ) -> Dict[str, Any]:
        """Attach a payment method and authorize. Field: payment_method."""
        return await self.client._request(
            "POST",
            f"/payment_intents/{payment_intent_id}/confirm",
            data=fields,
            idempotency_key=idempotency_key,
        )

    async def capture(
        self, payment_intent_id: str, *, idempotency_key: Optional[str] = None, **fields: Any
    ) -> Dict[str, Any]:
        """Capture an authorized payment. Field: amount_to_capture (optional, defaults to the full amount)."""
        return await self.client._request(
            "POST",
            f"/payment_intents/{payment_intent_id}/capture",
            data=fields,
            idempotency_key=idempotency_key,
        )

    async def cancel(
        self, payment_intent_id: str, *, idempotency_key: Optional[str] = None, **fields: Any
    ) -> Dict[str, Any]:
        """Cancel a payment intent that has not been captured. Field: cancellation_reason."""
        return await self.client._request(
            "POST",
            f"/payment_intents/{payment_intent_id}/cancel",
            data=fields,
            idempotency_key=idempotency_key,
        )

    async def list(self, **params: Any) -> Dict[str, Any]:
        """List payment intents. Not yet served by the core service (see the compatibility report)."""
        return await self.client._request("GET", "/payment_intents", params=params)


class Refunds(_Resource):
    """Refunds. Not yet served by the core service (see the compatibility report)."""

    async def create(self, *, idempotency_key: Optional[str] = None, **fields: Any) -> Dict[str, Any]:
        return await self.client._request("POST", "/refunds", data=fields, idempotency_key=idempotency_key)

    async def retrieve(self, refund_id: str) -> Dict[str, Any]:
        return await self.client._request("GET", f"/refunds/{refund_id}")

    async def list(self, **params: Any) -> Dict[str, Any]:
        return await self.client._request("GET", "/refunds", params=params)


class Subscriptions(_Resource):
    """Recurring charges. Test mode only (sk_test_...) until a live billing policy is decided.

    create() charges the first period before it returns; a declined first charge gives status
    past_due and the subscription is retried on the schedule. Later periods are charged automatically.
    """

    async def create(self, *, idempotency_key: Optional[str] = None, **fields: Any) -> Dict[str, Any]:
        """Create a subscription. Fields: amount, currency, interval (week|month|year), payment_method, metadata."""
        return await self.client._request(
            "POST", "/subscriptions", data=fields, idempotency_key=idempotency_key
        )

    async def list(self, *, limit: int = 10, starting_after: Optional[str] = None) -> Dict[str, Any]:
        """One page of subscriptions, newest first."""
        params: Dict[str, Any] = {"limit": limit}
        if starting_after is not None:
            params["starting_after"] = starting_after
        return await self.client._request("GET", "/subscriptions", params=params)

    async def retrieve(self, subscription_id: str) -> Dict[str, Any]:
        return await self.client._request("GET", f"/subscriptions/{subscription_id}")

    async def cancel(
        self, subscription_id: str, *, at_period_end: bool = False, idempotency_key: Optional[str] = None
    ) -> Dict[str, Any]:
        return await self.client._request(
            "POST",
            f"/subscriptions/{subscription_id}/cancel",
            data={"at_period_end": at_period_end},
            idempotency_key=idempotency_key,
        )


class Invoices(_Resource):
    """Invoices. Not yet served by the core service (see the compatibility report)."""

    async def retrieve(self, invoice_id: str) -> Dict[str, Any]:
        return await self.client._request("GET", f"/invoices/{invoice_id}")

    async def pay(self, invoice_id: str, *, idempotency_key: Optional[str] = None) -> Dict[str, Any]:
        return await self.client._request(
            "POST", f"/invoices/{invoice_id}/pay", idempotency_key=idempotency_key
        )
