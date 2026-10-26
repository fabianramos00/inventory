import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { bootstrap, resolveApiBaseUrl } from './bootstrap'
import { DEFAULT_API_BASE_URL, getApiBaseUrl, setApiBaseUrl } from './lib/apiBase'

const validUrl = 'http://127.0.0.1:4321/api/v1'

function stubBinding(impl: () => Promise<string>) {
  window.go = { main: { App: { GetAPIBaseURL: impl } } }
}

beforeEach(() => {
  setApiBaseUrl('http://reset.invalid')
})

afterEach(() => {
  delete window.go
  vi.unstubAllEnvs()
})

describe('resolveApiBaseUrl', () => {
  it('uses a valid Wails loopback binding in production', async () => {
    stubBinding(async () => validUrl)

    await expect(resolveApiBaseUrl({ isProduction: true })).resolves.toBe(validUrl)
  })

  it('accepts localhost as a loopback host', async () => {
    stubBinding(async () => 'http://localhost:4321/api/v1')

    await expect(resolveApiBaseUrl({ isProduction: true })).resolves.toBe(
      'http://localhost:4321/api/v1',
    )
  })

  it.each([
    'https://127.0.0.1:4321/api/v1',
    'http://evil.example:4321/api/v1',
    'http://127.0.0.1.evil.example:4321/api/v1',
    'http://127.0.0.1:4321/api/v1/extra',
    'http://127.0.0.1:4321/api/v1/',
    'http://127.0.0.1:4321',
    'http://127.0.0.1/api/v1',
    'http://127.0.0.1:4321/api/v1?query=1',
    'http://127.0.0.1:4321/api/v1#hash',
    'http://user:pass@127.0.0.1:4321/api/v1',
    'http://[::1]:4321/api/v1',
    'not-a-url',
    '',
  ])('rejects an invalid binding URL in production: %s', async (url) => {
    stubBinding(async () => url)

    await expect(resolveApiBaseUrl({ isProduction: true })).rejects.toThrow(/invalid URL/)
  })

  it('throws in production when the binding is missing', async () => {
    await expect(resolveApiBaseUrl({ isProduction: true })).rejects.toThrow(/unavailable/)
  })

  it('throws in production when the binding fails', async () => {
    stubBinding(async () => {
      throw new Error('runtime unavailable')
    })

    await expect(resolveApiBaseUrl({ isProduction: true })).rejects.toThrow(/binding failed/)
  })

  it('uses VITE_API_URL in development when the binding is missing', async () => {
    vi.stubEnv('VITE_API_URL', 'http://127.0.0.1:5555/api/v1')

    await expect(resolveApiBaseUrl({ isProduction: false })).resolves.toBe(
      'http://127.0.0.1:5555/api/v1',
    )
  })

  it('uses the default development URL when no env URL exists', async () => {
    vi.stubEnv('VITE_API_URL', '')

    await expect(resolveApiBaseUrl({ isProduction: false })).resolves.toBe(DEFAULT_API_BASE_URL)
  })

  it('falls back in development when the binding returns an invalid URL', async () => {
    stubBinding(async () => 'https://evil.example/api/v1')
    vi.stubEnv('VITE_API_URL', '')

    await expect(resolveApiBaseUrl({ isProduction: false })).resolves.toBe(DEFAULT_API_BASE_URL)
  })
})

describe('bootstrap', () => {
  it('sets the API base URL from the Wails binding', async () => {
    stubBinding(async () => validUrl)

    await bootstrap()

    expect(getApiBaseUrl()).toBe(validUrl)
  })
})
