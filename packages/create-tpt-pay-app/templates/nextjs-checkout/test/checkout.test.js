const test = require('node:test');
const assert = require('node:assert/strict');
const { createHmac } = require('node:crypto');
const { parseCheckout, createCheckout, MAX_AMOUNT } = require('../lib/checkout');
const { handleWebhook } = require('../lib/webhook');

const SECRET = 'whsec_test_secret';
const silent = { error() {} };

function fakeClient(impl) {
  const calls = [];
  return {
    calls,
    paymentIntents: {
      create: async (params) => {
        calls.push(params);
        if (impl) return impl(params);
        return { id: 'pi_1', status: 'requires_payment_method' };
      },
    },
  };
}

function sign(body, t = Math.floor(Date.now() / 1000)) {
  const v1 = createHmac('sha256', SECRET).update(`${t}.`).update(body).digest('hex');
  return `t=${t},v1=${v1}`;
}

test('parseCheckout accepts a whole-number amount and lower-cases the currency', () => {
  assert.deepEqual(parseCheckout({ amount: 2000, currency: 'EUR', order_id: 'o1' }), {
    ok: true,
    value: { amount: 2000, currency: 'eur', orderId: 'o1' },
  });
});

test('parseCheckout refuses bad amounts, currencies and order ids', () => {
  for (const amount of [0, -1, 1.5, '2000', null, MAX_AMOUNT + 1]) {
    assert.equal(parseCheckout({ amount }).ok, false, `amount ${amount}`);
  }
  assert.equal(parseCheckout({ amount: 100, currency: 'dollars' }).ok, false);
  assert.equal(parseCheckout({ amount: 100, order_id: 'x'.repeat(65) }).ok, false);
  assert.equal(parseCheckout(null).ok, false);
  assert.equal(parseCheckout([1]).ok, false);
});

test('createCheckout creates the intent with the order reference and returns its id', async () => {
  const tpt = fakeClient();
  const out = await createCheckout(tpt, { amount: 500, currency: 'usd', order_id: 'o-9' }, silent);
  assert.deepEqual(out, { status: 200, body: { id: 'pi_1', status: 'requires_payment_method' } });
  assert.deepEqual(tpt.calls[0], {
    amount: 500,
    currency: 'usd',
    capture_method: 'automatic',
    metadata: { order_id: 'o-9' },
  });
});

test('createCheckout does not call the API for invalid input', async () => {
  const tpt = fakeClient();
  const out = await createCheckout(tpt, { amount: 0 }, silent);
  assert.equal(out.status, 400);
  assert.equal(tpt.calls.length, 0);
});

test('an upstream failure is a generic 502 that does not expose the cause', async () => {
  const tpt = fakeClient(async () => {
    throw new Error('secret upstream detail');
  });
  const logged = [];
  const out = await createCheckout(tpt, { amount: 500 }, { error: (...a) => logged.push(a.join(' ')) });
  assert.equal(out.status, 502);
  assert.ok(!JSON.stringify(out.body).includes('upstream'));
  assert.ok(logged.some((l) => l.includes('secret upstream detail')), 'the cause is logged on the server');
});

test('a correctly signed webhook reaches onEvent and gets 200', () => {
  const raw = Buffer.from(JSON.stringify({ id: 'evt_1', type: 'payment_intent.succeeded' }));
  const seen = [];
  const out = handleWebhook({ raw, signature: sign(raw.toString()), secret: SECRET, onEvent: (e) => seen.push(e.id) });
  assert.equal(out.status, 200);
  assert.deepEqual(seen, ['evt_1']);
});

test('a tampered body, a wrong secret and a missing header are refused with 400', () => {
  const body = JSON.stringify({ id: 'evt_2', type: 'charge.refunded' });
  const signed = sign(body);
  const tamper = Buffer.from(body.replace('evt_2', 'evt_9'));
  assert.equal(handleWebhook({ raw: tamper, signature: signed, secret: SECRET, onEvent() {} }).status, 400);
  assert.equal(handleWebhook({ raw: Buffer.from(body), signature: sign(body), secret: 'whsec_other', onEvent() {} }).status, 400);
  assert.equal(handleWebhook({ raw: Buffer.from(body), signature: undefined, secret: SECRET, onEvent() {} }).status, 400);
});

test('a handler failure returns 500 so TPT Pay sends the event again', () => {
  const body = JSON.stringify({ id: 'evt_3', type: 'payment_intent.succeeded' });
  const out = handleWebhook({
    raw: Buffer.from(body),
    signature: sign(body),
    secret: SECRET,
    onEvent() {
      throw new Error('database down');
    },
    log: silent,
  });
  assert.equal(out.status, 500);
});
