import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { createElement, type ReactNode } from 'react'
import type { AxiosInstance, InternalAxiosRequestConfig } from 'axios'
import type { User } from '@/types'

type Response = { status: number; data?: unknown }

let axios: (typeof import('axios'))['default']
let api: AxiosInstance
let AuthProvider: (typeof import('@/context/AuthContext'))['AuthProvider']
let useAuth: (typeof import('@/context/AuthContext'))['useAuth']

function respond(handler: (config: InternalAxiosRequestConfig) => Response) {
  api.defaults.adapter = async (config) => {
    const { status, data } = handler(config)
    const response = { data: data ?? null, status, statusText: '', headers: {}, config }
    if (status >= 200 && status < 300) return response
    throw new axios.AxiosError('Request failed', 'ERR_BAD_REQUEST', config, undefined, response)
  }
}

function storage() {
  return {
    token: localStorage.getItem('token'),
    refreshToken: localStorage.getItem('refresh_token'),
    user: localStorage.getItem('user'),
  }
}

const user = { full_name: 'Ana', is_superuser: false } as User

beforeEach(async () => {
  vi.resetModules()
  localStorage.clear()
  axios = (await import('axios')).default
  const axiosModule = await import('@/lib/axios')
  api = axiosModule.default
  const context = await import('@/context/AuthContext')
  AuthProvider = context.AuthProvider
  useAuth = context.useAuth
  vi.spyOn(axios, 'post')
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('auth refresh lifecycle', () => {
  it('clears the session and emits auth:expired once when refresh fails', async () => {
    localStorage.setItem('token', 'old-token')
    localStorage.setItem('refresh_token', 'old-refresh')
    localStorage.setItem('user', JSON.stringify(user))
    const expired = vi.fn()
    window.addEventListener('auth:expired', expired)
    vi.mocked(axios.post).mockRejectedValue(new Error('refresh failed'))
    respond(() => ({ status: 401 }))

    await expect(api.get('/orders')).rejects.toThrow('refresh failed')
    expect(storage()).toEqual({ token: null, refreshToken: null, user: null })

    // ponytail: second 401 proves the guard stays armed for the rest of this expiry cycle
    await expect(api.get('/orders')).rejects.toBeTruthy()
    expect(expired).toHaveBeenCalledTimes(1)
    expect(axios.post).toHaveBeenCalledTimes(1)

    window.removeEventListener('auth:expired', expired)
  })

  it('re-arms the guard after a successful login so a later expiry emits again', async () => {
    const expired = vi.fn()
    window.addEventListener('auth:expired', expired)
    vi.mocked(axios.post).mockRejectedValue(new Error('refresh failed'))
    respond(() => ({ status: 401 }))

    const { result } = renderHook(() => useAuth(), {
      wrapper: ({ children }: { children: ReactNode }) => createElement(AuthProvider, null, children),
    })

    localStorage.setItem('refresh_token', 'old-refresh')
    await expect(api.get('/orders')).rejects.toBeTruthy()
    expect(expired).toHaveBeenCalledTimes(1)

    act(() => result.current.login('new-token', user, 'new-refresh'))

    await expect(api.get('/orders')).rejects.toBeTruthy()
    expect(expired).toHaveBeenCalledTimes(2)
    expect(storage()).toEqual({ token: null, refreshToken: null, user: null })

    window.removeEventListener('auth:expired', expired)
  })

  it('shares one refresh between concurrent 401s and retries both with the new token', async () => {
    localStorage.setItem('token', 'old-token')
    localStorage.setItem('refresh_token', 'old-refresh')
    const authorizations: string[] = []
    respond((config) => {
      const header = String(config.headers.Authorization)
      authorizations.push(header)
      return header === 'Bearer new-token' ? { status: 200, data: header } : { status: 401 }
    })

    let resolveRefresh!: (value: unknown) => void
    const refreshed = new Promise((resolve) => {
      resolveRefresh = resolve
    })
    vi.mocked(axios.post).mockReturnValue(refreshed as never)

    const first = api.get('/orders')
    const second = api.get('/clients')
    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(axios.post).toHaveBeenCalledTimes(1)

    resolveRefresh({ data: { access_token: 'new-token', refresh_token: 'new-refresh' } })
    const [firstResponse, secondResponse] = await Promise.all([first, second])

    expect(firstResponse.data).toBe('Bearer new-token')
    expect(secondResponse.data).toBe('Bearer new-token')
    expect(authorizations).toEqual([
      'Bearer old-token',
      'Bearer old-token',
      'Bearer new-token',
      'Bearer new-token',
    ])
  })

  it('stores the rotated access and refresh tokens from a successful refresh', async () => {
    const { setApiBaseUrl } = await import('@/lib/apiBase')
    setApiBaseUrl('http://127.0.0.1:9000/api/v1')
    localStorage.setItem('token', 'old-token')
    localStorage.setItem('refresh_token', 'old-refresh')
    respond((config) => {
      return String(config.headers.Authorization) === 'Bearer new-token'
        ? { status: 200, data: 'ok' }
        : { status: 401 }
    })
    vi.mocked(axios.post).mockResolvedValue({
      data: { access_token: 'new-token', refresh_token: 'new-refresh' },
    } as never)

    await expect(api.get('/orders')).resolves.toMatchObject({ data: 'ok' })

    expect(localStorage.getItem('token')).toBe('new-token')
    expect(localStorage.getItem('refresh_token')).toBe('new-refresh')
    expect(axios.post).toHaveBeenCalledWith(
      '/auth/refresh',
      { refresh_token: 'old-refresh' },
      expect.objectContaining({ baseURL: 'http://127.0.0.1:9000/api/v1' }),
    )
  })

  it('publishes rotated tokens so AuthContext state matches localStorage', async () => {
    localStorage.setItem('token', 'old-token')
    localStorage.setItem('refresh_token', 'old-refresh')
    respond((config) => {
      return String(config.headers.Authorization) === 'Bearer new-token'
        ? { status: 200, data: 'ok' }
        : { status: 401 }
    })
    vi.mocked(axios.post).mockResolvedValue({
      data: { access_token: 'new-token', refresh_token: 'new-refresh' },
    } as never)

    const { result } = renderHook(() => useAuth(), {
      wrapper: ({ children }: { children: ReactNode }) => createElement(AuthProvider, null, children),
    })
    expect(result.current.refreshToken).toBe('old-refresh')

    const refreshed = vi.fn()
    window.addEventListener('auth:refreshed', refreshed)

    await act(async () => {
      await api.get('/orders')
    })

    expect(refreshed).toHaveBeenCalledTimes(1)
    expect((refreshed.mock.calls[0][0] as CustomEvent).detail).toEqual({
      accessToken: 'new-token',
      refreshToken: 'new-refresh',
    })
    expect(result.current.refreshToken).toBe('new-refresh')
    expect(localStorage.getItem('refresh_token')).toBe('new-refresh')

    window.removeEventListener('auth:refreshed', refreshed)
  })

  it('does not refresh 401s from /auth/login or /auth/refresh', async () => {
    const expired = vi.fn()
    window.addEventListener('auth:expired', expired)
    respond(() => ({ status: 401 }))
    localStorage.setItem('refresh_token', 'old-refresh')

    await expect(api.post('/auth/login', {})).rejects.toBeTruthy()
    await expect(api.get('/auth/refresh')).rejects.toBeTruthy()

    expect(axios.post).not.toHaveBeenCalled()
    expect(localStorage.getItem('refresh_token')).toBe('old-refresh')
    expect(expired).not.toHaveBeenCalled()

    window.removeEventListener('auth:expired', expired)
  })
})
