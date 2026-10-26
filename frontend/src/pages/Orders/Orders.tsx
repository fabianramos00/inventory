import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Plus, Eye, Trash2 } from 'lucide-react'
import DataTable from '@/components/DataTable/DataTable'
import listStyles from '@/pages/listPage.module.css'
import styles from './Orders.module.css'
import PageHeader from '@/components/PageHeader/PageHeader'
import CommandBar from '@/components/CommandBar/CommandBar'
import DataCard from '@/components/DataCard/DataCard'
import DateFilterInput from '@/components/DateFilterInput/DateFilterInput'
import FilterSelect from '@/components/FilterSelect/FilterSelect'
import SearchSelect from '@/components/SearchSelect/SearchSelect'
import { PAGE_LIMIT, orderStatusLabel, orderStatusBadge, orderPaymentStatusLabel } from '@/lib/constants'
import { formatCurrency, formatDate } from '@/lib/utils'
import { providersApi } from '@/lib/api/providers'
import ConfirmDeleteModal from '@/components/ConfirmDeleteModal/ConfirmDeleteModal'
import { useOrders } from '@/hooks/useOrders'
import { useDeleteConfirm } from '@/hooks/useDeleteConfirm'

const fetchProviders = (search: string) =>
  providersApi.getProviders({ search: search || undefined, skip: 0, limit: PAGE_LIMIT }).then(r => (r.data.items || []).map(p => ({ id: p.id, name: p.name })))

export default function Orders() {
  const navigate = useNavigate()
  const [page, setPage] = useState(1)

  const [selectedProvider, setSelectedProvider] = useState<number | ''>('')
  const [selectedPaymentStatus, setSelectedPaymentStatus] = useState('')
  const [selectedStatus, setSelectedStatus] = useState('')
  const [minDate, setMinDate] = useState('')
  const [maxDate, setMaxDate] = useState('')

  const { orders, hasNext, isLoading, isFetching, isError, refetch, deleteMutation } = useOrders({
    page,
    filters: {
      provider_id: selectedProvider,
      payment_status: selectedPaymentStatus,
      status: selectedStatus,
      min_date: minDate,
      max_date: maxDate,
    },
  })

  const {
    target: orderToDelete,
    requestDelete: requestOrderDelete,
    close: closeOrderDelete,
    confirm: confirmOrderDelete,
  } = useDeleteConfirm<{ id: number; label: string }>(deleteMutation.mutateAsync)

  const hasActiveFilters = !!selectedProvider || !!selectedPaymentStatus || !!selectedStatus || !!minDate || !!maxDate

  function clearFilters() {
    setSelectedProvider('')
    setSelectedPaymentStatus('')
    setSelectedStatus('')
    setMinDate('')
    setMaxDate('')
    setPage(1)
  }

  return (
    <div className={listStyles.container}>
      <PageHeader
        activeLabel="Gestión"
        title="Pedidos de Compra"
        action={
          <button className="btn-new" onClick={() => navigate('/orders/create')}>
            <Plus size={16} strokeWidth={2.5} />
            <span>Nuevo Pedido</span>
          </button>
        }
      />

      <div className={listStyles.tableSection}>
        <CommandBar showSearch={false} hasActiveFilters={hasActiveFilters} onClearFilters={clearFilters}>

          <SearchSelect
            label="Proveedor"
            value={selectedProvider}
            onChange={v => { setSelectedProvider(v); setPage(1) }}
            fetchOptions={fetchProviders}
            emptyLabel="Todos los Proveedores"
          />

          <FilterSelect
            label="Estado pago"
            value={selectedPaymentStatus}
            onChange={v => { setSelectedPaymentStatus(v); setPage(1) }}
            options={Object.entries(orderPaymentStatusLabel).map(([value, label]) => ({ value, label }))}
          />

          <FilterSelect
            label="Estado"
            value={selectedStatus}
            onChange={v => { setSelectedStatus(v); setPage(1) }}
            options={Object.entries(orderStatusLabel).filter(([value]) => value !== 'delivered').map(([value, label]) => ({ value, label }))}
          />

          <DateFilterInput
            value={minDate}
            onChange={v => { setMinDate(v); setPage(1) }}
            label="Fecha inicio"
          />

          <DateFilterInput
            value={maxDate}
            onChange={v => { setMaxDate(v); setPage(1) }}
            label="Fecha fin"
          />

        </CommandBar>

        <DataCard>
          <DataTable
            loading={isLoading}
            empty={orders.length === 0}
            error={isError && orders.length === 0}
            onRetry={refetch}
            loadingText="Cargando pedidos..."
            emptyText="No se encontraron pedidos."
            minWidth="640px"
            skeletonRows={5}
            skeletonColumns={5}
            stickyFirstColumn
            currentPage={page}
            hasNext={hasNext}
            onPageChange={setPage}
            isPaginationLoading={isFetching && !isLoading}
          >
            <thead>
              <tr>
                <th scope="col">Pedido</th>
                <th scope="col">Proveedor</th>
                <th scope="col" className="num">Total</th>
                <th scope="col">Estado</th>
                <th scope="col" className="num">Acciones</th>
              </tr>
            </thead>
            <tbody>
              {orders.map(o => (
                <tr key={o.id}>
                  <td>
                    <button className={listStyles.codeLink} onClick={() => navigate(`/orders/${o.id}`)} title="Ver pedido">
                      #{o.id}
                    </button>
                    <span className={styles.cellSubline}>{formatDate(o.order_date)}</span>
                  </td>
                  <td><span className={listStyles.customerCell}>{o.provider.name}</span></td>
                  <td className="num"><span className={styles.totalCell}>{formatCurrency(o.total_amount)}</span></td>
                  <td>
                    <div className={styles.statusCell}>
                      <span className={'badge ' + (orderStatusBadge[o.status] ?? 'badge--neutral')}>
                        {orderStatusLabel[o.status] ?? o.status}
                      </span>
                      <span className={`${styles.payLine} ${o.payment_status === 'paid' ? styles.payPaid : ''}`}>
                        Pago: {orderPaymentStatusLabel[o.payment_status] ?? o.payment_status}
                      </span>
                    </div>
                  </td>
                  <td className="num">
                    <div className={styles.rowActions}>
                      <button
                        className="action-btn"
                        onClick={() => navigate(`/orders/${o.id}`)}
                        title="Ver pedido"
                      >
                        <Eye size={14} />
                      </button>
                      <button
                        className="action-btn action-btn--destructive"
                        onClick={() => requestOrderDelete({ id: o.id, label: `Pedido #${o.id} — ${o.provider.name}` })}
                        title="Eliminar pedido"
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
      {orderToDelete && (
        <ConfirmDeleteModal
          title="Eliminar Pedido"
          productName={orderToDelete.label}
          onConfirm={confirmOrderDelete}
          onClose={closeOrderDelete}
        />
      )}
    </div>
  )
}
