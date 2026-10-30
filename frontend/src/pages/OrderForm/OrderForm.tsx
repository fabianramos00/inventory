import { useState, useEffect, useRef } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { ChevronLeft, Search, X, Loader, Package, Pencil, Check, Trash2, Plus } from 'lucide-react'
import { useQueryClient } from '@tanstack/react-query'
import styles from '../formPage.module.css'
import { ordersApi } from '@/lib/api/orders'
import { inventoryApi } from '@/lib/api/inventory'
import { providersApi } from '@/lib/api/providers'
import ConfirmDeleteModal from '@/components/ConfirmDeleteModal/ConfirmDeleteModal'
import CreateFormModal from '@/components/CreateFormModal/CreateFormModal'
import SearchSelect from '@/components/SearchSelect/SearchSelect'
import Dropdown from '@/components/Dropdown/Dropdown'
import { orderStatusLabel, orderStatusBadge, orderPaymentStatusLabel, orderPaymentStatusBadge } from '@/lib/constants'
import { formatDate, formatCurrency } from '@/lib/utils'
import { getAPIErrorMessage } from '@/lib/apiError'
import {
  invalidateOrderDetails,
  invalidateOrderItems,
  invalidateOrderListData,
} from '@/lib/orderCache'
import { useOrder } from '@/hooks/useOrder'
import { useDebouncedFetch } from '@/hooks/useDebouncedFetch'
import { useClickOutside } from '@/hooks/useClickOutside'
import { PROVIDER_FIELDS, buildProviderCreatePayload } from '@/lib/formFields'
import {
  buildAddOrderItemPayload,
  buildCreateOrderPayload,
  buildUpdateOrderItemPayload,
  buildUpdateOrderPayload,
} from './orderPayload'
import type { Product, OrderItem, OrderStatus, OrderPaymentStatus, FilterOption } from '@/types'

interface OrderItemDraft {
  id?: number
  product_id: number
  product_name: string
  product_sku: string
  product_category: string
  product_brand: string
  product_material: string
  quantity: string
  unit_cost: string
  supplier_sku: string
}

const ORDER_STATUS_OPTIONS = (['pending', 'sent', 'received', 'cancelled'] as const)
  .map(value => ({ value, label: orderStatusLabel[value] }))

const ORDER_PAYMENT_STATUS_OPTIONS = (['pending', 'paid'] as const)
  .map(value => ({ value, label: orderPaymentStatusLabel[value] }))

