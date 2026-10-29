import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useOrders } from './useOrders'

const mocks = vi.hoisted(() => ({
  getOrders: vi.fn(),
  deleteOrder: vi.fn(),
}))

vi.mock('@/lib/api/orders', () => ({
  ordersApi: {
    getOrders: mocks.getOrders,
    deleteOrder: mocks.deleteOrder,
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

describe('useOrders', () => {
  beforeEach(() => {
    mocks.getOrders.mockReset().mockResolvedValue({
      data: { items: [{ id: 1 }], has_next: true, skip: 0, limit: 10 },
    })
    mocks.deleteOrder.mockReset().mockResolvedValue(undefined)
  })

  it('sends only the contract params and reads has_next', async () => {
    const queryClient = createQueryClient()
    const { result } = renderHook(
      () => useOrders({ page: 2, filters: { provider_id: '', status: '', payment_status: '' } }),
      { wrapper: wrapperFor(queryClient) },
    )

    await waitFor(() => expect(result.current.isLoading).toBe(false))

    expect(mocks.getOrders).toHaveBeenCalledWith({ skip: 10, limit: 10 })
    expect(result.current.orders).toEqual([{ id: 1 }])
    expect(result.current.hasNext).toBe(true)
  })

  it('maps every order filter to the contract query parameters', async () => {
    const queryClient = createQueryClient()
    const { result } = renderHook(
      () => useOrders({
        page: 1,
        filters: {
          provider_id: 3,
          status: 'sent',
          payment_status: 'pending',
          min_date: '2026-10-24',
          max_date: '2026-10-25',
        },
      }),
      { wrapper: wrapperFor(queryClient) },
    )

    await waitFor(() => expect(result.current.isLoading).toBe(false))

    expect(mocks.getOrders).toHaveBeenCalledWith({
      skip: 0,
      limit: 10,
      provider_id: 3,
      status: 'sent',
      payment_status: 'pending',
      min_date: '2026-10-24T05:00:00.000Z',
      max_date: '2026-10-26T04:59:59.999Z',
    })
  })

  it('invalidates orders and every dashboard key after deleting', async () => {
    const queryClient = createQueryClient()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')
    const { result } = renderHook(
      () => useOrders({ page: 1, filters: {} }),
      { wrapper: wrapperFor(queryClient) },
    )

    await waitFor(() => expect(result.current.isLoading).toBe(false))

    await act(async () => {
      await result.current.deleteMutation.mutateAsync(7)
    })

    expect(mocks.deleteOrder).toHaveBeenCalledWith(7)
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['orders'] })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['dashboardSummary'] })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['dashboardTrend'] })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['dashboardTopProducts'] })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['dashboardOrderStatus'] })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['dashboardLowStock'] })
  })
})
