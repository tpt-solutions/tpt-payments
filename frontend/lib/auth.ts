// Shared helpers for the dashboard's sign-in: the signing key, per-client rate limiting, and
// single-use pre-auth tokens. Users, passwords, second factors and account lockout live in the core
// (see dashboardCore.ts); nothing about a user is defined here.
import { randomUUID } from 'crypto'

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
