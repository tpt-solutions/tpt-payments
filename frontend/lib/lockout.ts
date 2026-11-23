// tpt-payments/frontend/lib/lockout.ts
// Per-account lockout after repeated failed password checks. This complements the per-client
// rate limit in lib/auth.ts: that limit slows one client, this one protects one account from
// being guessed from many clients.
//
// State is in process memory, like the rate limiter. Several server instances each keep their
// own counts until a shared store is added (see todo section 6).

export const LOCKOUT_MAX_FAILURES = 5
export const LOCKOUT_DURATION_MS = 15 * 60 * 1000

const MAX_TRACKED_ACCOUNTS = 10_000

interface AccountState {
  failures: number
  lockedUntil: number // epoch ms; 0 when not locked
  lastFailureAt: number
}

const accounts = new Map<string, AccountState>()

/** Normalize an account identifier so "Ada@Example.com " and "ada@example.com" share state. */
export function accountKey(email: string): string {
  return email.trim().toLowerCase()
}

/** Epoch ms when the lock ends, or null when the account is not locked. */
export function lockedUntil(key: string, now: number = Date.now()): number | null {
  const state = accounts.get(key)
  if (!state || state.lockedUntil <= now) return null
  return state.lockedUntil
}

/** Count a failed password check. The fifth failure in a row locks the account. */
export function recordFailedLogin(key: string, now: number = Date.now()): void {
  pruneIfNeeded(now)
  const state = accounts.get(key) ?? { failures: 0, lockedUntil: 0, lastFailureAt: now }
  // A failure long after the last one starts a new count.
  if (now - state.lastFailureAt > LOCKOUT_DURATION_MS) state.failures = 0
  state.failures += 1
  state.lastFailureAt = now
  if (state.failures >= LOCKOUT_MAX_FAILURES) {
    state.lockedUntil = now + LOCKOUT_DURATION_MS
    state.failures = 0
  }
  accounts.set(key, state)
}

/** A successful login clears the count. */
export function clearFailedLogins(key: string): void {
  accounts.delete(key)
}

/** Drop expired entries once the map gets large, so an attacker cannot grow it without bound. */
function pruneIfNeeded(now: number): void {
  if (accounts.size < MAX_TRACKED_ACCOUNTS) return
  for (const [key, state] of accounts) {
    if (state.lockedUntil <= now && now - state.lastFailureAt > LOCKOUT_DURATION_MS) {
      accounts.delete(key)
    }
  }
}
