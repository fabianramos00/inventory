import { clientsApi } from '@/lib/api/clients'
import type { Client } from '@/types'
import { usePagedList } from './usePagedList'

interface UseClientsParams {
  page: number
  search: string
}

export function useClients({ page, search }: UseClientsParams) {
  const { items, ...rest } = usePagedList<Client, { search: string }>({
    key: 'clients',
    page,
    filters: { search },
    fetchPage: ({ skip, limit, search }) =>
      clientsApi.getClients({ skip, limit, ...(search.trim() && { search }) }).then(response => response.data),
    deleteItem: clientsApi.deleteClient,
  })

  return { ...rest, clients: items }
}
