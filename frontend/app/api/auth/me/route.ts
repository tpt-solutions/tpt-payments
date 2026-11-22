// tpt-payments/frontend/app/api/auth/me/route.ts
import { NextResponse } from 'next/server'
import { cookies } from 'next/headers'
import * as jose from 'jose'
import { ConfigError, jwtKey } from '../../../../lib/auth'

export async function GET() {
  try {
    const cookieStore = await cookies()
    const sessionToken = cookieStore.get('session')?.value

    if (!sessionToken) {
      return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })
    }

    const { payload } = await jose.jwtVerify(sessionToken, jwtKey())
    if (payload.type === 'pre_auth') {
      return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })
    }

    return NextResponse.json({
      id: payload.sub,
      email: payload.email,
      merchantId: payload.merchantId,
      name: payload.name,
      twoFactorEnabled: payload.twoFactorEnabled,
    })
  } catch (error) {
    if (error instanceof ConfigError) {
      return NextResponse.json({ error: 'Authentication is not configured' }, { status: 503 })
    }
    return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })
  }
}
