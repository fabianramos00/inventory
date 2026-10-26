/* eslint-disable react-refresh/only-export-components -- context file exports both provider and hook by convention */
import { createContext, useContext, useState, useCallback, useEffect, type ReactNode } from 'react'
import type { User } from '@/types'
import { resetAuthRedirect, type AuthRefreshedDetail } from '@/lib/axios'

interface AuthContextValue {
  refreshToken: string | null
  user: User | null
  login: (token: string, user: User | null, refreshToken: string) => void
  logout: () => void
  isAuthenticated: boolean
}

const AuthContext = createContext<AuthContextValue | undefined>(undefined)

function readCachedUser(): User | null {
  const raw = localStorage.getItem('user')
  if (!raw) return null
  try {
    const cached = JSON.parse(raw) as Partial<User> | null
    if (!cached || typeof cached !== 'object') return null
    return {
      id: cached.id ?? 0,
      full_name: cached.full_name ?? '',
      email: cached.email ?? '',
      is_active: cached.is_active ?? true,
      is_superuser: cached.is_superuser ?? false,
    }
  } catch {
    return null
  }
}

function cacheUser(user: User | null) {
  if (!user) {
    localStorage.removeItem('user')
    return
  }
  localStorage.setItem('user', JSON.stringify(user))
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [token, setToken] = useState<string | null>(() => localStorage.getItem('token'))
  const [refreshToken, setRefreshToken] = useState<string | null>(() => localStorage.getItem('refresh_token'))
  const [user, setUser] = useState<User | null>(() => readCachedUser())

  useEffect(() => {
    function handleRefreshed(event: Event) {
      const { accessToken, refreshToken: newRefreshToken } =
        (event as CustomEvent<AuthRefreshedDetail>).detail
      setToken(accessToken)
      setRefreshToken(newRefreshToken)
    }
    window.addEventListener('auth:refreshed', handleRefreshed)
    return () => window.removeEventListener('auth:refreshed', handleRefreshed)
  }, [])

  const login = useCallback((newToken: string, newUser: User | null, newRefreshToken: string) => {
    resetAuthRedirect()
    localStorage.setItem('token', newToken)
    localStorage.setItem('refresh_token', newRefreshToken)
    cacheUser(newUser)
    setToken(newToken)
    setRefreshToken(newRefreshToken)
    setUser(newUser)
  }, [])

  const logout = useCallback(() => {
    localStorage.removeItem('token')
    localStorage.removeItem('refresh_token')
    localStorage.removeItem('user')
    setToken(null)
    setRefreshToken(null)
    setUser(null)
  }, [])

  return (
    <AuthContext.Provider value={{ refreshToken, user, login, logout, isAuthenticated: !!token }}>
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth() {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be used inside AuthProvider')
  return ctx
}
