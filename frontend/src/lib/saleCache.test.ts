import { QueryClient } from '@tanstack/react-query'
import { describe, expect, it, vi } from 'vitest'
import { invalidateSaleData } from './saleCache'

describe('invalidateSaleData', () => {
  it('refreshes sales, inventory, products and dashboard after a sale change', () => {
    const queryClient = new QueryClient()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    invalidateSaleData(queryClient)

    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['sales'] })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['salesStats'] })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['products'] })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['inventoryStats'] })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['dashboardSummary'] })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['dashboardTrend'] })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['dashboardTopProducts'] })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['dashboardLowStock'] })
  })

  it('also refreshes the sale and its items after an item mutation', () => {
    const queryClient = new QueryClient()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    invalidateSaleData(queryClient, 42)

    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['product'] })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['products'] })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['inventoryStats'] })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['sale', 42] })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['saleItems', 42] })
  })
})
