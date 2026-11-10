import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { describe, expect, it, vi } from 'vitest'
import { useClient } from './useClient'
import type { Client } from '@/types'

const mocks = vi.hoisted(() => ({
  getClient: vi.fn(),
  addClientPayment: vi.fn(),
}))

vi.mock('@/lib/api/clients', () => ({
  clientsApi: {
    getClient: mocks.getClient,
    addClientPayment: mocks.addClientPayment,
  },
}))

function client(): Client {
  return {
    id: 12,
    name: 'Ana Pérez',
    identity_card: '1020304050',
    email: 'ana@example.com',
    phone: '3001234567',
    debt_amount: 150,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: null,
  }
}

describe('useClient', () => {
  it('invalidates the client, sale, stats and dashboard keys after a payment', async () => {
    mocks.getClient.mockReset()
      .mockResolvedValueOnce({ data: client() })
      .mockResolvedValue({ data: { ...client(), debt_amount: 100 } })
    mocks.addClientPayment.mockReset().mockResolvedValue({
      data: {
        client_id: 12,
        amount: 50,
        remaining_debt: 100,
        allocations: [{ sale_id: 41, amount: 50 }],
      },
    })

    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')
    const setQueryDataSpy = vi.spyOn(queryClient, 'setQueryData')
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )

    const { result } = renderHook(() => useClient('12'), { wrapper })
    await waitFor(() => expect(result.current.client?.debt_amount).toBe(150))

    await act(async () => {
      await result.current.paymentMutation.mutateAsync({ amount: 50, payment_method: 'cash' })
    })

    expect(mocks.addClientPayment).toHaveBeenCalledWith(12, { amount: 50, payment_method: 'cash' })
    expect(setQueryDataSpy).toHaveBeenCalledWith(['client', 12], expect.any(Function))
    await waitFor(() => {
      expect(queryClient.getQueryData<Client>(['client', 12])?.debt_amount).toBe(100)
    })

    for (const queryKey of [
      ['clients'],
      ['sales'],
      ['salesStats'],
      ['dashboardSummary'],
      ['dashboardTrend'],
      ['dashboardTopProducts'],
      ['dashboardLowStock'],
      ['client', 12],
      ['sale', 41],
      ['saleItems', 41],
      ['salePayments', 41],
    ]) {
      expect(invalidateSpy).toHaveBeenCalledWith({ queryKey })
    }
  })
})
