import { useMutation, useQueryClient } from '@tanstack/react-query'
import { usersApi } from '@/lib/api/users'
import type { User } from '@/types'
import { usePagedList } from './usePagedList'

interface UseUsersParams {
  page: number
  search: string
}

export function useUsers({ page, search }: UseUsersParams) {
  const queryClient = useQueryClient()
  const { items, ...rest } = usePagedList<User, { search: string }>({
    key: 'users',
    page,
    filters: { search },
    fetchPage: ({ skip, limit, search }) =>
      usersApi.getUsers({ skip, limit, ...(search.trim() && { search }) }).then(response => response.data),
    deleteItem: usersApi.deleteUser,
  })

  const toggleActiveMutation = useMutation({
    mutationFn: (user: { id: number; is_active: boolean }) =>
      usersApi.updateUser(user.id, { is_active: !user.is_active }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['users'] })
    },
  })

  return { ...rest, users: items, toggleActiveMutation }
}
