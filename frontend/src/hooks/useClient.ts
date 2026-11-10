import { useMutation, useQuery, useQueryClient, type QueryClient } from '@tanstack/react-query'
import { clientsApi } from '@/lib/api/clients'
import type { Client, ClientPaymentResult, PaymentMethod } from '@/types'

const CLIENT_PAYMENT_KEYS = [
  'clients',
  'sales',
  'salesStats',
  'dashboardSummary',
  'dashboardTrend',
  'dashboardTopProducts',
  'dashboardLowStock',
]

export function applyClientPaymentResult(
  queryClient: QueryClient,
  clientId: number,
  result: ClientPaymentResult,
) {
  queryClient.setQueryData<Client>(['client', clientId], client =>
    client ? { ...client, debt_amount: result.remaining_debt } : client,
  )
  CLIENT_PAYMENT_KEYS.forEach(key => {
    queryClient.invalidateQueries({ queryKey: [key] })
  })
  queryClient.invalidateQueries({ queryKey: ['client', clientId] })
  result.allocations.forEach(({ sale_id }) => {
    ;[['sale', sale_id], ['saleItems', sale_id], ['salePayments', sale_id]].forEach(queryKey => {
      queryClient.invalidateQueries({ queryKey })
    })
  })
}

export function useClient(id: string | undefined) {
  const clientId = Number(id)
  const queryClient = useQueryClient()

  const query = useQuery<Client>({
    queryKey: ['client', clientId],
    queryFn: () => clientsApi.getClient(clientId).then(response => response.data),
    enabled: !!id,
  })

  const paymentMutation = useMutation({
    mutationFn: (data: { amount: number; payment_method: Exclude<PaymentMethod, null> }) =>
      clientsApi.addClientPayment(clientId, data),
    onSuccess: response => {
      applyClientPaymentResult(queryClient, clientId, response.data)
    },
  })

  return {
    client: query.data ?? null,
    isLoading: query.isLoading,
    isFetching: query.isFetching,
    error: query.error,
    refetch: query.refetch,
    invalidate: () => {
      queryClient.invalidateQueries({ queryKey: ['client', clientId] })
      queryClient.invalidateQueries({ queryKey: ['clients'] })
    },
    paymentMutation,
  }
}
