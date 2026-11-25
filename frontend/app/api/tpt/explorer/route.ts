// Sends one request from the API explorer. It is public (the docs are public), so it is strict:
//   - only the endpoints in ROUTES are sent, with payment intent ids in the documented form;
//   - the key must be a TEST key, and it is used for this request only: never stored, never logged;
//   - the request body is capped, and the core's own checks still apply.
import { type NextRequest, NextResponse } from "next/server"
import { coreCall } from "../../../../lib/tptCore"

const ID = "pi_[A-Za-z0-9]{1,64}"
const ROUTES: { method: "GET" | "POST"; pattern: RegExp }[] = [
  { method: "POST", pattern: /^\/payment_intents$/ },
  { method: "GET", pattern: new RegExp(`^/payment_intents/${ID}$`) },
  { method: "POST", pattern: new RegExp(`^/payment_intents/${ID}/(confirm|capture)$`) },
  { method: "POST", pattern: /^\/refunds$/ },
]
const KEY = /^(sk|pk)_test_[A-Za-z0-9_]{8,200}$/
const MAX_BODY_CHARS = 16 * 1024

export async function POST(request: NextRequest) {
  let input: { method?: unknown; path?: unknown; body?: unknown; key?: unknown }
  try {
    input = await request.json()
  } catch {
    return NextResponse.json({ error: "body must be JSON" }, { status: 400 })
  }

  // Accept the documented form with or without the /v1 prefix.
  const path = typeof input.path === "string" ? input.path.replace(/^\/v1(?=\/)/, "") : ""
  const route = ROUTES.find((r) => r.method === input.method && r.pattern.test(path))
  if (!route) {
    return NextResponse.json({ error: "that endpoint is not available in the explorer" }, { status: 400 })
  }

  if (typeof input.key !== "string" || !KEY.test(input.key)) {
    if (typeof input.key === "string" && /^(sk|pk)_live_/.test(input.key)) {
      return NextResponse.json({ error: "live keys are not accepted here; use a test key (sk_test_ or pk_test_)" }, { status: 400 })
    }
    return NextResponse.json({ error: "enter a test API key (sk_test_... or pk_test_...)" }, { status: 400 })
  }

  let body: unknown = undefined
  if (input.body !== undefined && input.body !== null) {
    if (typeof input.body !== "object" || Array.isArray(input.body)) {
      return NextResponse.json({ error: "body must be a JSON object" }, { status: 400 })
    }
    if (JSON.stringify(input.body).length > MAX_BODY_CHARS) {
      return NextResponse.json({ error: "body is too large for the explorer" }, { status: 413 })
    }
    body = input.body
  }

  const corePath = path
  const headers = route.method === "POST" ? { idempotencyKey: crypto.randomUUID() } : {}
  return coreCall(route.method, corePath, { body: route.method === "POST" ? body ?? {} : undefined, apiKey: input.key, ...headers })
}
