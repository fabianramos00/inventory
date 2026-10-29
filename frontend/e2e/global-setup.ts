import { request } from '@playwright/test'
import { mkdir, writeFile } from 'node:fs/promises'
import path from 'node:path'
import { ADMIN, API_ORIGIN, APP_URL, AUTH_FILE } from './helpers'

export default async function globalSetup() {
  const context = await request.newContext({ baseURL: API_ORIGIN })
  try {
    const response = await context.post('/api/v1/auth/login', {
      form: { username: ADMIN.email, password: ADMIN.password },
    })
    if (!response.ok()) {
      throw new Error(`Global setup login failed with ${response.status()}: ${await response.text()}`)
    }
    const auth = await response.json() as {
      access_token: string
      refresh_token: string
      user: unknown
    }

    await mkdir(path.dirname(AUTH_FILE), { recursive: true })
    await writeFile(AUTH_FILE, JSON.stringify({
      cookies: [],
      origins: [{
        origin: APP_URL,
        localStorage: [
          { name: 'token', value: auth.access_token },
          { name: 'refresh_token', value: auth.refresh_token },
          { name: 'user', value: JSON.stringify(auth.user) },
        ],
      }],
    }))
  } finally {
    await context.dispose()
  }
}
