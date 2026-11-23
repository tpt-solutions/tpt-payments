import { TPTPay, TPTPayError } from '../src/index';

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
