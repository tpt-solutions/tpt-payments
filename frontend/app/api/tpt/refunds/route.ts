// Refunds for the transaction pages: list a payment intent's refunds, and create a refund.
// Both go through the server, so the merchant's secret key stays here.
import { type NextRequest, NextResponse } from "next/server"
import { coreCall, hasSession, unauthorized } from "../../../../lib/tptCore"

const INTENT_ID = /^pi_[A-Za-z0-9]{1,64}$/
const KEY = /^[A-Za-z0-9_-]{1,255}$/

export async function GET(request: NextRequest) {
  if (!(await hasSession())) return unauthorized()
  const intent = request.nextUrl.searchParams.get("payment_intent")
  if (!intent || !INTENT_ID.test(intent)) {
    return NextResponse.json({ error: "payment_intent must be a payment intent id" }, { status: 400 })
  }
  const params = new URLSearchParams({ payment_intent: intent, limit: "100" })
  return coreCall("GET", "/refunds", { params })
}

export async function POST(request: NextRequest) {
  if (!(await hasSession())) return unauthorized()

  let input: { payment_intent?: unknown; amount?: unknown; reason?: unknown }
  try {
    input = await request.json()
  } catch {
    return NextResponse.json({ error: "body must be JSON" }, { status: 400 })
  }
  if (typeof input.payment_intent !== "string" || !INTENT_ID.test(input.payment_intent)) {
    return NextResponse.json({ error: "payment_intent must be a payment intent id" }, { status: 400 })
  }
  if (!Number.isInteger(input.amount) || (input.amount as number) <= 0) {
    return NextResponse.json({ error: "amount must be a positive integer in minor units" }, { status: 400 })
  }
  const body: Record<string, unknown> = { payment_intent: input.payment_intent, amount: input.amount }
  if (typeof input.reason === "string" && input.reason.length > 0) {
    body.reason = input.reason.slice(0, 500)
  }

  // The client sends one key per refund attempt, so a double click or a retry cannot refund twice.
  const key = request.headers.get("idempotency-key") ?? ""
  if (!KEY.test(key)) {
    return NextResponse.json({ error: "Idempotency-Key header is required" }, { status: 400 })
  }
  return coreCall("POST", "/refunds", { body, idempotencyKey: key })
}
