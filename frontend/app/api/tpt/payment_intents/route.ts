// Lists the merchant's payment intents for the transactions page. Only the documented filters are
// forwarded; anything else in the query string is dropped.
import { type NextRequest, NextResponse } from 'next/server'
import { coreCall, hasSession, unauthorized } from '../../../../lib/tptCore'

const STATUSES = new Set([
  'requires_payment_method',
  'requires_confirmation',
  'requires_action',
  'processing',
  'requires_capture',
  'succeeded',
  'canceled',
])

export async function GET(request: NextRequest) {
  if (!(await hasSession())) return unauthorized()

  const incoming = request.nextUrl.searchParams
  const params = new URLSearchParams()

  const limit = incoming.get('limit')
  if (limit !== null) {
    const n = Number(limit)
    if (!Number.isInteger(n) || n < 1 || n > 100) {
      return NextResponse.json({ error: 'limit must be an integer from 1 to 100' }, { status: 400 })
    }
    params.set('limit', String(n))
  }

  const after = incoming.get('starting_after')
  if (after !== null) {
    if (!/^pi_[A-Za-z0-9]{1,64}$/.test(after)) {
      return NextResponse.json({ error: 'starting_after must be a payment intent id' }, { status: 400 })
    }
    params.set('starting_after', after)
  }

  const status = incoming.get('status')
  if (status !== null && status !== 'all') {
    if (!STATUSES.has(status)) {
      return NextResponse.json({ error: 'unknown status' }, { status: 400 })
    }
    params.set('status', status)
  }

  return coreCall("GET", '/payment_intents', { params })
}
