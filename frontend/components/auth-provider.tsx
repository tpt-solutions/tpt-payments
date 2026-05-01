// tpt-payments/frontend/components/auth-provider.tsx
"use client"

import * as React from "react"
import { useRouter, usePathname } from "next/navigation"
import { createContext, useContext, useEffect, useState } from "react"

interface User {
  id: string
  email: string
  merchantId: string
  name: string
  twoFactorEnabled: boolean
}

interface AuthContextType {
  user: User | null
  loading: boolean
  login: (email: string, password: string) => Promise<void>
  logout: () => Promise<void>
  verifyTwoFactor: (token: string) => Promise<void>
}

const AuthContext = createContext<AuthContextType | undefined>(undefined)

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [user, setUser] = useState<User | null>(null)
  const [loading, setLoading] = useState(true)
  const router = useRouter()
  const pathname = usePathname()

  useEffect(() => {
    const checkAuth = async () => {
      try {
        const res = await fetch("/api/auth/me", {
          credentials: "include",
        })
        if (res.ok) {
          const userData = await res.json()
          setUser(userData)
        }
      } catch (error) {
        // Not authenticated
      } finally {
        setLoading(false)
      }
    }

    checkAuth()
  }, [])

  useEffect(() => {
    if (loading) return

    const publicPaths = ["/auth/login", "/auth/2fa"]
    const isPublicPath = publicPaths.includes(pathname)

    if (!user && !isPublicPath) {
      router.push("/auth/login")
    } else if (user && isPublicPath) {
      router.push("/dashboard")
    }
  }, [user, loading, pathname, router])

  const login = async (email: string, password: string) => {
    const res = await fetch("/api/auth/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "include",
      body: JSON.stringify({ email, password }),
    })

    if (!res.ok) {
      throw new Error("Invalid credentials")
    }

    const data = await res.json()

    if (data.requiresTwoFactor) {
      router.push("/auth/2fa")
      return
    }

    setUser(data.user)
    router.push("/dashboard")
  }

  const verifyTwoFactor = async (token: string) => {
    const res = await fetch("/api/auth/2fa/verify", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "include",
      body: JSON.stringify({ token }),
    })

    if (!res.ok) {
      throw new Error("Invalid 2FA token")
    }

    const data = await res.json()
    setUser(data.user)
    router.push("/dashboard")
  }

  const logout = async () => {
    await fetch("/api/auth/logout", {
      method: "POST",
      credentials: "include",
    })
    setUser(null)
    router.push("/auth/login")
  }

  return (
    <AuthContext.Provider value={{ user, loading, login, logout, verifyTwoFactor }}>
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth() {
  const context = useContext(AuthContext)
  if (context === undefined) {
    throw new Error("useAuth must be used within an AuthProvider")
  }
  return context
}