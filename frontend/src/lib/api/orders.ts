import api from '@/lib/axios'
import type { Order, OrderItem, OrderStatus, OrderPaymentStatus, CursorPaginatedResponse, CreateOrderInput } from '@/types'

export interface OrdersParams {
  skip?: number
  limit?: number
  provider_id?: number
  payment_status?: string
  status?: string
  min_date?: string
  max_date?: string
}

export interface OrderItemsParams {
  skip?: number
  limit?: number
}

export const ordersApi = {
  getOrders: (params?: OrdersParams) =>
    api.get<CursorPaginatedResponse<Order>>('/supply-chain/purchase-orders', { params }),

  getOrder: (id: number) =>
    api.get<Order>(`/supply-chain/purchase-orders/${id}`),

  getOrderItems: (id: number, params?: OrderItemsParams) =>
    api.get<CursorPaginatedResponse<OrderItem>>(`/supply-chain/purchase-orders/${id}/items`, { params }),

  createOrder: (data: CreateOrderInput) =>
    api.post<Order>('/supply-chain/purchase-orders', data),

  updateOrder: (id: number, data: { provider_id?: number; status?: OrderStatus; payment_status?: OrderPaymentStatus }) =>
    api.put<Order>(`/supply-chain/purchase-orders/${id}`, data),

  addOrderItem: (orderId: number, data: { product_id: number; quantity: number; unit_cost: number; supplier_sku?: string }) =>
    api.post<Order>(`/supply-chain/purchase-orders/${orderId}/items`, data),

  updateOrderItem: (orderId: number, itemId: number, data: { quantity?: number; unit_cost?: number; supplier_sku?: string | null }) =>
    api.put<Order>(`/supply-chain/purchase-orders/${orderId}/items/${itemId}`, data),

  deleteOrderItem: (orderId: number, itemId: number) =>
    api.delete<Order>(`/supply-chain/purchase-orders/${orderId}/items/${itemId}`),

  deleteOrder: (id: number) =>
    api.delete(`/supply-chain/purchase-orders/${id}`),
}
