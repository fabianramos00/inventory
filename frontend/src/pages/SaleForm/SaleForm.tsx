import { useState, useEffect, useRef, useCallback } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { ChevronLeft, Search, X, Loader, Package, Plus, Pencil, Check, Trash2 } from 'lucide-react'
import { useQueryClient } from '@tanstack/react-query'
import styles from '../formPage.module.css'
import { salesApi } from '@/lib/api/sales'
import { getAPIErrorMessage } from '@/lib/apiError'
import { clientsApi } from '@/lib/api/clients'
import { inventoryApi } from '@/lib/api/inventory'
import CreateFormModal from '@/components/CreateFormModal/CreateFormModal'
import ConfirmDeleteModal from '@/components/ConfirmDeleteModal/ConfirmDeleteModal'
import SearchSelect from '@/components/SearchSelect/SearchSelect'
import Dropdown from '@/components/Dropdown/Dropdown'
import DiscardChangesModal from './DiscardChangesModal'
import { paymentStatusLabel, paymentStatusBadge, deliveryStatusLabel, deliveryStatusBadge, paymentMethodLabel } from '@/lib/constants'
import { formatDate, formatCurrency, safeNumber } from '@/lib/utils'
import { invalidateSaleData } from '@/lib/saleCache'
import { useSale, type SaleItemView } from '@/hooks/useSale'
import { useDebouncedFetch } from '@/hooks/useDebouncedFetch'
import { useClickOutside } from '@/hooks/useClickOutside'
import { CLIENT_FIELDS, buildClientCreatePayload } from '@/lib/formFields'
import { buildCreateSalePayload } from './salePayload'
import type { Product, FilterOption, PaymentStatus, PaymentMethod } from '@/types'

interface SaleItemDraft {
  id?: number
  product_id: number
  product_name: string
  product_sku: string
  product_default_price: number
  product_stock: number
  product_category: string
  product_brand: string
  product_material: string
  quantity: string
  delivered_quantity: string
  unit_price: string
}

const PAYMENT_METHOD_OPTIONS = [
  { value: '', label: 'Sin especificar' },
  { value: 'cash', label: 'Efectivo' },
  { value: 'debit', label: 'Débito' },
  { value: 'credit', label: 'Crédito' },
]

