import axios, { AxiosError } from 'axios'
import { getApiBaseUrl } from '@/lib/apiBase'

type QueueItem = { resolve: (value: string) => void; reject: (reason: unknown) => void }
type RefreshResponse = { access_token: string; refresh_token: string }
export type AuthRefreshedDetail = { accessToken: string; refreshToken: string }

let isRedirecting = false
let isRefreshing = false
let failedQueue: QueueItem[] = []

function processQueue(error: unknown, token: string | null) {
  failedQueue.forEach((promise) => {
    if (error) {
      promise.reject(error)
    } else {
      promise.resolve(token!)
    }
  })
  failedQueue = []
}

async function attemptTokenRefresh(refreshToken: string): Promise<RefreshResponse> {
  const res = await axios.post<RefreshResponse>(
    '/auth/refresh',
    { refresh_token: refreshToken },
    {
      baseURL: getApiBaseUrl(),
      headers: { 'Content-Type': 'application/json' },
    },
  )
  return res.data
}

export function resetAuthRedirect() {
  isRedirecting = false
}

function redirectToLogin() {
  if (isRedirecting) return
  isRedirecting = true
  window.dispatchEvent(new CustomEvent('auth:expired'))
}

function enqueueRequest(config: AxiosError['config']) {
  return new Promise<string>((resolve, reject) => {
    failedQueue.push({ resolve, reject })
  }).then((token) => {
    if (config) {
      config.headers.Authorization = `Bearer ${token}`
      return api(config)
    }
    return Promise.reject(new Error('Request config missing'))
  })
}

const api = axios.create({
  headers: {
    'Content-Type': 'application/json',
  },
  withCredentials: false,
})

api.interceptors.request.use(
  (config) => {
    config.baseURL = getApiBaseUrl()
    const token = localStorage.getItem('token')
    if (token) {
      config.headers.Authorization = `Bearer ${token}`
    }
    return config
  },
  (error) => Promise.reject(error),
)

api.interceptors.response.use(
  (response) => response,
  async (error: AxiosError) => {
    if (error.response?.status !== 401) {
      if (import.meta.env.DEV) {
        console.error('API Error:', {
          status: error.response?.status,
          message: error.message,
          data: error.response?.data,
        })
      }
      return Promise.reject(error)
    }

    const requestUrl = error.config?.url ?? ''

    if (requestUrl.includes('/auth/login') || requestUrl.includes('/auth/refresh')) {
      return Promise.reject(error)
    }

    if (isRefreshing) {
      return enqueueRequest(error.config)
    }

    isRefreshing = true
    const storedRefreshToken = localStorage.getItem('refresh_token')

    if (!storedRefreshToken) {
      processQueue(new Error('No refresh token'), null)
      isRefreshing = false
      redirectToLogin()
      return Promise.reject(error)
    }

    try {
      const tokens = await attemptTokenRefresh(storedRefreshToken)
      localStorage.setItem('token', tokens.access_token)
      localStorage.setItem('refresh_token', tokens.refresh_token)
      window.dispatchEvent(new CustomEvent<AuthRefreshedDetail>('auth:refreshed', {
        detail: { accessToken: tokens.access_token, refreshToken: tokens.refresh_token },
      }))
      processQueue(null, tokens.access_token)
      if (error.config) {
        error.config.headers.Authorization = `Bearer ${tokens.access_token}`
        return api(error.config)
      }
      return Promise.reject(error)
    } catch (refreshError) {
      processQueue(refreshError, null)
      localStorage.removeItem('token')
      localStorage.removeItem('refresh_token')
      localStorage.removeItem('user')
      redirectToLogin()
      return Promise.reject(refreshError)
    } finally {
      isRefreshing = false
    }
  },
)

export default api
