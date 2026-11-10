import api from '@/lib/axios'
import type { CursorPaginatedResponse, Client, ClientPaymentResult, PaymentMethod } from '@/types'

export interface ClientsParams {
  search?: string
  skip?: number
  limit?: number
}

export const clientsApi = {
  getClients: (params?: ClientsParams) =>
    api.get<CursorPaginatedResponse<Client>>('/sales/clients', { params }),

  getClient: (id: number) =>
    api.get<Client>(`/sales/clients/${id}`),

  addClientPayment: (id: number, data: { amount: number; payment_method: Exclude<PaymentMethod, null> }) =>
    api.post<ClientPaymentResult>(`/sales/clients/${id}/payments`, data),

  createClient: (data: { name: string; identity_card?: string; email?: string; phone?: string }) =>
    api.post<Client>('/sales/clients', data),

  updateClient: (id: number, data: { name?: string; identity_card?: string | null; email?: string | null; phone?: string | null }) =>
    api.put<Client>(`/sales/clients/${id}`, data),

  deleteClient: (id: number) =>
    api.delete(`/sales/clients/${id}`),
}
