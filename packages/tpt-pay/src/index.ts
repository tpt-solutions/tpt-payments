/**
 * TPT Pay API client for JavaScript and TypeScript.
 *
 * Keys:
 *   - Secret keys (sk_test_..., sk_live_...) may call every endpoint. Use them on servers only.
 *   - Publishable keys (pk_test_..., pk_live_...) may retrieve and confirm payment intents,
 *     which is what a browser checkout needs. Other calls are rejected before any request.
 *
 * Writes carry an Idempotency-Key. If you do not pass one, a key is generated for the call
 * and reused across the client's own retries, so a retried payment is not created twice.
 */

import { errorFromResponse, TPTPayError } from './errors';
import type {
  CancelPaymentIntentParams,
  CapturePaymentIntentParams,
  ConfirmPaymentIntentParams,
  CreatePaymentIntentParams,
  Invoice,
  PaymentIntent,
  Refund,
  RequestOptions,
  Subscription,
} from './types';

export { TPTPayError, SignatureVerificationError } from './errors';
export type * from './types';

export const DEFAULT_API_BASE = 'https://api.tpt-payments.com/v1';

export interface TPTPayOptions {
  /** A secret (sk_...) or publishable (pk_...) key. */
  apiKey: string;
  /** Override the API base URL, for self-hosted deployments or local development. */
  apiBaseUrl?: string;
  /** Per-attempt timeout in milliseconds. Default 30000. */
  timeoutMs?: number;
  /** Retries for connection failures and retryable responses (409, 429, 5xx). Default 2. */
  maxRetries?: number;
  /** Base delay between retries in milliseconds, doubled each attempt with jitter. Default 500. */
  retryBaseDelayMs?: number;
}

type Method = 'GET' | 'POST' | 'DELETE';

const RETRYABLE = new Set([409, 429, 500, 502, 503, 504]);

export class TPTPay {
  private readonly apiKey: string;
  private readonly apiBaseUrl: string;
  private readonly timeoutMs: number;
  private readonly maxRetries: number;
  private readonly retryBaseDelayMs: number;

  constructor(options: TPTPayOptions) {
    if (!options || typeof options.apiKey !== 'string' || !/^(sk|pk)_/.test(options.apiKey)) {
      throw new TypeError('apiKey must be a secret (sk_...) or publishable (pk_...) key');
    }
    const maxRetries = options.maxRetries ?? 2;
    if (!Number.isInteger(maxRetries) || maxRetries < 0) {
      throw new RangeError('maxRetries must be a non-negative integer');
    }
    this.apiKey = options.apiKey;
    this.apiBaseUrl = (options.apiBaseUrl ?? DEFAULT_API_BASE).replace(/\/+$/, '');
    this.timeoutMs = options.timeoutMs ?? 30_000;
    this.maxRetries = maxRetries;
    this.retryBaseDelayMs = options.retryBaseDelayMs ?? 500;
  }

  /** Payment intents. Retrieve and confirm also work with a publishable key. */
  readonly paymentIntents = {
    create: (params: CreatePaymentIntentParams, options?: RequestOptions): Promise<PaymentIntent> =>
      this.request<PaymentIntent>('POST', '/payment_intents', { body: params, secret: true, options }),

    retrieve: (id: string): Promise<PaymentIntent> =>
      this.request<PaymentIntent>('GET', `/payment_intents/${encodeURIComponent(id)}`),

    confirm: (id: string, params: ConfirmPaymentIntentParams, options?: RequestOptions): Promise<PaymentIntent> =>
      this.request<PaymentIntent>('POST', `/payment_intents/${encodeURIComponent(id)}/confirm`, { body: params, options }),

    capture: (id: string, params: CapturePaymentIntentParams = {}, options?: RequestOptions): Promise<PaymentIntent> =>
      this.request<PaymentIntent>('POST', `/payment_intents/${encodeURIComponent(id)}/capture`, {
        body: params,
        secret: true,
        options,
      }),

    cancel: (id: string, params: CancelPaymentIntentParams = {}, options?: RequestOptions): Promise<PaymentIntent> =>
      this.request<PaymentIntent>('POST', `/payment_intents/${encodeURIComponent(id)}/cancel`, {
        body: params,
        secret: true,
        options,
      }),
  };

