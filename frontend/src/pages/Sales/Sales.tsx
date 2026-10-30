import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Plus, Eye, Trash2 } from 'lucide-react'
import DataTable from '@/components/DataTable/DataTable'
import KpiStrip from '@/components/KpiStrip/KpiStrip'
import styles from './Sales.module.css'
import listStyles from '@/pages/listPage.module.css'
import PageHeader from '@/components/PageHeader/PageHeader'
import CommandBar from '@/components/CommandBar/CommandBar'
import DataCard from '@/components/DataCard/DataCard'
import DateFilterInput from '@/components/DateFilterInput/DateFilterInput'
import FilterSelect from '@/components/FilterSelect/FilterSelect'
import SearchSelect from '@/components/SearchSelect/SearchSelect'
import { clientsApi } from '@/lib/api/clients'
import { PAGE_LIMIT, paymentStatusLabel, paymentStatusBadge, deliveryStatusLabel, paymentMethodLabel } from '@/lib/constants'
import { formatCurrency, formatDate, formatNumber } from '@/lib/utils'
import { usersApi } from '@/lib/api/users'
import { useSales } from '@/hooks/useSales'
import { useDeleteConfirm } from '@/hooks/useDeleteConfirm'
import ConfirmDeleteModal from '@/components/ConfirmDeleteModal/ConfirmDeleteModal'

const fetchUsers = (search: string) =>
  usersApi.getUsers({ search: search || undefined, skip: 0, limit: PAGE_LIMIT }).then(r => (r.data.items || []).map(u => ({ id: u.id, name: u.full_name })))

const fetchClients = (search: string) =>
  clientsApi.getClients({ search: search || undefined, skip: 0, limit: PAGE_LIMIT }).then(r => r.data.items || [])

