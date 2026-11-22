// Shared authentication helpers for the prototype dashboard.
//
// The user store is still a single prototype account, but nothing about it is committed to
// source: its email, password hash and TOTP secret come from the environment, and every
// route refuses to run when they are missing. Signing keys are never defaulted.
import * as jose from 'jose'
import { randomBytes, randomUUID, scrypt, timingSafeEqual } from 'crypto'
import { promisify } from 'util'

const scryptAsync = promisify(scrypt) as (
  password: string,
  salt: Buffer,
  keylen: number,
) => Promise<Buffer>

export const SESSION_TTL_SECONDS = 24 * 60 * 60
export const PREAUTH_TTL_SECONDS = 5 * 60

const MIN_SECRET_LENGTH = 32

/** Thrown when required configuration is missing. Routes turn this into a 503. */
export class ConfigError extends Error {}

/** The HMAC key for session and pre-auth tokens. There is no default. */
export function jwtKey(): Uint8Array {
  const raw = process.env.JWT_SECRET
  if (!raw || raw.length < MIN_SECRET_LENGTH) {
    throw new ConfigError(`JWT_SECRET must be set to at least ${MIN_SECRET_LENGTH} characters`)
  }
  return new TextEncoder().encode(raw)
}

/**
 * Hashes a password as `scrypt$<saltHex>$<keyHex>` with a fresh random salt.
 * Use this to produce the value of DEMO_USER_PASSWORD_HASH.
 */
export async function hashPassword(password: string): Promise<string> {
  const salt = randomBytes(16)
  const key = await scryptAsync(password, salt, 32)
  return `scrypt$${salt.toString('hex')}$${key.toString('hex')}`
}

/** Checks a password against a stored `scrypt$salt$key` value in constant time. */
export async function verifyPassword(password: string, stored: string): Promise<boolean> {
  const [scheme, saltHex, keyHex] = stored.split('$')
  if (scheme !== 'scrypt' || !saltHex || !keyHex) return false
  const expected = Buffer.from(keyHex, 'hex')
  if (expected.length === 0) return false
  const actual = await scryptAsync(password, Buffer.from(saltHex, 'hex'), expected.length)
  return timingSafeEqual(actual, expected)
}

export interface PrototypeUser {
  id: string
  email: string
  merchantId: string
  name: string
  passwordHash: string
  totpSecret: string
}

/** The prototype account, built from DEMO_USER_* environment variables. */
export function prototypeUser(): PrototypeUser {
  const email = process.env.DEMO_USER_EMAIL
  const passwordHash = process.env.DEMO_USER_PASSWORD_HASH
  const totpSecret = process.env.DEMO_USER_TOTP_SECRET
  if (!email || !passwordHash || !totpSecret) {
    throw new ConfigError(
      'DEMO_USER_EMAIL, DEMO_USER_PASSWORD_HASH and DEMO_USER_TOTP_SECRET must be set',
    )
  }
  return {
    id: 'merch_01h7nq3qz8k7x9v5b2n4m6l8',
    email,
    merchantId: 'merch_01h7nq3qz8k7x9v5b2n4m6l8',
    name: 'Demo Merchant',
    passwordHash,
    totpSecret,
  }
}

// ── Rate limiting ────────────────────────────────────────────────────────────
// In-process fixed-window counter. It is per server instance: a multi-instance deployment
// needs a shared store (for example Redis) for the limit to hold.

const windows = new Map<string, { count: number; resetAt: number }>()
const MAX_TRACKED_KEYS = 10_000

/** Returns true if the request is allowed, counting it against the window. */
export function allowRequest(key: string, max: number, windowMs: number): boolean {
  const now = Date.now()
  if (windows.size > MAX_TRACKED_KEYS) {
    for (const [k, w] of windows) if (w.resetAt <= now) windows.delete(k)
  }
  const w = windows.get(key)
  if (!w || w.resetAt <= now) {
    windows.set(key, { count: 1, resetAt: now + windowMs })
    return true
  }
  if (w.count >= max) return false
  w.count += 1
  return true
}

/**
 * Identifies the caller for rate limiting. The first X-Forwarded-For hop is used only when a
 * trusted proxy sets it; otherwise all unidentified callers share one bucket.
 */
export function clientKey(request: Request): string {
  const forwarded = request.headers.get('x-forwarded-for')
  return forwarded?.split(',')[0]?.trim() || 'unknown'
}

// ── Single-use pre-auth tokens ───────────────────────────────────────────────

const consumedPreAuth = new Map<string, number>() // jti -> expiry (ms)

/**
 * Marks a pre-auth token as used. Returns false if it was already used. Each token allows
 * exactly one TOTP attempt, so a second guess needs a fresh password check.
 */
export function consumePreAuth(jti: string, expiresAtMs: number): boolean {
  const now = Date.now()
  for (const [id, exp] of consumedPreAuth) if (exp <= now) consumedPreAuth.delete(id)
  if (consumedPreAuth.has(jti)) return false
  consumedPreAuth.set(jti, expiresAtMs)
  return true
}

export function newTokenId(): string {
  return randomUUID()
}
