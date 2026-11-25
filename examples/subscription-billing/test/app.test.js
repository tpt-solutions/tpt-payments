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
  const sub = (id, extra = {}) => ({
    id,
    status: 'active',
    current_period_end: 1800000000,
    cancel_at_period_end: false,
    ...extra,
  });
  return {
    calls,
    subscriptions: {
      create: record('create', (params) => sub('sub_1', { amount: params.amount })),
      retrieve: record('retrieve', (id) => sub(id)),
      cancel: record('cancel', (id, atPeriodEnd) => sub(id, { cancel_at_period_end: atPeriodEnd, status: atPeriodEnd ? 'active' : 'canceled' })),
    },
  };
}

function sign(body, t = Math.floor(Date.now() / 1000)) {
  const v1 = createHmac('sha256', SECRET).update(`${t}.`).update(body).digest('hex');
  return `t=${t},v1=${v1}`;
}

async function withApp(tpt, fn, options = {}) {
  const app = createApp({ tpt, webhookSecret: SECRET, ...options });
  const server = app.listen(0, '127.0.0.1');
  await new Promise((resolve) => server.once('listening', resolve));
  const base = `http://127.0.0.1:${server.address().port}`;
  try {
    await fn(base);
  } finally {
    await new Promise((resolve) => server.close(resolve));
  }
}

function post(base, path, body) {
  return fetch(base + path, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify(body),
  });
}

test('the browser chooses a plan, and the server sets the price', async () => {
  const tpt = fakeTpt();
  await withApp(tpt, async (base) => {
    const res = await post(base, '/api/subscriptions', { plan: 'pro', payment_method: 'pm_card_visa', amount: 1, customer_ref: 'c_1' });
    assert.equal(res.status, 200);
    const body = await res.json();
    assert.equal(body.id, 'sub_1');
    assert.equal(body.status, 'active');
    const [, params] = tpt.calls[0];
    assert.equal(params.amount, 1500, 'the amount comes from the plan table, not the request');
    assert.equal(params.interval, 'month');
    assert.deepEqual(params.metadata, { customer_ref: 'c_1' });
  });
});

test('an unknown plan or a missing payment method is refused before any call', async () => {
  const tpt = fakeTpt();
  await withApp(tpt, async (base) => {
    assert.equal((await post(base, '/api/subscriptions', { plan: 'free', payment_method: 'pm_card_visa' })).status, 400);
    assert.equal((await post(base, '/api/subscriptions', { plan: 'pro' })).status, 400);
    assert.equal(tpt.calls.length, 0);
  });
});

test('a declined subscription is returned with its API status and code', async () => {
  const tpt = fakeTpt({ create: new TPTPayError({ message: 'Your card was declined.', status: 402, type: 'card_error', code: 'card_declined' }) });
  await withApp(tpt, async (base) => {
    const res = await post(base, '/api/subscriptions', { plan: 'pro', payment_method: 'pm_card_declined' });
    assert.equal(res.status, 402);
    assert.equal((await res.json()).error.code, 'card_declined');
  });
});

test('cancel passes the period-end choice through', async () => {
  const tpt = fakeTpt();
  await withApp(tpt, async (base) => {
    const res = await post(base, '/api/subscriptions/sub_9/cancel', { at_period_end: true });
    assert.equal(res.status, 200);
    assert.equal((await res.json()).cancel_at_period_end, true);
    assert.deepEqual(tpt.calls[0], ['cancel', 'sub_9', true]);
  });
});

test('a verified subscription event reaches the handler; an unverified one does not', async () => {
  const seen = [];
  await withApp(fakeTpt(), async (base) => {
    const body = JSON.stringify({ id: 'evt_1', type: 'subscription.past_due', data: { object: { id: 'sub_1' } } });
    const good = await fetch(base + '/webhook', {
      method: 'POST',
      headers: { 'content-type': 'application/json', 'TPT-Signature': sign(body) },
      body,
    });
    assert.equal(good.status, 200);

    const forged = await fetch(base + '/webhook', {
      method: 'POST',
      headers: { 'content-type': 'application/json', 'TPT-Signature': sign(body).replace(/v1=./, 'v1=0') },
      body,
    });
    assert.equal(forged.status, 400);
  }, { onEvent: (event) => seen.push(event.type) });
  assert.deepEqual(seen, ['subscription.past_due'], 'only the verified event is handled');
});

test('a failing handler asks for a redelivery', async () => {
  await withApp(fakeTpt(), async (base) => {
    const body = JSON.stringify({ id: 'evt_2', type: 'subscription.canceled', data: { object: { id: 'sub_1' } } });
    const res = await fetch(base + '/webhook', {
      method: 'POST',
      headers: { 'content-type': 'application/json', 'TPT-Signature': sign(body) },
      body,
    });
    assert.equal(res.status, 500, 'a non-2xx response makes TPT Pay send the event again');
  }, { onEvent: () => { throw new Error('database down'); } });
});
