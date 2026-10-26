import { DEFAULT_API_BASE_URL, setApiBaseUrl } from '@/lib/apiBase'

declare global {
  interface Window {
    go?: {
      main?: {
        App?: {
          GetAPIBaseURL?: () => Promise<string>
        }
      }
    }
  }
}

const LOOPBACK_HOSTS = new Set(['127.0.0.1', 'localhost'])
const API_PATH = '/api/v1'

function isLoopbackApiBaseUrl(value: string): boolean {
  let url: URL
  try {
    url = new URL(value)
  } catch {
    return false
  }

  return (
    url.protocol === 'http:' &&
    LOOPBACK_HOSTS.has(url.hostname) &&
    url.port !== '' &&
    url.pathname === API_PATH &&
    url.search === '' &&
    url.hash === '' &&
    url.username === '' &&
    url.password === ''
  )
}

function devFallback(): string {
  return import.meta.env.VITE_API_URL || DEFAULT_API_BASE_URL
}

export async function resolveApiBaseUrl(
  options: { isProduction?: boolean } = {},
): Promise<string> {
  const isProduction = options.isProduction ?? !import.meta.env.DEV
  const binding = window.go?.main?.App?.GetAPIBaseURL

  if (typeof binding === 'function') {
    let value: string
    try {
      value = await binding()
    } catch {
      if (isProduction) throw new Error('API base URL binding failed')
      return devFallback()
    }

    if (isLoopbackApiBaseUrl(value)) return value
    if (isProduction) throw new Error('API base URL binding returned an invalid URL')
    return devFallback()
  }

  if (isProduction) throw new Error('API base URL binding is unavailable')
  return devFallback()
}

export async function bootstrap() {
  setApiBaseUrl(await resolveApiBaseUrl())
}
