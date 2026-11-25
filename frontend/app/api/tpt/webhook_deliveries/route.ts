// Delivery log for the webhooks page: what was sent to each endpoint, and whether it worked.
import { type NextRequest } from "next/server"
import { coreCall, hasSession, unauthorized } from "../../../../lib/tptCore"
import { ID_PATTERN, badRequest, listParams } from "../../../../lib/tpt_validate"

export async function GET(request: NextRequest) {
  if (!(await hasSession())) return unauthorized()
  const parsed = listParams(request.nextUrl.searchParams)
  if ("error" in parsed) return badRequest(parsed.error)
  const endpoint = request.nextUrl.searchParams.get("endpoint_id")
  if (endpoint !== null) {
    if (!ID_PATTERN.test(endpoint)) return badRequest("endpoint_id must be an endpoint id")
    parsed.params.set("endpoint_id", endpoint)
  }
  return coreCall("GET", "/webhook_deliveries", { params: parsed.params })
}

