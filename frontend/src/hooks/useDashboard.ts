import { useQuery } from '@tanstack/react-query'
import { dashboardApi } from '@/lib/api/dashboard'
import { inventoryApi } from '@/lib/api/inventory'
import { PAGE_LIMIT } from '@/lib/constants'

export function useDashboard() {
  const summary = useQuery({
    queryKey: ['dashboardSummary'],
    queryFn: async () => {
      const response = await dashboardApi.getSummary()
      return response.data
    },
  })

  const trend = useQuery({
    queryKey: ['dashboardTrend'],
    queryFn: async () => {
      const response = await dashboardApi.getSalesTrend()
      return response.data.data
    },
  })

  const topProducts = useQuery({
    queryKey: ['dashboardTopProducts'],
    queryFn: async () => {
      const response = await dashboardApi.getTopProducts()
      return response.data.products
    },
  })

  const orderStatus = useQuery({
    queryKey: ['dashboardOrderStatus'],
    queryFn: async () => {
      const response = await dashboardApi.getOrderStatusDistribution()
      return response.data.data
    },
  })

  const lowStock = useQuery({
    queryKey: ['dashboardLowStock'],
    queryFn: async () => {
      const response = await inventoryApi.getProducts({ skip: 0, limit: PAGE_LIMIT, stock_status: 'low_stock' })
      return { items: response.data.items, total: response.data.items.length }
    },
  })

  const stats = useQuery({
    queryKey: ['inventoryStats'],
    queryFn: async () => {
      const response = await inventoryApi.getStats()
      return response.data
    },
  })

  const lowStockTotal = stats.data?.low_stock_count ?? lowStock.data?.total

  return {
    summary: summary.data,
    trend: trend.data,
    topProducts: topProducts.data,
    orderStatus: orderStatus.data,
    lowStock: lowStock.data?.items,
    lowStockTotal,
    isLoading: summary.isLoading || trend.isLoading || topProducts.isLoading || orderStatus.isLoading || lowStock.isLoading || stats.isLoading,
    errors: {
      summary: summary.isError,
      trend: trend.isError,
      topProducts: topProducts.isError,
      orderStatus: orderStatus.isError,
      lowStock: lowStock.isError,
    },
    refetch: {
      summary: summary.refetch,
      trend: trend.refetch,
      topProducts: topProducts.refetch,
      orderStatus: orderStatus.refetch,
      lowStock: () => {
        lowStock.refetch()
        stats.refetch()
      },
    },
  }
}