  /** Planned: not yet served by the core service. Calls fail with 404 until implemented. */
  readonly refunds = {
    create: (params: { payment_intent: string; amount?: number; reason?: string }, options?: RequestOptions): Promise<Refund> =>
      this.request<Refund>('POST', '/refunds', { body: params, secret: true, options }),

    retrieve: (id: string): Promise<Refund> =>
      this.request<Refund>('GET', `/refunds/${encodeURIComponent(id)}`, { secret: true }),
  };

  /** Planned: not yet served by the core service. */
  readonly subscriptions = {
    create: (params: { customer: string; price: string }, options?: RequestOptions): Promise<Subscription> =>
      this.request<Subscription>('POST', '/subscriptions', { body: params, secret: true, options }),

    cancel: (id: string, params: { at_period_end?: boolean } = {}, options?: RequestOptions): Promise<Subscription> =>
      this.request<Subscription>('POST', `/subscriptions/${encodeURIComponent(id)}/cancel`, {
        body: params,
        secret: true,
        options,
      }),
  };

  /** Planned: not yet served by the core service. */
  readonly invoices = {
    retrieve: (id: string): Promise<Invoice> =>
      this.request<Invoice>('GET', `/invoices/${encodeURIComponent(id)}`, { secret: true }),

    pay: (id: string, options?: RequestOptions): Promise<Invoice> =>
      this.request<Invoice>('POST', `/invoices/${encodeURIComponent(id)}/pay`, { secret: true, options }),
  };

  private async request<T>(
    method: Method,
    path: string,
    opts: { body?: unknown; secret?: boolean; options?: RequestOptions } = {},
  ): Promise<T> {
    if (opts.secret && !this.apiKey.startsWith('sk_')) {
      throw new TPTPayError({
        message: `${method} ${path} requires a secret key (sk_...). Publishable keys cannot perform this operation.`,
        status: 403,
        type: 'invalid_request_error',
        code: 'permission_denied',
      });
    }

    const isWrite = method !== 'GET';
    const idempotencyKey = isWrite ? (opts.options?.idempotencyKey ?? randomKey()) : undefined;
    const attempts = this.maxRetries + 1;

    for (let attempt = 0; attempt < attempts; attempt++) {
      const last = attempt === attempts - 1;
      let response: Response;
      try {
        response = await this.send(method, path, opts.body, idempotencyKey);
      } catch (cause) {
        if (last) {
          throw new TPTPayError({
            message: `could not reach the TPT Pay API: ${(cause as Error).message}`,
            type: 'api_connection_error',
          });
        }
        await this.backoff(attempt);
        continue;
      }

      const text = await response.text();
      if (response.ok) {
        return (text ? JSON.parse(text) : {}) as T;
      }

      const error = errorFromResponse(response.status, text, response.headers.get('X-Request-ID') ?? undefined);
      if (RETRYABLE.has(response.status) && !last) {
        await this.backoff(attempt);
        continue;
      }
      throw error;
    }
    // Unreachable: the loop always returns or throws on its final attempt.
    throw new TPTPayError({ message: 'request failed' });
  }

  private async send(method: Method, path: string, body: unknown, idempotencyKey?: string): Promise<Response> {
    const headers: Record<string, string> = {
      Authorization: `Bearer ${this.apiKey}`,
      Accept: 'application/json',
    };
    if (body !== undefined) headers['Content-Type'] = 'application/json';
    if (idempotencyKey) headers['Idempotency-Key'] = idempotencyKey;

    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), this.timeoutMs);
    try {
      return await fetch(`${this.apiBaseUrl}${path}`, {
        method,
        headers,
        body: body !== undefined ? JSON.stringify(body) : undefined,
        signal: controller.signal,
      });
    } finally {
      clearTimeout(timer);
    }
  }

  private backoff(attempt: number): Promise<void> {
    const base = this.retryBaseDelayMs * 2 ** attempt;
    const delay = Math.min(base, 8_000) * (0.5 + Math.random() / 2);
    return new Promise((resolve) => setTimeout(resolve, delay));
  }
}

function randomKey(): string {
  return globalThis.crypto.randomUUID();
}

export default TPTPay;
