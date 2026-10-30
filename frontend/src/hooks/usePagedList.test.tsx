import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { usePagedList } from './usePagedList'
import { clientsApi } from '@/lib/api/clients'

const mocks = vi.hoisted(() => ({
  getClients: vi.fn(),
  deleteClient: vi.fn(),
}))

vi.mock('@/lib/api/clients', () => ({
  clientsApi: {
    getClients: mocks.getClients,
    deleteClient: mocks.deleteClient,
  },
}))

function wrapperFor(queryClient: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  }
}

function createQueryClient() {
  return new QueryClient({ defaultOptions: { queries: { retry: false } } })
}

describe('usePagedList', () => {
  beforeEach(() => {
    mocks.getClients.mockReset()
    mocks.deleteClient.mockReset()
  })

  it('keeps the old skip/limit/search params and query key', async () => {
    mocks.getClients.mockResolvedValue({
      data: { items: [{ id: 1, name: 'Ana' }], has_next: true, skip: 10, limit: 10 },
    })
    const queryClient = createQueryClient()
    const { result } = renderHook(
      () => usePagedList({
        key: 'clients',
        page: 2,
        filters: { search: '  ana  ' },
        fetchPage: ({ skip, limit, search }) =>
          clientsApi.getClients({ skip, limit, ...(search.trim() && { search }) }).then(response => response.data),
        deleteItem: clientsApi.deleteClient,
      }),
      { wrapper: wrapperFor(queryClient) },
    )

    await waitFor(() => expect(result.current.isLoading).toBe(false))

    expect(mocks.getClients).toHaveBeenCalledWith({ skip: 10, limit: 10, search: '  ana  ' })
    expect(queryClient.getQueryCache().getAll().map(query => query.queryKey)).toContainEqual([
      'clients',
      { page: 2, search: '  ana  ', limit: 10 },
    ])
    expect(result.current.items).toEqual([{ id: 1, name: 'Ana' }])
    expect(result.current.hasNext).toBe(true)
    expect(typeof result.current.invalidate).toBe('function')
    expect(result.current.deleteMutation).toBeDefined()
  })

  it('does not send an empty/whitespace search param', async () => {
    mocks.getClients.mockResolvedValue({ data: { items: [], has_next: false } })
    const queryClient = createQueryClient()
    const { result } = renderHook(
      () => usePagedList({
        key: 'clients',
        page: 1,
        filters: { search: '   ' },
        fetchPage: ({ skip, limit, search }) =>
          clientsApi.getClients({ skip, limit, ...(search.trim() && { search }) }).then(response => response.data),
        deleteItem: clientsApi.deleteClient,
      }),
      { wrapper: wrapperFor(queryClient) },
    )

    await waitFor(() => expect(result.current.isLoading).toBe(false))

    expect(mocks.getClients).toHaveBeenCalledWith({ skip: 0, limit: 10 })
    expect(mocks.getClients.mock.calls[0][0]).not.toHaveProperty('search')
  })

  it('deletes and invalidates the list prefix', async () => {
    mocks.getClients.mockResolvedValue({ data: { items: [], has_next: false } })
    mocks.deleteClient.mockResolvedValue(undefined)
    const queryClient = createQueryClient()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')
    const { result } = renderHook(
      () => usePagedList({
        key: 'clients',
        page: 1,
        filters: { search: '' },
        fetchPage: ({ skip, limit, search }) =>
          clientsApi.getClients({ skip, limit, ...(search.trim() && { search }) }).then(response => response.data),
        deleteItem: clientsApi.deleteClient,
      }),
      { wrapper: wrapperFor(queryClient) },
    )

    await waitFor(() => expect(result.current.isLoading).toBe(false))

    await act(async () => {
      await result.current.deleteMutation.mutateAsync(7)
    })

    expect(mocks.deleteClient).toHaveBeenCalledWith(7)
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['clients'] })
  })
})
