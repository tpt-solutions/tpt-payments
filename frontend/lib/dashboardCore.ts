// tpt-payments/frontend/lib/dashboardCore.ts
// Calls the core's internal dashboard-login routes. The core owns the users, their password hashes,
// their second factor and the lockout, so every server instance shares them. These routes are
// reachable only with the service token, and only from this server; the browser never calls them.
import { ConfigError } from './auth'

export interface CoreUser {
  id: string
  merchant_id: string
  email: string
  name: string
  totp_enabled: boolean
}

export type AuthResult =
  | { kind: 'ok'; user: CoreUser }
  | { kind: 'invalid' }
  | { kind: 'locked' }

export type TotpResult = { kind: 'ok' } | { kind: 'invalid' } | { kind: 'locked' } | { kind: 'not_enrolled' }

function coreConfig(): { base: string; token: string } {
  const base = process.env.TPT_CORE_URL
  const token = process.env.DASHBOARD_SERVICE_TOKEN
  if (!base || !token || token.length < 32) {
    throw new ConfigError('TPT_CORE_URL and DASHBOARD_SERVICE_TOKEN (at least 32 characters) must be set')
  }
  return { base: base.replace(/\/+$/, ''), token }
}

async function call(path: string, body: unknown): Promise<{ status: number; json: unknown }> {
  const { base, token } = coreConfig()
  const response = await fetch(`${base}/internal/dashboard/${path}`, {
    method: 'POST',
    headers: {
      'content-type': 'application/json',
      'X-Dashboard-Service-Token': token,
    },
    body: JSON.stringify(body),
    cache: 'no-store',
    signal: AbortSignal.timeout(5000),
  })
  const json: unknown = await response.json().catch(() => ({}))
  return { status: response.status, json }
}

/** Checks an email and password. */
export async function authenticate(email: string, password: string): Promise<AuthResult> {
  const { status, json } = await call('authenticate', { email, password })
  if (status === 200) {
    const user = (json as { user?: CoreUser }).user
    if (user) return { kind: 'ok', user }
  }
  if (status === 429) return { kind: 'locked' }
  if (status === 401) return { kind: 'invalid' }
  throw new Error(`core authenticate returned ${status}`)
}

/** Checks a six-digit second-factor code for a user. */
export async function verifyTotp(userId: string, code: string): Promise<TotpResult> {
  const { status } = await call('verify-totp', { user_id: userId, code })
  if (status === 200) return { kind: 'ok' }
  if (status === 401) return { kind: 'invalid' }
  if (status === 429) return { kind: 'locked' }
  if (status === 409) return { kind: 'not_enrolled' }
  throw new Error(`core verify-totp returned ${status}`)
}
