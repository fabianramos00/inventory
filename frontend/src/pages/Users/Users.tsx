import { useState } from 'react'
import { Plus, Loader, Trash2, Edit2 } from 'lucide-react'
import { z } from 'zod'
import DataTable from '@/components/DataTable/DataTable'
import styles from './Users.module.css'
import PageHeader from '@/components/PageHeader/PageHeader'
import CommandBar from '@/components/CommandBar/CommandBar'
import DataCard from '@/components/DataCard/DataCard'
import { usersApi, type CreateUserInput, type UpdateUserInput } from '@/lib/api/users'
import { getApiErrorCode } from '@/lib/apiError'
import { useDebouncedValue } from '@/hooks/useDebouncedValue'
import CreateFormModal, { type FieldConfig } from '@/components/CreateFormModal/CreateFormModal'
import ConfirmDeleteModal from '@/components/ConfirmDeleteModal/ConfirmDeleteModal'
import { useUsers } from '@/hooks/useUsers'
import { useDeleteConfirm } from '@/hooks/useDeleteConfirm'
import type { User } from '@/types'

const ROLE_OPTIONS = [
  { value: 'usuario', label: 'Usuario' },
  { value: 'administrador', label: 'Administrador' },
]

const USER_CREATE_FIELDS: FieldConfig[] = [
  { key: 'full_name', label: 'Nombre Completo', placeholder: 'Ej. Fabian Ramos', required: true },
  { key: 'email', label: 'Correo Electrónico', placeholder: 'Ej. fabian@gmail.com', type: 'email', required: true },
  { key: 'password', label: 'Contraseña', placeholder: 'Ej. ••••••••', type: 'password', required: true },
  { key: 'role', label: 'Rol', type: 'select', options: ROLE_OPTIONS, required: true },
]

const USER_EDIT_FIELDS: FieldConfig[] = [
  { key: 'full_name', label: 'Nombre Completo', placeholder: 'Ej. Fabian Ramos', required: true },
  { key: 'email', label: 'Correo Electrónico', placeholder: 'Ej. fabian@gmail.com', type: 'email', required: true },
  { key: 'password', label: 'Nueva Contraseña', placeholder: 'Dejar en blanco para no cambiar', type: 'password', required: false },
  { key: 'role', label: 'Rol', type: 'select', options: ROLE_OPTIONS, required: true },
]

const createUserSchema = z.object({
  full_name: z.string().min(2, 'El nombre debe tener al menos 2 caracteres'),
  email: z.string().min(1, 'El correo es requerido').email('Correo electrónico inválido'),
  password: z.string().min(8, 'La contraseña debe tener al menos 8 caracteres'),
  role: z.string().min(1, 'El rol es requerido'),
})

const editUserSchema = z.object({
  full_name: z.string().min(2, 'El nombre debe tener al menos 2 caracteres'),
  email: z.string().min(1, 'El correo es requerido').email('Correo electrónico inválido'),
  password: z.string().optional(),
  role: z.string().min(1, 'El rol es requerido'),
})

