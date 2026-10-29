import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import SaleForm from './SaleForm'

const mocks = vi.hoisted(() => ({
  useSale: vi.fn(),
  createSale: vi.fn(),
  updateSale: vi.fn(),
  deleteSale: vi.fn(),
  addSaleItem: vi.fn(),
  updateSaleItem: vi.fn(),
  deleteSaleItem: vi.fn(),
  getSaleItems: vi.fn(),
  getClients: vi.fn(),
  createClient: vi.fn(),
  getProducts: vi.fn(),
}))

vi.mock('@/hooks/useSale', () => ({
  useSale: mocks.useSale,
}))

vi.mock('@/lib/api/sales', () => ({
  salesApi: {
    createSale: mocks.createSale,
    updateSale: mocks.updateSale,
    deleteSale: mocks.deleteSale,
    addSaleItem: mocks.addSaleItem,
    updateSaleItem: mocks.updateSaleItem,
    deleteSaleItem: mocks.deleteSaleItem,
    getSaleItems: mocks.getSaleItems,
  },
}))

vi.mock('@/lib/api/clients', () => ({
  clientsApi: {
    getClients: mocks.getClients,
    createClient: mocks.createClient,
  },
}))

vi.mock('@/lib/api/inventory', () => ({
  inventoryApi: {
    getProducts: mocks.getProducts,
  },
}))

function sale(overrides: Record<string, unknown> = {}) {
  return {
    id: 1,
    total_amount: 20,
    debt_amount: 10,
    amount_paid: 10,
    payment_status: 'partial',
    delivery_status: 'pending',
    payment_method: 'cash',
    client_id: null,
    created_at: '2026-01-02T03:04:05Z',
    updated_at: null,
    created_by_id: 2,
    created_by: { id: 2, full_name: 'Ana Pérez', email: 'ana@example.com', is_active: true, is_superuser: false },
    client: null,
    ...overrides,
  }
}

function saleItem(overrides: Record<string, unknown> = {}) {
  return {
    id: 11,
    sale_id: 1,
    product_id: 3,
    quantity: 2,
    delivered_quantity: 1,
    unit_price: 10,
    subtotal: 20,
    product_name: 'Martillo',
    product_sku: 'SKU-3',
    brand_name: 'Truper',
    material_name: 'Acero',
    category_name: 'Herramientas',
    product_category: 'Herramientas',
    product_brand: 'Truper',
    product_material: 'Acero',
    ...overrides,
  }
}

function product(overrides: Record<string, unknown> = {}) {
  return {
    id: 5,
    name: 'Taladro',
    sku: 'SKU-5',
    price: 50,
    sale_price: 50,
    stock_quantity: 3,
    category: { id: 1, code: 'HER', name: 'Herramientas' },
    brand: { id: 1, name: 'Truper' },
    material: { id: 1, name: 'Acero' },
    ...overrides,
  }
}

