import { useState, useEffect } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { ChevronLeft, Pencil, Loader, Check, X, Trash2 } from 'lucide-react'
import { useQueryClient } from '@tanstack/react-query'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { inventoryApi } from '@/lib/api/inventory'
import { productStatusMap, productStatusLabel } from '@/lib/constants'
import { useProduct } from '@/hooks/useProduct'
import ProductDropdown from '@/components/ProductDropdown/ProductDropdown'
import CreateFormModal, { type FieldConfig } from '@/components/CreateFormModal/CreateFormModal'
import ConfirmDeleteModal from '@/components/ConfirmDeleteModal/ConfirmDeleteModal'
import StockMeter from '@/components/StockMeter/StockMeter'
import type { Product, FilterOption, CreateProductInput } from '@/types'
import styles from '../formPage.module.css'
import pageStyles from './ProductForm.module.css'

type EntityType = 'material' | 'category' | 'brand' | 'measurementUnit'

const ENTITY_TITLES: Record<EntityType, string> = {
  category: 'Agregar Categoría',
  brand: 'Agregar Marca',
  material: 'Agregar Material',
  measurementUnit: 'Agregar Unidad de Medida',
}

const ENTITY_FIELDS: Record<EntityType, FieldConfig[]> = {
  category: [
    { key: 'code', label: 'Código', placeholder: 'Código de la categoría', required: true },
    { key: 'name', label: 'Nombre', placeholder: 'Nombre de la categoría', required: true },
    { key: 'description', label: 'Descripción (opcional)', type: 'textarea', placeholder: 'Descripción de la categoría' },
  ],
  brand: [
    { key: 'name', label: 'Nombre', placeholder: 'Nombre de la marca', required: true },
    { key: 'logo_url', label: 'Logo URL (opcional)', type: 'url', placeholder: 'https://ejemplo.com/logo.png' },
  ],
  material: [
    { key: 'name', label: 'Nombre', placeholder: 'Nombre del material', required: true },
  ],
  measurementUnit: [
    { key: 'name', label: 'Nombre', placeholder: 'Centímetros', required: true },
    { key: 'abbreviation', label: 'Abreviatura', placeholder: 'cm', required: true },
  ],
}

const productSchema = z.object({
  name: z.string().min(1, 'Nombre es requerido').max(150, 'Nombre debe tener máximo 150 caracteres'),
  sku: z.string().max(100, 'SKU debe tener máximo 100 caracteres').optional(),
  brand_id: z.number().int().min(1, 'Marca es requerida'),
  category_id: z.number().int().min(1, 'Categoría es requerida'),
  material_id: z.number().int().min(1, 'Material es requerido'),
  size_value: z.string().max(20, 'Tamaño debe tener máximo 20 caracteres').nullable().optional(),
  measurement_unit_id: z.number().nullable().optional(),
  price: z.number().positive('Precio debe ser mayor a 0'),
  cost: z.number().positive('Costo debe ser mayor a 0'),
  stock_quantity: z.number().int().min(0, 'Cantidad no puede ser negativa'),
  min_stock_quantity: z.number().int().min(0, 'Cantidad mínima no puede ser negativa'),
})

type ProductFormInputs = z.infer<typeof productSchema>

function getProductStatus(stock: number, threshold: number) {
  if (stock === 0) return 'out_of_stock'
  return stock <= threshold ? 'low_stock' : 'in_stock'
}

function withOption(options: FilterOption[], option: FilterOption) {
  return options.some(o => o.id === option.id) ? options : [...options, option]
}

