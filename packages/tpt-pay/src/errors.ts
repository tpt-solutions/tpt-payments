/**
 * Errors raised by the TPT Pay client. Every failure is a TPTPayError, carrying the fields
 * of the API error envelope where the server provided them.
 */

export interface TPTPayErrorInit {
  message: string;
  status?: number;
  type?: string;
  code?: string;
  param?: string;
  requestId?: string;
}

export class TPTPayError extends Error {
  readonly status?: number;
  readonly type?: string;
  readonly code?: string;
  readonly param?: string;
  readonly requestId?: string;

  constructor(init: TPTPayErrorInit) {
    super(init.message);
    this.name = 'TPTPayError';
    this.status = init.status;
    this.type = init.type;
    this.code = init.code;
    this.param = init.param;
    this.requestId = init.requestId;
  }

  /** True when the card network declined the payment (HTTP 402). Safe to show the customer. */
  get isCardError(): boolean {
    return this.status === 402;
  }
}

/** Thrown when a webhook signature is missing, malformed, mismatched or outside tolerance. */
export class SignatureVerificationError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'SignatureVerificationError';
  }
}

/** Build a TPTPayError from an HTTP error response body. */
export function errorFromResponse(status: number, body: string, requestId?: string): TPTPayError {
  let message = body.slice(0, 500) || `HTTP ${status}`;
  let type: string | undefined;
  let code: string | undefined;
  let param: string | undefined;
  try {
    const parsed: unknown = JSON.parse(body);
    if (parsed && typeof parsed === 'object' && 'error' in parsed) {
      const err = (parsed as { error: Record<string, unknown> }).error;
      if (err && typeof err === 'object') {
        if (typeof err.message === 'string') message = err.message;
        if (typeof err.type === 'string') type = err.type;
        if (typeof err.code === 'string') code = err.code;
        if (typeof err.param === 'string') param = err.param;
      }
    }
  } catch {
    // Not JSON: keep the raw body as the message.
  }
  return new TPTPayError({ message, status, type, code, param, requestId });
}
