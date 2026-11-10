import { QueryClient } from '@tanstack/react-query'
import { describe, expect, it, vi } from 'vitest'
import { invalidateSaleData } from './saleCache'

describe('invalidateSaleData', () => {
  it('refreshes sales, clients, inventory, products and dashboard after a sale change', () => {
    const queryClient = new QueryClient()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    invalidateSaleData(queryClient)

    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['sales'] })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['salesStats'] })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['clients'] })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['client'] })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['products'] })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['inventoryStats'] })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['dashboardSummary'] })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['dashboardTrend'] })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['dashboardTopProducts'] })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['dashboardLowStock'] })
  })

  it('invalidates cached client list and detail queries even before they go stale', () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { staleTime: 5 * 60 * 1000 } },
    })
    queryClient.setQueryData(['clients'], [])
    queryClient.setQueryData(['client', 42], { id: 42 })

    invalidateSaleData(queryClient)

    expect(queryClient.getQueryState(['clients'])?.isInvalidated).toBe(true)
    expect(queryClient.getQueryState(['client', 42])?.isInvalidated).toBe(true)
  })

  it('also refreshes the sale, its items, and its payments after a payment or item mutation', () => {
    const queryClient = new QueryClient()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    invalidateSaleData(queryClient, 42)

    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['product'] })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['products'] })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['inventoryStats'] })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['sale', 42] })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['saleItems', 42] })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['salePayments', 42] })
  })
})
