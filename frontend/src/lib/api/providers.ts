import api from '@/lib/axios'
import type { PaginatedResponse, Provider } from '@/types'

export interface ProvidersParams {
  search?: string
  skip?: number
  limit?: number
}

export const providersApi = {
  getProviders: (params?: ProvidersParams) =>
    api.get<PaginatedResponse<Provider>>('/supply-chain/providers', { params }),

  createProvider: (data: Partial<Provider>) =>
    api.post<Provider>('/supply-chain/providers', data),

  updateProvider: (id: number, data: Partial<Provider>) =>
    api.put<Provider>(`/supply-chain/providers/${id}`, data),

  deleteProvider: (id: number) =>
    api.delete(`/supply-chain/providers/${id}`),
}
