import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useSales } from './useSales'

const mocks = vi.hoisted(() => ({
  getSales: vi.fn(),
  getStats: vi.fn(),
  deleteSale: vi.fn(),
}))

vi.mock('@/lib/api/sales', () => ({
  salesApi: {
    getSales: mocks.getSales,
    getStats: mocks.getStats,
    deleteSale: mocks.deleteSale,
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

const emptyStats = { unpaid_count: 0, undelivered_count: 0, total_amount_sum: 0, amount_paid_sum: 0 }

describe('useSales', () => {
  beforeEach(() => {
    mocks.getSales.mockReset().mockResolvedValue({ data: { items: [{ id: 1 }], has_next: true } })
    mocks.getStats.mockReset().mockResolvedValue({ data: emptyStats })
    mocks.deleteSale.mockReset().mockResolvedValue(undefined)
  })

  it('keeps the old skip/limit params and full filter query key', async () => {
    const queryClient = createQueryClient()
    const filters = {
      user_id: '' as const,
      client_id: 3,
      no_client: false,
      payment_method: '',
      delivery_status: '',
      payment_status: '',
      start_date: '2026-01-01',
      end_date: '',
    }
    const { result } = renderHook(
      () => useSales({ page: 2, filters }),
      { wrapper: wrapperFor(queryClient) },
    )

    await waitFor(() => expect(result.current.isLoading).toBe(false))

    expect(mocks.getSales).toHaveBeenCalledWith({
      skip: 10,
      limit: 10,
      client_id: 3,
      start_date: '2026-01-01',
    })
    expect(queryClient.getQueryCache().getAll().map(query => query.queryKey)).toContainEqual([
      'sales',
      { page: 2, ...filters, limit: 10 },
    ])
    expect(result.current.sales).toEqual([{ id: 1 }])
    expect(result.current.hasNext).toBe(true)
  })

  it('sends no_client instead of client_id when the user filters for sales without a client', async () => {
    const queryClient = createQueryClient()
    const { result } = renderHook(
      () => useSales({
        page: 1,
        filters: { client_id: 3, no_client: true },
      }),
      { wrapper: wrapperFor(queryClient) },
    )

    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(mocks.getSales).toHaveBeenCalledWith({ skip: 0, limit: 10, no_client: true })
  })

  it('invalidates both the sales list and stats after deleting', async () => {
    const queryClient = createQueryClient()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')
    const { result } = renderHook(
      () => useSales({ page: 1, filters: {} }),
      { wrapper: wrapperFor(queryClient) },
    )

    await waitFor(() => expect(result.current.isLoading).toBe(false))

    await act(async () => {
      await result.current.deleteMutation.mutateAsync(9)
    })

    expect(mocks.deleteSale).toHaveBeenCalledWith(9)
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['sales'] })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['salesStats'] })
  })
})
