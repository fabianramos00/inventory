import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { bogotaDayBounds, useSales } from './useSales'

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
      start_date: '2026-01-01T05:00:00.000Z',
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

  it('maps every sales filter to the contract query parameters', async () => {
    const queryClient = createQueryClient()
    const filters = {
      user_id: 7,
      client_id: 9,
      no_client: false,
      payment_method: 'cash',
      delivery_status: 'partial',
      payment_status: 'paid',
      start_date: '2026-01-01',
      end_date: '2026-01-02',
    }
    const { result } = renderHook(
      () => useSales({ page: 1, filters }),
      { wrapper: wrapperFor(queryClient) },
    )

    await waitFor(() => expect(result.current.isLoading).toBe(false))

    expect(mocks.getSales).toHaveBeenCalledWith({
      skip: 0,
      limit: 10,
      user_id: 7,
      client_id: 9,
      payment_method: 'cash',
      delivery_status: 'partial',
      payment_status: 'paid',
      start_date: '2026-01-01T05:00:00.000Z',
      end_date: '2026-01-03T04:59:59.999Z',
    })
  })

  it('maps the stats snake_case response to the UI model', async () => {
    mocks.getStats.mockResolvedValue({
      data: {
        unpaid_count: 2,
        undelivered_count: 3,
        total_amount_sum: 30.5,
        amount_paid_sum: 10.25,
      },
    })
    const queryClient = createQueryClient()
    const { result } = renderHook(
      () => useSales({ page: 1, filters: {} }),
      { wrapper: wrapperFor(queryClient) },
    )

    await waitFor(() => expect(result.current.stats.totalAmountSum).toBe(30.5))

    expect(result.current.stats).toEqual({
      unpaidCount: 2,
      undeliveredCount: 3,
      totalAmountSum: 30.5,
      amountPaidSum: 10.25,
    })
  })

  it('sends date filters as Bogota day bounds the Go API accepts', async () => {
    const queryClient = createQueryClient()
    const { result } = renderHook(
      () => useSales({
        page: 1,
        filters: { start_date: '2026-10-24', end_date: '2026-10-25' },
      }),
      { wrapper: wrapperFor(queryClient) },
    )

    await waitFor(() => expect(result.current.isLoading).toBe(false))

    const params = mocks.getSales.mock.calls[0][0] as { start_date?: string; end_date?: string }
    expect(params.start_date).toBe('2026-10-24T05:00:00.000Z')
    expect(params.end_date).toBe('2026-10-26T04:59:59.999Z')
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
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['products'] })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['inventoryStats'] })
  })
})

describe('bogotaDayBounds', () => {
  it('uses Bogota start of day and inclusive end of day', () => {
    expect(bogotaDayBounds('2026-10-24', '2026-10-25')).toEqual({
      start_date: '2026-10-24T05:00:00.000Z',
      end_date: '2026-10-26T04:59:59.999Z',
    })
  })

  it('leaves missing bounds undefined', () => {
    expect(bogotaDayBounds()).toEqual({ start_date: undefined, end_date: undefined })
    expect(bogotaDayBounds('2026-10-24')).toEqual({
      start_date: '2026-10-24T05:00:00.000Z',
      end_date: undefined,
    })
  })
})
