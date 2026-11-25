// tpt-payments/frontend/app/api/auth/2fa/verify/route.ts
import { NextResponse } from 'next/server'
import { cookies } from 'next/headers'
import * as jose from 'jose'
import {
  ConfigError,
  SESSION_TTL_SECONDS,
  allowRequest,
  clientKey,
  consumePreAuth,
  jwtKey,
  newTokenId,
} from '../../../../../lib/auth'
import { verifyTotp } from '../../../../../lib/dashboardCore'

const VERIFY_WINDOW_MS = 15 * 60 * 1000
const VERIFY_MAX_ATTEMPTS = 10

export async function POST(request: Request) {
  if (!allowRequest(`2fa:${clientKey(request)}`, VERIFY_MAX_ATTEMPTS, VERIFY_WINDOW_MS)) {
    return NextResponse.json({ error: 'Too many attempts. Try again later.' }, { status: 429 })
  }

  let key: Uint8Array
  try {
    key = jwtKey()
  } catch (error) {
    if (error instanceof ConfigError) {
      return NextResponse.json({ error: 'Authentication is not configured' }, { status: 503 })
    }
    throw error
  }

  let token: unknown
  try {
    token = (await request.json()).token
  } catch {
    return NextResponse.json({ error: 'Invalid request' }, { status: 400 })
  }
  if (typeof token !== 'string' || !/^\d{6}$/.test(token)) {
    return NextResponse.json({ error: 'Invalid 2FA token' }, { status: 401 })
  }

  const cookieStore = await cookies()
  const preAuthToken = cookieStore.get('pre_auth')?.value
  if (!preAuthToken) {
    return NextResponse.json({ error: 'No pre-authentication session found' }, { status: 400 })
  }

  let payload: jose.JWTPayload
  try {
    ;({ payload } = await jose.jwtVerify(preAuthToken, key))
  } catch {
    return NextResponse.json({ error: 'Invalid request' }, { status: 400 })
  }
  if (payload.type !== 'pre_auth' || !payload.jti || payload.exp === undefined || !payload.sub) {
    return NextResponse.json({ error: 'Invalid token type' }, { status: 400 })
  }

  // Single use: the token is spent on the first attempt, right or wrong.
  if (!consumePreAuth(payload.jti, payload.exp * 1000)) {
    cookieStore.delete('pre_auth')
    return NextResponse.json({ error: 'Pre-authentication session already used' }, { status: 401 })
  }
  cookieStore.delete('pre_auth')

  // The core counts wrong codes per user, shared by every server instance, and refuses a code that
  // was already used.
  let result
  try {
    result = await verifyTotp(payload.sub, token)
  } catch (error) {
    if (error instanceof ConfigError) {
      return NextResponse.json({ error: 'Authentication is not configured' }, { status: 503 })
    }
    console.error('two-factor check could not reach the core:', error)
    return NextResponse.json({ error: 'Sign-in is unavailable. Try again shortly.' }, { status: 503 })
  }
  if (result.kind === 'locked') {
    return NextResponse.json({ error: 'Account temporarily locked. Try again later.' }, { status: 429 })
  }
  if (result.kind !== 'ok') {
    return NextResponse.json({ error: 'Invalid 2FA token' }, { status: 401 })
  }

  const sessionToken = await new jose.SignJWT({
    email: payload.email,
    merchantId: payload.merchantId,
    name: payload.name,
    twoFactorEnabled: true,
  })
    .setProtectedHeader({ alg: 'HS256' })
    .setSubject(payload.sub)
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
      id: payload.sub,
      email: payload.email,
      merchantId: payload.merchantId,
      name: payload.name,
      twoFactorEnabled: true,
    },
  })
}