function renderSale(path = '/sales/1') {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/sales" element={<div>sales-list</div>} />
          <Route path="/sales/create" element={<SaleForm />} />
          <Route path="/sales/:id" element={<SaleForm />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

function editButtons() {
  return screen.getAllByRole('button', { name: 'Editar' })
}

describe('SaleForm payload wiring', () => {
  beforeEach(() => {
    mocks.useSale.mockReset().mockReturnValue({
      sale: sale(),
      saleItems: [saleItem()],
      isLoading: false,
      error: null,
    })
    mocks.updateSale.mockReset().mockResolvedValue({ data: sale() })
    mocks.deleteSale.mockReset().mockResolvedValue({ data: sale() })
    mocks.addSaleItem.mockReset().mockResolvedValue({ data: sale() })
    mocks.updateSaleItem.mockReset().mockResolvedValue({ data: sale() })
    mocks.deleteSaleItem.mockReset().mockResolvedValue({ data: sale() })
    mocks.getSaleItems.mockReset().mockResolvedValue({
      data: { items: [saleItem({ id: 12, product_id: 5 })], has_next: false, skip: 0, limit: 100 },
    })
    mocks.getProducts.mockReset().mockResolvedValue({ data: { items: [product()], has_next: false, skip: 0, limit: 8 } })
    mocks.getClients.mockReset().mockResolvedValue({ data: { items: [], has_next: false, skip: 0, limit: 10 } })
    mocks.createClient.mockReset()
  })

  it('sends the edit-details payload with explicit nulls for the nullable fields', async () => {
    renderSale()
    fireEvent.click(editButtons()[1])

    fireEvent.change(document.getElementById('edit-amount-paid')!, { target: { value: '12.50' } })
    fireEvent.click(screen.getByRole('button', { name: /Guardar/ }))

    await waitFor(() => expect(mocks.updateSale).toHaveBeenCalledWith(1, {
      payment_method: 'cash',
      amount_paid: 12.5,
      client_id: null,
    }))
  })

  it('clears the nullable edit-details fields to null', async () => {
    renderSale()
    fireEvent.click(editButtons()[1])

    fireEvent.click(screen.getByRole('button', { name: 'Efectivo' }))
    fireEvent.click(screen.getByRole('option', { name: 'Sin especificar' }))
    fireEvent.change(document.getElementById('edit-amount-paid')!, { target: { value: '0' } })
    fireEvent.click(screen.getByRole('button', { name: /Guardar/ }))

    await waitFor(() => expect(mocks.updateSale).toHaveBeenCalledWith(1, {
      payment_method: null,
      amount_paid: 0,
      client_id: null,
    }))
  })

  it('sends only the documented fields when updating an item', async () => {
    renderSale()
    fireEvent.click(editButtons()[0])

    fireEvent.change(document.getElementById('item-11-quantity')!, { target: { value: '4' } })
    fireEvent.change(document.getElementById('item-11-delivered')!, { target: { value: '2' } })
    fireEvent.change(document.getElementById('item-11-price')!, { target: { value: '12.50' } })
    fireEvent.click(screen.getByTitle('Guardar cambios'))

    await waitFor(() => expect(mocks.updateSaleItem).toHaveBeenCalledWith(1, 11, {
      quantity: 4,
      delivered_quantity: 2,
      unit_price: 12.5,
    }))
  })

  it('deletes an item through the sale item endpoint', async () => {
    renderSale()
    fireEvent.click(editButtons()[0])

    fireEvent.click(screen.getByTitle('Eliminar ítem'))

    await waitFor(() => expect(mocks.deleteSaleItem).toHaveBeenCalledWith(1, 11))
  })

  it('adds an item with product_id, quantity, delivered_quantity and unit_price', async () => {
    renderSale()
    fireEvent.click(editButtons()[0])

    fireEvent.change(
      screen.getByPlaceholderText('Agregar producto por nombre o SKU...'),
      { target: { value: 'tal' } },
    )

    await waitFor(
      () => expect(mocks.getProducts).toHaveBeenCalledWith({ search: 'tal', skip: 0, limit: 8 }),
      { timeout: 2000 },
    )
    fireEvent.click(await screen.findByText('Taladro'))

    const saveButtons = screen.getAllByTitle('Guardar cambios')
    fireEvent.click(saveButtons[saveButtons.length - 1])

    await waitFor(() => expect(mocks.addSaleItem).toHaveBeenCalledWith(1, {
      product_id: 5,
      quantity: 1,
      delivered_quantity: 1,
      unit_price: 50,
    }))
  })
})

describe('SaleForm create payload', () => {
  it('submits the create payload without payment_status or unknown fields', async () => {
    mocks.useSale.mockReturnValue({ sale: null, saleItems: [], isLoading: false, error: null })
    mocks.createSale.mockReset().mockResolvedValue({ data: sale() })
    renderSale('/sales/create')

    fireEvent.change(
      screen.getByPlaceholderText('Buscar producto por nombre o SKU...'),
      { target: { value: 'tal' } },
    )
    await waitFor(
      () => expect(mocks.getProducts).toHaveBeenCalledWith({ search: 'tal', skip: 0, limit: 8 }),
      { timeout: 2000 },
    )
    fireEvent.click(await screen.findByText('Taladro'))
    fireEvent.click(screen.getByRole('button', { name: /Guardar Venta/ }))

    await waitFor(() => expect(mocks.createSale).toHaveBeenCalledTimes(1))
    const payload = mocks.createSale.mock.calls[0][0] as Record<string, unknown>
    expect(payload).not.toHaveProperty('payment_status')
    expect(Object.keys(payload)).toEqual(['payment_method', 'amount_paid', 'client_id', 'items'])
    expect(payload).toEqual({
      payment_method: null,
      amount_paid: 0,
      client_id: null,
      items: [{ product_id: 5, quantity: 1, delivered_quantity: 1, unit_price: 50 }],
    })
  })


  it('shows the server-derived payment status with no manual dropdown', async () => {
    mocks.useSale.mockReturnValue({ sale: null, saleItems: [], isLoading: false, error: null })
    renderSale('/sales/create')

    expect(screen.getByText(/Calculado por el servidor/)).toBeTruthy()
    expect(screen.queryByRole('button', { name: 'Pendiente' })).toBeNull()

    fireEvent.change(
      screen.getByPlaceholderText('Buscar producto por nombre o SKU...'),
      { target: { value: 'tal' } },
    )
    await waitFor(
      () => expect(mocks.getProducts).toHaveBeenCalledWith({ search: 'tal', skip: 0, limit: 8 }),
      { timeout: 2000 },
    )
    fireEvent.click(await screen.findByText('Taladro'))
    fireEvent.change(document.getElementById('create-amount-paid')!, { target: { value: '50' } })

    await waitFor(() => expect(screen.getByText('Pagado')).toBeTruthy())
  })

  it('shows the delivered-over-quantity error and disables create submit', async () => {
    mocks.useSale.mockReturnValue({ sale: null, saleItems: [], isLoading: false, error: null })
    renderSale('/sales/create')

    fireEvent.change(
      screen.getByPlaceholderText('Buscar producto por nombre o SKU...'),
      { target: { value: 'tal' } },
    )
    await waitFor(
      () => expect(mocks.getProducts).toHaveBeenCalledWith({ search: 'tal', skip: 0, limit: 8 }),
      { timeout: 2000 },
    )
    fireEvent.click(await screen.findByText('Taladro'))

    fireEvent.change(document.getElementById('create-item-5-delivered')!, { target: { value: '2' } })

    expect(screen.getByText('máx. 1')).toBeTruthy()
    expect((screen.getByRole('button', { name: /Guardar Venta/ }) as HTMLButtonElement).disabled).toBe(true)
  })

  it('announces the empty-sale validation error with role="alert"', () => {
    mocks.useSale.mockReturnValue({ sale: null, saleItems: [], isLoading: false, error: null })
    renderSale('/sales/create')

    fireEvent.click(screen.getByRole('button', { name: /Guardar Venta/ }))

    expect(screen.getByText('Agrega al menos un producto a la venta.')).toBeTruthy()
    expect(screen.queryByRole('alert')).not.toBeNull()
  })
})
