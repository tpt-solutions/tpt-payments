import pytest
from aiohttp import web
from aiohttp.test_utils import TestServer

from tpt_payments import (
    APIError,
    AuthenticationError,
    CardError,
    InvalidRequestError,
    TPTPay,
)
from tpt_payments._version import __version__


def test_version_matches_package_metadata():
    tomllib = pytest.importorskip("tomllib")  # stdlib from Python 3.11
    from pathlib import Path

    meta = tomllib.loads((Path(__file__).parent.parent / "pyproject.toml").read_text())
    assert meta["project"]["version"] == __version__


def test_publishable_key_is_rejected():
    with pytest.raises(ValueError, match="secret key"):
        TPTPay("pk_test_abc")


async def make_server(handler_map):
    app = web.Application()
    seen = []

    async def record(request):
        seen.append({"method": request.method, "path": request.path, "headers": dict(request.headers)})
        return await handler_map[(request.method, request.path)](request, len(seen))

    for (method, path) in handler_map:
        app.router.add_route(method, path, record)
    server = TestServer(app)
    await server.start_server()
    return server, seen


async def test_write_requests_send_an_idempotency_key_and_reads_do_not():
    async def create(request, n):
        return web.json_response({"id": "pi_1", "status": "requires_payment_method"})

    async def get(request, n):
        return web.json_response({"id": "pi_1"})

    server, seen = await make_server({
        ("POST", "/v1/payment_intents"): create,
        ("GET", "/v1/payment_intents/pi_1"): get,
    })
    try:
        client = TPTPay("sk_test_abc", api_base=str(server.make_url("/v1")))
        await client.payment_intents.create(amount=1000, currency="usd")
        await client.payment_intents.retrieve("pi_1")
        await client.close()
    finally:
        await server.close()

    post, read = seen
    assert post["headers"].get("Idempotency-Key")
    assert "Idempotency-Key" not in read["headers"]
    assert post["headers"]["Authorization"] == "Bearer sk_test_abc"


async def test_card_decline_maps_to_card_error_with_code():
    async def decline(request, n):
        return web.json_response(
            {"error": {"type": "card_error", "code": "card_declined", "message": "Your card was declined."}},
            status=402,
        )

    server, _ = await make_server({("POST", "/v1/payment_intents/pi_1/confirm"): decline})
    try:
        client = TPTPay("sk_test_abc", api_base=str(server.make_url("/v1")))
        with pytest.raises(CardError) as info:
            await client.payment_intents.confirm("pi_1", payment_method="pm_x")
        assert info.value.code == "card_declined"
        assert info.value.status == 402
        await client.close()
    finally:
        await server.close()


async def test_unauthorized_maps_to_authentication_error():
    async def unauthorized(request, n):
        return web.json_response({"error": {"message": "Invalid API key."}}, status=401)

    server, _ = await make_server({("GET", "/v1/payment_intents/pi_1"): unauthorized})
    try:
        client = TPTPay("sk_test_bad", api_base=str(server.make_url("/v1")))
        with pytest.raises(AuthenticationError, match="Invalid API key"):
            await client.payment_intents.retrieve("pi_1")
        await client.close()
    finally:
        await server.close()


async def test_client_error_is_not_retried():
    attempts = 0

    async def bad(request, n):
        nonlocal attempts
        attempts += 1
        return web.json_response({"error": {"message": "bad amount"}}, status=400)

    server, _ = await make_server({("POST", "/v1/payment_intents"): bad})
    try:
        client = TPTPay("sk_test_abc", api_base=str(server.make_url("/v1")), max_retries=3)
        with pytest.raises(InvalidRequestError):
            await client.payment_intents.create(amount=1, currency="usd")
        await client.close()
    finally:
        await server.close()
    assert attempts == 1


