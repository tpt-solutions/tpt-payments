// tpt-payments/frontend/app/api/auth/login/route.ts
import { NextResponse } from 'next/server'
import { cookies } from 'next/headers'
import * as jose from 'jose'
import {
  ConfigError,
  PREAUTH_TTL_SECONDS,
  SESSION_TTL_SECONDS,
  allowRequest,
  clientKey,
  jwtKey,
  newTokenId,
} from '../../../../lib/auth'
import { authenticate } from '../../../../lib/dashboardCore'

const LOGIN_WINDOW_MS = 15 * 60 * 1000
const LOGIN_MAX_ATTEMPTS = 10

export async function POST(request: Request) {
  if (!allowRequest(`login:${clientKey(request)}`, LOGIN_MAX_ATTEMPTS, LOGIN_WINDOW_MS)) {
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

  let body: { email?: unknown; password?: unknown }
  try {
    body = await request.json()
  } catch {
    return NextResponse.json({ error: 'Invalid request' }, { status: 400 })
  }
  if (typeof body.email !== 'string' || typeof body.password !== 'string') {
    return NextResponse.json({ error: 'Invalid request' }, { status: 400 })
  }

  let result
  try {
    result = await authenticate(body.email, body.password)
  } catch (error) {
    if (error instanceof ConfigError) {
      return NextResponse.json({ error: 'Authentication is not configured' }, { status: 503 })
    }
    console.error('sign-in could not reach the core:', error)
    return NextResponse.json({ error: 'Sign-in is unavailable. Try again shortly.' }, { status: 503 })
  }
  if (result.kind === 'locked') {
    return NextResponse.json({ error: 'Account temporarily locked. Try again later.' }, { status: 429 })
  }
  if (result.kind === 'invalid') {
    return NextResponse.json({ error: 'Invalid credentials' }, { status: 401 })
  }

  const user = result.user
  const cookieStore = await cookies()
  const claims = { email: user.email, merchantId: user.merchant_id, name: user.name }

  // Two-factor users get a short-lived, single-use pre-auth token instead of a session. It names the
  // user, so the second step does not need the password again.
  if (user.totp_enabled) {
    const preAuthToken = await new jose.SignJWT({ ...claims, type: 'pre_auth' })
      .setProtectedHeader({ alg: 'HS256' })
      .setSubject(user.id)
      .setJti(newTokenId())
      .setIssuedAt()
      .setExpirationTime(`${PREAUTH_TTL_SECONDS}s`)
      .sign(key)

    cookieStore.set('pre_auth', preAuthToken, {
      httpOnly: true,
      secure: process.env.NODE_ENV === 'production',
      sameSite: 'strict',
      maxAge: PREAUTH_TTL_SECONDS,
      path: '/',
    })
    return NextResponse.json({ requiresTwoFactor: true })
  }

  const token = await new jose.SignJWT({ ...claims, twoFactorEnabled: false })
    .setProtectedHeader({ alg: 'HS256' })
    .setSubject(user.id)
    .setJti(newTokenId())
    .setIssuedAt()
    .setExpirationTime(`${SESSION_TTL_SECONDS}s`)
    .sign(key)

  cookieStore.set('session', token, {
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
      merchantId: user.merchant_id,
      name: user.name,
      twoFactorEnabled: false,
    },
  })
}
