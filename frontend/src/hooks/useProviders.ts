import { providersApi } from '@/lib/api/providers'
import type { Provider } from '@/types'
import { usePagedList } from './usePagedList'

interface UseProvidersParams {
  page: number
  search: string
}

export function useProviders({ page, search }: UseProvidersParams) {
  const { items, ...rest } = usePagedList<Provider, { search: string }>({
    key: 'providers',
    page,
    filters: { search },
    fetchPage: ({ skip, limit, search }) =>
      providersApi.getProviders({ skip, limit, ...(search.trim() && { search }) }).then(response => response.data),
    deleteItem: providersApi.deleteProvider,
  })

  return { ...rest, providers: items }
}
