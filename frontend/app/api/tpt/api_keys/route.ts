// API keys: list the merchant's keys in the dashboard's mode, or create one. The key's secret comes
// back only from the create call.
import { type NextRequest } from "next/server"
import { coreCall, hasSession, unauthorized } from "../../../../lib/tptCore"
import { badRequest, idempotencyKey, listParams } from "../../../../lib/tpt_validate"

export async function GET(request: NextRequest) {
  if (!(await hasSession())) return unauthorized()
  const parsed = listParams(request.nextUrl.searchParams)
  if ("error" in parsed) return badRequest(parsed.error)
  return coreCall("GET", "/api_keys", { params: parsed.params })
}

export async function POST(request: NextRequest) {
  if (!(await hasSession())) return unauthorized()
  let input: { type?: unknown; name?: unknown }
  try {
    input = await request.json()
  } catch {
    return badRequest("body must be JSON")
  }
  if (input.type !== undefined && input.type !== "secret" && input.type !== "publishable") {
    return badRequest("type must be 'secret' or 'publishable'")
  }
  if (typeof input.name !== "string" || input.name.trim().length === 0 || input.name.length > 100) {
    return badRequest("name is required (up to 100 characters)")
  }
  const key = idempotencyKey(request)
  if (!key) return badRequest("Idempotency-Key header is required")
  return coreCall("POST", "/api_keys", {
    body: { type: input.type ?? "secret", name: input.name.trim() },
    idempotencyKey: key,
  })
}
