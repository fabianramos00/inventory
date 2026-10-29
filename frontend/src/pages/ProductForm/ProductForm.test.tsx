import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ProductForm from './ProductForm'

const mocks = vi.hoisted(() => ({
  createProduct: vi.fn(),
  getProduct: vi.fn(),
  updateProduct: vi.fn(),
  deleteProduct: vi.fn(),
  createCategory: vi.fn(),
  createBrand: vi.fn(),
  getCategories: vi.fn(),
  getBrands: vi.fn(),
  getMaterials: vi.fn(),
  getMeasurementUnits: vi.fn(),
}))

vi.mock('@/lib/api/inventory', () => ({
  inventoryApi: {
    createProduct: mocks.createProduct,
    getProduct: mocks.getProduct,
    updateProduct: mocks.updateProduct,
    deleteProduct: mocks.deleteProduct,
    createCategory: mocks.createCategory,
    createBrand: mocks.createBrand,
    getCategories: mocks.getCategories,
    getBrands: mocks.getBrands,
    getMaterials: mocks.getMaterials,
    getMeasurementUnits: mocks.getMeasurementUnits,
  },
}))

const productData = {
  id: 1,
  name: 'Martillo',
  sku: 'SKU-1',
  category: { id: 1, code: 'HER', name: 'Herramientas' },
  brand: { id: 1, name: 'Truper' },
  material: { id: 1, name: 'Acero' },
  stock: 4,
  stock_quantity: 4,
  min_stock: 5,
  low_stock_threshold: 5,
  price: 19.9,
  cost: 10,
  size_value: '10',
  measurement_unit: { id: 1, name: 'Centímetros', abbreviation: 'cm' },
}

const plainProduct = {
  id: 1,
  name: 'Martillo',
  sku: 'SKU-1',
  category_id: 1,
  brand_id: 1,
  material_id: 1,
  measurement_unit_id: 1,
  stock_quantity: 4,
  low_stock_threshold: 5,
  price: 19.9,
  cost: 10,
  size_value: '10',
}

const expectedKeys = ['products', 'inventoryStats', 'dashboardSummary', 'dashboardTopProducts', 'dashboardLowStock']

function renderProductForm(path = '/inventory/product/1') {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

  render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/inventory/create" element={<ProductForm />} />
          <Route path="/inventory/product/:id" element={<ProductForm />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )

  return { invalidateSpy }
}

function dropdownTrigger(label: string) {
  const addButton = screen.getByTitle(`Agregar nuevo ${label}`)
  return within(addButton.parentElement as HTMLElement).getAllByRole('button')[0]
}

async function selectDropdown(label: string, option: string) {
  fireEvent.click(dropdownTrigger(label))
  fireEvent.click(await screen.findByRole('button', { name: option }, { timeout: 2000 }))
}

function fillCreateProduct() {
  fireEvent.change(screen.getByPlaceholderText('Nombre del producto'), { target: { value: 'Martillo' } })
  const [price, cost, stock, minStock] = screen.getAllByRole('spinbutton')
  fireEvent.change(price, { target: { value: '19.9' } })
  fireEvent.change(cost, { target: { value: '10' } })
  fireEvent.change(stock, { target: { value: '4' } })
  fireEvent.change(minStock, { target: { value: '5' } })
  fireEvent.change(screen.getByPlaceholderText('ej: 10, 5.5, 12 x 5'), { target: { value: '10' } })
}

