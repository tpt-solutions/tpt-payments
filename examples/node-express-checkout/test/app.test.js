const test = require('node:test');
const assert = require('node:assert/strict');
const { createHmac } = require('node:crypto');
const { TPTPayError } = require('@tpt/pay');
const { createApp } = require('../server');

const SECRET = 'whsec_test_secret';

// A stand-in for TPTPay that records calls. No network is used.
function fakeTpt(behaviour = {}) {
  const calls = [];
  const record = (name, impl) => async (...args) => {
    calls.push([name, ...args]);
    if (behaviour[name]) throw behaviour[name];
    return impl(...args);
  };
  return {
    calls,
    paymentIntents: {
      create: record('create', () => ({ id: 'pi_1', status: 'requires_payment_method' })),
      confirm: record('confirm', (id) => ({ id, status: 'requires_capture' })),
      retrieve: record('retrieve', (id) => ({ id, status: 'succeeded' })),
    },
  };
}

function sign(body, t = Math.floor(Date.now() / 1000)) {
  const v1 = createHmac('sha256', SECRET).update(`${t}.`).update(body).digest('hex');
  return `t=${t},v1=${v1}`;
}

async function withApp(tpt, fn) {
  const app = createApp({ tpt, webhookSecret: SECRET });
  const server = app.listen(0, '127.0.0.1');
  await new Promise((resolve) => server.once('listening', resolve));
  const base = `http://127.0.0.1:${server.address().port}`;
  try {
    await fn(base);
  } finally {
    await new Promise((resolve) => server.close(resolve));
  }
}

function json(method, url, body, headers = {}) {
  return fetch(url, {
    method,
    headers: { 'Content-Type': 'application/json', ...headers },
    body: body === undefined ? undefined : typeof body === 'string' ? body : JSON.stringify(body),
  });
}

test('POST /api/checkout creates an intent with the lower-cased currency and order reference', async () => {
  const tpt = fakeTpt();
  await withApp(tpt, async (base) => {
    const res = await json('POST', `${base}/api/checkout`, { amount: 2000, currency: 'EUR', order_id: 'o_9' });
    assert.equal(res.status, 200);
    assert.deepEqual(await res.json(), { id: 'pi_1', status: 'requires_payment_method' });
  });
  assert.equal(tpt.calls[0][0], 'create');
  assert.deepEqual(tpt.calls[0][1], {
    amount: 2000,
    currency: 'eur',
    capture_method: 'automatic',
    metadata: { order_id: 'o_9' },
  });
});

test('POST /api/checkout refuses a non-integer or non-positive amount without calling the API', async () => {
  const tpt = fakeTpt();
  await withApp(tpt, async (base) => {
    for (const body of [{ amount: 0 }, { amount: -5 }, { amount: 10.5 }, { amount: '100' }, {}]) {
      const res = await json('POST', `${base}/api/checkout`, body);
      assert.equal(res.status, 400, JSON.stringify(body));
    }
  });
  assert.equal(tpt.calls.length, 0);
});

test('POST /api/checkout/:id/confirm passes the payment method through', async () => {
  const tpt = fakeTpt();
  await withApp(tpt, async (base) => {
    const res = await json('POST', `${base}/api/checkout/pi_1/confirm`, { payment_method: 'pm_card_visa' });
    assert.equal(res.status, 200);
    assert.deepEqual(await res.json(), { id: 'pi_1', status: 'requires_capture' });
  });
  assert.deepEqual(tpt.calls[0], ['confirm', 'pi_1', { payment_method: 'pm_card_visa' }]);
});

test('confirm without a payment method is a 400 and does not call the API', async () => {
  const tpt = fakeTpt();
  await withApp(tpt, async (base) => {
    const res = await json('POST', `${base}/api/checkout/pi_1/confirm`, {});
    assert.equal(res.status, 400);
  });
  assert.equal(tpt.calls.length, 0);
});

test('GET /api/checkout/:id retrieves the intent', async () => {
  const tpt = fakeTpt();
  await withApp(tpt, async (base) => {
    const res = await fetch(`${base}/api/checkout/pi_7`);
    assert.equal(res.status, 200);
    assert.deepEqual(await res.json(), { id: 'pi_7', status: 'succeeded' });
  });
});

test('a card decline keeps its status and code for the browser', async () => {
  const decline = new TPTPayError({ message: 'Your card was declined.', status: 402, code: 'card_declined' });
  const tpt = fakeTpt({ confirm: decline });
  await withApp(tpt, async (base) => {
    const res = await json('POST', `${base}/api/checkout/pi_1/confirm`, { payment_method: 'pm_card_declined' });
    assert.equal(res.status, 402);
    assert.deepEqual(await res.json(), { error: { code: 'card_declined', message: 'Your card was declined.' } });
  });
});

test('an upstream failure returns a generic 502 with no detail', async () => {
  const tpt = fakeTpt({ create: new TPTPayError({ message: 'internal stack trace here', status: 500 }) });
  await withApp(tpt, async (base) => {
    const res = await json('POST', `${base}/api/checkout`, { amount: 100 });
    assert.equal(res.status, 502);
    const body = await res.text();
    assert.ok(!body.includes('stack trace'), 'upstream detail must not reach the client');
  });
});

test('a correctly signed webhook is accepted', async () => {
  const tpt = fakeTpt();
  const body = JSON.stringify({ id: 'evt_1', type: 'payment_intent.succeeded', data: { object: { id: 'pi_1' } } });
  await withApp(tpt, async (base) => {
    const res = await json('POST', `${base}/webhook`, body, { 'TPT-Signature': sign(body) });
    assert.equal(res.status, 200);
  });
});

test('a tampered webhook body, a wrong signature and a missing header are all refused', async () => {
  const body = JSON.stringify({ id: 'evt_2', type: 'charge.refunded', data: {} });
  const signed = sign(body);
  await withApp(fakeTpt(), async (base) => {
    const tampered = await json('POST', `${base}/webhook`, body.replace('evt_2', 'evt_9'), { 'TPT-Signature': signed });
    assert.equal(tampered.status, 400);
    const wrongSecret = await json('POST', `${base}/webhook`, body, {
      'TPT-Signature': `t=${Math.floor(Date.now() / 1000)},v1=${'0'.repeat(64)}`,
    });
    assert.equal(wrongSecret.status, 400);
    const missing = await json('POST', `${base}/webhook`, body);
    assert.equal(missing.status, 400);
    assert.deepEqual(await missing.json(), { error: 'signature check failed' });
  });
});

test('unknown routes return 404', async () => {
  await withApp(fakeTpt(), async (base) => {
    const res = await fetch(`${base}/nope`);
    assert.equal(res.status, 404);
  });
});
