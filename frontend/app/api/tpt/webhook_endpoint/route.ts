// Disables one webhook endpoint (the core keeps its delivery history). Query form, not a path
// segment: see the note on /api/tpt/payment_intent about the /api rewrite.
import { type NextRequest } from "next/server"
import { coreCall, hasSession, unauthorized } from "../../../../lib/tptCore"
import { ID_PATTERN, badRequest } from "../../../../lib/tpt_validate"

export async function DELETE(request: NextRequest) {
  if (!(await hasSession())) return unauthorized()
  const id = request.nextUrl.searchParams.get("id") ?? ""
  if (!ID_PATTERN.test(id)) return badRequest("id must be an endpoint id")
  return coreCall("DELETE", `/webhook_endpoints/${encodeURIComponent(id)}`)
}