export default function OrderForm() {
  const navigate = useNavigate()
  const { id } = useParams<{ id?: string }>()
  const isCreateMode = !id
  const queryClient = useQueryClient()

  const { order, orderItems, isLoading: pageLoading, error: orderError } = useOrder(id)

  useEffect(() => {
    if (!isCreateMode && orderError) navigate('/orders')
  }, [isCreateMode, orderError, navigate])

  const [items, setItems] = useState<OrderItemDraft[]>([])
  const [isEditingItems, setIsEditingItems] = useState(false)
  const itemsEditable = isCreateMode || isEditingItems

  const [isEditingDetails, setIsEditingDetails] = useState(false)
  const detailsEditable = isCreateMode || isEditingDetails
  const [isSaving, setIsSaving] = useState(false)
  const [isSubmitting, setIsSubmitting] = useState(false)

  const [savingItemId, setSavingItemId] = useState<number | null>(null)
  const [deletingItemId, setDeletingItemId] = useState<number | null>(null)

  const [showDeleteModal, setShowDeleteModal] = useState(false)
  const [showCreateProvider, setShowCreateProvider] = useState(false)

  const [providerId, setProviderId] = useState<number | ''>('')
  const [selectedProvider, setSelectedProvider] = useState<FilterOption | null>(null)

  const [status, setStatus] = useState<OrderStatus>('pending')
  const [paymentStatus, setPaymentStatus] = useState<OrderPaymentStatus>('pending')

  const [productSearch, setProductSearch] = useState('')
  const [productSearchOpen, setProductSearchOpen] = useState(false)
  const productSearchRef = useRef<HTMLDivElement>(null)

  const [apiError, setApiError] = useState<string | null>(null)
  const [validationError, setValidationError] = useState<string | null>(null)

  const { data: productResults, loading: productLoading } = useDebouncedFetch(
    () => inventoryApi.getProducts({ search: productSearch, skip: 0, limit: 8 }).then(res => res.data.items || []),
    [productSearch],
    300,
    itemsEditable && productSearch.trim() !== '',
  )

  useClickOutside(productSearchRef, () => setProductSearchOpen(false), productSearchOpen)

  function handleStartEditItems() {
    setItems(orderItems.map(item => ({
      id: item.id,
      product_id: item.product_id,
      product_name: item.product_name,
      product_sku: item.product_sku,
      product_category: '',
      product_brand: '',
      product_material: '',
      quantity: String(item.quantity),
      unit_cost: String(item.unit_cost),
      supplier_sku: item.supplier_sku ?? '',
    })))
    setIsEditingItems(true)
  }

  function handleCancelEditItems() {
    setIsEditingItems(false)
    setItems([])
    setProductSearch('')
    setProductSearchOpen(false)
  }

  function handleStartEditDetails() {
    if (!order) return
    setProviderId(order.provider_id)
    setSelectedProvider({ id: order.provider.id, name: order.provider.name })
    setStatus(order.status)
    setPaymentStatus(order.payment_status)
    setIsEditingDetails(true)
  }

  function handleCancelEditDetails() {
    setIsEditingDetails(false)
  }

  async function handleSaveDetails() {
    if (!order) return
    setIsSaving(true)
    try {
      const res = await ordersApi.updateOrder(order.id, buildUpdateOrderPayload({
        providerId,
        status,
        paymentStatus,
      }))
      queryClient.setQueryData(['order', order.id], res.data)
      const becameReceived = order.status !== 'received' && status === 'received'
      invalidateOrderDetails(queryClient, order.id, becameReceived)
      setIsEditingDetails(false)
    } catch (error) {
      setApiError(getAPIErrorMessage(error, 'Error al guardar los detalles del pedido. Intenta de nuevo.'))
    } finally {
      setIsSaving(false)
    }
  }

  async function handleDeleteOrder() {
    if (!order) return
    await ordersApi.deleteOrder(order.id)
    invalidateOrderListData(queryClient)
    navigate('/orders')
  }

  async function submitCreateProvider(values: Record<string, string>) {
    const res = await providersApi.createProvider(buildProviderCreatePayload(values))
    return { id: res.data.id, name: res.data.name }
  }

  function handleProviderCreated(provider: { id: number; name: string }) {
    setSelectedProvider(provider)
    setProviderId(provider.id)
    setShowCreateProvider(false)
  }

  function handleAddItem(product: Product) {
    setProductSearch('')
    setProductSearchOpen(false)
    if (items.some(i => i.product_id === product.id)) return
    setItems(prev => [...prev, {
      product_id: product.id,
      product_name: product.name,
      product_sku: product.sku,
      product_category: product.category?.name ?? '',
      product_brand: product.brand?.name ?? '',
      product_material: product.material?.name ?? '',
      quantity: '1',
      unit_cost: product.cost ? String(product.cost) : '',
      supplier_sku: '',
    }])
  }

  function updateItem(productId: number, field: 'quantity' | 'unit_cost' | 'supplier_sku', value: string) {
    setItems(prev => prev.map(item => item.product_id === productId ? { ...item, [field]: value } : item))
  }

  async function handleRemoveItem(productId: number) {
    const item = items.find(i => i.product_id === productId)
    if (!item) return
    if (!item.id) {
      setItems(prev => prev.filter(i => i.product_id !== productId))
      return
    }
    if (!order) return
    setDeletingItemId(item.id)
    try {
      const res = await ordersApi.deleteOrderItem(order.id, item.id)
      queryClient.setQueryData(['order', order.id], res.data)
      queryClient.setQueryData<OrderItem[]>(['orderItems', order.id], (prev) =>
        (prev ?? []).filter(orderItem => orderItem.id !== item.id)
      )
      invalidateOrderItems(queryClient, order.id)
      setItems(prev => prev.filter(i => i.product_id !== productId))
    } catch (error) {
      setApiError(getAPIErrorMessage(error, 'Error al eliminar el artículo. Intenta de nuevo.'))
    } finally {
      setDeletingItemId(null)
    }
  }

  async function handleSaveItem(productId: number) {
    const item = items.find(i => i.product_id === productId)
    if (!item || !order) return

    if (item.id) {
      setSavingItemId(item.id)
      try {
        const payload = buildUpdateOrderItemPayload(item)
        const res = await ordersApi.updateOrderItem(order.id, item.id, payload)
        queryClient.setQueryData(['order', order.id], res.data)
        const quantity = payload.quantity
        const cost = payload.unit_cost
        queryClient.setQueryData<OrderItem[]>(['orderItems', order.id], (prev) =>
          (prev ?? []).map(orderItem => orderItem.id === item.id ? {
            ...orderItem,
            quantity,
            unit_cost: cost,
            subtotal: quantity * cost,
            supplier_sku: payload.supplier_sku,
          } : orderItem)
        )
        invalidateOrderItems(queryClient, order.id)
      } catch (error) {
        setApiError(getAPIErrorMessage(error, 'Error al guardar el artículo. Intenta de nuevo.'))
      } finally {
        setSavingItemId(null)
      }
      return
    }

    try {
      const res = await ordersApi.addOrderItem(order.id, buildAddOrderItemPayload(item))
      queryClient.setQueryData(['order', order.id], res.data)
      const itemsRes = await ordersApi.getOrderItems(order.id, { skip: 0, limit: 100 })
      const freshItems = itemsRes.data.items
      queryClient.setQueryData(['orderItems', order.id], freshItems)
      invalidateOrderItems(queryClient, order.id)
      const newItem = freshItems.find(orderItem => orderItem.product_id === productId)
      if (newItem) {
        setItems(prev => prev.map(i => i.product_id === productId ? { ...i, id: newItem.id } : i))
      }
    } catch (error) {
      setApiError(getAPIErrorMessage(error, 'Error al agregar el artículo. Intenta de nuevo.'))
    }
  }

  const draftTotal = items.reduce((acc, item) => {
    const cost = item.unit_cost !== '' ? parseFloat(item.unit_cost) || 0 : 0
    return acc + cost * Number(item.quantity)
  }, 0)

  const displayTotal = isCreateMode ? draftTotal : isEditingItems ? draftTotal : order?.total_amount ?? 0

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!isCreateMode) return
    setValidationError(null)
    setApiError(null)

    if (!providerId) {
      setValidationError('Selecciona un proveedor.')
      return
    }
    if (items.length === 0) {
      setValidationError('Agrega al menos un producto al pedido.')
      return
    }
    for (const item of items) {
      if (!item.quantity || Number(item.quantity) < 1) {
        setValidationError('Todos los ítems deben tener una cantidad mayor a 0.')
        return
      }
    }

    setIsSubmitting(true)
    try {
      await ordersApi.createOrder(buildCreateOrderPayload({
        providerId: providerId as number,
        status,
        paymentStatus,
        items,
      }))
      invalidateOrderListData(queryClient)
      navigate('/orders')
    } catch (error) {
      setApiError(getAPIErrorMessage(error, 'Error al crear el pedido.'))
    } finally {
      setIsSubmitting(false)
    }
  }

  if (!isCreateMode && (pageLoading || !order)) {
    return (
      <div className={styles.pageContainer}>
        <div className={styles.loadingState}>
          <Loader size={24} className={styles.spinner} />
          <p>Cargando pedido...</p>
        </div>
      </div>
    )
  }

  return (
    <form onSubmit={handleSubmit} className={styles.pageContainer}>
      <div className={styles.pageHeader}>
        <div className={styles.headerLeft}>
          <button type="button" onClick={() => navigate('/orders')} className={styles.backButton} title="Volver a pedidos">
            <ChevronLeft size={20} />
          </button>
          <div className={styles.headerContent}>
            <div className={styles.breadcrumb}>
              <span>Pedidos</span>
              <span className={styles.breadcrumbDivider}>/</span>
              <span className={styles.breadcrumbActive}>
                {isCreateMode ? 'Nuevo pedido' : `Pedido #${order?.id}`}
              </span>
            </div>
            <div className={styles.titleRow}>
              <h1 className={styles.pageTitle}>
                {isCreateMode ? 'Crear pedido' : (order?.provider.name ?? '')}
              </h1>
              {!isCreateMode && order && (
                <span className={`badge ${orderStatusBadge[order.status] ?? 'badge--warning'}`}>
                  {orderStatusLabel[order.status] ?? order.status}
                </span>
              )}
            </div>
          </div>
        </div>
        {isCreateMode && (
          <div className={styles.headerActions}>
            <button type="button" onClick={() => navigate('/orders')} className="btn-ghost">
              Cancelar
            </button>
            <button type="submit" disabled={isSubmitting} className={`btn-primary ${styles.btnDisabled}`}>
              {isSubmitting ? <><Loader size={15} className={styles.spinnerDark} /> Guardando...</> : 'Guardar Pedido'}
            </button>
          </div>
        )}
      </div>

      {apiError && <div className={styles.errorBox} role="alert">{apiError}</div>}
      {validationError && <div className={styles.errorBox} role="alert">{validationError}</div>}

      <div className={styles.sectionsGrid}>
        <section className={styles.card}>
          {isCreateMode ? (
            <h2 className={styles.cardTitle}>Productos</h2>
          ) : (
            <div className={styles.cardHeader}>
              <h2 className={styles.cardTitle}>Productos</h2>
              <div className={styles.cardActions}>
                {isEditingItems ? (
                  <button type="button" onClick={handleCancelEditItems} className="btn-ghost">
                    <X size={15} />
                    Cancelar
                  </button>
                ) : (
                  <button type="button" onClick={handleStartEditItems} className="btn-ghost">
                    <Pencil size={15} />
                    Editar
                  </button>
                )}
              </div>
            </div>
          )}

          {itemsEditable && (
            <div className={styles.productSearchWrapper} ref={productSearchRef}>
              <div className={styles.productSearchField}>
                <Search size={14} className={styles.searchIcon} />
                <input
                  type="text"
                  className={styles.productSearchInput}
                  placeholder={isCreateMode ? 'Buscar producto por nombre o SKU...' : 'Agregar producto por nombre o SKU...'}
                  value={productSearch}
                  onChange={e => {
                    setProductSearch(e.target.value)
                    setProductSearchOpen(e.target.value.trim() !== '')
                  }}
                  autoComplete="off"
                />
                {productLoading && <Loader size={13} className={styles.searchSpinner} />}
              </div>
              {productSearchOpen && (
                <div className={styles.productResults}>
                  {productLoading ? (
                    <div className={`${styles.productOption} ${styles.optionLoading}`}>
                      <Loader size={14} className={styles.spinner} />
                    </div>
                  ) : productResults.length === 0 ? (
                    <div className={`${styles.productOption} ${styles.productOptionEmpty}`}>Sin resultados</div>
                  ) : (
                    productResults.map(product => (
                      <button
                        key={product.id}
                        type="button"
                        className={`${styles.productOption} ${items.some(i => i.product_id === product.id) ? styles.productOptionAdded : ''}`}
                        onClick={() => handleAddItem(product)}
                      >
                        <div className={styles.productOptionInfo}>
                          <span className={styles.productOptionName}>{product.name}</span>
                          {product.size_value && <span className={styles.productOptionSize}>{product.size_value} {product.measurement_unit?.abbreviation}</span>}
                          <div className={styles.productOptionTags}>
                            {product.category?.name && <span className={styles.productOptionTag}>{product.category.name}</span>}
                            {product.brand?.name && <span className={styles.productOptionTag}>{product.brand.name}</span>}
                            {product.material?.name && <span className={styles.productOptionTag}>{product.material.name}</span>}
                          </div>
                        </div>
                        <div className={styles.productOptionMeta}>
                          <span className={styles.productOptionSku}>{product.sku}</span>
                          <span className={styles.productOptionPrice}>$ {(product.cost ?? 0).toFixed(2)}</span>
                        </div>
                      </button>
                    ))
                  )}
                </div>
              )}
            </div>
          )}

          {(isCreateMode ? items.length === 0 : orderItems.length === 0 && items.length === 0) ? (
            <div className={styles.emptyItems}>
              <Package size={28} className={styles.emptyIcon} />
              <p>{isCreateMode ? 'Busca y selecciona productos para agregar al pedido' : 'Sin productos registrados'}</p>
            </div>
          ) : (
            <div className={styles.itemsList}>
              {itemsEditable ? items.map(item => {
                const isPending = !isCreateMode && !item.id
                return (
                  <div key={item.product_id} className={`${styles.itemCard} ${isPending ? styles.itemCardPending : ''}`}>
                    <div className={styles.itemHeader}>
                      <div className={styles.itemIdentity}>
                        <div className={styles.itemNameRow}>
                          <span className={styles.itemName}>{item.product_name}</span>
                          {item.product_sku && <span className={styles.itemSku}>{item.product_sku}</span>}
                        </div>
                        {(!item.id || isCreateMode) && (
                          <div className={styles.itemTags}>
                            {item.product_category && <span className={styles.itemTag}>{item.product_category}</span>}
                            {item.product_brand && <span className={styles.itemTag}>{item.product_brand}</span>}
                            {item.product_material && <span className={styles.itemTag}>{item.product_material}</span>}
                          </div>
                        )}
                      </div>
                      <div className={styles.itemEditActions}>
                        {!isCreateMode && (
                          <button
                            type="button"
                            className={styles.itemSaveBtn}
                            onClick={() => handleSaveItem(item.product_id)}
                            disabled={savingItemId === item.id || deletingItemId === item.id}
                            title="Guardar cambios"
                          >
                            {savingItemId === item.id
                              ? <Loader size={13} className={styles.spinner} />
                              : <Check size={13} />}
                          </button>
                        )}
                        <button
                          type="button"
                          className={styles.removeBtn}
                          onClick={() => handleRemoveItem(item.product_id)}
                          disabled={deletingItemId === item.id || savingItemId === item.id}
                          title={isCreateMode ? 'Quitar producto' : item.id ? 'Eliminar ítem' : 'Descartar'}
                        >
                          {deletingItemId === item.id
                            ? <Loader size={13} className={styles.spinner} />
                            : <X size={14} />}
                        </button>
                      </div>
                    </div>
                    <div className={styles.itemFieldsRow}>
                      <div className={styles.itemField}>
                        <label className={styles.itemFieldLabel}>Cantidad</label>
                        <input
                          type="number"
                          className={`${styles.inputField} ${styles.mono}`}
                          min={1}
                          value={item.quantity}
                          onChange={e => updateItem(item.product_id, 'quantity', e.target.value)}
                        />
                      </div>
                      <div className={styles.itemField}>
                        <label className={styles.itemFieldLabel}>Costo unit.</label>
                        <input
                          type="number"
                          className={`${styles.inputField} ${styles.mono}`}
                          min={0}
                          step="0.01"
                          placeholder={isCreateMode ? '0.00' : undefined}
                          value={item.unit_cost}
                          onChange={e => updateItem(item.product_id, 'unit_cost', e.target.value)}
                        />
                      </div>
                      <div className={styles.itemField}>
                        <label className={styles.itemFieldLabel}>SKU prov.</label>
                        <input
                          type="text"
                          className={styles.inputField}
                          placeholder="Opcional"
                          value={item.supplier_sku}
                          onChange={e => updateItem(item.product_id, 'supplier_sku', e.target.value)}
                        />
                      </div>
                    </div>
                  </div>
                )
              }) : orderItems.map(item => (
                <div key={item.id} className={styles.itemCard}>
                  <div className={styles.itemHeader}>
                    <div className={styles.itemIdentity}>
                      <div className={styles.itemNameRow}>
                        <span className={styles.itemName}>{item.product_name}</span>
                        {item.product_sku && <span className={styles.itemSku}>{item.product_sku}</span>}
                      </div>
                    </div>
                  </div>
                  <div className={styles.itemFieldsRowFour}>
                    <div className={styles.itemField}>
                      <span className={styles.itemFieldLabel}>Cantidad</span>
                      <span className={styles.mono}>{item.quantity}</span>
                    </div>
                    <div className={styles.itemField}>
                      <span className={styles.itemFieldLabel}>Costo unit.</span>
                      <span className={styles.mono}>$ {item.unit_cost.toFixed(2)}</span>
                    </div>
                    <div className={styles.itemField}>
                      <span className={styles.itemFieldLabel}>SKU prov.</span>
                      <span className={`${styles.mono} ${!item.supplier_sku ? styles.emptyFieldValue : ''}`}>
                        {item.supplier_sku || '—'}
                      </span>
                    </div>
                    <div className={styles.itemField}>
                      <span className={styles.itemFieldLabel}>Subtotal</span>
                      <span className={styles.mono}>$ {item.subtotal.toFixed(2)}</span>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </section>

        <div className={styles.rightColumn}>
          <section className={styles.receipt}>
            <div className={styles.receiptHead}>
              <h2 className={styles.receiptTitle}>Resumen</h2>
              <span className={styles.receiptMeta}>{isCreateMode ? 'Borrador' : `Pedido #${order?.id}`}</span>
            </div>
            <dl className={styles.receiptRows}>
              <div className={styles.receiptRow}>
                <dt className={styles.receiptLabel}>Ítems</dt>
                <dd className={styles.receiptValue}>{isCreateMode ? items.length : orderItems.length}</dd>
              </div>
            </dl>
            <div className={styles.receiptTotalRow}>
              <span className={styles.receiptTotalLabel}>{isCreateMode ? 'Total estimado' : 'Total'}</span>
              <span className={styles.receiptTotalValue}>{formatCurrency(displayTotal)}</span>
            </div>
          </section>

          <section className={styles.card}>
            {isCreateMode ? (
              <h2 className={styles.cardTitle}>Detalles del pedido</h2>
            ) : (
              <div className={styles.cardHeader}>
                <h2 className={styles.cardTitle}>Detalles del pedido</h2>
                <div className={styles.cardActions}>
                  {isEditingDetails ? (
                    <>
                      <button type="button" onClick={handleCancelEditDetails} className="btn-ghost">
                        <X size={15} />
                        Cancelar
                      </button>
                      <button
                        type="button"
                        onClick={handleSaveDetails}
                        disabled={isSaving || !providerId}
                        className={`btn-ghost ${styles.btnDisabled}`}
                      >
                        {isSaving ? <><Loader size={15} className={styles.spinner} /> Guardando...</> : <><Check size={15} /> Guardar</>}
                      </button>
                    </>
                  ) : (
                    <button type="button" onClick={handleStartEditDetails} className="btn-ghost">
                      <Pencil size={15} />
                      Editar
                    </button>
                  )}
                </div>
              </div>
            )}
            <dl className={styles.fieldList}>
              <div className={detailsEditable ? styles.fieldRowEditing : styles.fieldRow}>
                <dt className={styles.fieldLabel}>{isCreateMode ? 'Proveedor *' : 'Proveedor'}</dt>
                {detailsEditable ? (
                  <dd className={styles.fieldValueFull}>
                    <div className={styles.selectWithAction}>
                      <SearchSelect
                        label="Proveedor"
                        placeholder="Sin proveedor"
                        emptyLabel="Sin proveedor"
                        className={styles.formSelect}
                        value={providerId}
                        onChange={(value, option) => {
                          setProviderId(value)
                          setSelectedProvider(option ?? null)
                        }}
                        fetchOptions={search =>
                          providersApi.getProviders({ search: search || undefined, skip: 0, limit: 10 })
                            .then(res => res.data.items || [])
                        }
                        selectedOption={selectedProvider ?? undefined}
                        ariaLabel={`Proveedor: ${selectedProvider?.name ?? 'Sin proveedor'}`}
                      />
                      <button
                        type="button"
                        className={styles.addClientBtn}
                        onClick={() => setShowCreateProvider(true)}
                        title="Crear nuevo proveedor"
                      >
                        <Plus size={14} />
                      </button>
                    </div>
                  </dd>
                ) : (
                  <dd className={styles.fieldValue}>{order?.provider.name}</dd>
                )}
              </div>

              <div className={detailsEditable ? styles.fieldRowEditing : styles.fieldRow}>
                <dt className={styles.fieldLabel}>Estado</dt>
                {detailsEditable ? (
                  <dd className={styles.fieldValueFull}>
                    <Dropdown
                      value={status}
                      options={ORDER_STATUS_OPTIONS}
                      onChange={value => setStatus(value as OrderStatus)}
                      ariaLabel={`Estado: ${orderStatusLabel[status]}`}
                    />
                  </dd>
                ) : (
                  <dd className={styles.fieldValue}>
                    <span className={`badge ${orderStatusBadge[order?.status ?? 'pending'] ?? 'badge--warning'}`}>
                      {orderStatusLabel[order?.status ?? 'pending'] ?? order?.status}
                    </span>
                  </dd>
                )}
              </div>

              <div className={detailsEditable ? styles.fieldRowEditing : styles.fieldRow}>
                <dt className={styles.fieldLabel}>Estado pago</dt>
                {detailsEditable ? (
                  <dd className={styles.fieldValueFull}>
                    <Dropdown
                      value={paymentStatus}
                      options={ORDER_PAYMENT_STATUS_OPTIONS}
                      onChange={value => setPaymentStatus(value as OrderPaymentStatus)}
                      ariaLabel={`Estado pago: ${orderPaymentStatusLabel[paymentStatus]}`}
                    />
                  </dd>
                ) : (
                  <dd className={styles.fieldValue}>
                    <span className={`badge ${orderPaymentStatusBadge[order?.payment_status ?? 'pending'] ?? 'badge--warning'}`}>
                      {orderPaymentStatusLabel[order?.payment_status ?? 'pending'] ?? order?.payment_status}
                    </span>
                  </dd>
                )}
              </div>

              {!isCreateMode && order && (
                <div className={styles.fieldRow}>
                  <dt className={styles.fieldLabel}>Fecha</dt>
                  <dd className={`${styles.fieldValue} ${styles.mono}`}>{formatDate(order.order_date)}</dd>
                </div>
              )}
            </dl>
          </section>
        </div>
      </div>

      {!isCreateMode && (
        <section className={styles.dangerZone}>
          <div>
            <h2 className={styles.cardTitle}>Zona de riesgo</h2>
            <p className={styles.dangerZoneText}>
              Eliminar este pedido lo quita del historial. Esta acción no se puede deshacer.
            </p>
          </div>
          <button type="button" onClick={() => setShowDeleteModal(true)} className={styles.dangerButton}>
            <Trash2 size={15} />
            Eliminar
          </button>
        </section>
      )}

      {showCreateProvider && (
        <CreateFormModal
          title="Agregar Proveedor"
          fields={PROVIDER_FIELDS}
          onSubmit={submitCreateProvider}
          onClose={() => setShowCreateProvider(false)}
          onCreated={handleProviderCreated}
        />
      )}
      {showDeleteModal && order && (
        <ConfirmDeleteModal
          title="Eliminar Pedido"
          productName={`Pedido #${order.id} — ${order.provider.name}`}
          onConfirm={handleDeleteOrder}
          onClose={() => setShowDeleteModal(false)}
        />
      )}
    </form>
  )
}
