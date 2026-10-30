import { useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { Plus, Eye, Trash2 } from 'lucide-react'
import styles from './Inventory.module.css'
import PageHeader from '@/components/PageHeader/PageHeader'
import CommandBar from '@/components/CommandBar/CommandBar'
import DataCard from '@/components/DataCard/DataCard'
import FilterSelect from '@/components/FilterSelect/FilterSelect'
import SearchSelect from '@/components/SearchSelect/SearchSelect'
import { inventoryApi } from '@/lib/api/inventory'
import { PAGE_LIMIT } from '@/lib/constants'
import { formatCurrency, formatNumber, getProductStatus } from '@/lib/utils'
import ConfirmDeleteModal from '@/components/ConfirmDeleteModal/ConfirmDeleteModal'
import AttributeTab from '@/components/AttributeTab/AttributeTab'
import DataTable from '@/components/DataTable/DataTable'
import KpiStrip from '@/components/KpiStrip/KpiStrip'
import StockMeter from '@/components/StockMeter/StockMeter'
import { useInventory } from '@/hooks/useInventory'
import { useDebouncedValue } from '@/hooks/useDebouncedValue'
import { useDeleteConfirm } from '@/hooks/useDeleteConfirm'
import type { FieldConfig } from '@/components/CreateFormModal/CreateFormModal'

interface AttributeTabConfig {
  key: string
  label: string
  columns: Array<{ key: string; label: string }>
  fields: FieldConfig[]
  createTitle: string
  editTitle: string
  deleteTitle: string
  fetchFn: (params: { search?: string; skip?: number; limit?: number }) => Promise<{ data: { items: { id: number; name: string }[] } }>
  createFn: (data: unknown) => Promise<unknown>
  updateFn: (id: number, data: unknown) => Promise<unknown>
  deleteFn: (id: number) => Promise<unknown>
}

const ATTRIBUTE_TABS: AttributeTabConfig[] = [
  {
    key: 'categorias',
    label: 'Categorías',
    columns: [{ key: 'code', label: 'Código' }, { key: 'name', label: 'Nombre' }, { key: 'description', label: 'Descripción' }],
    fields: [
      { key: 'code', label: 'Código', placeholder: 'Código de la categoría', required: true },
      { key: 'name', label: 'Nombre', placeholder: 'Nombre de la categoría', required: true },
      { key: 'description', label: 'Descripción (opcional)', type: 'textarea', placeholder: 'Descripción de la categoría' },
    ],
    createTitle: 'Nueva Categoría',
    editTitle: 'Editar Categoría',
    deleteTitle: 'Eliminar Categoría',
    fetchFn: inventoryApi.getCategories,
    createFn: (data) => inventoryApi.createCategory(data as { code: string; name: string; description?: string }),
    updateFn: (id, data) => inventoryApi.updateCategory(id, data as { code: string; name: string; description?: string }),
    deleteFn: inventoryApi.deleteCategory,
  },
  {
    key: 'marcas',
    label: 'Marcas',
    columns: [{ key: 'name', label: 'Nombre' }, { key: 'logo_url', label: 'Logo URL' }],
    fields: [
      { key: 'name', label: 'Nombre', placeholder: 'Nombre de la marca', required: true },
      { key: 'logo_url', label: 'Logo URL (opcional)', type: 'url', placeholder: 'https://ejemplo.com/logo.png' },
    ],
    createTitle: 'Nueva Marca',
    editTitle: 'Editar Marca',
    deleteTitle: 'Eliminar Marca',
    fetchFn: inventoryApi.getBrands,
    createFn: (data) => inventoryApi.createBrand(data as { name: string; logo_url?: string }),
    updateFn: (id, data) => inventoryApi.updateBrand(id, data as { name: string; logo_url?: string }),
    deleteFn: inventoryApi.deleteBrand,
  },
  {
    key: 'materiales',
    label: 'Materiales',
    columns: [{ key: 'name', label: 'Nombre' }],
    fields: [
      { key: 'name', label: 'Nombre', placeholder: 'Nombre del material', required: true },
    ],
    createTitle: 'Nuevo Material',
    editTitle: 'Editar Material',
    deleteTitle: 'Eliminar Material',
    fetchFn: inventoryApi.getMaterials,
    createFn: (data) => inventoryApi.createMaterial(data as { name: string }),
    updateFn: (id, data) => inventoryApi.updateMaterial(id, data as { name: string }),
    deleteFn: inventoryApi.deleteMaterial,
  },
  {
    key: 'unidades',
    label: 'Unidades de Medida',
    columns: [{ key: 'name', label: 'Nombre' }, { key: 'abbreviation', label: 'Abreviatura' }],
    fields: [
      { key: 'name', label: 'Nombre', placeholder: 'Centímetros', required: true },
      { key: 'abbreviation', label: 'Abreviatura', placeholder: 'cm', required: true },
    ],
    createTitle: 'Nueva Unidad',
    editTitle: 'Editar Unidad',
    deleteTitle: 'Eliminar Unidad',
    fetchFn: inventoryApi.getMeasurementUnits,
    createFn: (data) => inventoryApi.createMeasurementUnit(data as { name: string; abbreviation: string }),
    updateFn: (id, data) => inventoryApi.updateMeasurementUnit(id, data as { name: string; abbreviation: string }),
    deleteFn: inventoryApi.deleteMeasurementUnit,
  },
]

const fetchCategories = (search: string) =>
  inventoryApi.getCategories({ search: search || undefined, skip: 0, limit: PAGE_LIMIT }).then(r => r.data.items || [])

const fetchMaterials = (search: string) =>
  inventoryApi.getMaterials({ search: search || undefined, skip: 0, limit: PAGE_LIMIT }).then(r => r.data.items || [])

const fetchBrands = (search: string) =>
  inventoryApi.getBrands({ search: search || undefined, skip: 0, limit: PAGE_LIMIT }).then(r => r.data.items || [])

const VALID_TABS = ['productos', 'categorias', 'marcas', 'materiales', 'unidades'] as const

export default function Inventory() {
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const tabParam = searchParams.get('tab')
  const activeTab = tabParam && VALID_TABS.includes(tabParam as typeof VALID_TABS[number]) ? tabParam : 'productos'
  const [search, setSearch] = useState('')
  const [selectedCategory, setSelectedCategory] = useState<number | ''>('')
  const [selectedMaterial, setSelectedMaterial] = useState<number | ''>('')
  const [selectedBrand, setSelectedBrand] = useState<number | ''>('')
  const [selectedAvailability, setSelectedAvailability] = useState('')

  const [page, setPage] = useState(1)

  const debouncedSearch = useDebouncedValue(search, 300, () => setPage(1))

  const { products, hasNext, stats, isLoading, isFetching, isError, refetch, deleteMutation } = useInventory({
    page,
    search: debouncedSearch,
    filters: {
      category_id: selectedCategory,
      material_id: selectedMaterial,
      brand_id: selectedBrand,
      stock_status: selectedAvailability || undefined,
    },
  })

  const {
    target: productToDelete,
    requestDelete: requestProductDelete,
    close: closeProductDelete,
    confirm: confirmProductDelete,
  } = useDeleteConfirm<{ id: number; name: string }>(deleteMutation.mutateAsync)

  const hasActiveFilters = search !== '' || !!selectedCategory || !!selectedMaterial || !!selectedBrand || !!selectedAvailability

  function clearFilters() {
    setSearch('')
    setSelectedCategory('')
    setSelectedMaterial('')
    setSelectedBrand('')
    setSelectedAvailability('')
    setPage(1)
  }

  const activeLabelMap: Record<string, string> = {
    productos: 'Productos',
    ...Object.fromEntries(ATTRIBUTE_TABS.map(t => [t.key, t.label])),
  }

  return (
    <div className={styles.container}>
      <PageHeader
        activeLabel={activeLabelMap[activeTab] ?? 'Productos'}
        title="Inventario Principal"
        action={
          <button
            className="btn-new"
            onClick={() => navigate('/inventory/create')}
          >
            <Plus size={16} strokeWidth={2.5} />
            <span>Nuevo Producto</span>
          </button>
        }
      />

      <KpiStrip cards={[
        { label: 'Productos', value: formatNumber(stats?.totalProducts ?? 0) },
        { label: 'Stock total', value: formatNumber(stats?.totalStock ?? 0) },
        { label: 'Stock bajo', value: formatNumber(stats?.lowStockCount ?? 0) },
        { label: 'Valor total', value: formatCurrency(stats?.totalValue ?? 0) },
      ]} />

      <nav className={styles.tabBar}>
        <button
          className={`${styles.tab} ${activeTab === 'productos' ? styles.tabActive : ''}`}
          onClick={() => setSearchParams({})}
        >
          Productos
        </button>
        {ATTRIBUTE_TABS.map(tab => (
          <button
            key={tab.key}
            className={`${styles.tab} ${activeTab === tab.key ? styles.tabActive : ''}`}
            onClick={() => setSearchParams({ tab: tab.key })}
          >
            {tab.label}
          </button>
        ))}
      </nav>

      {activeTab !== 'productos' && (() => {
        const tabConfig = ATTRIBUTE_TABS.find(t => t.key === activeTab)
        if (!tabConfig) return null
        return (
          <AttributeTab
            key={activeTab}
            fetchFn={tabConfig.fetchFn}
            searchKey="name"
            columns={tabConfig.columns}
            fields={tabConfig.fields}
            createTitle={tabConfig.createTitle}
            editTitle={tabConfig.editTitle}
            deleteTitle={tabConfig.deleteTitle}
            searchLabel={tabConfig.label}
            createFn={tabConfig.createFn}
            updateFn={tabConfig.updateFn}
            deleteFn={tabConfig.deleteFn}
          />
        )
      })()}

      {activeTab === 'productos' && <>
      <div className={styles.tableSection}>
        <CommandBar
          search={search}
          onSearchChange={setSearch}
          placeholder="Nombre o ID..."
          hasActiveFilters={hasActiveFilters}
          onClearFilters={clearFilters}
        >

            <SearchSelect
              label="Categoría"
              value={selectedCategory}
              onChange={v => { setSelectedCategory(v); setPage(1) }}
              fetchOptions={fetchCategories}
              emptyLabel="Todas las Categorías"
            />

            <SearchSelect
              label="Material"
              value={selectedMaterial}
              onChange={v => { setSelectedMaterial(v); setPage(1) }}
              fetchOptions={fetchMaterials}
              emptyLabel="Todos los Materiales"
            />

            <SearchSelect
              label="Marca"
              value={selectedBrand}
              onChange={v => { setSelectedBrand(v); setPage(1) }}
              fetchOptions={fetchBrands}
              emptyLabel="Todas las Marcas"
            />

            <FilterSelect
              label="Disponibilidad"
              value={selectedAvailability}
              onChange={v => { setSelectedAvailability(v); setPage(1) }}
              options={[
                { value: 'in_stock', label: 'En Stock' },
                { value: 'low_stock', label: 'Stock Bajo' },
                { value: 'out_of_stock', label: 'Sin Stock' },
              ]}
            />
        </CommandBar>

        <DataCard>
          <DataTable
            loading={isLoading}
            empty={products.length === 0}
            error={isError && products.length === 0}
            onRetry={refetch}
            loadingText="Cargando productos..."
            emptyText="No se encontraron productos."
            emptyAction={hasActiveFilters ? (
              <button className={styles.emptyAction} onClick={clearFilters}>Quitar filtros</button>
            ) : undefined}
            minWidth="760px"
            currentPage={page}
            hasNext={hasNext}
            skeletonRows={5}
            skeletonColumns={7}
            stickyFirstColumn
            onPageChange={setPage}
            isPaginationLoading={isFetching && !isLoading}
          >
            <thead>
              <tr>
                <th scope="col" className={styles.colProduct}>Producto</th>
                <th scope="col" className={styles.colCategory}>Categoría</th>
                <th scope="col" className={styles.colBrand}>Marca</th>
                <th scope="col" className={`${styles.colStock} num`}>Stock</th>
                <th scope="col" className={`${styles.colPrice} num`}>Precio</th>
                <th scope="col" className={styles.colStatus}>Estado</th>
                <th scope="col" className={`${styles.colActions} num`}>Acciones</th>
              </tr>
            </thead>
            <tbody>
              {products.map(p => {
                const stock = p.stock_quantity || 0
                const threshold = p.low_stock_threshold || 0
                const status = getProductStatus(stock, threshold)
                const unit = p.measurement_unit?.abbreviation || p.unit || ''
                const size = p.size_value
                  ? [p.size_value, p.measurement_unit?.abbreviation].filter(Boolean).join(' ')
                  : ''
                return (
                  <tr key={p.id}>
                    <td className={styles.colProduct}>
                      <button
                        className={styles.productName}
                        onClick={() => navigate(`/inventory/product/${p.id}`)}
                        title="Ver producto"
                      >
                        {p.name || '-'}
                      </button>
                      <span className={styles.productMeta}>{p.sku}{size ? ` · ${size}` : ''}</span>
                      <span className={styles.mobileMeta}>
                        {[p.category?.name, size, formatCurrency(p.price || 0)].filter(Boolean).join(' · ')}
                      </span>
                    </td>
                    <td className={styles.colCategory}>
                      <span className={styles.categoryName}>{p.category?.name || '-'}</span>
                      <span className={styles.cellMeta}>{p.material?.name || 'Sin material'}</span>
                    </td>
                    <td className={styles.colBrand}><span className={styles.brandCell}>{p.brand?.name || '-'}</span></td>
                    <td className={`${styles.colStock} num`}>
                      <span className={styles.stockValue}>{formatNumber(stock)} <span className={styles.stockUnit}>{unit}</span></span>
                      <StockMeter
                        value={stock}
                        max={threshold || stock || 1}
                        tone={status === 'out_of_stock' ? 'danger' : status === 'low_stock' ? 'warning' : 'neutral'}
                        label={`${p.name}: ${stock} ${unit} de mínimo ${threshold}`}
                      />
                      <span className={styles.minText}>Mín. {formatNumber(threshold)} {unit}</span>
                    </td>
                    <td className={`${styles.colPrice} num`}>
                      <span className={styles.priceCell}>{formatCurrency(p.price || 0)}</span>
                    </td>
                    <td className={styles.colStatus}>
                      {status === 'out_of_stock' && <span className="badge badge--destructive">Sin stock</span>}
                      {status === 'low_stock' && <span className="badge badge--warning">Bajo mínimo</span>}
                    </td>
                    <td className={`${styles.colActions} num`}>
                      <div className={styles.rowActions}>
                        <button
                          className="action-btn"
                          title="Ver producto"
                          onClick={() => navigate(`/inventory/product/${p.id}`)}
                        >
                          <Eye size={15} />
                        </button>
                        <button
                          className="action-btn action-btn--destructive"
                          title="Eliminar"
                          onClick={() => requestProductDelete({ id: p.id, name: p.name + ' ' + p.size_value + ' ' + p.measurement_unit?.abbreviation })}
                        >
                          <Trash2 size={15} />
                        </button>
                      </div>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </DataTable>
        </DataCard>
      </div>
      </>}

      {productToDelete && (
        <ConfirmDeleteModal
          title="Eliminar Producto"
          productName={productToDelete.name}
          onConfirm={confirmProductDelete}
          onClose={closeProductDelete}
        />
      )}
    </div>
  )
}
