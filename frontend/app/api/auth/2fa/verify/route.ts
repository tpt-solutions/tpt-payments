// tpt-payments/frontend/app/api/auth/2fa/verify/route.ts
import { NextResponse } from 'next/server'
import { cookies } from 'next/headers'
import * as jose from 'jose'
import * as OTPAuth from 'otpauth'

export async function POST(request: Request) {
  try {
    const { token } = await request.json()
    const cookieStore = await cookies()
    const preAuthToken = cookieStore.get('pre_auth')?.value

    if (!preAuthToken) {
      return NextResponse.json({ error: 'No pre-authentication session found' }, { status: 400 })
    }

    // Verify pre-auth token
    const secret = new TextEncoder().encode(process.env.JWT_SECRET || 'development-secret-key-change-in-production')
    const { payload } = await jose.jwtVerify(preAuthToken, secret)

    if (payload.type !== 'pre_auth') {
      return NextResponse.json({ error: 'Invalid token type' }, { status: 400 })
    }

    // In production: fetch TOTP secret from user record
    // Mock TOTP secret for demonstration
    const totp = new OTPAuth.TOTP({
      issuer: 'TPT Pay',
      label: payload.email as string,
      algorithm: 'SHA1',
      digits: 6,
      period: 30,
      secret: 'JBSWY3DPEHPK3PXP', // Base32 secret
    })

    const delta = totp.validate({ token, window: 1 })

    if (delta === null) {
      return NextResponse.json({ error: 'Invalid 2FA token' }, { status: 401 })
    }

    // Clear pre-auth token
    cookieStore.delete('pre_auth')

    // Create full session token
    const sessionToken = await new jose.SignJWT({
      sub: payload.sub,
      email: payload.email,
      merchantId: 'merch_01h7nq3qz8k7x9v5b2n4m6l8',
      name: 'Demo Merchant',
      twoFactorEnabled: true,
    })
      .setProtectedHeader({ alg: 'HS256' })
      .setIssuedAt()
      .setExpirationTime('24h')
      .sign(secret)

    cookieStore.set('session', sessionToken, {
      httpOnly: true,
      secure: process.env.NODE_ENV === 'production',
      sameSite: 'strict',
      maxAge: 86400, // 24 hours
      path: '/',
    })

    return NextResponse.json({
      user: {
        id: payload.sub,
        email: payload.email,
        merchantId: 'merch_01h7nq3qz8k7x9v5b2n4m6l8',
        name: 'Demo Merchant',
        twoFactorEnabled: true,
      },
    })
  } catch (error) {
    return NextResponse.json({ error: 'Invalid request' }, { status: 400 })
  }
}