import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { PAGE_LIMIT } from '@/lib/constants'
import type { PaginatedResponse, CursorPaginatedResponse } from '@/types'

interface UsePagedListOptions<T, F extends object> {
  key: string
  page: number
  filters: F
  fetchPage: (params: { skip: number; limit: number } & F) => Promise<PaginatedResponse<T> | CursorPaginatedResponse<T>>
  deleteItem: (id: number) => Promise<unknown>
  invalidateKeys?: string[]
}

export function usePagedList<T, F extends object>({
  key,
  page,
  filters,
  fetchPage,
  deleteItem,
  invalidateKeys = [],
}: UsePagedListOptions<T, F>) {
  const queryClient = useQueryClient()
  const limit = PAGE_LIMIT
  const skip = (page - 1) * limit
  const keysToInvalidate = [key, ...invalidateKeys]

  const query = useQuery({
    queryKey: [key, { page, ...filters, limit }],
    queryFn: () => fetchPage({ skip, limit, ...filters }),
  })

  const deleteMutation = useMutation({
    mutationFn: (id: number) => deleteItem(id),
    onSuccess: () => {
      keysToInvalidate.forEach(invalidateKey => {
        queryClient.invalidateQueries({ queryKey: [invalidateKey] })
      })
    },
  })

  return {
    items: query.data?.items ?? [],
    hasNext: query.data && 'has_next' in query.data ? query.data.has_next : false,
    totalPages: query.data && 'pages' in query.data ? query.data.pages : 1,
    isLoading: query.isLoading,
    isFetching: query.isFetching,
    error: query.error,
    isError: query.isError,
    refetch: query.refetch,
    deleteMutation,
    invalidate: () => {
      keysToInvalidate.forEach(invalidateKey => {
        queryClient.invalidateQueries({ queryKey: [invalidateKey] })
      })
    },
  }
}
