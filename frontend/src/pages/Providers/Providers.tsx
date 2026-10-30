import { useState } from 'react'
import { Plus, Phone, Mail, Edit, Trash2 } from 'lucide-react'
import styles from './Providers.module.css'
import PageHeader from '@/components/PageHeader/PageHeader'
import CommandBar from '@/components/CommandBar/CommandBar'
import { providersApi } from '@/lib/api/providers'
import { useDebouncedValue } from '@/hooks/useDebouncedValue'
import CreateFormModal from '@/components/CreateFormModal/CreateFormModal'
import ConfirmDeleteModal from '@/components/ConfirmDeleteModal/ConfirmDeleteModal'
import ErrorState from '@/components/ErrorState/ErrorState'
import CardGridSkeleton from '@/components/CardGridSkeleton/CardGridSkeleton'
import Pagination from '@/components/Pagination'
import { usePagedList } from '@/hooks/usePagedList'
import { useDeleteConfirm } from '@/hooks/useDeleteConfirm'
import { PROVIDER_FIELDS, buildProviderCreatePayload } from '@/lib/formFields'
import type { Provider } from '@/types'

export default function Providers() {
  const [search, setSearch] = useState('')
  const [page, setPage] = useState(1)

  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false)
  const [isEditModalOpen, setIsEditModalOpen] = useState(false)
  const [editingProvider, setEditingProvider] = useState<Provider | null>(null)

  const debouncedSearch = useDebouncedValue(search, 300, () => setPage(1))

  const { items: providers, hasNext, isLoading, isFetching, isError, refetch, deleteMutation, invalidate } =
    usePagedList<Provider, { search: string }>({
      key: 'providers',
      page,
      filters: { search: debouncedSearch },
      fetchPage: ({ skip, limit, search }) =>
        providersApi.getProviders({ skip, limit, ...(search.trim() && { search }) }).then(response => response.data),
      deleteItem: providersApi.deleteProvider,
    })

  const {
    target: providerToDelete,
    requestDelete: requestProviderDelete,
    close: closeProviderDelete,
    confirm: confirmProviderDelete,
  } = useDeleteConfirm<{ id: number; name: string }>(deleteMutation.mutateAsync)

  function openEditModal(provider: Provider) {
    setEditingProvider(provider)
    setIsEditModalOpen(true)
  }

  function handleProviderCreated() {
    setIsCreateModalOpen(false)
    invalidate()
  }

  async function submitEditProvider(values: Record<string, string>) {
    if (!editingProvider) return editingProvider
    const payload = {
      name: values.name,
      contact_info: values.contact_info || null,
      email: values.email || null,
      phone: values.phone || null,
    }
    const response = await providersApi.updateProvider(editingProvider.id, payload)
    return response.data
  }

  function handleProviderEdited() {
    setIsEditModalOpen(false)
    setEditingProvider(null)
    invalidate()
  }

  async function submitCreateProvider(values: Record<string, string>) {
    const response = await providersApi.createProvider(buildProviderCreatePayload(values))
    return response.data
  }

  const editFormValues: Record<string, string> = editingProvider
    ? {
        name: editingProvider.name,
        contact_info: editingProvider.contact_info || '',
        email: editingProvider.email || '',
        phone: editingProvider.phone || '',
      }
    : {
        name: '',
        contact_info: '',
        email: '',
        phone: '',
      }

  return (
    <div className={styles.container}>
      <PageHeader
        activeLabel="Gestión"
        title="Proveedores"
        action={
          <button className="btn-new" onClick={() => setIsCreateModalOpen(true)}>
            <Plus size={16} strokeWidth={2.5} />
            <span>Nuevo Proveedor</span>
          </button>
        }
      />

      <CommandBar
        search={search}
        onSearchChange={setSearch}
        placeholder="Buscar proveedor por nombre, correo o teléfono..."
      />

      {isLoading ? (
        <CardGridSkeleton label="Cargando proveedores..." />
      ) : isError && providers.length === 0 ? (
        <ErrorState message="No se pudieron cargar los proveedores." onRetry={refetch} />
      ) : (
        <>
          <div className={styles.gridContainer}>
            {providers.map(p => (
              <div key={p.id} className={styles.providerCard}>
                <div className={styles.cardHeader}>
                  <div className={styles.identity}>
                    <span className={styles.monogram} aria-hidden="true">{p.name.charAt(0).toUpperCase()}</span>
                    <div className={styles.providerInfo}>
                      <div className={styles.providerName}>{p.name}</div>
                      {p.contact_info && <div className={styles.providerContact}>{p.contact_info}</div>}
                    </div>
                  </div>
                  <div className={styles.cardActions}>
                    <button className="action-btn" onClick={() => openEditModal(p)} title="Editar">
                      <Edit size={14} />
                    </button>
                    <button className="action-btn action-btn--destructive" onClick={() => requestProviderDelete({ id: p.id, name: p.name })} title="Eliminar">
                      <Trash2 size={14} />
                    </button>
                  </div>
                </div>

                <div className={styles.detailsList}>
                  {p.email && (
                    <div className={styles.detailItem}>
                      <Mail size={14} className={styles.detailIcon} />
                      <span>{p.email}</span>
                    </div>
                  )}
                  {p.phone && (
                    <div className={styles.detailItem}>
                      <Phone size={14} className={styles.detailIcon} />
                      <span>{p.phone}</span>
                    </div>
                  )}
                </div>
              </div>
            ))}
          </div>

          {providers.length === 0 && !isLoading && (
            <div className={styles.emptyState}>No se encontraron proveedores.</div>
          )}

          <Pagination
            currentPage={page}
            onPageChange={setPage}
            isLoading={isFetching}
            hasNext={hasNext}
          />
        </>
      )}

      {isCreateModalOpen && (
        <CreateFormModal
          title="Crear Nuevo Proveedor"
          fields={PROVIDER_FIELDS}
          onSubmit={submitCreateProvider}
          onClose={() => setIsCreateModalOpen(false)}
          onCreated={handleProviderCreated}
        />
      )}

      {isEditModalOpen && editingProvider && (
        <CreateFormModal
          title={`Editar Proveedor: ${editingProvider.name}`}
          fields={PROVIDER_FIELDS}
          initialValues={editFormValues}
          onSubmit={submitEditProvider}
          onClose={() => {
            setIsEditModalOpen(false)
            setEditingProvider(null)
          }}
          onCreated={handleProviderEdited}
        />
      )}

      {providerToDelete && (
        <ConfirmDeleteModal
          title="Eliminar Proveedor"
          productName={providerToDelete.name}
          onConfirm={confirmProviderDelete}
          onClose={closeProviderDelete}
        />
      )}
    </div>
  )
}
