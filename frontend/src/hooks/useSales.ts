import { useQuery } from '@tanstack/react-query'
import { salesApi, type SalesParams } from '@/lib/api/sales'
import type { Sale } from '@/types'
import { usePagedList } from './usePagedList'

export interface SalesFilters {
  user_id?: number | ''
  client_id?: number | ''
  no_client?: boolean
  payment_method?: string
  delivery_status?: string
  payment_status?: string
  start_date?: string
  end_date?: string
}

interface UseSalesParams {
  page: number
  filters: SalesFilters
}

function buildSalesParams({ skip, limit, user_id, no_client, client_id, payment_method, delivery_status, payment_status, start_date, end_date }: { skip: number; limit: number } & SalesFilters): SalesParams {
  const params: SalesParams = { skip, limit }
  if (user_id) params.user_id = user_id
  if (no_client) params.no_client = true
  else if (client_id) params.client_id = client_id
  if (payment_method) params.payment_method = payment_method
  if (delivery_status) params.delivery_status = delivery_status
  if (payment_status) params.payment_status = payment_status
  if (start_date) params.start_date = start_date
  if (end_date) params.end_date = end_date
  return params
}

export function useSales({ page, filters }: UseSalesParams) {
  const { items, ...rest } = usePagedList<Sale, SalesFilters>({
    key: 'sales',
    page,
    filters,
    fetchPage: params => salesApi.getSales(buildSalesParams(params)).then(response => response.data),
    deleteItem: salesApi.deleteSale,
    invalidateKeys: ['salesStats'],
  })

  const { data: statsData, isLoading: statsLoading } = useQuery({
    queryKey: ['salesStats'],
    queryFn: async () => {
      const response = await salesApi.getStats()
      return response.data
    },
  })

  return {
    ...rest,
    sales: items,
    stats: statsData ? {
      unpaidCount: statsData.unpaid_count || 0,
      undeliveredCount: statsData.undelivered_count || 0,
      totalAmountSum: statsData.total_amount_sum || 0,
      amountPaidSum: statsData.amount_paid_sum || 0,
    } : { unpaidCount: 0, undeliveredCount: 0, totalAmountSum: 0, amountPaidSum: 0 },
    statsLoading,
  }
}
