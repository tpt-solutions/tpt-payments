// tpt-payments/frontend/app/api/auth/2fa/verify/route.ts
import { NextResponse } from 'next/server'
import { cookies } from 'next/headers'
import * as jose from 'jose'
import * as OTPAuth from 'otpauth'
import {
  ConfigError,
  SESSION_TTL_SECONDS,
  allowRequest,
  clientKey,
  consumePreAuth,
  jwtKey,
  newTokenId,
  prototypeUser,
} from '../../../../../lib/auth'

const VERIFY_WINDOW_MS = 15 * 60 * 1000
const VERIFY_MAX_ATTEMPTS = 10

export async function POST(request: Request) {
  if (!allowRequest(`2fa:${clientKey(request)}`, VERIFY_MAX_ATTEMPTS, VERIFY_WINDOW_MS)) {
    return NextResponse.json({ error: 'Too many attempts. Try again later.' }, { status: 429 })
  }

  let user
  let key: Uint8Array
  try {
    user = prototypeUser()
    key = jwtKey()
  } catch (error) {
    if (error instanceof ConfigError) {
      return NextResponse.json({ error: 'Authentication is not configured' }, { status: 503 })
    }
    throw error
  }

  try {
    const { token } = await request.json()
    if (typeof token !== 'string' || !/^\d{6}$/.test(token)) {
      return NextResponse.json({ error: 'Invalid 2FA token' }, { status: 401 })
    }

    const cookieStore = await cookies()
    const preAuthToken = cookieStore.get('pre_auth')?.value
    if (!preAuthToken) {
      return NextResponse.json({ error: 'No pre-authentication session found' }, { status: 400 })
    }

    const { payload } = await jose.jwtVerify(preAuthToken, key)
    if (payload.type !== 'pre_auth' || !payload.jti || payload.exp === undefined) {
      return NextResponse.json({ error: 'Invalid token type' }, { status: 400 })
    }

    // Single use: the token is spent on the first attempt, right or wrong.
    if (!consumePreAuth(payload.jti, payload.exp * 1000)) {
      cookieStore.delete('pre_auth')
      return NextResponse.json({ error: 'Pre-authentication session already used' }, { status: 401 })
    }

    const totp = new OTPAuth.TOTP({
      issuer: 'TPT Pay',
      label: user.email,
      algorithm: 'SHA1',
      digits: 6,
      period: 30,
      secret: OTPAuth.Secret.fromBase32(user.totpSecret),
    })

    const delta = totp.validate({ token, window: 1 })
    cookieStore.delete('pre_auth')
    if (delta === null) {
      return NextResponse.json({ error: 'Invalid 2FA token' }, { status: 401 })
    }

    const sessionToken = await new jose.SignJWT({
      email: user.email,
      merchantId: user.merchantId,
      name: user.name,
      twoFactorEnabled: true,
    })
      .setProtectedHeader({ alg: 'HS256' })
      .setSubject(user.id)
      .setJti(newTokenId())
      .setIssuedAt()
      .setExpirationTime(`${SESSION_TTL_SECONDS}s`)
      .sign(key)

    cookieStore.set('session', sessionToken, {
      httpOnly: true,
      secure: process.env.NODE_ENV === 'production',
      sameSite: 'strict',
      maxAge: SESSION_TTL_SECONDS,
      path: '/',
    })

    return NextResponse.json({
      user: {
        id: user.id,
        email: user.email,
        merchantId: user.merchantId,
        name: user.name,
        twoFactorEnabled: true,
      },
    })
  } catch {
    return NextResponse.json({ error: 'Invalid request' }, { status: 400 })
  }
}
