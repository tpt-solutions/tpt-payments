import { TPTPay, TPTPayError, parseRetryAfter } from '../src/index';

type FetchMock = jest.Mock<Promise<Response>, [string, RequestInit?]>;

function json(status: number, body: unknown, headers: Record<string, string> = {}): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json', ...headers },
  });
}

function mockFetch(...responses: Response[]): FetchMock {
  const fn: FetchMock = jest.fn();
  for (const r of responses) fn.mockResolvedValueOnce(r);
  global.fetch = fn as unknown as typeof fetch;
  return fn;
}

const intent = {
  id: 'pi_1',
  object: 'payment_intent',
  amount: 1000,
  currency: 'USD',
  status: 'requires_payment_method',
  capture_method: 'automatic',
  confirmation_method: 'automatic',
  created: 1,
  livemode: false,
};

describe('construction', () => {
  it('rejects keys that are not TPT Pay keys', () => {
    expect(() => new TPTPay({ apiKey: 'abc' })).toThrow(TypeError);
  });

  it('accepts secret and publishable keys', () => {
    expect(() => new TPTPay({ apiKey: 'sk_test_abc' })).not.toThrow();
    expect(() => new TPTPay({ apiKey: 'pk_test_abc' })).not.toThrow();
  });
});

describe('key permissions', () => {
  it('rejects server-only calls with a publishable key before sending anything', async () => {
    const fetchMock = mockFetch();
    const client = new TPTPay({ apiKey: 'pk_test_abc' });

    await expect(client.paymentIntents.create({ amount: 100, currency: 'usd' })).rejects.toMatchObject({
      status: 403,
      code: 'permission_denied',
    });
    await expect(client.refunds.create({ payment_intent: 'pi_1' })).rejects.toBeInstanceOf(TPTPayError);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('lets a publishable key retrieve and confirm an intent', async () => {
    const fetchMock = mockFetch(json(200, intent), json(200, { ...intent, status: 'succeeded' }));
    const client = new TPTPay({ apiKey: 'pk_test_abc', retryBaseDelayMs: 0 });

    await client.paymentIntents.retrieve('pi_1');
    const confirmed = await client.paymentIntents.confirm('pi_1', { payment_method: 'pm_card_visa' });

    expect(confirmed.status).toBe('succeeded');
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });
});

describe('requests', () => {
  it('sends an Idempotency-Key on writes and none on reads', async () => {
    const fetchMock = mockFetch(json(200, intent), json(200, intent));
    const client = new TPTPay({ apiKey: 'sk_test_abc', retryBaseDelayMs: 0 });

    await client.paymentIntents.create({ amount: 1000, currency: 'usd' });
    await client.paymentIntents.retrieve('pi_1');

    const [, writeInit] = fetchMock.mock.calls[0];
    const [, readInit] = fetchMock.mock.calls[1];
    const writeHeaders = writeInit?.headers as Record<string, string>;
    const readHeaders = readInit?.headers as Record<string, string>;
    expect(writeHeaders['Idempotency-Key']).toBeTruthy();
    expect(readHeaders['Idempotency-Key']).toBeUndefined();
    expect(writeHeaders.Authorization).toBe('Bearer sk_test_abc');
  });

  it('maps a card decline to a TPTPayError with isCardError', async () => {
    mockFetch(
      json(402, { error: { type: 'card_error', code: 'card_declined', message: 'Your card was declined.' } }),
    );
    const client = new TPTPay({ apiKey: 'sk_test_abc', retryBaseDelayMs: 0 });

    const err = await client.paymentIntents.confirm('pi_1', { payment_method: 'pm_x' }).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(TPTPayError);
    expect((err as TPTPayError).code).toBe('card_declined');
    expect((err as TPTPayError).isCardError).toBe(true);
  });

  it('does not retry client errors', async () => {
    const fetchMock = mockFetch(json(400, { error: { message: 'bad amount', param: 'amount' } }));
    const client = new TPTPay({ apiKey: 'sk_test_abc', maxRetries: 3, retryBaseDelayMs: 0 });

    await expect(client.paymentIntents.create({ amount: 0, currency: 'usd' })).rejects.toMatchObject({
      status: 400,
      param: 'amount',
    });
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it('retries transient failures with the same idempotency key', async () => {
    const fetchMock = mockFetch(json(503, { error: { message: 'busy' } }), json(503, { error: { message: 'busy' } }), json(200, intent));
    const client = new TPTPay({ apiKey: 'sk_test_abc', maxRetries: 2, retryBaseDelayMs: 0 });

    const result = await client.paymentIntents.create({ amount: 1000, currency: 'usd' });

    expect(result.id).toBe('pi_1');
    expect(fetchMock).toHaveBeenCalledTimes(3);
    const keys = fetchMock.mock.calls.map(([, init]) => (init?.headers as Record<string, string>)['Idempotency-Key']);
    expect(new Set(keys).size).toBe(1);
  });

  it('reuses a caller-supplied idempotency key', async () => {
    const fetchMock = mockFetch(json(200, intent));
    const client = new TPTPay({ apiKey: 'sk_test_abc', retryBaseDelayMs: 0 });

    await client.paymentIntents.create({ amount: 1000, currency: 'usd' }, { idempotencyKey: 'order-42' });

    const headers = fetchMock.mock.calls[0][1]?.headers as Record<string, string>;
    expect(headers['Idempotency-Key']).toBe('order-42');
  });

  it('reports connection failures after retries are exhausted', async () => {
    const fetchMock: FetchMock = jest.fn().mockRejectedValue(new Error('ECONNRESET'));
    global.fetch = fetchMock as unknown as typeof fetch;
    const client = new TPTPay({ apiKey: 'sk_test_abc', maxRetries: 1, retryBaseDelayMs: 0 });

    await expect(client.paymentIntents.retrieve('pi_1')).rejects.toMatchObject({ type: 'api_connection_error' });
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });
});

describe('Retry-After', () => {
  it('waits for the server-specified delay before retrying a 429', async () => {
    jest.useFakeTimers();
    try {
      const fetchMock = mockFetch(
        json(429, { error: { message: 'slow down' } }, { 'Retry-After': '3' }),
        json(200, intent),
      );
      const client = new TPTPay({ apiKey: 'sk_test_abc', maxRetries: 1, retryBaseDelayMs: 0 });
      const pending = client.paymentIntents.retrieve('pi_1');
      await jest.advanceTimersByTimeAsync(2_999);
      expect(fetchMock).toHaveBeenCalledTimes(1);
      await jest.advanceTimersByTimeAsync(1);
      await expect(pending).resolves.toMatchObject({ id: 'pi_1' });
      expect(fetchMock).toHaveBeenCalledTimes(2);
    } finally {
      jest.useRealTimers();
    }
  });

  it('caps an excessive Retry-After at 60 seconds', async () => {
    jest.useFakeTimers();
    try {
      const fetchMock = mockFetch(json(503, { error: { message: 'busy' } }, { 'Retry-After': '86400' }), json(200, intent));
      const client = new TPTPay({ apiKey: 'sk_test_abc', maxRetries: 1, retryBaseDelayMs: 0 });
      const pending = client.paymentIntents.retrieve('pi_1');
      await jest.advanceTimersByTimeAsync(60_000);
      await expect(pending).resolves.toMatchObject({ id: 'pi_1' });
      expect(fetchMock).toHaveBeenCalledTimes(2);
    } finally {
      jest.useRealTimers();
    }
  });

  it('parses delay-seconds, HTTP-dates and rejects junk', () => {
    const now = Date.UTC(2030, 0, 1, 0, 0, 0);
    expect(parseRetryAfter('5', now)).toBe(5_000);
    expect(parseRetryAfter(' 0 ', now)).toBe(0);
    expect(parseRetryAfter(new Date(now + 10_000).toUTCString(), now)).toBe(10_000);
    expect(parseRetryAfter(new Date(now - 10_000).toUTCString(), now)).toBe(0);
    expect(parseRetryAfter('soon', now)).toBeUndefined();
    expect(parseRetryAfter(null, now)).toBeUndefined();
  });
});

describe('test clocks', () => {
  const clock = { object: 'test_clock', frozen_time: '2030-01-01T01:00:00Z', outages: [] };

  it('advances the sandbox clock with a test key', async () => {
    const fetchMock = mockFetch(json(200, clock));
    const client = new TPTPay({ apiKey: 'sk_test_abc', retryBaseDelayMs: 0 });
    const result = await client.testClocks.advance({ seconds: 3600 });
    expect(result.frozen_time).toBe('2030-01-01T01:00:00Z');
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toMatch(/\/test_clocks\/advance$/);
    expect(init?.method).toBe('POST');
    expect(JSON.parse(init?.body as string)).toEqual({ seconds: 3600 });
  });

  it('schedules and clears outages', async () => {
    const fetchMock = mockFetch(json(200, clock), json(200, clock));
    const client = new TPTPay({ apiKey: 'sk_test_abc', retryBaseDelayMs: 0 });
    await client.testClocks.scheduleOutage({ from: '2030-01-01T00:30:00Z', to: '2030-01-01T02:00:00Z' });
    await client.testClocks.clearOutages();
    expect(fetchMock.mock.calls[0][0]).toMatch(/\/test_clocks\/outages$/);
    expect(fetchMock.mock.calls[1][1]?.method).toBe('DELETE');
  });

  it('refuses test-clock calls with a publishable key before sending anything', async () => {
    const fetchMock = mockFetch();
    const client = new TPTPay({ apiKey: 'pk_test_abc', retryBaseDelayMs: 0 });
    await expect(client.testClocks.retrieve()).rejects.toMatchObject({ status: 403 });
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
