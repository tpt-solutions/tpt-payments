const test = require('node:test');
const assert = require('node:assert/strict');
const http = require('node:http');
const { createHmac } = require('node:crypto');
const { createWebhookHandler } = require('../server');

const SECRET = 'whsec_test_secret';

function sign(body, t = Math.floor(Date.now() / 1000), secret = SECRET) {
  const v1 = createHmac('sha256', secret).update(`${t}.`).update(body).digest('hex');
  return `t=${t},v1=${v1}`;
}

async function withServer(options, fn) {
  const handler = createWebhookHandler(options);
  const server = http.createServer((req, res) => handler(req, res));
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
  const { port } = server.address();
  try {
    await fn(`http://127.0.0.1:${port}/webhook`);
  } finally {
    await new Promise((resolve) => server.close(resolve));
  }
}

async function post(url, body, headers = {}) {
  const res = await fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', ...headers },
    body,
  });
  return res.status;
}

test('a correctly signed delivery reaches onEvent and gets 200', async () => {
  const seen = [];
  const body = JSON.stringify({ id: 'evt_1', type: 'payment_intent.succeeded', data: { object: { id: 'pi_1' } } });
  await withServer({ secret: SECRET, onEvent: (e) => seen.push(e) }, async (url) => {
    const status = await post(url, body, { 'TPT-Signature': sign(body) });
    assert.equal(status, 200);
  });
  assert.equal(seen.length, 1);
  assert.equal(seen[0].type, 'payment_intent.succeeded');
});

test('a tampered body is refused and onEvent is not called', async () => {
  const seen = [];
  const body = JSON.stringify({ id: 'evt_2', type: 'refund.created' });
  const header = sign(body);
  await withServer({ secret: SECRET, onEvent: (e) => seen.push(e) }, async (url) => {
    const status = await post(url, body.replace('refund', 'charge'), { 'TPT-Signature': header });
    assert.equal(status, 400);
  });
  assert.equal(seen.length, 0);
});

test('a signature made with another secret is refused', async () => {
  const body = '{"id":"evt_3"}';
  await withServer({ secret: SECRET, onEvent: () => {} }, async (url) => {
    const status = await post(url, body, { 'TPT-Signature': sign(body, undefined, 'whsec_other') });
    assert.equal(status, 400);
  });
});

test('a missing signature header is refused', async () => {
  await withServer({ secret: SECRET, onEvent: () => {} }, async (url) => {
    assert.equal(await post(url, '{"id":"evt_4"}'), 400);
  });
});

test('a stale timestamp is refused', async () => {
  const body = '{"id":"evt_5"}';
  const old = Math.floor(Date.now() / 1000) - 3600;
  await withServer({ secret: SECRET, onEvent: () => {} }, async (url) => {
    assert.equal(await post(url, body, { 'TPT-Signature': sign(body, old) }), 400);
  });
});

test('a handler failure returns 500 so the sender retries', async () => {
  const body = '{"id":"evt_6","type":"x"}';
  await withServer(
    {
      secret: SECRET,
      onEvent: () => {
        throw new Error('database down');
      },
    },
    async (url) => {
      assert.equal(await post(url, body, { 'TPT-Signature': sign(body) }), 500);
    },
  );
});

test('GET is not allowed', async () => {
  await withServer({ secret: SECRET, onEvent: () => {} }, async (url) => {
    const res = await fetch(url);
    assert.equal(res.status, 405);
  });
});
