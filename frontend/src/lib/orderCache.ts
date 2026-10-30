import type { QueryClient } from '@tanstack/react-query'

export const DASHBOARD_KEYS = [
  'dashboardSummary',
  'dashboardTrend',
  'dashboardTopProducts',
  'dashboardOrderStatus',
  'dashboardLowStock',
] as const

function invalidate(queryClient: QueryClient, keys: readonly string[]) {
  keys.forEach(key => {
    void queryClient.invalidateQueries({ queryKey: [key] })
  })
}

export function invalidateOrderListData(queryClient: QueryClient) {
  invalidate(queryClient, ['orders', ...DASHBOARD_KEYS])
}

export function invalidateOrderDetails(
  queryClient: QueryClient,
  orderId: number,
  becameReceived: boolean,
) {
  invalidate(queryClient, ['orders', ...DASHBOARD_KEYS])
  void queryClient.invalidateQueries({ queryKey: ['order', orderId] })

  if (becameReceived) {
    invalidate(queryClient, ['product', 'products', 'inventoryStats'])
  }
}

export function invalidateOrderItems(queryClient: QueryClient, orderId: number) {
  invalidate(queryClient, ['orders'])
  void queryClient.invalidateQueries({ queryKey: ['order', orderId] })
  void queryClient.invalidateQueries({ queryKey: ['orderItems', orderId] })
}
