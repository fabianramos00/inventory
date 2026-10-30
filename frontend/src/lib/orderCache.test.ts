import { QueryClient } from '@tanstack/react-query'
import { describe, expect, it, vi } from 'vitest'
import {
  DASHBOARD_KEYS,
  invalidateOrderDetails,
  invalidateOrderItems,
  invalidateOrderListData,
} from './orderCache'

function setup() {
  const queryClient = new QueryClient()
  const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')
  return { queryClient, invalidateSpy }
}

function expectKeys(invalidateSpy: ReturnType<typeof setup>['invalidateSpy'], keys: string[]) {
  keys.forEach(key => {
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: [key] })
  })
}

describe('order cache invalidation', () => {
  it('invalidates orders and dashboard keys after create or delete', () => {
    const { queryClient, invalidateSpy } = setup()

    invalidateOrderListData(queryClient)

    expectKeys(invalidateSpy, ['orders', ...DASHBOARD_KEYS])
  })

  it('invalidates order details plus dashboard keys on a normal update', () => {
    const { queryClient, invalidateSpy } = setup()

    invalidateOrderDetails(queryClient, 4, false)

    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['order', 4] })
    expectKeys(invalidateSpy, ['orders', ...DASHBOARD_KEYS])
    expect(invalidateSpy).not.toHaveBeenCalledWith({ queryKey: ['products'] })
    expect(invalidateSpy).not.toHaveBeenCalledWith({ queryKey: ['product'] })
    expect(invalidateSpy).not.toHaveBeenCalledWith({ queryKey: ['inventoryStats'] })
  })

  it('refreshes stock when the order becomes received', () => {
    const { queryClient, invalidateSpy } = setup()

    invalidateOrderDetails(queryClient, 4, true)

    expectKeys(invalidateSpy, ['orders', 'product', 'products', 'inventoryStats', ...DASHBOARD_KEYS])
  })

  it('invalidates order and item caches after an item mutation without touching stock', () => {
    const { queryClient, invalidateSpy } = setup()

    invalidateOrderItems(queryClient, 4)

    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['order', 4] })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['orderItems', 4] })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['orders'] })
    expect(invalidateSpy).not.toHaveBeenCalledWith({ queryKey: ['products'] })
    expect(invalidateSpy).not.toHaveBeenCalledWith({ queryKey: ['inventoryStats'] })
  })
})
