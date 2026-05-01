// tpt-payments/frontend/app/api/auth/login/route.ts
import { NextResponse } from 'next/server'
import { cookies } from 'next/headers'
import * as jose from 'jose'
import { scrypt, timingSafeEqual } from 'crypto'
import { promisify } from 'util'

const scryptAsync = promisify(scrypt)

export async function POST(request: Request) {
  try {
    const { email, password } = await request.json()

    // In production: fetch user from database
    // Mock user for demonstration
    const mockUser = {
      id: 'merch_01h7nq3qz8k7x9v5b2n4m6l8',
      email: 'demo@tptpay.com',
      merchantId: 'merch_01h7nq3qz8k7x9v5b2n4m6l8',
      name: 'Demo Merchant',
      passwordHash: '3d828504576381c50b331d0e57b1262e8a99f012d5d8a7e3f2b1c0d9e8f7a6b5c4d3e2f1', // "password123"
      twoFactorEnabled: true,
    }

    // Verify password
    const passwordBuffer = Buffer.from(password)
    const hashBuffer = Buffer.from(mockUser.passwordHash, 'hex')
    const derivedKey = await scryptAsync(passwordBuffer, 'tpt-salt', 32) as Buffer

    if (!timingSafeEqual(derivedKey, hashBuffer)) {
      return NextResponse.json({ error: 'Invalid credentials' }, { status: 401 })
    }

    // Handle 2FA flow
    if (mockUser.twoFactorEnabled) {
      // Create short-lived pre-auth token for 2FA verification
      const secret = new TextEncoder().encode(process.env.JWT_SECRET || 'development-secret-key-change-in-production')
      const preAuthToken = await new jose.SignJWT({
        sub: mockUser.id,
        email: mockUser.email,
        type: 'pre_auth',
      })
        .setProtectedHeader({ alg: 'HS256' })
        .setIssuedAt()
        .setExpirationTime('5m')
        .sign(secret)

      const cookieStore = await cookies()
      cookieStore.set('pre_auth', preAuthToken, {
        httpOnly: true,
        secure: process.env.NODE_ENV === 'production',
        sameSite: 'strict',
        maxAge: 300, // 5 minutes
        path: '/',
      })

      return NextResponse.json({
        requiresTwoFactor: true,
      })
    }

    // Create full session token
    const secret = new TextEncoder().encode(process.env.JWT_SECRET || 'development-secret-key-change-in-production')
    const token = await new jose.SignJWT({
      sub: mockUser.id,
      email: mockUser.email,
      merchantId: mockUser.merchantId,
      name: mockUser.name,
      twoFactorEnabled: mockUser.twoFactorEnabled,
    })
      .setProtectedHeader({ alg: 'HS256' })
      .setIssuedAt()
      .setExpirationTime('24h')
      .sign(secret)

    const cookieStore = await cookies()
    cookieStore.set('session', token, {
      httpOnly: true,
      secure: process.env.NODE_ENV === 'production',
      sameSite: 'strict',
      maxAge: 86400, // 24 hours
      path: '/',
    })

    return NextResponse.json({
      user: {
        id: mockUser.id,
        email: mockUser.email,
        merchantId: mockUser.merchantId,
        name: mockUser.name,
        twoFactorEnabled: mockUser.twoFactorEnabled,
      },
    })
  } catch (error) {
    return NextResponse.json({ error: 'Invalid request' }, { status: 400 })
  }
}