// tpt-payments/frontend/app/api/auth/me/route.ts
import { NextResponse } from 'next/server'
import { cookies } from 'next/headers'
import * as jose from 'jose'

export async function GET() {
  try {
    const cookieStore = cookies()
    const sessionToken = cookieStore.get('session')?.value

    if (!sessionToken) {
      return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })
    }

    const secret = new TextEncoder().encode(process.env.JWT_SECRET || 'development-secret-key-change-in-production')
    const { payload } = await jose.jwtVerify(sessionToken, secret)

    return NextResponse.json({
      id: payload.sub,
      email: payload.email,
      merchantId: payload.merchantId,
      name: payload.name,
      twoFactorEnabled: payload.twoFactorEnabled,
    })
  } catch (error) {
    return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })
  }
}