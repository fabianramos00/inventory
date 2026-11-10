import { afterEach, describe, expect, it, vi } from 'vitest'
import api from '@/lib/axios'
import { usersApi } from './users'

afterEach(() => {
  vi.restoreAllMocks()
})

describe('usersApi', () => {
  it('fetches filter options from /users/options', async () => {
    const get = vi.spyOn(api, 'get').mockResolvedValue({
      data: { items: [{ id: 1, full_name: 'Ana Pérez' }], has_next: false, skip: 0, limit: 20 },
    })

    await expect(usersApi.getUserOptions({ search: 'ana', skip: 0, limit: 20 })).resolves.toMatchObject({
      data: { items: [{ id: 1, full_name: 'Ana Pérez' }] },
    })
    expect(get).toHaveBeenCalledWith('/users/options', { params: { search: 'ana', skip: 0, limit: 20 } })
  })

  it('keeps the admin list on /users', async () => {
    const get = vi.spyOn(api, 'get').mockResolvedValue({
      data: { items: [], has_next: false, skip: 0, limit: 20 },
    })

    await usersApi.getUsers({ skip: 0, limit: 20 })
    expect(get).toHaveBeenCalledWith('/users', { params: { skip: 0, limit: 20 } })
  })
})