export default function Users() {
  const [search, setSearch] = useState('')
  const [page, setPage] = useState(1)

  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false)
  const [isEditModalOpen, setIsEditModalOpen] = useState(false)
  const [editingUser, setEditingUser] = useState<User | null>(null)

  const debouncedSearch = useDebouncedValue(search, 300, () => setPage(1))

  const { users, totalPages, isLoading, isFetching, isError, refetch, toggleActiveMutation, deleteMutation, invalidate } = useUsers({ page, search: debouncedSearch })

  async function deleteUser(id: number) {
    try {
      await deleteMutation.mutateAsync(id)
    } catch (err: unknown) {
      if (getApiErrorCode(err) === 'FOREIGN_KEY_VIOLATION') {
        throw new Error('No se puede eliminar el usuario porque está asociado a una venta.', { cause: err })
      }
      throw err
    }
  }

  const {
    target: userToDelete,
    requestDelete: requestUserDelete,
    close: closeUserDelete,
    confirm: confirmUserDelete,
  } = useDeleteConfirm<{ id: number; name: string }>(deleteUser)

  function openEditModal(user: User) {
    setEditingUser(user)
    setIsEditModalOpen(true)
  }

  function handleUserCreated() {
    setIsCreateModalOpen(false)
    invalidate()
  }

  async function submitEditUser(values: Record<string, string>) {
    if (!editingUser) {
      throw new Error('No user selected')
    }
    const updateData: UpdateUserInput = {
      full_name: values.full_name,
      email: values.email,
      is_superuser: values.role === 'administrador',
    }
    if (values.password?.trim()) {
      updateData.password = values.password
    }
    const result = await usersApi.updateUser(editingUser.id, updateData)
    return result.data
  }

  function handleUserEdited() {
    setIsEditModalOpen(false)
    setEditingUser(null)
    invalidate()
  }

  function handleToggleActive(user: User) {
    toggleActiveMutation.mutate(user)
  }

  async function submitCreateUser(values: Record<string, string>) {
    const createData: CreateUserInput = {
      full_name: values.full_name,
      email: values.email,
      password: values.password,
      is_superuser: values.role === 'administrador',
    }
    const result = await usersApi.createUser(createData)
    return result.data
  }

  return (
    <div className={styles.container}>
      <PageHeader
        activeLabel="Gestión"
        title="Usuarios"
        action={
          <button className="btn-new" onClick={() => setIsCreateModalOpen(true)}>
            <Plus size={16} /> Nuevo Usuario
          </button>
        }
      />

      <div className={styles.tableSection}>
        <CommandBar
          search={search}
          onSearchChange={setSearch}
          placeholder="Buscar usuario..."
        />

        <DataCard>
          <DataTable
            loading={isLoading}
            empty={users.length === 0}
            error={isError && users.length === 0}
            onRetry={refetch}
            loadingText="Cargando usuarios..."
            emptyText="No se encontraron usuarios."
            minWidth="900px"
            currentPage={page}
            totalPages={totalPages}
            skeletonRows={5}
            skeletonColumns={5}
            stickyFirstColumn
            onPageChange={setPage}
            isPaginationLoading={isFetching && !isLoading}
          >
            <thead>
              <tr>
                <th scope="col">Usuario</th>
                <th scope="col">Correo</th>
                <th scope="col">Rol</th>
                <th scope="col">Estado</th>
                <th scope="col">Acciones</th>
              </tr>
            </thead>
            <tbody>
              {users.map((user) => (
                <tr key={user.id}>
                  <td>
                    <div className={styles.userCell}>
                      <div className={styles.avatar} aria-hidden="true">
                        {user.full_name.charAt(0).toUpperCase()}
                      </div>
                      <span className={styles.userName}>{user.full_name}</span>
                    </div>
                  </td>
                  <td><span className={styles.emailCell}>{user.email}</span></td>
                  <td>
                    {user.is_superuser
                      ? <span className="badge badge--blue">Administrador</span>
                      : <span className={styles.rolePlain}>Usuario</span>}
                  </td>
                  <td>
                    <button
                      className={styles.stateToggle}
                      onClick={() => handleToggleActive(user)}
                      disabled={toggleActiveMutation.isPending && toggleActiveMutation.variables?.id === user.id}
                      title={user.is_active ? 'Desactivar usuario' : 'Activar usuario'}
                    >
                      {toggleActiveMutation.isPending && toggleActiveMutation.variables?.id === user.id ? (
                        <Loader size={16} className={styles.toggleSpinner} />
                      ) : (
                        <div className={`${styles.toggleSwitch} ${user.is_active ? styles.toggleActive : ''}`} />
                      )}
                    </button>
                  </td>
                  <td>
                    <div className="flex items-center gap-2">
                      <button
                        className="action-btn"
                        onClick={() => openEditModal(user)}
                        title="Editar usuario"
                      >
                        <Edit2 size={14} />
                      </button>
                      <button
                        className="action-btn action-btn--destructive"
                        onClick={() => requestUserDelete({ id: user.id, name: user.full_name })}
                        title="Eliminar usuario"
                      >
                        <Trash2 size={14} />
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </DataTable>
        </DataCard>
      </div>

      {isCreateModalOpen && (
        <CreateFormModal
          title="Nuevo Usuario"
          fields={USER_CREATE_FIELDS}
          onSubmit={submitCreateUser}
          onClose={() => setIsCreateModalOpen(false)}
          onCreated={handleUserCreated}
          validationSchema={createUserSchema}
        />
      )}

      {isEditModalOpen && editingUser && (
        <CreateFormModal
          title="Editar Usuario"
          fields={USER_EDIT_FIELDS}
          onSubmit={submitEditUser}
          onClose={() => setIsEditModalOpen(false)}
          onCreated={handleUserEdited}
          initialValues={{
            full_name: editingUser.full_name,
            email: editingUser.email,
            password: '',
            role: editingUser.is_superuser ? 'administrador' : 'usuario',
          }}
          validationSchema={editUserSchema}
        />
      )}

      {userToDelete && (
        <ConfirmDeleteModal
          title="Eliminar Usuario"
          productName={userToDelete.name}
          onConfirm={confirmUserDelete}
          onClose={closeUserDelete}
        />
      )}
    </div>
  )
}
