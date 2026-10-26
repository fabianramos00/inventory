import api from '@/lib/axios'
import type { CursorPaginatedResponse, Client } from '@/types'

export interface ClientsParams {
  search?: string
  skip?: number
  limit?: number
}

export const clientsApi = {
  getClients: (params?: ClientsParams) =>
    api.get<CursorPaginatedResponse<Client>>('/sales/clients', { params }),

  createClient: (data: { name: string; identity_card?: string; email?: string; phone?: string }) =>
    api.post<Client>('/sales/clients', data),

  updateClient: (id: number, data: { name?: string; identity_card?: string; email?: string; phone?: string }) =>
    api.put<Client>(`/sales/clients/${id}`, data),

  deleteClient: (id: number) =>
    api.delete(`/sales/clients/${id}`),
}