describe('ProductForm', () => {
  beforeEach(() => {
    mocks.getProduct.mockReset().mockResolvedValue({ data: productData })
    mocks.updateProduct.mockReset().mockResolvedValue({ data: productData })
    mocks.createProduct.mockReset().mockResolvedValue({ data: plainProduct })
    mocks.deleteProduct.mockReset().mockResolvedValue(undefined)
    mocks.createCategory.mockReset()
    mocks.createBrand.mockReset().mockResolvedValue({ data: { id: 2, name: 'Truper' } })
    mocks.getCategories.mockReset().mockResolvedValue({
      data: { items: [{ id: 1, code: 'HER', name: 'Herramientas' }, { id: 2, code: 'FIJ', name: 'Fijaciones' }] },
    })
    mocks.getBrands.mockReset().mockResolvedValue({ data: { items: [{ id: 1, name: 'Truper' }] } })
    mocks.getMaterials.mockReset().mockResolvedValue({ data: { items: [{ id: 1, name: 'Acero' }] } })
    mocks.getMeasurementUnits.mockReset().mockResolvedValue({
      data: { items: [{ id: 1, name: 'Centímetros', abbreviation: 'cm' }] },
    })
  })

  it('creates a category with the entered code and name', async () => {
    mocks.createCategory.mockResolvedValue({ data: { id: 2, code: 'HER', name: 'Herramientas' } })
    renderProductForm()

    await screen.findByRole('heading', { name: 'Martillo' })
    fireEvent.click(screen.getByRole('button', { name: 'Editar' }))
    fireEvent.click(screen.getByTitle('Agregar nuevo categoría'))

    const dialog = screen.getByRole('dialog', { name: 'Agregar Categoría' })
    fireEvent.change(within(dialog).getByLabelText('Código'), { target: { value: 'HER' } })
    fireEvent.change(within(dialog).getByLabelText('Nombre'), { target: { value: 'Herramientas' } })
    fireEvent.click(within(dialog).getByRole('button', { name: 'Guardar' }))

    await waitFor(() =>
      expect(mocks.createCategory).toHaveBeenCalledWith({ code: 'HER', name: 'Herramientas' })
    )
  })

  it('sends exactly low_stock_threshold when creating a product', async () => {
    renderProductForm('/inventory/create')

    await screen.findByRole('heading', { name: 'Registro de producto' })
    fillCreateProduct()
    await selectDropdown('categoría', 'Herramientas')
    await selectDropdown('marca', 'Truper')
    await selectDropdown('material', 'Acero')
    await selectDropdown('unidad de medida', 'cm — Centímetros')

    fireEvent.click(screen.getByRole('button', { name: 'Guardar Producto' }))

    await waitFor(() => expect(mocks.createProduct).toHaveBeenCalledTimes(1))
    const payload = mocks.createProduct.mock.calls[0][0]
    expect(payload).toEqual({
      name: 'Martillo',
      sku: '',
      brand_id: 1,
      category_id: 1,
      material_id: 1,
      low_stock_threshold: 5,
      cost: 10,
      price: 19.9,
      stock_quantity: 4,
      size_value: '10',
      measurement_unit_id: 1,
    })
    expect(Object.keys(payload).sort()).toEqual([
      'brand_id', 'category_id', 'cost', 'low_stock_threshold', 'material_id', 'measurement_unit_id',
      'name', 'price', 'size_value', 'sku', 'stock_quantity',
    ])
    expect(payload).not.toHaveProperty('min_stock_quantity')
  })

  it('invalidates lists, stats and dashboard after updating with the exact payload', async () => {
    const { invalidateSpy } = renderProductForm()

    await screen.findByRole('heading', { name: 'Martillo' })
    fireEvent.click(screen.getByRole('button', { name: 'Editar' }))
    fireEvent.click(screen.getByRole('button', { name: 'Guardar' }))

    await waitFor(() => expect(mocks.updateProduct).toHaveBeenCalledTimes(1))
    expect(mocks.updateProduct).toHaveBeenCalledWith(1, {
      name: 'Martillo',
      sku: 'SKU-1',
      brand_id: 1,
      category_id: 1,
      material_id: 1,
      low_stock_threshold: 5,
      cost: 10,
      price: 19.9,
      stock_quantity: 4,
      size_value: '10',
      measurement_unit_id: 1,
    })
    const payload = mocks.updateProduct.mock.calls[0][1]
    expect(payload).not.toHaveProperty('min_stock_quantity')
    await waitFor(() => {
      expectedKeys.forEach(key => expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: [key] }))
    })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['product', 1] })
  })

  it('keeps relation selections when the update response has no relations', async () => {
    mocks.updateProduct.mockResolvedValue({ data: plainProduct })
    renderProductForm()

    await screen.findByRole('heading', { name: 'Martillo' })
    fireEvent.click(screen.getByRole('button', { name: 'Editar' }))
    fireEvent.click(screen.getByRole('button', { name: 'Guardar' }))

    await waitFor(() => expect(mocks.updateProduct).toHaveBeenCalledTimes(1))
    fireEvent.click(await screen.findByRole('button', { name: 'Editar' }))

    expect(dropdownTrigger('categoría').textContent).toContain('Herramientas')
    expect(dropdownTrigger('marca').textContent).toContain('Truper')
    expect(dropdownTrigger('material').textContent).toContain('Acero')
    expect(dropdownTrigger('unidad de medida').textContent).toContain('cm — Centímetros')
  })

  it('shows the saved values after a plain update response', async () => {
    mocks.updateProduct.mockResolvedValue({ data: { ...plainProduct, name: 'Martillo Nuevo' } })
    renderProductForm()

    await screen.findByRole('heading', { name: 'Martillo' })
    mocks.getProduct.mockResolvedValue({ data: { ...productData, name: 'Martillo Nuevo' } })
    fireEvent.click(screen.getByRole('button', { name: 'Editar' }))
    fireEvent.change(screen.getByPlaceholderText('Nombre del producto'), { target: { value: 'Martillo Nuevo' } })
    fireEvent.click(screen.getByRole('button', { name: 'Guardar' }))

    await waitFor(() => expect(mocks.updateProduct).toHaveBeenCalledTimes(1))
    expect(screen.getByRole('heading', { name: 'Martillo Nuevo' })).toBeTruthy()
  })

  it('keeps the new relation label after a plain update response', async () => {
    mocks.updateProduct.mockResolvedValue({ data: { ...plainProduct, category_id: 2 } })
    renderProductForm()

    await screen.findByRole('heading', { name: 'Martillo' })
    mocks.getProduct.mockResolvedValue({ data: { ...plainProduct, category_id: 2 } })
    fireEvent.click(screen.getByRole('button', { name: 'Editar' }))
    fireEvent.click(dropdownTrigger('categoría'))
    fireEvent.click(await screen.findByRole('button', { name: 'Fijaciones' }, { timeout: 2000 }))
    expect(dropdownTrigger('categoría').textContent).toContain('Fijaciones')

    fireEvent.click(screen.getByRole('button', { name: 'Guardar' }))
    await waitFor(() => expect(mocks.updateProduct).toHaveBeenCalledTimes(1))

    fireEvent.click(await screen.findByRole('button', { name: 'Editar' }))
    expect(dropdownTrigger('categoría').textContent).toContain('Fijaciones')
  })

  it('blocks oversized and non-positive product values', async () => {
    renderProductForm()

    await screen.findByRole('heading', { name: 'Martillo' })
    fireEvent.click(screen.getByRole('button', { name: 'Editar' }))

    fireEvent.change(screen.getByPlaceholderText('Nombre del producto'), { target: { value: 'a'.repeat(151) } })
    fireEvent.change(screen.getByDisplayValue('SKU-1'), { target: { value: 'b'.repeat(101) } })
    fireEvent.change(screen.getByPlaceholderText('ej: 10, 5.5, 12 x 5'), { target: { value: 'c'.repeat(21) } })
    const [price, cost, stock, minStock] = screen.getAllByRole('spinbutton')
    fireEvent.change(price, { target: { value: '0' } })
    fireEvent.change(cost, { target: { value: '-5' } })
    fireEvent.change(stock, { target: { value: '-1' } })
    fireEvent.change(minStock, { target: { value: '-2' } })

    fireEvent.click(screen.getByRole('button', { name: 'Guardar' }))

    expect(await screen.findByText('Nombre debe tener máximo 150 caracteres')).toBeTruthy()
    expect(screen.getByText('SKU debe tener máximo 100 caracteres')).toBeTruthy()
    expect(screen.getByText('Precio debe ser mayor a 0')).toBeTruthy()
    expect(screen.getByText('Costo debe ser mayor a 0')).toBeTruthy()
    expect(screen.getByText('Cantidad no puede ser negativa')).toBeTruthy()
    expect(screen.getByText('Cantidad mínima no puede ser negativa')).toBeTruthy()
    expect(mocks.updateProduct).not.toHaveBeenCalled()
  })

  it('shows an inline error and blocks an oversized size', async () => {
    renderProductForm()

    await screen.findByRole('heading', { name: 'Martillo' })
    fireEvent.click(screen.getByRole('button', { name: 'Editar' }))
    fireEvent.change(screen.getByPlaceholderText('ej: 10, 5.5, 12 x 5'), { target: { value: 'c'.repeat(21) } })
    fireEvent.click(screen.getByRole('button', { name: 'Guardar' }))

    expect(await screen.findByText('Tamaño debe tener máximo 20 caracteres')).toBeTruthy()
    expect(mocks.updateProduct).not.toHaveBeenCalled()
  })

  it('blocks create when brand, category or material is missing', async () => {
    renderProductForm('/inventory/create')

    await screen.findByRole('heading', { name: 'Registro de producto' })
    fireEvent.click(screen.getByRole('button', { name: 'Guardar Producto' }))

    expect(await screen.findByText('Marca es requerida')).toBeTruthy()
    expect(screen.getByText('Categoría es requerida')).toBeTruthy()
    expect(screen.getByText('Material es requerido')).toBeTruthy()
    expect(mocks.createProduct).not.toHaveBeenCalled()
  })

  it('sends only known fields when creating a brand from the modal', async () => {
    renderProductForm()

    await screen.findByRole('heading', { name: 'Martillo' })
    fireEvent.click(screen.getByRole('button', { name: 'Editar' }))
    fireEvent.click(screen.getByTitle('Agregar nuevo marca'))

    const dialog = screen.getByRole('dialog', { name: 'Agregar Marca' })
    fireEvent.change(within(dialog).getByLabelText('Nombre'), { target: { value: 'Truper' } })
    fireEvent.click(within(dialog).getByRole('button', { name: 'Guardar' }))

    await waitFor(() => expect(mocks.createBrand).toHaveBeenCalledWith({ name: 'Truper', logo_url: '' }))
    expect(Object.keys(mocks.createBrand.mock.calls[0][0]).sort()).toEqual(['logo_url', 'name'])
  })

  it('invalidates lists, stats and dashboard after deleting', async () => {
    const { invalidateSpy } = renderProductForm()

    await screen.findByRole('heading', { name: 'Martillo' })
    fireEvent.click(screen.getByRole('button', { name: 'Eliminar' }))
    fireEvent.click(screen.getAllByRole('button', { name: 'Eliminar' }).at(-1)!)

    await waitFor(() => expect(mocks.deleteProduct).toHaveBeenCalledWith(1))
    await waitFor(() => {
      expectedKeys.forEach(key => expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: [key] }))
    })
  })
})
