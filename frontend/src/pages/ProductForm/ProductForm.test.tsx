import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ProductForm from './ProductForm'

const mocks = vi.hoisted(() => ({
  getProduct: vi.fn(),
  updateProduct: vi.fn(),
  deleteProduct: vi.fn(),
  createCategory: vi.fn(),
}))

vi.mock('@/lib/api/inventory', () => ({
  inventoryApi: {
    getProduct: mocks.getProduct,
    updateProduct: mocks.updateProduct,
    deleteProduct: mocks.deleteProduct,
    createCategory: mocks.createCategory,
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

const expectedKeys = ['products', 'inventoryStats', 'dashboardSummary', 'dashboardTopProducts', 'dashboardLowStock']

function renderProductForm() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

  render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={['/inventory/product/1']}>
        <Routes>
          <Route path="/inventory/product/:id" element={<ProductForm />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )

  return { invalidateSpy }
}

describe('ProductForm', () => {
  beforeEach(() => {
    mocks.getProduct.mockReset().mockResolvedValue({ data: productData })
    mocks.updateProduct.mockReset().mockResolvedValue({ data: productData })
    mocks.deleteProduct.mockReset().mockResolvedValue(undefined)
    mocks.createCategory.mockReset()
  })

  it('creates a category with the entered code and name', async () => {
    mocks.createCategory.mockResolvedValue({ data: { id: 2, code: 'HER', name: 'Herramientas' } })
    renderProductForm()

    await screen.findByRole('heading', { name: 'Martillo' })
    fireEvent.click(screen.getByRole('button', { name: 'Editar' }))
    fireEvent.click(screen.getByTitle('Agregar nuevo categoría'))

    const dialog = screen.getByRole('dialog', { name: 'Agregar Categoría' })
    fireEvent.change(screen.getByLabelText('Código'), { target: { value: 'HER' } })
    fireEvent.change(screen.getByLabelText('Nombre'), { target: { value: 'Herramientas' } })
    fireEvent.click(within(dialog).getByRole('button', { name: 'Guardar' }))

    await waitFor(() =>
      expect(mocks.createCategory).toHaveBeenCalledWith({ code: 'HER', name: 'Herramientas' })
    )
  })

  it('invalidates lists, stats and dashboard after updating', async () => {
    const { invalidateSpy } = renderProductForm()

    await screen.findByRole('heading', { name: 'Martillo' })
    fireEvent.click(screen.getByRole('button', { name: 'Editar' }))
    fireEvent.click(screen.getByRole('button', { name: 'Guardar' }))

    await waitFor(() => expect(mocks.updateProduct).toHaveBeenCalledWith(1, expect.objectContaining({ name: 'Martillo' })))
    await waitFor(() => {
      expectedKeys.forEach(key => expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: [key] }))
    })
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
