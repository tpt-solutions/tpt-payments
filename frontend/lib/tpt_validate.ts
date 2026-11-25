// Input checks shared by the proxy routes. Each returns an error message, or null when valid.
import { NextResponse } from "next/server"

export const ID_PATTERN = /^[a-z]{2,4}_[A-Za-z0-9]{1,64}$/
export const IDEMPOTENCY_KEY = /^[A-Za-z0-9_-]{1,255}$/

export function badRequest(message: string): NextResponse {
  return NextResponse.json({ error: message }, { status: 400 })
}

// listParams copies limit and starting_after (when valid) into a new query string.
export function listParams(search: URLSearchParams): { params: URLSearchParams } | { error: string } {
  const params = new URLSearchParams()
  const limit = search.get("limit")
  if (limit !== null) {
    const n = Number(limit)
    if (!Number.isInteger(n) || n < 1 || n > 100) return { error: "limit must be an integer from 1 to 100" }
    params.set("limit", String(n))
  }
  const after = search.get("starting_after")
  if (after !== null) {
    if (!ID_PATTERN.test(after)) return { error: "starting_after must be an id" }
    params.set("starting_after", after)
  }
  return { params }
}

// idempotencyKey reads the header a write must carry.
export function idempotencyKey(request: Request): string | null {
  const key = request.headers.get("idempotency-key") ?? ""
  return IDEMPOTENCY_KEY.test(key) ? key : null
}
