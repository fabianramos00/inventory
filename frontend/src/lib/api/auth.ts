import api from '@/lib/axios'
import type { AuthResponse, LoginPayload } from '@/types'

export const authApi = {
  login: (payload: LoginPayload) => {
    const formData = new URLSearchParams()
    formData.append('username', payload.email)
    formData.append('password', payload.password)
    return api.post<AuthResponse>('/auth/login', formData, {
      headers: {
        'Content-Type': 'application/x-www-form-urlencoded',
      },
    })
  },

  logout: (refreshToken: string | null) => api.post('/auth/logout', { refresh_token: refreshToken ?? '' }),
}
