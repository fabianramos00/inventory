import api from '@/lib/axios'
import type { CursorPaginatedResponse, Provider } from '@/types'

export interface ProvidersParams {
  search?: string
  skip?: number
  limit?: number
}

interface ProviderWriteInput {
  name?: string
  contact_info?: string | null
  email?: string | null
  phone?: string | null
}

export const providersApi = {
  getProviders: (params?: ProvidersParams) =>
    api.get<CursorPaginatedResponse<Provider>>('/supply-chain/providers', { params }),

  createProvider: (data: ProviderWriteInput) =>
    api.post<Provider>('/supply-chain/providers', data),

  updateProvider: (id: number, data: ProviderWriteInput) =>
    api.put<Provider>(`/supply-chain/providers/${id}`, data),

  deleteProvider: (id: number) =>
    api.delete(`/supply-chain/providers/${id}`),
}
