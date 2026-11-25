// Returns one payment intent for the transaction detail page: GET /api/tpt/payment_intent?id=pi_...
//
// A query parameter rather than a path segment on purpose: next.config.js rewrites /api/* to the
// core, and Next applies afterFiles rewrites before dynamic routes, so a /[id] route under /api
// would never be reached.
import { type NextRequest, NextResponse } from "next/server"
import { coreCall, hasSession, unauthorized } from "../../../../lib/tptCore"

const INTENT_ID = /^pi_[A-Za-z0-9]{1,64}$/

export async function GET(request: NextRequest) {
  if (!(await hasSession())) return unauthorized()
  const id = request.nextUrl.searchParams.get("id") ?? ""
  if (!INTENT_ID.test(id)) {
    return NextResponse.json({ error: "id must be a payment intent id" }, { status: 400 })
  }
  return coreCall("GET", `/payment_intents/${encodeURIComponent(id)}`)
}
