import api from '@/lib/axios'

export interface DashboardSummary {
  inventory_value: number
  total_sales_today: number
  pending_orders: number
  delivered_orders: number
  pending_payment_sales: number
}

export interface SalesTrendData {
  date: string
  sales_amount: number
  orders_cost: number
}

export interface SalesTrendResponse {
  data: SalesTrendData[]
}

export interface TopProduct {
  product_id: number
  product_name: string
  product_sku: string
  brand_name: string
  category_name: string
  material_name: string
  total_quantity_sold: number
  total_sales_amount: number
}

export interface TopProductsResponse {
  products: TopProduct[]
}

export interface OrderStatusData {
  status: string
  count: number
  percentage: number
}

export interface OrderStatusResponse {
  data: OrderStatusData[]
}

export const dashboardApi = {
  getSummary: () => api.get<DashboardSummary>('/dashboard/summary'),
  getSalesTrend: () => api.get<SalesTrendResponse>('/dashboard/sales-trend'),
  getTopProducts: () => api.get<TopProductsResponse>('/dashboard/top-products'),
  getOrderStatusDistribution: () =>
    api.get<OrderStatusResponse>('/dashboard/order-status-distribution'),
}
