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
  prototypeUser,
  verifyPassword,
} from '../../../../lib/auth'

const LOGIN_WINDOW_MS = 15 * 60 * 1000
const LOGIN_MAX_ATTEMPTS = 10

export async function POST(request: Request) {
  if (!allowRequest(`login:${clientKey(request)}`, LOGIN_MAX_ATTEMPTS, LOGIN_WINDOW_MS)) {
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
    const { email, password } = await request.json()
    if (typeof email !== 'string' || typeof password !== 'string') {
      return NextResponse.json({ error: 'Invalid request' }, { status: 400 })
    }

    // Check the password even when the email is wrong, so the response time does not reveal
    // which part failed.
    const emailMatches = email.trim().toLowerCase() === user.email.toLowerCase()
    const passwordMatches = await verifyPassword(password, user.passwordHash)
    if (!emailMatches || !passwordMatches) {
      return NextResponse.json({ error: 'Invalid credentials' }, { status: 401 })
    }

    const cookieStore = await cookies()

    // Two-factor users get a short-lived, single-use pre-auth token instead of a session.
    if (user.totpSecret) {
      const preAuthToken = await new jose.SignJWT({
        email: user.email,
        type: 'pre_auth',
      })
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

    const token = await new jose.SignJWT({
      email: user.email,
      merchantId: user.merchantId,
      name: user.name,
      twoFactorEnabled: false,
    })
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
        merchantId: user.merchantId,
        name: user.name,
        twoFactorEnabled: false,
      },
    })
  } catch {
    return NextResponse.json({ error: 'Invalid request' }, { status: 400 })
  }
}
