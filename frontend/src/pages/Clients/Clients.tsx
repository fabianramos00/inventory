import { useRef, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Plus, Phone, Mail, Edit, Trash2, Wallet } from 'lucide-react'
import styles from './Clients.module.css'
import PageHeader from '@/components/PageHeader/PageHeader'
import CommandBar from '@/components/CommandBar/CommandBar'
import { clientsApi } from '@/lib/api/clients'
import { formatCurrency, formatDate } from '@/lib/utils'
import { useDebouncedValue } from '@/hooks/useDebouncedValue'
import CreateFormModal from '@/components/CreateFormModal/CreateFormModal'
import ConfirmDeleteModal from '@/components/ConfirmDeleteModal/ConfirmDeleteModal'
import ClientPaymentModal from '@/components/ClientPaymentModal/ClientPaymentModal'
import InlineNotice from '@/components/InlineNotice/InlineNotice'
import ErrorState from '@/components/ErrorState/ErrorState'
import CardGridSkeleton from '@/components/CardGridSkeleton/CardGridSkeleton'
import Pagination from '@/components/Pagination'
import { usePagedList } from '@/hooks/usePagedList'
import { useDeleteConfirm } from '@/hooks/useDeleteConfirm'
import { applyClientPaymentResult } from '@/hooks/useClient'
import { CLIENT_FIELDS, buildClientCreatePayload } from '@/lib/formFields'
import type { Client, ClientPaymentResult, PaymentMethod } from '@/types'

export default function Clients() {
  const [search, setSearch] = useState('')
  const [page, setPage] = useState(1)
  const queryClient = useQueryClient()

  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false)
  const [isEditModalOpen, setIsEditModalOpen] = useState(false)
  const [editingClient, setEditingClient] = useState<Client | null>(null)
  const [payingClient, setPayingClient] = useState<Client | null>(null)
  const [notice, setNotice] = useState<string | null>(null)
  const payTriggerRef = useRef<HTMLButtonElement | null>(null)
  const successRef = useRef<HTMLDivElement>(null)

  const paymentMutation = useMutation({
    mutationFn: ({ clientId, data }: { clientId: number; data: { amount: number; payment_method: Exclude<PaymentMethod, null> } }) =>
      clientsApi.addClientPayment(clientId, data),
    onSuccess: (response, { clientId }) => {
      applyClientPaymentResult(queryClient, clientId, response.data)
    },
  })

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
    confirm: deleteClient,
  } = useDeleteConfirm<{ id: number; name: string }>(deleteMutation.mutateAsync)

  async function confirmClientDelete() {
    await deleteClient()
    setNotice('Cliente eliminado exitosamente.')
  }

  function openEditModal(client: Client) {
    setEditingClient(client)
    setIsEditModalOpen(true)
  }

  function handleClientCreated() {
    setIsCreateModalOpen(false)
    invalidate()
    setNotice('Cliente creado exitosamente.')
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
    setNotice('Cliente actualizado exitosamente.')
  }

  function openPaymentModal(client: Client, trigger: HTMLButtonElement) {
    payTriggerRef.current = trigger
    setNotice(null)
    setPayingClient(client)
  }

  async function submitClientPayment(data: { amount: number; payment_method: Exclude<PaymentMethod, null> }) {
    if (!payingClient) throw new Error('Client not loaded')
    try {
      const response = await paymentMutation.mutateAsync({ clientId: payingClient.id, data })
      return response.data
    } catch (error) {
      queryClient.invalidateQueries({ queryKey: ['clients'] })
      queryClient.invalidateQueries({ queryKey: ['client', payingClient.id] })
      throw error
    }
  }

  function handlePaymentSuccess(result: ClientPaymentResult) {
    setPayingClient(null)
    setNotice(`Pago registrado. Saldo pendiente: ${formatCurrency(result.remaining_debt)}.`)
    requestAnimationFrame(() => {
      if (result.remaining_debt > 0) payTriggerRef.current?.focus()
      else successRef.current?.focus()
    })
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

  const paymentClient = payingClient
    ? clients.find(c => c.id === payingClient.id) ?? payingClient
    : null

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

      {notice && (
        <InlineNotice ref={successRef} tone="success" onClose={() => setNotice(null)}>
          {notice}
        </InlineNotice>
      )}

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
                      <a
                        className={styles.clientName}
                        href={`#/clients/${c.id}`}
                        title={`Ver cliente ${c.name}`}
                      >
                        {c.name}
                      </a>
                      {c.identity_card && <div className={styles.clientIdCard}>ID: {c.identity_card}</div>}
                    </div>
                  </div>
                  <div className={styles.cardActions}>
                    {c.debt_amount > 0 && (
                      <button
                        type="button"
                        className={styles.payButton}
                        onClick={event => openPaymentModal(c, event.currentTarget)}
                        title="Pagar"
                        aria-label={`Pagar deuda de ${c.name}`}
                      >
                        <Wallet size={14} aria-hidden="true" />
                        <span>Pagar</span>
                      </button>
                    )}
                    <button className="action-btn" onClick={() => openEditModal(c)} title="Editar">
                      <Edit size={14} />
                    </button>
                    <button className="action-btn action-btn--destructive" onClick={() => requestClientDelete({ id: c.id, name: c.name })} title="Eliminar">
                      <Trash2 size={14} />
                    </button>
                  </div>
                </div>

                <div className={styles.detailsList}>
                  <div className={styles.debtRow}>
                    <span className={styles.dateLabel}>Saldo pendiente</span>
                    <span className={c.debt_amount > 0 ? styles.debtAmount : styles.debtSettled}>
                      {c.debt_amount > 0 ? formatCurrency(c.debt_amount) : 'Sin deuda'}
                    </span>
                  </div>
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

      {paymentClient && (
        <ClientPaymentModal
          client={paymentClient}
          onClose={() => setPayingClient(null)}
          onSubmit={submitClientPayment}
          onSuccess={handlePaymentSuccess}
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
