import { useQuery } from '@tanstack/react-query'
import { ordersApi } from '@/lib/api/orders'
import type { Order, OrderItem } from '@/types'

export function useOrder(id: string | undefined) {
  const orderId = Number(id)

  const orderQuery = useQuery<Order>({
    queryKey: ['order', orderId],
    queryFn: () => ordersApi.getOrder(orderId).then(res => res.data),
    enabled: !!id,
  })

  const itemsQuery = useQuery<OrderItem[]>({
    queryKey: ['orderItems', orderId],
    queryFn: () =>
      ordersApi
        .getOrderItems(orderId, { skip: 0, limit: 100 })
        .then(res => res.data.items),
    enabled: !!id,
  })

  return {
    order: orderQuery.data ?? null,
    orderItems: itemsQuery.data ?? [],
    isLoading: orderQuery.isLoading || itemsQuery.isLoading,
    error: orderQuery.error ?? itemsQuery.error,
  }
}
