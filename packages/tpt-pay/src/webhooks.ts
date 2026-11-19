/**
 * Verify and parse TPT Pay webhook deliveries. Server-side only (uses node:crypto).
 *
 * Header: `TPT-Signature: t=<unix>,v1=<hex>[,v1=<hex>...]`. Each v1 is HMAC-SHA256 over
 * `"<t>." + raw body`, keyed with the endpoint's signing secret (whsec_...).
 *
 * Pass the raw body exactly as received. Parsing it first changes the bytes and breaks
 * the signature.
 */

import { createHmac, timingSafeEqual } from 'node:crypto';
import { SignatureVerificationError } from './errors';

export { SignatureVerificationError } from './errors';

export const SIGNATURE_HEADER = 'TPT-Signature';
export const DEFAULT_TOLERANCE_SECONDS = 300;
const MAX_CLOCK_SKEW_SECONDS = 60;

export interface VerifyOptions {
  /** Maximum age of the signed timestamp, in seconds. Default 300. */
  toleranceSeconds?: number;
  /** Current time in Unix seconds. Defaults to the system clock (useful in tests). */
  now?: number;
}

/**
 * Verify a delivery. Returns normally when valid.
 * @throws SignatureVerificationError when the header is malformed, no signature matches,
 *         or the timestamp is too old or too far in the future.
 */
export function verifySignature(
  payload: string | Uint8Array,
  header: string,
  secret: string,
  options: VerifyOptions = {},
): void {
  const { timestamp, signatures } = parseHeader(header);
  const issued = Number(timestamp);
  if (!Number.isInteger(issued)) {
    throw new SignatureVerificationError('malformed timestamp in signature header');
  }

  const body = typeof payload === 'string' ? Buffer.from(payload, 'utf8') : Buffer.from(payload);
  const expected = createHmac('sha256', secret)
    .update(`${timestamp}.`)
    .update(body)
    .digest();

  const matched = signatures.some((hex) => {
    if (!/^[0-9a-f]+$/i.test(hex) || hex.length !== expected.length * 2) return false;
    return timingSafeEqual(Buffer.from(hex, 'hex'), expected);
  });
  if (!matched) {
    throw new SignatureVerificationError('no signature matches the payload and secret');
  }

  const now = options.now ?? Math.floor(Date.now() / 1000);
  if (issued - now > MAX_CLOCK_SKEW_SECONDS) {
    throw new SignatureVerificationError('timestamp is in the future');
  }
  const tolerance = options.toleranceSeconds ?? DEFAULT_TOLERANCE_SECONDS;
  if (now - issued > tolerance) {
    throw new SignatureVerificationError(`timestamp too old (tolerance ${tolerance}s)`);
  }
}

/**
 * Verify a delivery and return the parsed event.
 * @throws SignatureVerificationError if verification fails.
 */
export function constructEvent<T = unknown>(
  payload: string | Uint8Array,
  header: string,
  secret: string,
  options: VerifyOptions = {},
): T {
  verifySignature(payload, header, secret, options);
  const text = typeof payload === 'string' ? payload : Buffer.from(payload).toString('utf8');
  return JSON.parse(text) as T;
}

function parseHeader(header: string): { timestamp: string; signatures: string[] } {
  let timestamp: string | undefined;
  const signatures: string[] = [];
  for (const part of header.split(',')) {
    const eq = part.indexOf('=');
    if (eq < 0) continue;
    const key = part.slice(0, eq).trim();
    const value = part.slice(eq + 1).trim();
    if (key === 't') {
      if (timestamp !== undefined) {
        throw new SignatureVerificationError('duplicate timestamp in signature header');
      }
      timestamp = value;
    } else if (key === 'v1') {
      signatures.push(value);
    }
  }
  if (timestamp === undefined) {
    throw new SignatureVerificationError('missing timestamp in signature header');
  }
  if (signatures.length === 0) {
    throw new SignatureVerificationError('missing v1 signature in signature header');
  }
  return { timestamp, signatures };
}
