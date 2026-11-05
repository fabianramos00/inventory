import api from '@/lib/axios'
import type { CursorPaginatedResponse, Sale, SaleItem, SalePayment, CreateSaleInput, PaymentMethod } from '@/types'

export interface SalesParams {
  skip?: number
  limit?: number
  user_id?: number
  client_id?: number
  no_client?: boolean
  payment_method?: string
  delivery_status?: string
  payment_status?: string
  start_date?: string
  end_date?: string
}

export const salesApi = {
  getSales: (params?: SalesParams) =>
    api.get<CursorPaginatedResponse<Sale>>('/sales', { params }),

  getSale: (id: number) =>
    api.get<Sale>(`/sales/${id}`),

  getSaleItems: (id: number, params?: { skip?: number; limit?: number }) =>
    api.get<CursorPaginatedResponse<SaleItem>>(`/sales/${id}/items`, { params }),

  getStats: () =>
    api.get<{ unpaid_count: number; undelivered_count: number; total_amount_sum: number; amount_paid_sum: number }>('/sales/stats'),

  createSale: (data: CreateSaleInput) =>
    api.post<Sale>('/sales', data),

  deleteSale: (id: number) =>
    api.delete(`/sales/${id}`),

  updateSale: (id: number, data: { client_id: number | null }) =>
    api.put<Sale>(`/sales/${id}`, data),

  getSalePayments: (id: number, params?: { skip?: number; limit?: number }) =>
    api.get<CursorPaginatedResponse<SalePayment>>(`/sales/${id}/payments`, { params }),

  addSalePayment: (id: number, data: { amount: number; payment_method: Exclude<PaymentMethod, null> }) =>
    api.post<Sale>(`/sales/${id}/payments`, data),

  deleteSalePayment: (id: number, paymentId: number) =>
    api.delete<Sale>(`/sales/${id}/payments/${paymentId}`),

  addSaleItem: (saleId: number, data: { product_id: number; quantity: number; delivered_quantity: number; unit_price?: number | null }) =>
    api.post<Sale>(`/sales/${saleId}/items`, data),

  updateSaleItem: (saleId: number, itemId: number, data: { quantity?: number; delivered_quantity?: number; unit_price?: number | null }) =>
    api.put<Sale>(`/sales/${saleId}/items/${itemId}`, data),

  deleteSaleItem: (saleId: number, itemId: number) =>
    api.delete<Sale>(`/sales/${saleId}/items/${itemId}`),
}
