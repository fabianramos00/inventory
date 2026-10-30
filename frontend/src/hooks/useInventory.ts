import { useQuery } from '@tanstack/react-query'
import { inventoryApi, type ProductsParams } from '@/lib/api/inventory'
import { usePagedList } from './usePagedList'
import type { Product } from '@/types'

interface InventoryFilters {
  search: string
  category_id?: number | ''
  material_id?: number | ''
  brand_id?: number | ''
  stock_status?: string
}

interface UseInventoryParams {
  page: number
  search: string
  filters: {
    category_id?: number | ''
    material_id?: number | ''
    brand_id?: number | ''
    stock_status?: string
  }
}

export function useInventory({ page, search, filters }: UseInventoryParams) {
  const list = usePagedList<Product, InventoryFilters>({
    key: 'products',
    page,
    filters: { search, ...filters },
    fetchPage: async ({ skip, limit, search, ...attrs }) => {
      const params: ProductsParams = { skip, limit }
      if (search) params.search = search
      if (attrs.category_id) params.category_id = attrs.category_id
      if (attrs.material_id) params.material_id = attrs.material_id
      if (attrs.brand_id) params.brand_id = attrs.brand_id
      if (attrs.stock_status) params.stock_status = attrs.stock_status
      return (await inventoryApi.getProducts(params)).data
    },
    deleteItem: inventoryApi.deleteProduct,
    invalidateKeys: ['inventoryStats'],
  })

  const { data: statsData } = useQuery({
    queryKey: ['inventoryStats'],
    queryFn: async () => {
      const response = await inventoryApi.getStats()
      return response.data
    },
  })

  return {
    products: list.items,
    hasNext: list.hasNext,
    stats: statsData ? {
      totalProducts: statsData.total_products || 0,
      totalStock: statsData.total_stock || 0,
      lowStockCount: statsData.low_stock_count || 0,
      totalValue: statsData.total_inventory_value || 0,
    } : null,
    isLoading: list.isLoading,
    isFetching: list.isFetching,
    error: list.error,
    isError: list.isError,
    refetch: list.refetch,
    deleteMutation: list.deleteMutation,
    invalidate: list.invalidate,
  }
}
