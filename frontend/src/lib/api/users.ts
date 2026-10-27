import api from '@/lib/axios'
import type { CursorPaginatedResponse, User } from '@/types'

export interface CreateUserInput {
  full_name: string
  email: string
  password: string
  is_superuser?: boolean
}

export interface UpdateUserInput extends Partial<User> {
  password?: string
  is_superuser?: boolean
}

export interface UsersParams {
  search?: string
  skip?: number
  limit?: number
}

export const usersApi = {
  getUsers: (params?: UsersParams) =>
    api.get<CursorPaginatedResponse<User>>('/users', { params }),

  createUser: (data: CreateUserInput) =>
    api.post<User>('/users', data),

  updateUser: (id: number, data: UpdateUserInput) =>
    api.put<User>(`/users/${id}`, data),

  deleteUser: (id: number) =>
    api.delete(`/users/${id}`),
}
