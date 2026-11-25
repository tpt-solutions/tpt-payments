// Webhook endpoints: list the merchant's endpoints, or create one. The signing secret comes back
// only from the create call, and the browser shows it once.
import { type NextRequest, NextResponse } from "next/server"
import { coreCall, hasSession, unauthorized } from "../../../../lib/tptCore"
import { badRequest, idempotencyKey, listParams } from "../../../../lib/tpt_validate"

// The core's list of events it emits (webhook.KnownEventTypes).
const KNOWN_EVENTS = new Set([
  "payment_intent.created",
  "payment_intent.succeeded",
  "payment_intent.payment_failed",
  "payment_intent.canceled",
  "charge.refunded",
])

export async function GET(request: NextRequest) {
  if (!(await hasSession())) return unauthorized()
  const parsed = listParams(request.nextUrl.searchParams)
  if ("error" in parsed) return badRequest(parsed.error)
  return coreCall("GET", "/webhook_endpoints", { params: parsed.params })
}

export async function POST(request: NextRequest) {
  if (!(await hasSession())) return unauthorized()
  let input: { url?: unknown; enabled_events?: unknown }
  try {
    input = await request.json()
  } catch {
    return badRequest("body must be JSON")
  }
  if (typeof input.url !== "string" || input.url.length === 0 || input.url.length > 2048) {
    return badRequest("url is required")
  }
  let events: string[] = []
  if (input.enabled_events !== undefined) {
    if (!Array.isArray(input.enabled_events) || !input.enabled_events.every((e) => typeof e === "string")) {
      return badRequest("enabled_events must be a list of event names")
    }
    events = input.enabled_events as string[]
    if (events.some((e) => !KNOWN_EVENTS.has(e))) return badRequest("unknown event name")
  }
  const key = idempotencyKey(request)
  if (!key) return badRequest("Idempotency-Key header is required")
  return coreCall("POST", "/webhook_endpoints", {
    body: { url: input.url, enabled_events: events },
    idempotencyKey: key,
  })
}
