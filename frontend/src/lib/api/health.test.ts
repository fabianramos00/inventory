import { afterEach, describe, expect, it, vi } from 'vitest'
import axios from 'axios'
import { DEFAULT_API_BASE_URL, setApiBaseUrl } from '@/lib/apiBase'
import { fetchHealth } from '@/lib/api/health'

afterEach(() => {
  vi.restoreAllMocks()
  setApiBaseUrl(DEFAULT_API_BASE_URL)
})

describe('fetchHealth', () => {
  it('requests /healthz from the API root', async () => {
    setApiBaseUrl('http://127.0.0.1:4321/api/v1')
    const get = vi.spyOn(axios, 'get').mockResolvedValue({
      data: { status: 'ok', database: 'sqlite' },
    })

    await expect(fetchHealth()).resolves.toEqual({ status: 'ok', database: 'sqlite' })
    expect(get).toHaveBeenCalledWith('http://127.0.0.1:4321/healthz')
  })
})
