import { ordersApi, type OrdersParams } from '@/lib/api/orders'
import { bogotaDayBounds } from '@/lib/dateBounds'
import { DASHBOARD_KEYS } from '@/lib/orderCache'
import type { Order } from '@/types'
import { usePagedList } from './usePagedList'

export interface OrderFilters {
  provider_id?: number | ''
  payment_status?: string
  status?: string
  min_date?: string
  max_date?: string
}

interface UseOrdersParams {
  page: number
  filters: OrderFilters
}

function buildOrderParams({ skip, limit, provider_id, payment_status, status, min_date, max_date }: { skip: number; limit: number } & OrderFilters): OrdersParams {
  const params: OrdersParams = { skip, limit }
  if (provider_id) params.provider_id = provider_id
  if (payment_status) params.payment_status = payment_status
  if (status) params.status = status
  const bounds = bogotaDayBounds(min_date, max_date)
  if (bounds.start_date) params.min_date = bounds.start_date
  if (bounds.end_date) params.max_date = bounds.end_date
  return params
}

export function useOrders({ page, filters }: UseOrdersParams) {
  const { items, ...rest } = usePagedList<Order, OrderFilters>({
    key: 'orders',
    page,
    filters,
    fetchPage: params => ordersApi.getOrders(buildOrderParams(params)).then(response => response.data),
    deleteItem: ordersApi.deleteOrder,
    invalidateKeys: [...DASHBOARD_KEYS],
  })

  return { ...rest, orders: items }
}
