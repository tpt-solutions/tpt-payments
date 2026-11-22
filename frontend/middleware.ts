// Redirects requests for merchant pages to login unless they carry a valid session.
// The signature is verified here, not just the cookie's presence, so a forged or expired
// cookie never reaches a page.
import { NextResponse, type NextRequest } from 'next/server'
import * as jose from 'jose'

async function hasValidSession(request: NextRequest): Promise<boolean> {
  const token = request.cookies.get('session')?.value
  const secret = process.env.JWT_SECRET
  if (!token || !secret || secret.length < 32) return false
  try {
    const { payload } = await jose.jwtVerify(token, new TextEncoder().encode(secret))
    return payload.type !== 'pre_auth'
  } catch {
    return false
  }
}

export async function middleware(request: NextRequest) {
  if (await hasValidSession(request)) {
    return NextResponse.next()
  }
  const url = request.nextUrl.clone()
  url.pathname = '/auth/login'
  url.search = `?next=${encodeURIComponent(request.nextUrl.pathname)}`
  return NextResponse.redirect(url)
}

export const config = {
  matcher: [
    '/dashboard/:path*',
    '/api-keys/:path*',
    '/transactions/:path*',
    '/webhooks/:path*',
    '/settings/:path*',
  ],
}