export default function ProductForm() {
  const { id } = useParams<{ id?: string }>()
  const isCreateMode = !id
  const navigate = useNavigate()
  const queryClient = useQueryClient()

  const { product: productData, isLoading: productLoading, error: productError } = useProduct(id)
  const [isEditing, setIsEditing] = useState(isCreateMode)
  const [apiError, setApiError] = useState<string | null>(null)
  const [openDropdownId, setOpenDropdownId] = useState<string | null>(null)
  const [activeModal, setActiveModal] = useState<EntityType | null>(null)
  const [showDeleteModal, setShowDeleteModal] = useState(false)

  const [categoryInitialOptions, setCategoryInitialOptions] = useState<FilterOption[]>([])
  const [brandInitialOptions, setBrandInitialOptions] = useState<FilterOption[]>([])
  const [materialInitialOptions, setMaterialInitialOptions] = useState<FilterOption[]>([])
  const [measurementUnitInitialOptions, setMeasurementUnitInitialOptions] = useState<FilterOption[]>([])

  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
    watch,
    setValue,
  } = useForm<ProductFormInputs>({
    resolver: zodResolver(productSchema),
    defaultValues: {
      name: '',
      sku: '',
      brand_id: 0,
      category_id: 0,
      material_id: 0,
      size_value: undefined,
      measurement_unit_id: undefined,
      price: 0,
      cost: 0,
      stock_quantity: 0,
      min_stock_quantity: 0,
    },
  })

  function populateForm(p: Product) {
    setValue('name', p.name)
    setValue('sku', p.sku)
    if (p.category) {
      setCategoryInitialOptions([{ id: p.category.id, name: p.category.name }])
      setValue('category_id', p.category.id)
    } else if (p.category_id != null) {
      setValue('category_id', p.category_id)
    }
    if (p.brand) {
      setBrandInitialOptions([{ id: p.brand.id, name: p.brand.name }])
      setValue('brand_id', p.brand.id)
    } else if (p.brand_id != null) {
      setValue('brand_id', p.brand_id)
    }
    if (p.material) {
      setMaterialInitialOptions([{ id: p.material.id, name: p.material.name }])
      setValue('material_id', p.material.id)
    } else if (p.material_id != null) {
      setValue('material_id', p.material_id)
    }
    setValue('size_value', p.size_value ?? undefined)
    if (p.measurement_unit) {
      setMeasurementUnitInitialOptions([{
        id: p.measurement_unit.id,
        name: p.measurement_unit.name,
        abbreviation: p.measurement_unit.abbreviation,
      }])
      setValue('measurement_unit_id', p.measurement_unit.id)
    } else if (p.measurement_unit_id != null) {
      setValue('measurement_unit_id', p.measurement_unit_id)
    }
    setValue('price', p.price ?? 0)
    setValue('cost', p.cost ?? 0)
    setValue('stock_quantity', p.stock_quantity ?? p.stock ?? 0)
    setValue('min_stock_quantity', p.low_stock_threshold ?? p.min_stock ?? 0)
  }

  useEffect(() => {
    if (!isCreateMode && productData) {
      populateForm(productData)
    }
  }, [isCreateMode, productData])

  useEffect(() => {
    if (!isCreateMode && productError) {
      navigate('/inventory')
    }
  }, [isCreateMode, productError, navigate])

  async function submitCreateEntity(values: Record<string, string>): Promise<FilterOption> {
    switch (activeModal) {
      case 'category': {
        const { code, name, description } = values
        return (await inventoryApi.createCategory({ code, name, ...(description && { description }) })).data
      }
      case 'brand': return (await inventoryApi.createBrand(values as { name: string; logo_url?: string })).data
      case 'material': return (await inventoryApi.createMaterial(values as { name: string })).data
      case 'measurementUnit': return (await inventoryApi.createMeasurementUnit(values as { name: string; abbreviation: string })).data
      default: throw new Error('Unknown entity type')
    }
  }

  function handleEntityCreated(entity: FilterOption) {
    switch (activeModal) {
      case 'category':
        setCategoryInitialOptions(prev => [...prev, entity])
        setValue('category_id', entity.id, { shouldValidate: true })
        break
      case 'brand':
        setBrandInitialOptions(prev => [...prev, entity])
        setValue('brand_id', entity.id, { shouldValidate: true })
        break
      case 'material':
        setMaterialInitialOptions(prev => [...prev, entity])
        setValue('material_id', entity.id, { shouldValidate: true })
        break
      case 'measurementUnit':
        setMeasurementUnitInitialOptions(prev => [...prev, entity])
        setValue('measurement_unit_id', entity.id, { shouldValidate: true })
        break
    }
    setActiveModal(null)
  }

  function handleCancel() {
    if (productData) populateForm(productData)
    setIsEditing(false)
    setOpenDropdownId(null)
    setActiveModal(null)
    setApiError(null)
  }

  async function handleDelete() {
    await inventoryApi.deleteProduct(Number(id))
    queryClient.invalidateQueries({ queryKey: ['products'] })
    queryClient.invalidateQueries({ queryKey: ['inventoryStats'] })
    queryClient.invalidateQueries({ queryKey: ['dashboardSummary'] })
    queryClient.invalidateQueries({ queryKey: ['dashboardTopProducts'] })
    queryClient.invalidateQueries({ queryKey: ['dashboardLowStock'] })
    navigate('/inventory', { state: { successMessage: 'Producto eliminado exitosamente' } })
  }

  async function onSubmit(data: ProductFormInputs) {
    setApiError(null)
    try {
      const { price, sku, ...rest } = data
      const payload: CreateProductInput = {
        name: rest.name,
        brand_id: rest.brand_id,
        category_id: rest.category_id,
        material_id: rest.material_id,
        low_stock_threshold: rest.min_stock_quantity,
        cost: rest.cost,
        price: price,
        stock_quantity: rest.stock_quantity,
        size_value: !rest.size_value || rest.size_value.trim() === '' ? null : rest.size_value,
        measurement_unit_id: rest.measurement_unit_id ?? null,
      }
      if (isCreateMode) {
        await inventoryApi.createProduct({ ...payload, sku })
        queryClient.invalidateQueries({ queryKey: ['products'] })
        queryClient.invalidateQueries({ queryKey: ['inventoryStats'] })
        queryClient.invalidateQueries({ queryKey: ['dashboardSummary'] })
        queryClient.invalidateQueries({ queryKey: ['dashboardTopProducts'] })
        queryClient.invalidateQueries({ queryKey: ['dashboardLowStock'] })
        navigate('/inventory', { state: { successMessage: 'Producto creado exitosamente' } })
      } else {
        const response = await inventoryApi.updateProduct(Number(id), { ...payload, sku })
        queryClient.invalidateQueries({ queryKey: ['product', Number(id)] })
        queryClient.invalidateQueries({ queryKey: ['products'] })
        queryClient.invalidateQueries({ queryKey: ['inventoryStats'] })
        queryClient.invalidateQueries({ queryKey: ['dashboardSummary'] })
        queryClient.invalidateQueries({ queryKey: ['dashboardTopProducts'] })
        queryClient.invalidateQueries({ queryKey: ['dashboardLowStock'] })
        populateForm(response.data)
        setIsEditing(false)
        setOpenDropdownId(null)
      }
    } catch (error) {
      const msg = error instanceof Error ? error.message :
        isCreateMode ? 'Error al crear producto' : 'Error al actualizar producto'
      setApiError(msg)
    }
  }

  if (productLoading) {
    return (
      <div className={styles.pageContainer}>
        <div className={styles.loadingState}>
          <Loader size={24} className={styles.spinner} />
          <p>Cargando producto...</p>
        </div>
      </div>
    )
  }

  const stock = productData ? (productData.stock_quantity ?? productData.stock ?? 0) : 0
  const threshold = productData ? (productData.low_stock_threshold ?? productData.min_stock ?? 0) : 0
  const status = getProductStatus(stock, threshold)
  const isFormActive = isEditing || isCreateMode

  return (
    <form onSubmit={handleSubmit(onSubmit)} className={styles.pageContainer}>
      <div className={styles.pageHeader}>
        <div className={styles.headerLeft}>
          <button
            type="button"
            onClick={() => navigate('/inventory')}
            className={styles.backButton}
            title="Volver al inventario"
          >
            <ChevronLeft size={20} />
          </button>
          <div className={styles.headerContent}>
            <div className={styles.breadcrumb}>
              <span>Inventario</span>
              <span className={styles.breadcrumbDivider}>/</span>
              <span className={styles.breadcrumbActive}>
                {isCreateMode ? 'Registro' : 'Producto'}
              </span>
            </div>
            {isCreateMode ? (
              <h1 className={styles.pageTitle}>Registro de producto</h1>
            ) : (
              <div className={styles.titleRow}>
                <h1 className={styles.pageTitle}>{productData?.name || '-'}</h1>
                <span className={styles.chip}>{productData?.sku}</span>
                <span className={`badge ${productStatusMap[status]}`}>
                  {productStatusLabel[status]}
                </span>
              </div>
            )}
          </div>
        </div>
        <div className={styles.headerActions}>
          {isCreateMode ? (
            <button type="submit" disabled={isSubmitting} className={`btn-primary ${styles.btnDisabled}`}>
              {isSubmitting ? (
                <><Loader size={15} className={styles.spinnerDark} /> Guardando...</>
              ) : (
                'Guardar Producto'
              )}
            </button>
          ) : isEditing ? (
            <>
              <button
                key="btn-cancel"
                type="button"
                onClick={handleCancel}
                className="btn-ghost"
              >
                <X size={15} />
                Cancelar
              </button>
              <button
                key="btn-save"
                type="submit"
                disabled={isSubmitting}
                className={`btn-primary ${styles.btnDisabled}`}
              >
                {isSubmitting ? <Loader size={15} className={styles.spinnerDark} /> : <Check size={15} />}
                {isSubmitting ? 'Guardando...' : 'Guardar'}
              </button>
            </>
          ) : (
            <button
              key="btn-edit"
              type="button"
              onClick={() => setIsEditing(true)}
              className="btn-primary"
            >
              <Pencil size={15} />
              Editar
            </button>
          )}
        </div>
      </div>

      {apiError && <div className={styles.errorBox}>{apiError}</div>}

      <div className={styles.sectionsGrid}>
        <section className={styles.card}>
          <h2 className={styles.cardTitle}>Información general</h2>
          <dl className={styles.fieldList}>

            <div className={isFormActive ? styles.fieldRowEditing : styles.specField}>
              <dt className={isFormActive ? styles.fieldLabel : styles.specLabel}>Nombre</dt>
              {isFormActive ? (
                <dd className={styles.fieldValueFull}>
                  <input
                    type="text"
                    className={styles.inputField}
                    placeholder="Nombre del producto"
                    {...register('name')}
                  />
                  {errors.name && <p className={styles.errorMessage}>{errors.name.message}</p>}
                </dd>
              ) : (
                <dd className={styles.specValue}>{productData?.name || '-'}</dd>
              )}
            </div>

            {/* SKU — only shown in view/edit mode */}
            {!isCreateMode && (
              <div className={isEditing ? styles.fieldRowEditing : styles.specField}>
                <dt className={isFormActive ? styles.fieldLabel : styles.specLabel}>SKU</dt>
                {isEditing ? (
                  <dd className={styles.fieldValueFull}>
                    <input
                      type="text"
                      className={`${styles.inputField} ${styles.mono}`}
                      {...register('sku')}
                    />
                    {errors.sku && <p className={styles.errorMessage}>{errors.sku.message}</p>}
                  </dd>
                ) : (
                  <dd className={`${styles.specValue} ${styles.mono}`}>{productData?.sku}</dd>
                )}
              </div>
            )}

            <div className={isFormActive ? styles.fieldRowEditing : styles.specField}>
              <dt className={isFormActive ? styles.fieldLabel : styles.specLabel}>Categoría</dt>
              {isFormActive ? (
                <dd className={styles.fieldValueFull}>
                  <ProductDropdown
                    id="category"
                    label="categoría"
                    value={watch('category_id') || null}
                    openDropdownId={openDropdownId}
                    onOpenChange={setOpenDropdownId}
                    fetchOptions={(search) =>
                      inventoryApi.getCategories({ search: search || undefined, skip: 0, limit: 10 })
                        .then(r => r.data.items || [])
                    }
                    onSelect={(opt) => {
                      setCategoryInitialOptions(prev => withOption(prev, opt))
                      setValue('category_id', opt.id, { shouldValidate: true })
                    }}
                    onAddNew={() => setActiveModal('category')}
                    initialOptions={categoryInitialOptions}
                    error={errors.category_id?.message}
                  />
                </dd>
              ) : (
                <dd className={styles.specValue}>{productData?.category?.name || '-'}</dd>
              )}
            </div>

            <div className={isFormActive ? styles.fieldRowEditing : styles.specField}>
              <dt className={isFormActive ? styles.fieldLabel : styles.specLabel}>Marca</dt>
              {isFormActive ? (
                <dd className={styles.fieldValueFull}>
                  <ProductDropdown
                    id="brand"
                    label="marca"
                    value={watch('brand_id') || null}
                    openDropdownId={openDropdownId}
                    onOpenChange={setOpenDropdownId}
                    fetchOptions={(search) =>
                      inventoryApi.getBrands({ search: search || undefined, skip: 0, limit: 10 })
                        .then(r => r.data.items || [])
                    }
                    onSelect={(opt) => {
                      setBrandInitialOptions(prev => withOption(prev, opt))
                      setValue('brand_id', opt.id, { shouldValidate: true })
                    }}
                    onAddNew={() => setActiveModal('brand')}
                    initialOptions={brandInitialOptions}
                    error={errors.brand_id?.message}
                  />
                </dd>
              ) : (
                <dd className={styles.specValue}>{productData?.brand?.name || '-'}</dd>
              )}
            </div>

            <div className={isFormActive ? styles.fieldRowEditing : styles.specField}>
              <dt className={isFormActive ? styles.fieldLabel : styles.specLabel}>Material</dt>
              {isFormActive ? (
                <dd className={styles.fieldValueFull}>
                  <ProductDropdown
                    id="material"
                    label="material"
                    value={watch('material_id') || null}
                    openDropdownId={openDropdownId}
                    onOpenChange={setOpenDropdownId}
                    fetchOptions={(search) =>
                      inventoryApi.getMaterials({ search: search || undefined, skip: 0, limit: 10 })
                        .then(r => r.data.items || [])
                    }
                    onSelect={(opt) => {
                      setMaterialInitialOptions(prev => withOption(prev, opt))
                      setValue('material_id', opt.id, { shouldValidate: true })
                    }}
                    onAddNew={() => setActiveModal('material')}
                    initialOptions={materialInitialOptions}
                    error={errors.material_id?.message}
                  />
                </dd>
              ) : (
                <dd className={styles.specValue}>{productData?.material?.name || '-'}</dd>
              )}
            </div>

            <div className={isFormActive ? styles.fieldRowEditing : styles.specField}>
              <dt className={isFormActive ? styles.fieldLabel : styles.specLabel}>Tamaño</dt>
              {isFormActive ? (
                <dd className={styles.fieldValueFull}>
                  <input
                    type="text"
                    className={`${styles.inputField} ${styles.mono}`}
                    placeholder="ej: 10, 5.5, 12 x 5"
                    {...register('size_value')}
                  />
                  {errors.size_value && <p className={styles.errorMessage}>{errors.size_value.message}</p>}
                </dd>
              ) : (
                <dd className={`${styles.specValue} ${styles.mono}`}>{productData?.size_value || '-'}</dd>
              )}
            </div>

            <div className={isFormActive ? styles.fieldRowEditing : styles.specField}>
              <dt className={isFormActive ? styles.fieldLabel : styles.specLabel}>Unidad de medida</dt>
              {isFormActive ? (
                <dd className={styles.fieldValueFull}>
                  <ProductDropdown
                    id="measurementUnit"
                    label="unidad de medida"
                    value={watch('measurement_unit_id') || null}
                    openDropdownId={openDropdownId}
                    onOpenChange={setOpenDropdownId}
                    fetchOptions={(search) =>
                      inventoryApi.getMeasurementUnits({ search: search || undefined, skip: 0, limit: 10 })
                        .then(r => r.data.items || [])
                    }
                    onSelect={(opt) => {
                      setMeasurementUnitInitialOptions(prev => withOption(prev, opt))
                      setValue('measurement_unit_id', opt.id, { shouldValidate: true })
                    }}
                    onAddNew={() => setActiveModal('measurementUnit')}
                    formatLabel={(opt) => opt.abbreviation ? `${opt.abbreviation} — ${opt.name}` : opt.name}
                    initialOptions={measurementUnitInitialOptions}
                    error={errors.measurement_unit_id?.message}
                  />
                </dd>
              ) : (
                <dd className={styles.specValue}>
                  {productData?.measurement_unit
                    ? `${productData.measurement_unit.abbreviation} — ${productData.measurement_unit.name}`
                    : '-'}
                </dd>
              )}
            </div>

          </dl>
        </section>

        <div className={styles.rightColumn}>
          <section className={styles.card}>
            <h2 className={styles.cardTitle}>Valores económicos</h2>
            <dl className={styles.fieldList}>
              <div className={isFormActive ? styles.fieldRowEditing : styles.specField}>
                <dt className={isFormActive ? styles.fieldLabel : styles.specLabel}>Precio de venta</dt>
                {isFormActive ? (
                  <dd className={styles.fieldValueFull}>
                    <input
                      type="number"
                      className={`${styles.inputField} ${styles.mono} ${styles.numeric}`}
                      placeholder="0.00"
                      step="0.01"
                      {...register('price', { valueAsNumber: true })}
                    />
                    {errors.price && <p className={styles.errorMessage}>{errors.price.message}</p>}
                  </dd>
                ) : (
                  <dd className={`${styles.specValue} ${styles.mono}`}>
                    $ {(productData?.price ?? 0).toFixed(2)}
                  </dd>
                )}
              </div>

              <div className={isFormActive ? styles.fieldRowEditing : styles.specField}>
                <dt className={isFormActive ? styles.fieldLabel : styles.specLabel}>Costo</dt>
                {isFormActive ? (
                  <dd className={styles.fieldValueFull}>
                    <input
                      type="number"
                      className={`${styles.inputField} ${styles.mono} ${styles.numeric}`}
                      placeholder="0.00"
                      step="0.01"
                      {...register('cost', { valueAsNumber: true })}
                    />
                    {errors.cost && <p className={styles.errorMessage}>{errors.cost.message}</p>}
                  </dd>
                ) : (
                  <dd className={`${styles.specValue} ${styles.mono}`}>
                    $ {(productData?.cost ?? 0).toFixed(2)}
                  </dd>
                )}
              </div>
            </dl>
          </section>

          <section className={styles.card}>
            <h2 className={styles.cardTitle}>Control de stock</h2>
            <dl className={styles.fieldList}>
              <div className={isFormActive ? styles.fieldRowEditing : styles.specField}>
                <dt className={isFormActive ? styles.fieldLabel : styles.specLabel}>
                  {isCreateMode ? 'Cantidad inicial' : 'Stock actual'}
                </dt>
                {isFormActive ? (
                  <dd className={styles.fieldValueFull}>
                    <input
                      type="number"
                      className={`${styles.inputField} ${styles.mono} ${styles.numeric}`}
                      placeholder="0"
                      {...register('stock_quantity', { valueAsNumber: true })}
                    />
                    {errors.stock_quantity && (
                      <p className={styles.errorMessage}>{errors.stock_quantity.message}</p>
                    )}
                  </dd>
                ) : (
                  <dd className={`${styles.specValue} ${styles.mono} ${status === 'low_stock' || status === 'out_of_stock' ? styles.valueDanger : ''}`}>
                    {stock}
                  </dd>
                )}
              </div>

              <div className={isFormActive ? styles.fieldRowEditing : styles.specField}>
                <dt className={isFormActive ? styles.fieldLabel : styles.specLabel}>Cantidad mínima</dt>
                {isFormActive ? (
                  <dd className={styles.fieldValueFull}>
                    <input
                      type="number"
                      className={`${styles.inputField} ${styles.mono} ${styles.numeric}`}
                      placeholder="0"
                      {...register('min_stock_quantity', { valueAsNumber: true })}
                    />
                    {errors.min_stock_quantity && (
                      <p className={styles.errorMessage}>{errors.min_stock_quantity.message}</p>
                    )}
                  </dd>
                ) : (
                  <dd className={`${styles.specValue} ${styles.mono}`}>{threshold}</dd>
                )}
              </div>

              {!isCreateMode && (
                <div className={styles.specField}>
                  <dt className={isFormActive ? styles.fieldLabel : styles.specLabel}>Disponibilidad</dt>
                  <dd className={styles.specValue}>
                    <span className={`badge ${productStatusMap[status]}`}>{productStatusLabel[status]}</span>
                  </dd>
                </div>
              )}
            </dl>
            {!isCreateMode && (
              <div className={pageStyles.stockMeterBlock}>
                <StockMeter
                  value={stock}
                  max={Math.max(threshold, 1)}
                  tone={status === 'out_of_stock' ? 'danger' : status === 'low_stock' ? 'warning' : 'neutral'}
                  label={`Stock ${stock} de mínimo ${threshold}`}
                />
                <div className={pageStyles.stockMeta}>
                  <span>Mínimo {threshold}</span>
                  <span>{stock} en stock</span>
                </div>
              </div>
            )}
          </section>
        </div>
      </div>

      {!isCreateMode && (
        <section className={styles.dangerZone}>
          <div>
            <h2 className={styles.cardTitle}>Zona de riesgo</h2>
            <p className={styles.dangerZoneText}>
              Eliminar este producto lo quita del inventario. Esta acción no se puede deshacer.
            </p>
          </div>
          <button
            type="button"
            onClick={() => setShowDeleteModal(true)}
            className={styles.dangerButton}
          >
            <Trash2 size={15} />
            Eliminar
          </button>
        </section>
      )}

      {activeModal && (
        <CreateFormModal<FilterOption>
          title={ENTITY_TITLES[activeModal]}
          fields={ENTITY_FIELDS[activeModal]}
          onSubmit={submitCreateEntity}
          onClose={() => setActiveModal(null)}
          onCreated={handleEntityCreated}
        />
      )}

      {showDeleteModal && productData && (
        <ConfirmDeleteModal
          title="Eliminar Producto"
          productName={productData.name}
          onConfirm={handleDelete}
          onClose={() => setShowDeleteModal(false)}
        />
      )}
    </form>
  )
}
