// Revokes one API key. Query form, not a path segment: see /api/tpt/payment_intent.
import { type NextRequest } from "next/server"
import { coreCall, hasSession, unauthorized } from "../../../../lib/tptCore"
import { ID_PATTERN, badRequest } from "../../../../lib/tpt_validate"

export async function DELETE(request: NextRequest) {
  if (!(await hasSession())) return unauthorized()
  const id = request.nextUrl.searchParams.get("id") ?? ""
  if (!ID_PATTERN.test(id)) return badRequest("id must be a key id")
  return coreCall("DELETE", `/api_keys/${encodeURIComponent(id)}`)
}
