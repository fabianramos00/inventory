import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { inventoryApi, type ProductsParams } from '@/lib/api/inventory'
import { PAGE_LIMIT } from '@/lib/constants'

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
  const queryClient = useQueryClient()
  const limit = PAGE_LIMIT
  const skip = (page - 1) * limit

  const { data, isLoading, isFetching, error, isError, refetch } = useQuery({
    queryKey: ['products', { page, search, ...filters, limit }],
    queryFn: async () => {
      const params: ProductsParams = { skip, limit }
      if (search) params.search = search
      if (filters.category_id) params.category_id = filters.category_id
      if (filters.material_id) params.material_id = filters.material_id
      if (filters.brand_id) params.brand_id = filters.brand_id
      if (filters.stock_status) params.stock_status = filters.stock_status
      const response = await inventoryApi.getProducts(params)
      return response.data
    },
  })

  const { data: statsData } = useQuery({
    queryKey: ['inventoryStats'],
    queryFn: async () => {
      const response = await inventoryApi.getStats()
      return response.data
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (id: number) => inventoryApi.deleteProduct(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['products'] })
      queryClient.invalidateQueries({ queryKey: ['inventoryStats'] })
    },
  })

  return {
    products: data?.items ?? [],
    total: data?.total ?? 0,
    totalPages: data?.pages ?? 1,
    stats: statsData ? {
      totalProducts: statsData.total_products || 0,
      totalStock: statsData.total_stock || 0,
      lowStockCount: statsData.low_stock_count || 0,
      totalValue: statsData.total_inventory_value || 0,
    } : null,
    isLoading,
    isFetching,
    error,
    isError,
    refetch,
    deleteMutation,
    invalidate: () => {
      queryClient.invalidateQueries({ queryKey: ['products'] })
      queryClient.invalidateQueries({ queryKey: ['inventoryStats'] })
    },
  }
}
