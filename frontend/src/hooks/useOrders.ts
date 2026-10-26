import { ordersApi, type OrdersParams } from '@/lib/api/orders'
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
  if (min_date) params.min_date = min_date
  if (max_date) params.max_date = max_date
  return params
}

export function useOrders({ page, filters }: UseOrdersParams) {
  const { items, ...rest } = usePagedList<Order, OrderFilters>({
    key: 'orders',
    page,
    filters,
    fetchPage: params => ordersApi.getOrders(buildOrderParams(params)).then(response => response.data),
    deleteItem: ordersApi.deleteOrder,
  })

  return { ...rest, orders: items }
}
