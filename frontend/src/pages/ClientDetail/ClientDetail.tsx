import { useRef, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import axios from 'axios'
import { ChevronLeft, Eye, Mail, Phone, Wallet } from 'lucide-react'
import styles from './ClientDetail.module.css'
import listStyles from '@/pages/listPage.module.css'
import CommandBar from '@/components/CommandBar/CommandBar'
import DataCard from '@/components/DataCard/DataCard'
import DataTable from '@/components/DataTable/DataTable'
import DateFilterInput from '@/components/DateFilterInput/DateFilterInput'
import ErrorState from '@/components/ErrorState/ErrorState'
import FilterSelect from '@/components/FilterSelect/FilterSelect'
import CreateFormModal from '@/components/CreateFormModal/CreateFormModal'
import ClientPaymentModal from '@/components/ClientPaymentModal/ClientPaymentModal'
import InlineNotice from '@/components/InlineNotice/InlineNotice'
import { clientsApi } from '@/lib/api/clients'
import { CLIENT_FIELDS } from '@/lib/formFields'
import { deliveryStatusLabel, paymentStatusBadge, paymentStatusLabel } from '@/lib/constants'
import { formatCurrency, formatDate } from '@/lib/utils'
import { useClient } from '@/hooks/useClient'
import { useSales } from '@/hooks/useSales'
import type { ClientPaymentResult } from '@/types'

const PAYMENT_STATUS_OPTIONS = [
  { value: 'pending', label: 'Pendiente' },
  { value: 'partial', label: 'Parcial' },
  { value: 'paid', label: 'Pagada' },
]

const DELIVERY_STATUS_OPTIONS = [
  { value: 'pending', label: 'Pendiente' },
  { value: 'partial', label: 'Parcial' },
  { value: 'delivered', label: 'Entregado' },
]

export default function ClientDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { client, isLoading, error, refetch, invalidate, paymentMutation } = useClient(id)

  const [page, setPage] = useState(1)
  const [paymentStatus, setPaymentStatus] = useState('')
  const [deliveryStatus, setDeliveryStatus] = useState('')
  const [startDate, setStartDate] = useState('')
  const [endDate, setEndDate] = useState('')

  const [isEditOpen, setIsEditOpen] = useState(false)
  const [isPaymentOpen, setIsPaymentOpen] = useState(false)
  const [announcement, setAnnouncement] = useState<string | null>(null)
  const payButtonRef = useRef<HTMLButtonElement>(null)
  const successRef = useRef<HTMLDivElement>(null)

  const dateRangeInvalid = !!startDate && !!endDate && startDate > endDate
  const hasActiveFilters = !!paymentStatus || !!deliveryStatus || !!startDate || !!endDate

  const { sales, hasNext, isLoading: salesLoading, isFetching, isError: salesError, refetch: refetchSales } = useSales({
    page,
    filters: {
      client_id: id ? Number(id) : '',
      payment_status: paymentStatus,
      delivery_status: deliveryStatus,
      start_date: dateRangeInvalid ? '' : startDate,
      end_date: dateRangeInvalid ? '' : endDate,
    },
  })

  function clearFilters() {
    setPaymentStatus('')
    setDeliveryStatus('')
    setStartDate('')
    setEndDate('')
    setPage(1)
  }

  async function submitEditClient(values: Record<string, string>) {
    if (!client) throw new Error('Client not loaded')
    const response = await clientsApi.updateClient(client.id, {
      name: values.name,
      identity_card: values.identity_card || null,
      email: values.email || null,
      phone: values.phone || null,
    })
    return response.data
  }

  function handleClientEdited() {
    setIsEditOpen(false)
    invalidate()
    setAnnouncement('Cliente actualizado exitosamente.')
  }

  function handlePaymentSuccess(result: ClientPaymentResult) {
    setIsPaymentOpen(false)
    setAnnouncement(`Pago registrado. Saldo pendiente: ${formatCurrency(result.remaining_debt)}.`)
    requestAnimationFrame(() => {
      if (result.remaining_debt > 0) payButtonRef.current?.focus()
      else successRef.current?.focus()
    })
  }

  async function submitClientPayment(data: { amount: number; payment_method: 'cash' | 'credit' | 'debit' }) {
    try {
      const response = await paymentMutation.mutateAsync(data)
      return response.data
    } catch (error) {
      invalidate()
      throw error
    }
  }

  const notFound = axios.isAxiosError(error) && error.response?.status === 404

  return (
    <div className={styles.container}>
      <button type="button" className={styles.backLink} onClick={() => navigate('/clients')}>
        <ChevronLeft size={18} />
        Volver a clientes
      </button>

      {isLoading ? (
        <section className={styles.headerCard} aria-label="Cargando cliente">
          <div className={styles.skeletonLineWide} />
          <div className={styles.skeletonLine} />
          <div className={styles.skeletonLineShort} />
        </section>
      ) : notFound ? (
        <div className={styles.emptyPanel}>
          <p className={styles.emptyTitle}>Cliente no encontrado</p>
          <button type="button" className="btn-ghost" onClick={() => navigate('/clients')}>
            Volver a clientes
          </button>
        </div>
      ) : !client ? (
        <ErrorState message="No se pudo cargar el cliente." onRetry={() => { void refetch() }} />
      ) : (        <>
          <section className={styles.headerCard}>
            <div className={styles.identity}>
              <h1 className={styles.title}>{client.name}</h1>
              <div className={styles.metaList}>
                <span className={styles.metaItem}>{client.identity_card ? `CC ${client.identity_card}` : '—'}</span>
                <span className={styles.metaItem}><Phone size={13} />{client.phone ?? '—'}</span>
                <span className={styles.metaItem}><Mail size={13} />{client.email ?? '—'}</span>
                <span className={styles.metaItem}>Creado {formatDate(client.created_at)}</span>
              </div>
            </div>

            <div className={styles.debtBlock}>
              <span className={styles.debtLabel}>Saldo pendiente</span>
              <span className={client.debt_amount > 0 ? styles.debtAmount : styles.debtSettled}>
                {client.debt_amount > 0 ? formatCurrency(client.debt_amount) : 'Sin deuda'}
              </span>
            </div>

            <div className={styles.headerActions}>
              <button type="button" className="btn-ghost" onClick={() => setIsEditOpen(true)}>
                Editar cliente
              </button>
              {client.debt_amount > 0 && (
                <button
                  ref={payButtonRef}
                  type="button"
                  className="btn-primary"
                  onClick={() => setIsPaymentOpen(true)}
                >
                  <Wallet size={15} />
                  Pagar deuda
                </button>
              )}
            </div>
          </section>

          {announcement && (
            <InlineNotice ref={successRef} tone="success" onClose={() => setAnnouncement(null)}>
              {announcement}
            </InlineNotice>
          )}

          <section className={styles.salesSection}>
            <h2 className={styles.sectionTitle}>Ventas del cliente</h2>

            <CommandBar
              showSearch={false}
              hasActiveFilters={hasActiveFilters}
              onClearFilters={clearFilters}
            >
              <FilterSelect
                label="Estado de pago"
                value={paymentStatus}
                onChange={value => { setPaymentStatus(value); setPage(1) }}
                options={PAYMENT_STATUS_OPTIONS}
                emptyLabel="Todas"
              />
              <FilterSelect
                label="Estado de entrega"
                value={deliveryStatus}
                onChange={value => { setDeliveryStatus(value); setPage(1) }}
                options={DELIVERY_STATUS_OPTIONS}
                emptyLabel="Todos"
              />
              <DateFilterInput
                value={startDate}
                onChange={value => { setStartDate(value); setPage(1) }}
                label="Desde"
              />
              <DateFilterInput
                value={endDate}
                onChange={value => { setEndDate(value); setPage(1) }}
                label="Hasta"
              />
            </CommandBar>

            {dateRangeInvalid && (
              <p className={styles.dateError} role="alert">
                La fecha inicial no puede ser posterior a la fecha final.
              </p>
            )}

            <DataCard>
              <DataTable
                loading={salesLoading}
                empty={sales.length === 0}
                error={salesError && sales.length === 0}
                errorText="No se pudieron cargar las ventas."
                onRetry={refetchSales}
                loadingText="Cargando ventas..."
                emptyText={hasActiveFilters
                  ? 'Este cliente no tiene ventas con estos filtros.'
                  : 'Este cliente aún no tiene ventas.'}
                emptyAction={hasActiveFilters
                  ? <button type="button" className="btn-ghost" onClick={clearFilters}>Limpiar filtros</button>
                  : undefined}
                minWidth="860px"
                skeletonRows={5}
                skeletonColumns={7}
                currentPage={page}
                hasNext={hasNext}
                onPageChange={setPage}
                isPaginationLoading={isFetching && !salesLoading}
              >
                <thead>
                  <tr>
                    <th scope="col" className={styles.colDate}>Fecha</th>
                    <th scope="col" className={`${styles.colMoney} num`}>Total</th>
                    <th scope="col" className={`${styles.colMoney} num`}>Pagado</th>
                    <th scope="col" className={`${styles.colMoney} num`}>Saldo</th>
                    <th scope="col" className={styles.colStatus}>Estado pago</th>
                    <th scope="col" className={styles.colStatus}>Estado entrega</th>
                    <th scope="col" className={styles.colAction}>Ver</th>
                  </tr>
                </thead>
                <tbody>
                  {sales.map(sale => (
                    <tr
                      key={sale.id}
                      className={styles.saleRow}
                      onClick={() => navigate(`/sales/${sale.id}`)}
                    >
                      <td className={`${styles.colDate} ${listStyles.dateCell}`}>
                        {formatDate(sale.created_at)}
                      </td>
                      <td className={`${styles.colMoney} ${listStyles.totalCell}`}>{formatCurrency(sale.total_amount)}</td>
                      <td className={`${styles.colMoney} ${listStyles.totalCell}`}>{formatCurrency(sale.amount_paid)}</td>
                      <td className={`${styles.colMoney} ${sale.debt_amount > 0 ? styles.debtCell : listStyles.totalCell}`}>
                        {formatCurrency(sale.debt_amount)}
                      </td>
                      <td className={styles.colStatus}>
                        <span className={'badge ' + (paymentStatusBadge[sale.payment_status] ?? 'badge--neutral')}>
                          {paymentStatusLabel[sale.payment_status] ?? sale.payment_status}
                        </span>
                      </td>
                      <td className={styles.colStatus}>
                        <span className={styles.deliveryText} data-status={sale.delivery_status}>
                          {deliveryStatusLabel[sale.delivery_status] ?? sale.delivery_status}
                        </span>
                      </td>
                      <td className={styles.colAction}>
                        <button
                          type="button"
                          className={styles.viewButton}
                          onClick={event => { event.stopPropagation(); navigate(`/sales/${sale.id}`) }}
                          aria-label={`Ver venta #${sale.id}`}
                        >
                          <Eye size={14} />
                          Ver
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </DataTable>
            </DataCard>
          </section>
        </>
      )}

      {isEditOpen && client && (
        <CreateFormModal
          title={`Editar Cliente: ${client.name}`}
          fields={CLIENT_FIELDS}
          initialValues={{
            name: client.name,
            identity_card: client.identity_card || '',
            email: client.email || '',
            phone: client.phone || '',
          }}
          onSubmit={submitEditClient}
          onClose={() => setIsEditOpen(false)}
          onCreated={handleClientEdited}
        />
      )}

      {isPaymentOpen && client && (
        <ClientPaymentModal
          client={client}
          onClose={() => setIsPaymentOpen(false)}
          onSubmit={submitClientPayment}
          onSuccess={handlePaymentSuccess}
        />
      )}
    </div>
  )
}