async def test_transient_failure_is_retried_with_the_same_idempotency_key():
    keys = []

    async def flaky(request, n):
        keys.append(request.headers.get("Idempotency-Key"))
        if n < 3:
            return web.json_response({"error": {"message": "try again"}}, status=503)
        return web.json_response({"id": "pi_ok"})

    server, _ = await make_server({("POST", "/v1/payment_intents"): flaky})
    try:
        client = TPTPay("sk_test_abc", api_base=str(server.make_url("/v1")), max_retries=2)
        result = await client.payment_intents.create(amount=500, currency="usd")
        await client.close()
    finally:
        await server.close()

    assert result["id"] == "pi_ok"
    assert len(keys) == 3
    assert len(set(keys)) == 1, "every retry must reuse the idempotency key"


async def test_retry_after_is_honored_on_a_429(monkeypatch):
    waits = []

    async def fake_sleep(seconds):
        waits.append(seconds)

    monkeypatch.setattr("tpt_payments.client._sleep", fake_sleep)

    async def limited(request, n):
        if n < 2:
            return web.json_response({"error": {"message": "slow down"}}, status=429, headers={"Retry-After": "7"})
        return web.json_response({"id": "pi_ok"})

    server, _ = await make_server({("POST", "/v1/payment_intents"): limited})
    try:
        client = TPTPay("sk_test_abc", api_base=str(server.make_url("/v1")), max_retries=2)
        result = await client.payment_intents.create(amount=500, currency="usd")
        await client.close()
    finally:
        await server.close()

    assert result["id"] == "pi_ok"
    assert waits == [7.0]


def test_parse_retry_after_forms():
    from datetime import datetime, timedelta, timezone
    from email.utils import format_datetime

    from tpt_payments.client import MAX_RETRY_AFTER, parse_retry_after

    now = datetime(2030, 1, 1, tzinfo=timezone.utc)
    assert parse_retry_after("5", now) == 5.0
    assert parse_retry_after(" 0 ", now) == 0.0
    assert parse_retry_after(format_datetime(now + timedelta(seconds=10)), now) == pytest.approx(10.0)
    assert parse_retry_after(format_datetime(now - timedelta(seconds=10)), now) == 0.0
    assert parse_retry_after("soon", now) is None
    assert parse_retry_after(None, now) is None
    assert MAX_RETRY_AFTER == 60.0


async def test_persistent_server_error_raises_api_error_after_retries():
    async def broken(request, n):
        return web.json_response({"error": {"message": "boom"}}, status=500)

    server, _ = await make_server({("GET", "/v1/payment_intents/pi_1"): broken})
    try:
        client = TPTPay("sk_test_abc", api_base=str(server.make_url("/v1")), max_retries=1)
        with pytest.raises(APIError):
            await client.payment_intents.retrieve("pi_1")
        await client.close()
    finally:
        await server.close()


async def test_subscription_create_list_and_cancel_at_period_end():
    seen = {}

    async def create(request, n):
        seen["create"] = await request.json()
        return web.json_response({"id": "sub_1", "object": "subscription", "status": "active"})

    async def listing(request, n):
        seen["list"] = dict(request.query)
        return web.json_response({"object": "list", "data": [], "has_more": False})

    async def cancel(request, n):
        seen["cancel"] = await request.json()
        return web.json_response({"id": "sub_1", "object": "subscription", "status": "active", "cancel_at_period_end": True})

    server, _ = await make_server({
        ("POST", "/v1/subscriptions"): create,
        ("GET", "/v1/subscriptions"): listing,
        ("POST", "/v1/subscriptions/sub_1/cancel"): cancel,
    })
    try:
        client = TPTPay("sk_test_abc", api_base=str(server.make_url("/v1")))
        created = await client.subscriptions.create(amount=1500, currency="usd", interval="month", payment_method="pm_card_visa")
        page = await client.subscriptions.list(limit=5, starting_after="sub_0")
        canceled = await client.subscriptions.cancel("sub_1", at_period_end=True)
        await client.close()
    finally:
        await server.close()

    assert created["status"] == "active"
    assert seen["create"]["interval"] == "month"
    assert page["has_more"] is False
    assert seen["list"] == {"limit": "5", "starting_after": "sub_0"}
    assert canceled["cancel_at_period_end"] is True
    assert seen["cancel"] == {"at_period_end": True}
