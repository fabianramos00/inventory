import { useState } from 'react'
import { Plus, Phone, Mail, Edit, Trash2 } from 'lucide-react'
import styles from './Clients.module.css'
import PageHeader from '@/components/PageHeader/PageHeader'
import CommandBar from '@/components/CommandBar/CommandBar'
import { clientsApi } from '@/lib/api/clients'
import { formatDate } from '@/lib/utils'
import { useDebouncedValue } from '@/hooks/useDebouncedValue'
import CreateFormModal from '@/components/CreateFormModal/CreateFormModal'
import ConfirmDeleteModal from '@/components/ConfirmDeleteModal/ConfirmDeleteModal'
import ErrorState from '@/components/ErrorState/ErrorState'
import CardGridSkeleton from '@/components/CardGridSkeleton/CardGridSkeleton'
import Pagination from '@/components/Pagination'
import { usePagedList } from '@/hooks/usePagedList'
import { useDeleteConfirm } from '@/hooks/useDeleteConfirm'
import { CLIENT_FIELDS, buildClientCreatePayload } from '@/lib/formFields'
import type { Client } from '@/types'

export default function Clients() {
  const [search, setSearch] = useState('')
  const [page, setPage] = useState(1)

  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false)
  const [isEditModalOpen, setIsEditModalOpen] = useState(false)
  const [editingClient, setEditingClient] = useState<Client | null>(null)

  const debouncedSearch = useDebouncedValue(search, 300, () => setPage(1))

  const { items: clients, hasNext, isLoading, isFetching, isError, refetch, deleteMutation, invalidate } =
    usePagedList<Client, { search: string }>({
      key: 'clients',
      page,
      filters: { search: debouncedSearch },
      fetchPage: ({ skip, limit, search }) =>
        clientsApi.getClients({ skip, limit, ...(search.trim() && { search }) }).then(response => response.data),
      deleteItem: clientsApi.deleteClient,
    })

  const {
    target: clientToDelete,
    requestDelete: requestClientDelete,
    close: closeClientDelete,
    confirm: confirmClientDelete,
  } = useDeleteConfirm<{ id: number; name: string }>(deleteMutation.mutateAsync)

  function openEditModal(client: Client) {
    setEditingClient(client)
    setIsEditModalOpen(true)
  }

  function handleClientCreated() {
    setIsCreateModalOpen(false)
    invalidate()
  }

  async function submitEditClient(values: Record<string, string>) {
    if (!editingClient) return editingClient
    const payload = {
      name: values.name,
      identity_card: values.identity_card || null,
      email: values.email || null,
      phone: values.phone || null,
    }
    const response = await clientsApi.updateClient(editingClient.id, payload)
    return response.data
  }

  function handleClientEdited() {
    setIsEditModalOpen(false)
    setEditingClient(null)
    invalidate()
  }

  async function submitCreateClient(values: Record<string, string>) {
    const response = await clientsApi.createClient(buildClientCreatePayload(values))
    return response.data
  }

  const editFormValues: Record<string, string> = editingClient
    ? {
        name: editingClient.name,
        identity_card: editingClient.identity_card || '',
        email: editingClient.email || '',
        phone: editingClient.phone || '',
      }
    : {
        name: '',
        identity_card: '',
        email: '',
        phone: '',
      }

  return (
    <div className={styles.container}>
      <PageHeader
        activeLabel="Gestión"
        title="Clientes"
        action={
          <button className="btn-new" onClick={() => setIsCreateModalOpen(true)}>
            <Plus size={16} strokeWidth={2.5} />
            <span>Nuevo Cliente</span>
          </button>
        }
      />

      <CommandBar
        search={search}
        onSearchChange={setSearch}
        placeholder="Buscar cliente por nombre, cédula o contacto..."
      />

      {isLoading ? (
        <CardGridSkeleton label="Cargando clientes..." />
      ) : isError && clients.length === 0 ? (
        <ErrorState message="No se pudieron cargar los clientes." onRetry={refetch} />
      ) : (
        <>
          <div className={styles.gridContainer}>
            {clients.map(c => (
              <div key={c.id} className={styles.clientCard}>
                <div className={styles.cardHeader}>
                  <div className={styles.identity}>
                    <span className={styles.monogram} aria-hidden="true">{c.name.charAt(0).toUpperCase()}</span>
                    <div className={styles.clientInfo}>
                      <div className={styles.clientName}>{c.name}</div>
                      {c.identity_card && <div className={styles.clientIdCard}>ID: {c.identity_card}</div>}
                    </div>
                  </div>
                  <div className={styles.cardActions}>
                    <button className="action-btn" onClick={() => openEditModal(c)} title="Editar">
                      <Edit size={14} />
                    </button>
                    <button className="action-btn action-btn--destructive" onClick={() => requestClientDelete({ id: c.id, name: c.name })} title="Eliminar">
                      <Trash2 size={14} />
                    </button>
                  </div>
                </div>

                <div className={styles.detailsList}>
                  {c.email && (
                    <div className={styles.detailItem}>
                      <Mail size={14} className={styles.detailIcon} />
                      <span>{c.email}</span>
                    </div>
                  )}
                  {c.phone && (
                    <div className={styles.detailItem}>
                      <Phone size={14} className={styles.detailIcon} />
                      <span>{c.phone}</span>
                    </div>
                  )}
                  <div className={styles.detailItem}>
                    <span className={styles.dateLabel}>Creado:</span>
                    <span className={styles.dateValue}>{formatDate(c.created_at)}</span>
                  </div>
                </div>
              </div>
            ))}
          </div>

          {clients.length === 0 && !isLoading && (
            <div className={styles.emptyState}>No se encontraron clientes.</div>
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
          title="Crear Nuevo Cliente"
          fields={CLIENT_FIELDS}
          onSubmit={submitCreateClient}
          onClose={() => setIsCreateModalOpen(false)}
          onCreated={handleClientCreated}
        />
      )}

      {isEditModalOpen && editingClient && (
        <CreateFormModal
          title={`Editar Cliente: ${editingClient.name}`}
          fields={CLIENT_FIELDS}
          initialValues={editFormValues}
          onSubmit={submitEditClient}
          onClose={() => {
            setIsEditModalOpen(false)
            setEditingClient(null)
          }}
          onCreated={handleClientEdited}
        />
      )}

      {clientToDelete && (
        <ConfirmDeleteModal
          title="Eliminar Cliente"
          productName={clientToDelete.name}
          onConfirm={confirmClientDelete}
          onClose={closeClientDelete}
        />
      )}
    </div>
  )
}
