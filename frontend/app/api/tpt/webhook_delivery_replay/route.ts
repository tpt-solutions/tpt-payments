// Queues one webhook delivery to be sent again. Delivery IDs are UUIDs. The core answers 202: the
// delivery is queued and sent by the workers. Query form, not a path segment: see /api/tpt/payment_intent.
import { type NextRequest } from "next/server"
import { coreCall, hasSession, unauthorized } from "../../../../lib/tptCore"
import { badRequest } from "../../../../lib/tpt_validate"

const DELIVERY_ID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i

export async function POST(request: NextRequest) {
  if (!(await hasSession())) return unauthorized()
  const id = request.nextUrl.searchParams.get("id") ?? ""
  if (!DELIVERY_ID.test(id)) return badRequest("id must be a delivery id")
  return coreCall("POST", `/webhook_deliveries/${encodeURIComponent(id)}/replay`, { body: {} })
}
