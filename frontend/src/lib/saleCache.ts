import type { QueryClient } from '@tanstack/react-query'

export const SALE_RELATED_KEYS = [
  'salesStats',
  'product',
  'products',
  'inventoryStats',
  'dashboardSummary',
  'dashboardTrend',
  'dashboardTopProducts',
  'dashboardLowStock',
] as const

export function invalidateSaleData(queryClient: QueryClient, saleId?: number) {
  const keys: (string | number)[][] = [['sales'], ...SALE_RELATED_KEYS.map(key => [key])]
  if (saleId !== undefined) {
    keys.push(['sale', saleId], ['saleItems', saleId])
  }
  keys.forEach(queryKey => {
    void queryClient.invalidateQueries({ queryKey })
  })
}
