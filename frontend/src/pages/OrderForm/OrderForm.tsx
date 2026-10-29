import { useId, useState, useEffect, useRef } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { ChevronLeft, Search, X, Loader, ChevronDown, Package, Pencil, Check, Trash2, Plus } from 'lucide-react'
import { useQueryClient } from '@tanstack/react-query'
import styles from '../formPage.module.css'
import { ordersApi } from '@/lib/api/orders'
import { inventoryApi } from '@/lib/api/inventory'
import { providersApi } from '@/lib/api/providers'
import ConfirmDeleteModal from '@/components/ConfirmDeleteModal/ConfirmDeleteModal'
import CreateFormModal, { type FieldConfig } from '@/components/CreateFormModal/CreateFormModal'
import { orderStatusLabel, orderStatusBadge, orderPaymentStatusLabel, orderPaymentStatusBadge } from '@/lib/constants'
import { formatDate, formatCurrency } from '@/lib/utils'
import { getAPIErrorMessage } from '@/lib/apiError'
import {
  invalidateOrderDetails,
  invalidateOrderItems,
  invalidateOrderListData,
} from '@/lib/orderCache'
import { useOrder } from '@/hooks/useOrder'
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

const PROVIDER_FIELDS: FieldConfig[] = [
  { key: 'name', label: 'Nombre', placeholder: 'Nombre del proveedor', required: true },
  { key: 'contact_info', label: 'Información de contacto (opcional)', placeholder: 'Persona de contacto u otra info' },
  { key: 'email', label: 'Correo electrónico (opcional)', placeholder: 'correo@ejemplo.com', type: 'email' },
  { key: 'phone', label: 'Teléfono (opcional)', placeholder: 'Número de teléfono', type: 'tel' },
]

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
  const [providerOptions, setProviderOptions] = useState<FilterOption[]>([])
  const [providerSearch, setProviderSearch] = useState('')
  const [providerOpen, setProviderOpen] = useState(false)
  const [providerLoading, setProviderLoading] = useState(false)
  const providerRef = useRef<HTMLDivElement>(null)
  const providerTimeout = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)

  const [status, setStatus] = useState<OrderStatus>('pending')
  const [paymentStatus, setPaymentStatus] = useState<OrderPaymentStatus>('pending')
  const [openFormDropdown, setOpenFormDropdown] = useState<string | null>(null)
  const statusRef = useRef<HTMLDivElement>(null)
  const paymentStatusRef = useRef<HTMLDivElement>(null)
  const statusTriggerRef = useRef<HTMLButtonElement>(null)
  const paymentStatusTriggerRef = useRef<HTMLButtonElement>(null)
  const providerTriggerRef = useRef<HTMLButtonElement>(null)
  const providerListId = useId()
  const statusListId = useId()
  const paymentStatusListId = useId()

  const [productSearch, setProductSearch] = useState('')
  const [productResults, setProductResults] = useState<Product[]>([])
  const [productSearchOpen, setProductSearchOpen] = useState(false)
  const [productLoading, setProductLoading] = useState(false)
  const productSearchRef = useRef<HTMLDivElement>(null)
  const productSearchTimeout = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)

  const [apiError, setApiError] = useState<string | null>(null)
  const [validationError, setValidationError] = useState<string | null>(null)

  useEffect(() => {
    if (providerTimeout.current) clearTimeout(providerTimeout.current)
    if (!providerOpen) return
    providerTimeout.current = setTimeout(async () => {
      setProviderLoading(true)
      try {
        const res = await providersApi.getProviders({ search: providerSearch || undefined, skip: 0, limit: 10 })
        setProviderOptions((res.data.items || []).map(provider => ({ id: provider.id, name: provider.name })))
      } catch {
        setProviderOptions([])
      } finally {
        setProviderLoading(false)
      }
    }, 300)
    return () => { if (providerTimeout.current) clearTimeout(providerTimeout.current) }
  }, [providerSearch, providerOpen])

  useEffect(() => {
    if (productSearchTimeout.current) clearTimeout(productSearchTimeout.current)
    if (!itemsEditable || !productSearch.trim()) {
      // eslint-disable-next-line react-hooks/set-state-in-effect -- clear stale results when the search becomes empty/disabled
      setProductResults([])
      setProductSearchOpen(false)
      return
    }
    productSearchTimeout.current = setTimeout(async () => {
      setProductLoading(true)
      setProductSearchOpen(true)
      try {
        const res = await inventoryApi.getProducts({ search: productSearch, skip: 0, limit: 8 })
        setProductResults(res.data.items || [])
      } catch {
        setProductResults([])
      } finally {
        setProductLoading(false)
      }
    }, 300)
    return () => { if (productSearchTimeout.current) clearTimeout(productSearchTimeout.current) }
  }, [productSearch, itemsEditable])

  useEffect(() => {
    if (openFormDropdown === 'status') {
      statusRef.current?.querySelector<HTMLElement>('[role="option"]')?.focus()
    } else if (openFormDropdown === 'payment_status') {
      paymentStatusRef.current?.querySelector<HTMLElement>('[role="option"]')?.focus()
    }
  }, [openFormDropdown])

  useEffect(() => {
    const handler = (e: MouseEvent) => {
      const target = e.target as Node
      if (!productSearchRef.current?.contains(target)) setProductSearchOpen(false)
      if (!providerRef.current?.contains(target)) setProviderOpen(false)
      if (!statusRef.current?.contains(target) && !paymentStatusRef.current?.contains(target)) {
        setOpenFormDropdown(null)
      }
    }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [])

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
    setProviderOptions([{ id: order.provider.id, name: order.provider.name }])
    setStatus(order.status)
    setPaymentStatus(order.payment_status)
    setIsEditingDetails(true)
  }

  function handleCancelEditDetails() {
    setIsEditingDetails(false)
    setOpenFormDropdown(null)
    setProviderOpen(false)
  }

  function closeFormMenu(menu: 'status' | 'payment_status') {
    setOpenFormDropdown(null)
    const trigger = menu === 'status' ? statusTriggerRef : paymentStatusTriggerRef
    trigger.current?.focus()
  }

  function handleFormMenuTriggerKeyDown(
    event: React.KeyboardEvent<HTMLButtonElement>,
    menu: 'status' | 'payment_status',
  ) {
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault()
      setOpenFormDropdown(menu)
    } else if (event.key === 'Escape' && openFormDropdown === menu) {
      event.preventDefault()
      closeFormMenu(menu)
    }
  }

  function closeProviderMenu() {
    setProviderOpen(false)
    providerTriggerRef.current?.focus()
  }

  function handleProviderTriggerKeyDown(event: React.KeyboardEvent<HTMLButtonElement>) {
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault()
      setProviderOpen(true)
    } else if (event.key === 'Escape' && providerOpen) {
      event.preventDefault()
      closeProviderMenu()
    }
  }

  function handleProviderSearchKeyDown(event: React.KeyboardEvent<HTMLInputElement>) {
    if (event.key !== 'ArrowDown' && event.key !== 'ArrowUp' && event.key !== 'Escape') return
    event.preventDefault()
    if (event.key === 'Escape') {
      closeProviderMenu()
      return
    }
    const options = providerRef.current?.querySelectorAll<HTMLElement>('[role="option"]')
    if (!options?.length) return
    options[event.key === 'ArrowDown' ? 0 : options.length - 1]?.focus()
  }

  function handleOptionKeyDown(
    event: React.KeyboardEvent<HTMLButtonElement>,
    container: HTMLElement | null,
    onEscape: () => void,
  ) {
    const options = Array.from(container?.querySelectorAll<HTMLElement>('[role="option"]') ?? [])
    const index = options.indexOf(event.currentTarget)
    if (index === -1) return

    if (event.key === 'Escape') {
      event.preventDefault()
      onEscape()
    } else if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault()
      const next = Math.min(Math.max(index + (event.key === 'ArrowDown' ? 1 : -1), 0), options.length - 1)
      options[next]?.focus()
    } else if (event.key === 'Home' || event.key === 'End') {
      event.preventDefault()
      options[event.key === 'Home' ? 0 : options.length - 1]?.focus()
    }
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

  async function submitCreateProvider(v: Record<string, string>) {
    const res = await providersApi.createProvider({
      name: v.name,
      ...(v.contact_info && { contact_info: v.contact_info }),
      ...(v.email && { email: v.email }),
      ...(v.phone && { phone: v.phone }),
    })
    return { id: res.data.id, name: res.data.name }
  }

  function handleProviderCreated(provider: { id: number; name: string }) {
    setProviderOptions(prev => [provider, ...prev.filter(p => p.id !== provider.id)])
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
  const selectedProviderName = providerOptions.find(provider => provider.id === providerId)?.name || 'Sin proveedor'

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
                  onChange={e => setProductSearch(e.target.value)}
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
                      <div className={styles.selectDropdown} ref={providerRef}>
                        <button
                          ref={providerTriggerRef}
                          type="button"
                          className={`${styles.selectTrigger} ${providerId ? styles.selectTriggerActive : ''}`}
                          onClick={() => { setProviderOpen(open => !open); setProviderSearch('') }}
                          onKeyDown={handleProviderTriggerKeyDown}
                          aria-haspopup="listbox"
                          aria-expanded={providerOpen}
                          aria-controls={providerListId}
                          aria-label={`Proveedor: ${selectedProviderName}`}
                        >
                          <span>{selectedProviderName}</span>
                          <ChevronDown size={13} />
                        </button>
                        {providerOpen && (
                          <div className={styles.selectContent}>
                            <input
                              type="text"
                              className={styles.selectSearch}
                              placeholder="Buscar proveedor..."
                              value={providerSearch}
                              onChange={e => setProviderSearch(e.target.value)}
                              onKeyDown={handleProviderSearchKeyDown}
                              autoFocus
                            />
                            <div className={styles.selectOptions} id={providerListId} role="listbox">
                              {providerLoading ? (
                                <div className={`${styles.selectOption} ${styles.optionLoading}`}><Loader size={13} className={styles.spinner} /></div>
                              ) : providerOptions.map(provider => (
                                <button
                                  key={provider.id}
                                  type="button"
                                  role="option"
                                  aria-selected={providerId === provider.id}
                                  tabIndex={-1}
                                  className={`${styles.selectOption} ${providerId === provider.id ? styles.selectOptionActive : ''}`}
                                  onClick={() => { setProviderId(provider.id); setProviderOpen(false) }}
                                  onKeyDown={e => handleOptionKeyDown(e, providerRef.current, closeProviderMenu)}
                                >
                                  {provider.name}
                                </button>
                              ))}
                            </div>
                          </div>
                        )}
                      </div>
                      <button
                        type="button"
                        className={styles.addClientBtn}
                        onClick={() => { setProviderOpen(false); setShowCreateProvider(true) }}
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
                    <div className={styles.selectDropdown} ref={statusRef}>
                      <button
                        ref={statusTriggerRef}
                        type="button"
                        className={`${styles.selectTrigger} ${styles.selectTriggerActive}`}
                        onClick={() => setOpenFormDropdown(openFormDropdown === 'status' ? null : 'status')}
                        onKeyDown={e => handleFormMenuTriggerKeyDown(e, 'status')}
                        aria-haspopup="listbox"
                        aria-expanded={openFormDropdown === 'status'}
                        aria-controls={statusListId}
                        aria-label={`Estado: ${orderStatusLabel[status]}`}
                      >
                        <span>{orderStatusLabel[status]}</span>
                        <ChevronDown size={13} />
                      </button>
                      {openFormDropdown === 'status' && (
                        <div className={styles.selectContent}>
                          <div className={styles.selectOptions} id={statusListId} role="listbox">
                            {(['pending', 'sent', 'received', 'cancelled'] as const).map(value => (
                              <button
                                key={value}
                                type="button"
                                role="option"
                                aria-selected={status === value}
                                tabIndex={-1}
                                className={`${styles.selectOption} ${status === value ? styles.selectOptionActive : ''}`}
                                onClick={() => { setStatus(value); setOpenFormDropdown(null) }}
                                onKeyDown={e => handleOptionKeyDown(e, statusRef.current, () => closeFormMenu('status'))}
                              >
                                {orderStatusLabel[value]}
                              </button>
                            ))}
                          </div>
                        </div>
                      )}
                    </div>
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
                    <div className={styles.selectDropdown} ref={paymentStatusRef}>
                      <button
                        ref={paymentStatusTriggerRef}
                        type="button"
                        className={`${styles.selectTrigger} ${styles.selectTriggerActive}`}
                        onClick={() => setOpenFormDropdown(openFormDropdown === 'payment_status' ? null : 'payment_status')}
                        onKeyDown={e => handleFormMenuTriggerKeyDown(e, 'payment_status')}
                        aria-haspopup="listbox"
                        aria-expanded={openFormDropdown === 'payment_status'}
                        aria-controls={paymentStatusListId}
                        aria-label={`Estado pago: ${orderPaymentStatusLabel[paymentStatus]}`}
                      >
                        <span>{orderPaymentStatusLabel[paymentStatus]}</span>
                        <ChevronDown size={13} />
                      </button>
                      {openFormDropdown === 'payment_status' && (
                        <div className={styles.selectContent}>
                          <div className={styles.selectOptions} id={paymentStatusListId} role="listbox">
                            {(['pending', 'paid'] as const).map(value => (
                              <button
                                key={value}
                                type="button"
                                role="option"
                                aria-selected={paymentStatus === value}
                                tabIndex={-1}
                                className={`${styles.selectOption} ${paymentStatus === value ? styles.selectOptionActive : ''}`}
                                onClick={() => { setPaymentStatus(value); setOpenFormDropdown(null) }}
                                onKeyDown={e => handleOptionKeyDown(e, paymentStatusRef.current, () => closeFormMenu('payment_status'))}
                              >
                                {orderPaymentStatusLabel[value]}
                              </button>
                            ))}
                          </div>
                        </div>
                      )}
                    </div>
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
