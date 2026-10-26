import api from '@/lib/axios'
import type { PaginatedResponse, Product, CreateProductInput, FilterOption, InventoryStatsData } from '@/types'

export interface ProductsParams {
  skip?: number
  limit?: number
  search?: string
  category_id?: number
  material_id?: number
  brand_id?: number
  stock_status?: string
}

export interface CatalogParams {
  search?: string
  skip?: number
  limit?: number
}

export const inventoryApi = {
  getProducts: (params?: ProductsParams) =>
    api.get<PaginatedResponse<Product>>('/inventory/products', { params }),

  getProduct: (id: number) =>
    api.get<Product>(`/inventory/products/${id}`),

  createProduct: (data: CreateProductInput) =>
    api.post<Product>('/inventory/products', data),

  updateProduct: (id: number, data: Partial<CreateProductInput>) =>
    api.put<Product>(`/inventory/products/${id}`, data),

  deleteProduct: (id: number) =>
    api.delete(`/inventory/products/${id}`),

  getMaterials: (params?: CatalogParams) =>
    api.get<PaginatedResponse<FilterOption>>('/inventory/materials', { params }),

  getCategories: (params?: CatalogParams) =>
    api.get<PaginatedResponse<FilterOption>>('/inventory/categories', { params }),

  getBrands: (params?: CatalogParams) =>
    api.get<PaginatedResponse<FilterOption>>('/inventory/brands', { params }),

  getMeasurementUnits: (params?: CatalogParams) =>
    api.get<PaginatedResponse<FilterOption>>('/inventory/measurement-units', { params }),

  createMaterial: (data: { name: string }) =>
    api.post<FilterOption>('/inventory/materials', data),

  createCategory: (data: { code: string; name: string; description?: string }) =>
    api.post<FilterOption>('/inventory/categories', data),

  createBrand: (data: { name: string; logo_url?: string }) =>
    api.post<FilterOption>('/inventory/brands', data),

  createMeasurementUnit: (data: { name: string; abbreviation: string }) =>
    api.post<FilterOption>('/inventory/measurement-units', data),

  updateCategory: (id: number, data: { code: string; name: string; description?: string }) =>
    api.put<FilterOption>(`/inventory/categories/${id}`, data),
  deleteCategory: (id: number) => api.delete(`/inventory/categories/${id}`),

  updateBrand: (id: number, data: { name: string; logo_url?: string }) =>
    api.put<FilterOption>(`/inventory/brands/${id}`, data),
  deleteBrand: (id: number) => api.delete(`/inventory/brands/${id}`),

  updateMaterial: (id: number, data: { name: string }) =>
    api.put<FilterOption>(`/inventory/materials/${id}`, data),
  deleteMaterial: (id: number) => api.delete(`/inventory/materials/${id}`),

  updateMeasurementUnit: (id: number, data: { name: string; abbreviation: string }) =>
    api.put<FilterOption>(`/inventory/measurement-units/${id}`, data),
  deleteMeasurementUnit: (id: number) => api.delete(`/inventory/measurement-units/${id}`),

  getStats: () =>
    api.get<InventoryStatsData>('/inventory/products/stats/overview'),
}
