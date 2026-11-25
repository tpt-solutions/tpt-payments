// Server-side client for the TPT Pay core API. It runs only in route handlers, so the merchant's
// secret key never reaches the browser. Config comes from the server environment:
//   TPT_API_BASE            e.g. https://api.tpt-payments.com/v1
//   TPT_DASHBOARD_SECRET_KEY  the secret key the dashboard reads with (sk_test_ while developing)
import { NextResponse } from 'next/server'
import * as jose from 'jose'
import { cookies } from 'next/headers'
import { ConfigError, jwtKey } from './auth'

// True only for a valid, non-pre-auth session. Mirrors middleware.ts, which the proxy routes
// need because the matcher there does not cover /api.
export async function hasSession(): Promise<boolean> {
  const token = (await cookies()).get('session')?.value
  if (!token) return false
  try {
    const { payload } = await jose.jwtVerify(token, jwtKey())
    return payload.type !== 'pre_auth'
  } catch {
    return false
  }
}

export class CoreUnavailable extends Error {}

function coreConfig(): { base: string; key: string } {
  const base = process.env.TPT_API_BASE
  const key = process.env.TPT_DASHBOARD_SECRET_KEY
  if (!base || !key) {
    throw new CoreUnavailable('TPT_API_BASE and TPT_DASHBOARD_SECRET_KEY must be set on the server')
  }
  return { base: base.replace(/\/$/, ''), key }
}

// coreCall calls the core API and passes its status and JSON body back. Only the paths and
// query parameters the caller names are sent. For writes, idempotencyKey makes a retry safe.
export async function coreCall(
  method: "GET" | "POST" | "DELETE",
  path: string,
  options: { params?: URLSearchParams; body?: unknown; idempotencyKey?: string } = {},
): Promise<NextResponse> {
  let cfg
  try {
    cfg = coreConfig()
  } catch (err) {
    if (err instanceof CoreUnavailable) {
      return NextResponse.json({ error: "Payments API is not configured" }, { status: 503 })
    }
    throw err
  }
  const query = options.params?.toString()
  const url = `${cfg.base}${path}${query ? `?${query}` : ""}`
  const headers: Record<string, string> = { Authorization: `Bearer ${cfg.key}` }
  if (options.body !== undefined) headers["Content-Type"] = "application/json"
  if (options.idempotencyKey) headers["Idempotency-Key"] = options.idempotencyKey
  try {
    const res = await fetch(url, {
      method,
      headers,
      body: options.body === undefined ? undefined : JSON.stringify(options.body),
      cache: "no-store",
      signal: AbortSignal.timeout(15_000),
    })
    const body = await res.json().catch(() => ({ error: "unexpected response from the payments API" }))
    if (res.status >= 500) {
      // Upstream detail stays on the server.
      return NextResponse.json({ error: "payments API unavailable" }, { status: 502 })
    }
    return NextResponse.json(body, { status: res.status })
  } catch {
    return NextResponse.json({ error: "payments API unreachable" }, { status: 502 })
  }
}

export function unauthorized(): NextResponse {
  return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })
}

export { ConfigError }