export default function SaleForm() {
  const navigate = useNavigate()
  const { id } = useParams<{ id?: string }>()
  const isCreateMode = !id
  const queryClient = useQueryClient()

  const { sale, saleItems, isLoading: pageLoading, error: saleError } = useSale(id)

  useEffect(() => {
    if (!isCreateMode && saleError) navigate('/sales')
  }, [isCreateMode, saleError, navigate])

  const [items, setItems] = useState<SaleItemDraft[]>([])
  const [isEditingItems, setIsEditingItems] = useState(false)
  const itemsEditable = isCreateMode || isEditingItems

  const [isEditingDetails, setIsEditingDetails] = useState(false)
  const detailsEditable = isCreateMode || isEditingDetails
  const [isSaving, setIsSaving] = useState(false)
  const [isSubmitting, setIsSubmitting] = useState(false)

  const [savingItemId, setSavingItemId] = useState<number | null>(null)
  const [deletingItemId, setDeletingItemId] = useState<number | null>(null)
  const [itemError, setItemError] = useState<{ productId: number; message: string } | null>(null)

  const [showDeleteModal, setShowDeleteModal] = useState(false)
  const [showDiscardModal, setShowDiscardModal] = useState(false)
  const [showCreateClient, setShowCreateClient] = useState(false)

  const [paymentStatus, setPaymentStatus] = useState<PaymentStatus>('pending')
  const [paymentMethod, setPaymentMethod] = useState<string>('')
  const [amountPaid, setAmountPaid] = useState('0')

  const [clientId, setClientId] = useState<number | ''>('')
  const [selectedClient, setSelectedClient] = useState<FilterOption | null>(null)

  const [productSearch, setProductSearch] = useState('')
  const [productSearchOpen, setProductSearchOpen] = useState(false)
  const productSearchRef = useRef<HTMLDivElement>(null)
  const productResultsFocusedIndex = useRef<number>(-1)

  const [apiError, setApiError] = useState<string | null>(null)
  const [validationError, setValidationError] = useState<string | null>(null)

  const { data: productResults, loading: productLoading } = useDebouncedFetch(
    () => inventoryApi.getProducts({ search: productSearch, skip: 0, limit: 8 }).then(res => res.data.items || []),
    [productSearch],
    300,
    itemsEditable && productSearch.trim() !== '',
  )

  useClickOutside(productSearchRef, () => setProductSearchOpen(false), productSearchOpen)

  useEffect(() => {
    if (!isCreateMode) return
    const handler = (e: BeforeUnloadEvent) => {
      if (items.length > 0) e.preventDefault()
    }
    window.addEventListener('beforeunload', handler)
    return () => window.removeEventListener('beforeunload', handler)
  }, [isCreateMode, items.length])

  function handleStartEditItems() {
    if (!sale) return
    setItems(saleItems.map(item => ({
      id: item.id,
      product_id: item.product_id,
      product_name: item.product_name,
      product_sku: item.product_sku,
      product_default_price: item.unit_price,
      product_stock: 0,
      product_category: item.product_category,
      product_brand: item.product_brand,
      product_material: item.product_material,
      quantity: String(item.quantity),
      delivered_quantity: String(item.delivered_quantity),
      unit_price: String(item.unit_price),
    })))
    setIsEditingItems(true)
    setItemError(null)
  }

  function handleCancelEditItems() {
    setIsEditingItems(false)
    setItems([])
    setProductSearch('')
    setProductSearchOpen(false)
    productResultsFocusedIndex.current = -1
    setItemError(null)
  }

  function handleStartEditDetails() {
    if (!sale) return
    setPaymentMethod(sale.payment_method ?? '')
    setAmountPaid(String(sale.amount_paid))
    setClientId(sale.client?.id ?? '')
    setSelectedClient(sale.client ? { id: sale.client.id, name: sale.client.name } : null)
    setIsEditingDetails(true)
  }

  function handleCancelEditDetails() {
    setIsEditingDetails(false)
  }

  async function handleSaveDetails() {
    if (!sale) return
    setIsSaving(true)
    try {
      const res = await salesApi.updateSale(sale.id, {
        payment_method: (paymentMethod as PaymentMethod) || null,
        amount_paid: parseFloat(amountPaid) || 0,
        client_id: clientId || null,
      })
      queryClient.setQueryData(['sale', sale.id], res.data)
      invalidateSaleData(queryClient, sale.id)
      setIsEditingDetails(false)
    } catch (err) {
      setApiError(getAPIErrorMessage(err, 'Error al guardar los detalles de la venta. Intenta de nuevo.'))
    } finally {
      setIsSaving(false)
    }
  }

  async function handleDeleteSale() {
    if (!sale) return
    await salesApi.deleteSale(sale.id)
    invalidateSaleData(queryClient)
    navigate('/sales')
  }

  async function submitCreateClient(values: Record<string, string>) {
    const res = await clientsApi.createClient(buildClientCreatePayload(values))
    return res.data
  }

  function handleClientCreated(client: { id: number; name: string }) {
    setSelectedClient(client)
    setClientId(client.id)
    setShowCreateClient(false)
  }

  function handleAddItem(product: Product) {
    setProductSearch('')
    setProductSearchOpen(false)
    productResultsFocusedIndex.current = -1
    if ((product.stock_quantity ?? 0) === 0) return
    if (items.some(i => i.product_id === product.id)) return
    const defaultPrice = product.sale_price ?? product.price ?? 0
    setItems(prev => [...prev, {
      product_id: product.id,
      product_name: product.name,
      product_sku: product.sku,
      product_default_price: defaultPrice,
      product_stock: product.stock_quantity ?? 0,
      product_category: product.category?.name ?? '',
      product_brand: product.brand?.name ?? '',
      product_material: product.material?.name ?? '',
      quantity: '1',
      delivered_quantity: '1',
      unit_price: defaultPrice > 0 ? String(defaultPrice) : '',
    }])
  }

  function updateItem(productId: number, field: 'quantity' | 'delivered_quantity' | 'unit_price', value: string) {
    setItems(prev => prev.map(item => {
      if (item.product_id !== productId) return item
      if (field === 'quantity' && (isCreateMode || !item.id)) {
        const limited = Math.min(parseFloat(value) || 0, item.product_stock)
        return {
          ...item,
          quantity: String(limited),
          ...(isCreateMode && { delivered_quantity: String(limited) }),
        }
      }
      return { ...item, [field]: value }
    }))
  }

  function stockErrorMessage(err: unknown, fallback: string) {
    const detail = getAPIErrorMessage(err, fallback)
    if (detail.includes('Insufficient stock')) {
      const match = detail.match(/Available: (\d+), Requested: (\d+)/)
      if (match) return `Stock insuficiente. Disponible: ${match[1]}, solicitado: ${match[2]}`
    }
    return detail
  }

  async function handleRemoveItem(productId: number) {
    const item = items.find(i => i.product_id === productId)
    if (!item) return
    if (!item.id) {
      setItems(prev => prev.filter(i => i.product_id !== productId))
      return
    }
    if (!sale) return
    setDeletingItemId(item.id)
    try {
      await salesApi.deleteSaleItem(sale.id, item.id)
      invalidateSaleData(queryClient, sale.id)
      setItems(prev => prev.filter(i => i.product_id !== productId))
    } catch (err) {
      setItemError({ productId, message: getAPIErrorMessage(err, 'Error al eliminar el artículo') })
    } finally {
      setDeletingItemId(null)
    }
  }

  async function handleSaveItem(productId: number) {
    const item = items.find(i => i.product_id === productId)
    if (!item || !sale) return

    if (item.id) {
      setSavingItemId(item.id)
      try {
        await salesApi.updateSaleItem(sale.id, item.id, {
          quantity: parseFloat(item.quantity) || 1,
          delivered_quantity: parseFloat(item.delivered_quantity) || 0,
          unit_price: item.unit_price !== '' ? parseFloat(item.unit_price) || null : null,
        })
        invalidateSaleData(queryClient, sale.id)
        setValidationError(null)
        setItemError(null)
      } catch (err) {
        setItemError({ productId, message: stockErrorMessage(err, 'Error al actualizar el artículo') })
      } finally {
        setSavingItemId(null)
      }
      return
    }

    try {
      const res = await salesApi.addSaleItem(sale.id, {
        product_id: item.product_id,
        quantity: parseFloat(item.quantity) || 1,
        delivered_quantity: parseFloat(item.delivered_quantity) || 0,
        unit_price: item.unit_price !== '' ? parseFloat(item.unit_price) || null : null,
      })
      queryClient.setQueryData(['sale', sale.id], res.data)
      invalidateSaleData(queryClient, sale.id)
      const freshItems = await queryClient.fetchQuery<SaleItemView[]>({
        queryKey: ['saleItems', sale.id],
        queryFn: () =>
          salesApi
            .getSaleItems(sale.id, { skip: 0, limit: 100 })
            .then(r =>
              r.data.items.map(saleItem => ({
                ...saleItem,
                product_category: saleItem.category_name ?? '',
                product_brand: saleItem.brand_name ?? '',
                product_material: saleItem.material_name ?? '',
              }))
            ),
        staleTime: 0,
      })
      const newItem = freshItems?.find(i => i.product_id === productId)
      if (newItem) {
        setItems(prev => prev.map(i => i.product_id === productId ? { ...i, id: newItem.id } : i))
      }
      setItemError(null)
    } catch (err) {
      setItemError({ productId, message: stockErrorMessage(err, 'Error al agregar el artículo') })
    }
  }

  const draftTotal = items.reduce((acc, item) => {
    const price = item.unit_price !== '' ? parseFloat(item.unit_price) || 0 : item.product_default_price
    return acc + price * Number(item.quantity)
  }, 0)

  useEffect(() => {
    if (!isCreateMode) return
    const paid = parseFloat(amountPaid) || 0
    if (paid === 0) {
      // eslint-disable-next-line react-hooks/set-state-in-effect -- derive payment status from the amount paid
      setPaymentStatus('pending')
    } else if (paid >= draftTotal && draftTotal > 0) {
      setPaymentStatus('paid')
    } else if (paid > 0) {
      setPaymentStatus('partial')
    }
  }, [isCreateMode, amountPaid, draftTotal])

  const displayTotal = isCreateMode ? draftTotal : isEditingItems ? draftTotal : sale?.total_amount ?? 0
  const displayDebt = displayTotal - (detailsEditable ? safeNumber(amountPaid, 0) : sale?.amount_paid ?? 0)
  const amountPaidError = (() => {
    const paid = parseFloat(amountPaid)
    if (!isNaN(paid) && displayTotal > 0 && paid > displayTotal) return `máx. $ ${displayTotal.toFixed(2)}`
    return null
  })()
  const hasLiveErrors = isCreateMode && (!!amountPaidError || items.some(item => Number(item.delivered_quantity) > Number(item.quantity)))

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!isCreateMode) return
    setValidationError(null)
    setApiError(null)

    if (items.length === 0) {
      setValidationError('Agrega al menos un producto a la venta.')
      return
    }
    for (const item of items) {
      if (!item.quantity || Number(item.quantity) < 1) {
        setValidationError('Todos los ítems deben tener una cantidad mayor a 0.')
        return
      }
      if (Number(item.delivered_quantity) > Number(item.quantity)) {
        setValidationError(`La cantidad entregada de "${item.product_name}" no puede superar la cantidad (${item.quantity}).`)
        return
      }
    }
    const paid = parseFloat(amountPaid) || 0
    if (paid > draftTotal) {
      setValidationError(`El monto pagado ($${paid.toFixed(2)}) no puede superar el total estimado ($${draftTotal.toFixed(2)}).`)
      return
    }

    setIsSubmitting(true)
    try {
      const payload = buildCreateSalePayload({
        paymentMethod,
        amountPaid,
        clientId,
        items,
      })
      await salesApi.createSale(payload)
      invalidateSaleData(queryClient)
      navigate('/sales')
    } catch (error) {
      setApiError(getAPIErrorMessage(error, 'Error al crear la venta.'))
    } finally {
      setIsSubmitting(false)
    }
  }

  const handleCancelCreate = useCallback(() => {
    if (items.length > 0) {
      setShowDiscardModal(true)
      return
    }
    navigate('/sales')
  }, [items.length, navigate])

  function handleDropdownKeyDown(
    e: React.KeyboardEvent,
    optionsCount: number,
    focusedIndex: React.MutableRefObject<number>,
    onSelect: (index: number) => void,
    onClose: () => void
  ) {
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      const next = Math.min(focusedIndex.current + 1, optionsCount - 1)
      focusedIndex.current = next
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      const prev = Math.max(focusedIndex.current - 1, 0)
      focusedIndex.current = prev
    } else if (e.key === 'Enter') {
      e.preventDefault()
      if (focusedIndex.current >= 0 && focusedIndex.current < optionsCount) {
        onSelect(focusedIndex.current)
      }
    } else if (e.key === 'Escape') {
      e.preventDefault()
      onClose()
    }
  }

  if (!isCreateMode && (pageLoading || !sale)) {
    return (
      <div className={styles.pageContainer}>
        <div className={styles.loadingState}>
          <Loader size={24} className={styles.spinner} />
          <p>Cargando venta...</p>
        </div>
      </div>
    )
  }

  return (
    <form onSubmit={handleSubmit} className={styles.pageContainer}>
      <div className={styles.pageHeader}>
        <div className={styles.headerLeft}>
          <button
            type="button"
            onClick={isCreateMode ? handleCancelCreate : () => navigate('/sales')}
            className={styles.backButton}
            title="Volver a ventas"
          >
            <ChevronLeft size={20} />
          </button>
          <div className={styles.headerContent}>
            <div className={styles.breadcrumb}>
              <span>Ventas</span>
              <span className={styles.breadcrumbDivider}>/</span>
              <span className={styles.breadcrumbActive}>
                {isCreateMode ? 'Nueva venta' : `Venta #${sale?.id}`}
              </span>
            </div>
            <div className={styles.titleRow}>
              <h1 className={styles.pageTitle}>
                {isCreateMode ? 'Crear venta' : (sale?.client?.name ?? 'Sin cliente')}
              </h1>
              {!isCreateMode && sale && (
                <span className={`badge ${paymentStatusBadge[sale.payment_status] ?? 'badge--warning'}`}>
                  {paymentStatusLabel[sale.payment_status] ?? sale.payment_status}
                </span>
              )}
            </div>
          </div>
        </div>
        {isCreateMode && (
          <div className={styles.headerActions}>
            <button type="button" onClick={handleCancelCreate} className="btn-ghost">
              Cancelar
            </button>
            <button
              type="submit"
              disabled={isSubmitting || hasLiveErrors}
              className={`btn-primary ${styles.btnDisabled}`}
            >
              {isSubmitting ? <><Loader size={15} className={styles.spinnerDark} /> Guardando...</> : 'Guardar Venta'}
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
                    productResultsFocusedIndex.current = -1
                  }}
                  autoComplete="off"
                  aria-expanded={productSearchOpen}
                  aria-controls={isCreateMode ? 'product-search-listbox' : 'product-results-list'}
                  aria-autocomplete="list"
                  role="combobox"
                />
                {productLoading && <Loader size={13} className={styles.searchSpinner} />}
              </div>
              {productSearchOpen && (
                <div
                  className={styles.productResults}
                  id={isCreateMode ? 'product-search-listbox' : 'product-results-list'}
                  role="listbox"
                  onKeyDown={e => handleDropdownKeyDown(
                    e,
                    productResults.length,
                    productResultsFocusedIndex,
                    (idx) => {
                      const product = productResults[idx]
                      if (product) handleAddItem(product)
                    },
                    () => setProductSearchOpen(false)
                  )}
                >
                  {productLoading ? (
                    <div className={`${styles.productOption} ${styles.optionLoading}`}>
                      <Loader size={14} className={styles.spinner} />
                    </div>
                  ) : productResults.length === 0 ? (
                    <div className={`${styles.productOption} ${styles.productOptionEmpty}`}>Sin resultados</div>
                  ) : (
                    productResults.map((product, idx) => {
                      const isAdded = items.some(i => i.product_id === product.id)
                      const hasNoStock = (product.stock_quantity ?? 0) === 0
                      return (
                        <button
                          key={product.id}
                          type="button"
                          disabled={hasNoStock}
                          role="option"
                          aria-selected={isAdded}
                          className={`${styles.productOption} ${isAdded ? styles.productOptionAdded : ''} ${hasNoStock ? styles.productOptionDisabled : ''}`}
                          onClick={() => handleAddItem(product)}
                          title={hasNoStock ? 'Sin stock disponible' : ''}
                          data-index={idx}
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
                            <div className={styles.productOptionPriceRow}>
                              <span className={styles.productOptionPrice}>$ {(product.sale_price ?? product.price ?? 0).toFixed(2)}</span>
                              <span className={hasNoStock ? styles.stockInfoWarning : styles.stockInfo}>
                                Stock: <span className={hasNoStock ? styles.stockInfoWarning : styles.stockInfoValue}>{product.stock_quantity ?? 0}</span>
                              </span>
                            </div>
                          </div>
                        </button>
                      )
                    })
                  )}
                </div>
              )}
            </div>
          )}

          {(isCreateMode ? items.length === 0 : saleItems.length === 0 && items.length === 0) ? (
            <div className={styles.emptyItems}>
              <Package size={28} className={styles.emptyIcon} />
              <p>{isCreateMode ? 'Busca y selecciona productos para agregar a la venta' : 'Sin productos registrados'}</p>
            </div>
          ) : (
            <div className={styles.itemsList}>
              {itemsEditable ? items.map(item => {
                const deliveredError = (isCreateMode || item.id) && Number(item.delivered_quantity) > Number(item.quantity)
                  ? `máx. ${item.quantity}`
                  : null
                const isPending = !isCreateMode && !item.id
                const fieldPrefix = isCreateMode
                  ? `create-item-${item.product_id}`
                  : item.id ? `item-${item.id}` : `pending-${item.product_id}`
                return (
                  <div
                    key={item.product_id}
                    className={`${styles.itemCard} ${isPending ? styles.itemCardPending : ''} ${itemError?.productId === item.product_id ? styles.itemCardError : ''}`}
                  >
                    {itemError?.productId === item.product_id && (
                      <div className={styles.errorBanner} role="alert">{itemError.message}</div>
                    )}
                    <div className={styles.itemHeader}>
                      <div className={styles.itemIdentity}>
                        <div className={styles.itemNameRow}>
                          <span className={styles.itemName}>{item.product_name}</span>
                          {item.product_sku && <span className={styles.itemSku}>{item.product_sku}</span>}
                        </div>
                        <div className={styles.itemTags}>
                          {item.product_category && <span className={styles.itemTag}>{item.product_category}</span>}
                          {item.product_brand && <span className={styles.itemTag}>{item.product_brand}</span>}
                          {item.product_material && <span className={styles.itemTag}>{item.product_material}</span>}
                        </div>
                        {(isCreateMode || isPending) && (
                          <div className={styles.stockInfo}>
                            Stock disponible: <span className={styles.stockInfoValue}>{item.product_stock}</span>
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
                        <label className={styles.itemFieldLabel} htmlFor={`${fieldPrefix}-quantity`}>Cantidad</label>
                        <input
                          id={`${fieldPrefix}-quantity`}
                          type="number"
                          className={`form-input ${styles.mono}`}
                          min={1}
                          value={item.quantity}
                          onChange={e => updateItem(item.product_id, 'quantity', e.target.value)}
                        />
                      </div>
                      <div className={styles.itemField}>
                        <label className={styles.itemFieldLabel} htmlFor={`${fieldPrefix}-delivered`}>Cant. entregada</label>
                        <input
                          id={`${fieldPrefix}-delivered`}
                          type="number"
                          className={`form-input ${styles.mono} ${deliveredError ? styles.inputError : ''}`}
                          min={0}
                          value={item.delivered_quantity}
                          onChange={e => updateItem(item.product_id, 'delivered_quantity', e.target.value)}
                        />
                        {deliveredError && <span className={styles.fieldError}>{deliveredError}</span>}
                      </div>
                      <div className={styles.itemField}>
                        <label className={styles.itemFieldLabel} htmlFor={`${fieldPrefix}-price`}>Precio unit.</label>
                        <input
                          id={`${fieldPrefix}-price`}
                          type="number"
                          className={`form-input ${styles.mono}`}
                          min={0}
                          step="0.01"
                          placeholder={isCreateMode
                            ? item.product_default_price.toFixed(2)
                            : isPending ? (item.product_default_price > 0 ? String(item.product_default_price) : '0.00') : undefined}
                          value={item.unit_price}
                          onChange={e => updateItem(item.product_id, 'unit_price', e.target.value)}
                        />
                      </div>
                    </div>
                  </div>
                )
              }) : saleItems.map(item => (
                <div key={item.id} className={styles.itemCard}>
                  <div className={styles.itemHeader}>
                    <div className={styles.itemIdentity}>
                      <div className={styles.itemNameRow}>
                        <span className={styles.itemName}>{item.product_name}</span>
                        {item.product_sku && <span className={styles.itemSku}>{item.product_sku}</span>}
                      </div>
                      <div className={styles.itemTags}>
                        {item.product_category && <span className={styles.itemTag}>{item.product_category}</span>}
                        {item.product_brand && <span className={styles.itemTag}>{item.product_brand}</span>}
                        {item.product_material && <span className={styles.itemTag}>{item.product_material}</span>}
                      </div>
                    </div>
                  </div>
                  <div className={styles.itemFieldsRowFour}>
                    <div className={styles.itemField}>
                      <span className={styles.itemFieldLabel}>Cantidad</span>
                      <span className={styles.mono}>{item.quantity}</span>
                    </div>
                    <div className={styles.itemField}>
                      <span className={styles.itemFieldLabel}>Entregado</span>
                      <span className={styles.mono}>{item.delivered_quantity}</span>
                    </div>
                    <div className={styles.itemField}>
                      <span className={styles.itemFieldLabel}>P. unit.</span>
                      <span className={styles.mono}>$ {item.unit_price.toFixed(2)}</span>
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
              <span className={styles.receiptMeta}>{isCreateMode ? 'Borrador' : `Venta #${sale?.id}`}</span>
            </div>
            <dl className={styles.receiptRows}>
              <div className={styles.receiptRow}>
                <dt className={styles.receiptLabel}>Ítems</dt>
                <dd className={styles.receiptValue}>
                  {isCreateMode && items.length === 0 ? '—' : (isCreateMode ? items.length : saleItems.length)}
                </dd>
              </div>
              <div className={styles.receiptRow}>
                <dt className={styles.receiptLabel}>{isCreateMode ? 'Deuda estimada' : 'Deuda'}</dt>
                <dd className={`${styles.receiptValue} ${displayDebt > 0 ? styles.receiptDebtPositive : styles.receiptDebtSettled}`}>
                  {formatCurrency(displayDebt)}
                </dd>
              </div>
            </dl>
            {isCreateMode && items.length === 0 && (
              <p className={styles.receiptHint}>Agrega productos para ver el resumen</p>
            )}
            <div className={styles.receiptTotalRow}>
              <span className={styles.receiptTotalLabel}>{isCreateMode ? 'Total estimado' : 'Total'}</span>
              <span className={styles.receiptTotalValue}>{formatCurrency(displayTotal)}</span>
            </div>
          </section>

          <section className={styles.card}>
            {isCreateMode ? (
              <h2 className={styles.cardTitle}>Detalles de venta</h2>
            ) : (
              <div className={styles.cardHeader}>
                <h2 className={styles.cardTitle}>Detalles de venta</h2>
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
                        disabled={isSaving || safeNumber(amountPaid, 0) > displayTotal}
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
                <dt className={styles.fieldLabel}>Cliente</dt>
                {detailsEditable ? (
                  <dd className={styles.fieldValueFull}>
                    <div className={styles.selectWithAction}>
                      <SearchSelect
                        label="Cliente"
                        placeholder="Sin cliente"
                        emptyLabel="Sin cliente"
                        className={styles.formSelect}
                        value={clientId}
                        onChange={(value, option) => {
                          setClientId(value)
                          setSelectedClient(option ?? null)
                        }}
                        fetchOptions={search =>
                          clientsApi.getClients({ search: search || undefined, skip: 0, limit: 10 })
                            .then(res => res.data.items || [])
                        }
                        selectedOption={selectedClient ?? undefined}
                        ariaLabel={`Cliente: ${selectedClient?.name ?? 'Sin cliente'}`}
                      />
                      {isCreateMode && (
                        <button
                          type="button"
                          className={styles.addClientBtn}
                          onClick={() => setShowCreateClient(true)}
                          title="Crear nuevo cliente"
                        >
                          <Plus size={14} />
                        </button>
                      )}
                    </div>
                  </dd>
                ) : (
                  <dd className={styles.fieldValue}>{sale?.client?.name ?? <span className={styles.summaryMuted}>Sin cliente</span>}</dd>
                )}
              </div>

              {isCreateMode ? (
                <div className={styles.fieldRow}>
                  <dt className={styles.fieldLabel}>Estado pago</dt>
                  <dd className={styles.fieldValue}>
                    <span className={`badge ${paymentStatusBadge[paymentStatus]}`}>
                      {paymentStatusLabel[paymentStatus]}
                    </span>
                    <span className={styles.summaryMuted}> Calculado por el servidor</span>
                  </dd>
                </div>
              ) : (
                <>
                  <div className={styles.fieldRow}>
                    <dt className={styles.fieldLabel}>Estado pago</dt>
                    <dd className={styles.fieldValue}>
                      <span className={`badge ${paymentStatusBadge[sale?.payment_status ?? 'pending'] ?? 'badge--warning'}`}>
                        {paymentStatusLabel[sale?.payment_status ?? 'pending'] ?? sale?.payment_status}
                      </span>
                    </dd>
                  </div>
                  <div className={styles.fieldRow}>
                    <dt className={styles.fieldLabel}>Estado entrega</dt>
                    <dd className={styles.fieldValue}>
                      <span className={`badge ${deliveryStatusBadge[sale?.delivery_status ?? 'pending'] ?? 'badge--warning'}`}>
                        {deliveryStatusLabel[sale?.delivery_status ?? 'pending'] ?? sale?.delivery_status}
                      </span>
                    </dd>
                  </div>
                </>
              )}

              <div className={detailsEditable ? styles.fieldRowEditing : styles.fieldRow}>
                <dt className={styles.fieldLabel}>Método pago</dt>
                {detailsEditable ? (
                  <dd className={styles.fieldValueFull}>
                    <Dropdown
                      value={paymentMethod}
                      options={PAYMENT_METHOD_OPTIONS}
                      onChange={setPaymentMethod}
                      placeholder="Sin especificar"
                    />
                  </dd>
                ) : (
                  <dd className={styles.fieldValue}>
                    {sale?.payment_method ? paymentMethodLabel[sale.payment_method] : <span className={styles.summaryMuted}>Sin especificar</span>}
                  </dd>
                )}
              </div>

              <div className={detailsEditable ? styles.fieldRowEditing : styles.fieldRow}>
                <dt className={styles.fieldLabel}>Monto pagado</dt>
                {detailsEditable ? (
                  <dd className={styles.fieldValueFull}>
                    <input
                      id={isCreateMode ? 'create-amount-paid' : 'edit-amount-paid'}
                      type="number"
                      className={`form-input ${styles.mono} ${amountPaidError ? styles.inputError : ''}`}
                      min={0}
                      step="0.01"
                      placeholder="0.00"
                      value={amountPaid}
                      onChange={e => setAmountPaid(e.target.value)}
                    />
                    {amountPaidError && <span className={styles.fieldError}>{amountPaidError}</span>}
                  </dd>
                ) : (
                  <dd className={`${styles.fieldValue} ${styles.mono}`}>$ {(sale?.amount_paid ?? 0).toFixed(2)}</dd>
                )}
              </div>

              {!isCreateMode && sale && (
                <>
                  <div className={styles.fieldRow}>
                    <dt className={styles.fieldLabel}>Registrado por</dt>
                    <dd className={styles.fieldValue}>{sale.created_by.full_name}</dd>
                  </div>
                  <div className={styles.fieldRow}>
                    <dt className={styles.fieldLabel}>Fecha</dt>
                    <dd className={`${styles.fieldValue} ${styles.mono}`}>{formatDate(sale.created_at)}</dd>
                  </div>
                </>
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
              Eliminar esta venta la quita del historial. Esta acción no se puede deshacer.
            </p>
          </div>
          <button type="button" onClick={() => setShowDeleteModal(true)} className={styles.dangerButton}>
            <Trash2 size={15} />
            Eliminar
          </button>
        </section>
      )}

      {showCreateClient && (
        <CreateFormModal
          title="Agregar Cliente"
          fields={CLIENT_FIELDS}
          onSubmit={submitCreateClient}
          onClose={() => setShowCreateClient(false)}
          onCreated={handleClientCreated}
        />
      )}
      {showDeleteModal && sale && (
        <ConfirmDeleteModal
          title="Eliminar Venta"
          productName={`Venta #${sale.id}`}
          onConfirm={handleDeleteSale}
          onClose={() => setShowDeleteModal(false)}
        />
      )}
      <DiscardChangesModal
        open={showDiscardModal}
        itemCount={items.length}
        totalAmount={`$ ${draftTotal.toFixed(2)}`}
        onStay={() => setShowDiscardModal(false)}
        onDiscard={() => {
          setShowDiscardModal(false)
          navigate('/sales')
        }}
      />
    </form>
  )
}