export default function Sales() {
  const navigate = useNavigate()
  const [page, setPage] = useState(1)

  const [selectedUser, setSelectedUser] = useState<number | ''>('')
  const [selectedClient, setSelectedClient] = useState<number | ''>('')
  const [noClient, setNoClient] = useState(false)
  const [selectedPaymentMethod, setSelectedPaymentMethod] = useState('')
  const [selectedDeliveryStatus, setSelectedDeliveryStatus] = useState('')
  const [selectedPaymentStatus, setSelectedPaymentStatus] = useState('')
  const [startDate, setStartDate] = useState('')
  const [endDate, setEndDate] = useState('')

  const { sales, hasNext, stats, isLoading, isFetching, isError, refetch, deleteMutation } = useSales({
    page,
    filters: {
      user_id: selectedUser,
      client_id: selectedClient,
      no_client: noClient,
      payment_method: selectedPaymentMethod,
      delivery_status: selectedDeliveryStatus,
      payment_status: selectedPaymentStatus,
      start_date: startDate,
      end_date: endDate,
    },
  })

  const {
    target: saleToDelete,
    requestDelete: requestSaleDelete,
    close: closeSaleDelete,
    confirm: confirmSaleDelete,
  } = useDeleteConfirm<{ id: number; label: string }>(deleteMutation.mutateAsync)

  const clientFilterActive = noClient || !!selectedClient

  const hasActiveFilters = !!selectedUser || clientFilterActive || !!selectedPaymentMethod || !!selectedDeliveryStatus || !!selectedPaymentStatus || !!startDate || !!endDate

  function clearFilters() {
    setSelectedUser('')
    setSelectedClient('')
    setNoClient(false)
    setSelectedPaymentMethod('')
    setSelectedDeliveryStatus('')
    setSelectedPaymentStatus('')
    setStartDate('')
    setEndDate('')
    setPage(1)
  }

  function renderCobroCell(debtAmount: number, totalAmount: number) {
    return (
      <div className={styles.cobroCell}>
        <span className={styles.cobroTotal}>
          {Number.isFinite(totalAmount) ? formatCurrency(totalAmount) : '—'}
        </span>
        {debtAmount > 0 && <span className={styles.cobroDebt}>Debe {formatCurrency(debtAmount)}</span>}
      </div>
    )
  }

  const porCobrar = Math.max(stats.totalAmountSum - stats.amountPaidSum, 0)

  return (
    <div className={listStyles.container}>
      <PageHeader
        activeLabel="Gestión"
        title="Ventas"
        action={
          <button className="btn-new" onClick={() => navigate('/sales/create')}>
            <Plus size={16} strokeWidth={2.5} />
            <span>Nueva Venta</span>
          </button>
        }
      />

      <KpiStrip cards={[
        { label: 'Total vendido', value: formatCurrency(stats.totalAmountSum) },
        { label: 'Cobrado', value: formatCurrency(stats.amountPaidSum) },
        { label: 'Por cobrar', value: formatCurrency(porCobrar), tone: porCobrar > 0 ? 'negative' : undefined },
        { label: 'Sin entregar', value: formatNumber(stats.undeliveredCount) },
      ]} />

      <div className={listStyles.tableSection}>
        <CommandBar showSearch={false} hasActiveFilters={hasActiveFilters} onClearFilters={clearFilters}>

          <SearchSelect
            label="Usuario"
            value={selectedUser}
            onChange={v => { setSelectedUser(v); setPage(1) }}
            fetchOptions={fetchUsers}
            emptyLabel="Todos los Usuarios"
          />

          <SearchSelect
            label="Cliente"
            value={noClient ? -1 : selectedClient}
            onChange={v => {
              setNoClient(v === -1)
              setSelectedClient(v === -1 ? '' : v)
              setPage(1)
            }}
            fetchOptions={fetchClients}
            emptyLabel="Todos los Clientes"
            specialOption={{ value: -1, label: 'Sin cliente' }}
          />

          <FilterSelect
            label="Método pago"
            value={selectedPaymentMethod}
            onChange={v => { setSelectedPaymentMethod(v); setPage(1) }}
            options={Object.entries(paymentMethodLabel).map(([value, label]) => ({ value, label }))}
          />

          <FilterSelect
            label="Entrega"
            value={selectedDeliveryStatus}
            onChange={v => { setSelectedDeliveryStatus(v); setPage(1) }}
            options={Object.entries(deliveryStatusLabel).map(([value, label]) => ({ value, label }))}
          />

          <FilterSelect
            label="Estado pago"
            value={selectedPaymentStatus}
            onChange={v => { setSelectedPaymentStatus(v); setPage(1) }}
            options={Object.entries(paymentStatusLabel).map(([value, label]) => ({ value, label }))}
          />

          <DateFilterInput
            value={startDate}
            onChange={v => { setStartDate(v); setPage(1) }}
            label="Fecha inicio"
          />

          <DateFilterInput
            value={endDate}
            onChange={v => { setEndDate(v); setPage(1) }}
            label="Fecha fin"
          />

        </CommandBar>

        <DataCard>
          <DataTable
            loading={isLoading}
            empty={sales.length === 0}
            error={isError && sales.length === 0}
            onRetry={refetch}
            loadingText="Cargando ventas..."
            emptyText="No se encontraron ventas."
            minWidth="940px"
            skeletonRows={5}
            skeletonColumns={8}
            stickyFirstColumn
            currentPage={page}
            hasNext={hasNext}
            onPageChange={setPage}
            isPaginationLoading={isFetching && !isLoading}
          >
            <thead>
              <tr>
                <th scope="col" className={styles.colSale}>Venta</th>
                <th scope="col" className={styles.colDate}>Fecha</th>
                <th scope="col" className={styles.colClient}>Cliente</th>
                <th scope="col" className={`${styles.colCobro} num`}>Total / Cobro</th>
                <th scope="col" className={styles.colPayment}>Estado pago</th>
                <th scope="col" className={styles.colDelivery}>Entrega</th>
                <th scope="col" className={styles.colUser}>Usuario</th>
                <th scope="col" className={`${styles.colActions} num`}>Acciones</th>
              </tr>
            </thead>
            <tbody>
              {sales.map(s => (
                <tr key={s.id}>
                  <td className={styles.colSale}>
                    <button
                      className={listStyles.codeLink}
                      onClick={() => navigate(`/sales/${s.id}`)}
                      title="Ver venta"
                      aria-label={`Ver venta #${s.id}`}
                    >
                      #{s.id}
                    </button>
                  </td>
                  <td className={`${styles.colDate} ${listStyles.dateCell}`}>
                    {s.created_at ? formatDate(s.created_at) : '—'}
                  </td>
                  <td><span className={listStyles.customerCell}>{s.client?.name ?? <span className={styles.emptyCell}>Sin cliente</span>}</span></td>
                  <td className={`${styles.colCobro} num`}>{renderCobroCell(s.debt_amount, s.total_amount)}</td>
                  <td className={styles.colPayment}>
                    <span className={'badge ' + (paymentStatusBadge[s.payment_status] ?? 'badge--neutral')}>
                      {paymentStatusLabel[s.payment_status] ?? s.payment_status}
                    </span>
                  </td>
                  <td className={styles.colDelivery}>
                    <span className={styles.deliveryText} data-status={s.delivery_status}>
                      {deliveryStatusLabel[s.delivery_status] ?? s.delivery_status}
                    </span>
                  </td>
                  <td className={styles.colUser}><span className={styles.userCell}>{s.created_by.full_name}</span></td>
                  <td className={`${styles.colActions} num`}>
                    <div className={styles.rowActions}>
                      <button
                        className="action-btn"
                        onClick={() => navigate(`/sales/${s.id}`)}
                        title="Ver venta"
                      >
                        <Eye size={14} />
                      </button>
                      <button
                        className="action-btn action-btn--destructive"
                        onClick={() => requestSaleDelete({ id: s.id, label: `Venta #${s.id}${s.client ? ` — ${s.client.name}` : ''}` })}
                        title="Eliminar venta"
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
      {saleToDelete && (
        <ConfirmDeleteModal
          title="Eliminar Venta"
          productName={saleToDelete.label}
          onConfirm={confirmSaleDelete}
          onClose={closeSaleDelete}
        />
      )}
    </div>
  )
}
