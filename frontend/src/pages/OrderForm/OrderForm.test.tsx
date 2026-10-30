import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import OrderForm from './OrderForm'

vi.setConfig({ testTimeout: 15000 })

const mocks = vi.hoisted(() => ({
  useOrder: vi.fn(),
  createOrder: vi.fn(),
  updateOrder: vi.fn(),
  deleteOrder: vi.fn(),
  addOrderItem: vi.fn(),
  updateOrderItem: vi.fn(),
  deleteOrderItem: vi.fn(),
  getOrderItems: vi.fn(),
  getProviders: vi.fn(),
  createProvider: vi.fn(),
  getProducts: vi.fn(),
}))

vi.mock('@/hooks/useOrder', () => ({
  useOrder: mocks.useOrder,
}))

vi.mock('@/lib/api/orders', () => ({
  ordersApi: {
    createOrder: mocks.createOrder,
    updateOrder: mocks.updateOrder,
    deleteOrder: mocks.deleteOrder,
    addOrderItem: mocks.addOrderItem,
    updateOrderItem: mocks.updateOrderItem,
    deleteOrderItem: mocks.deleteOrderItem,
    getOrderItems: mocks.getOrderItems,
  },
}))

vi.mock('@/lib/api/providers', () => ({
  providersApi: {
    getProviders: mocks.getProviders,
    createProvider: mocks.createProvider,
  },
}))

vi.mock('@/lib/api/inventory', () => ({
  inventoryApi: {
    getProducts: mocks.getProducts,
  },
}))

function order(overrides: Record<string, unknown> = {}) {
  return {
    id: 1,
    provider_id: 2,
    status: 'pending',
    payment_status: 'pending',
    order_date: '2026-01-02T03:04:05Z',
    total_amount: 20,
    provider: { id: 2, name: 'Distribuidora Metálica', contact_info: null, email: null, phone: null },
    ...overrides,
  }
}

function orderItem(overrides: Record<string, unknown> = {}) {
  return {
    id: 11,
    order_id: 1,
    product_id: 5,
    quantity: 2,
    unit_cost: 10,
    subtotal: 20,
    supplier_sku: 'PROV-1',
    product_name: 'Martillo',
    product_sku: 'SKU-5',
    ...overrides,
  }
}

function product() {
  return {
    id: 5,
    name: 'Martillo',
    sku: 'SKU-5',
    category: { id: 1, code: 'HER', name: 'Herramientas' },
    price: 15,
    cost: 10,
    brand: { id: 1, name: 'Truper' },
    material: { id: 1, name: 'Acero' },
  }
}

function renderOrderForm(path = '/orders/1') {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/orders" element={<div>orders-list</div>} />
          <Route path="/orders/create" element={<OrderForm />} />
          <Route path="/orders/:id" element={<OrderForm />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
  return queryClient
}

async function chooseProvider() {
  fireEvent.click(screen.getByRole('button', { name: /Proveedor:/ }))
  fireEvent.click(await screen.findByRole('option', { name: 'Distribuidora Metálica' }))
}

async function addProduct() {
  fireEvent.change(
    screen.getByPlaceholderText('Buscar producto por nombre o SKU...'),
    { target: { value: 'mar' } },
  )
  await waitFor(
    () => expect(mocks.getProducts).toHaveBeenCalledWith({ search: 'mar', skip: 0, limit: 8 }),
    { timeout: 2000 },
  )
  fireEvent.click(await screen.findByText('Martillo'))
}

describe('OrderForm wiring', () => {
  beforeEach(() => {
    mocks.useOrder.mockReset().mockReturnValue({
      order: order(),
      orderItems: [orderItem()],
      isLoading: false,
      error: null,
    })
    mocks.createOrder.mockReset().mockResolvedValue({ data: order() })
    mocks.updateOrder.mockReset().mockResolvedValue({ data: order() })
    mocks.deleteOrder.mockReset().mockResolvedValue({ data: order() })
    mocks.addOrderItem.mockReset().mockResolvedValue({ data: order() })
    mocks.updateOrderItem.mockReset().mockResolvedValue({ data: order() })
    mocks.deleteOrderItem.mockReset().mockResolvedValue({ data: order() })
    mocks.getOrderItems.mockReset().mockResolvedValue({
      data: { items: [orderItem()], has_next: false, skip: 0, limit: 100 },
    })
    mocks.getProviders.mockReset().mockResolvedValue({
      data: {
        items: [{ id: 2, name: 'Distribuidora Metálica', contact_info: null, email: null, phone: null }],
        has_next: false,
        skip: 0,
        limit: 10,
      },
    })
    mocks.getProducts.mockReset().mockResolvedValue({
      data: { items: [product()], has_next: false, skip: 0, limit: 8 },
    })
  })

  it('submits the create payload with only the contract fields', async () => {
    mocks.useOrder.mockReturnValue({ order: null, orderItems: [], isLoading: false, error: null })
    const queryClient = renderOrderForm('/orders/create')
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    await chooseProvider()
    await addProduct()
    fireEvent.click(screen.getByRole('button', { name: /Guardar Pedido/ }))

    await waitFor(() => expect(mocks.createOrder).toHaveBeenCalledTimes(1))
    expect(mocks.createOrder).toHaveBeenCalledWith({
      provider_id: 2,
      status: 'pending',
      payment_status: 'pending',
      items: [{ product_id: 5, quantity: 1, unit_cost: 10 }],
    })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['orders'] })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['dashboardSummary'] })
  })

  it('sends the details update and refreshes stock when the order becomes received', async () => {
    mocks.updateOrder.mockResolvedValue({ data: order({ status: 'received' }) })
    const queryClient = renderOrderForm()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    fireEvent.click(screen.getAllByRole('button', { name: 'Editar' })[1])
    fireEvent.click(screen.getByRole('button', { name: 'Estado: Pendiente' }))
    fireEvent.click(screen.getByRole('option', { name: 'Recibido' }))
    fireEvent.click(screen.getByRole('button', { name: /Guardar/ }))

    await waitFor(() => expect(mocks.updateOrder).toHaveBeenCalledWith(1, {
      provider_id: 2,
      status: 'received',
      payment_status: 'pending',
    }))
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['order', 1] })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['orders'] })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['products'] })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['inventoryStats'] })
  })

  it('clears supplier_sku on item updates and invalidates only order caches', async () => {
    const queryClient = renderOrderForm()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    fireEvent.click(screen.getAllByRole('button', { name: 'Editar' })[0])
    fireEvent.change(screen.getByPlaceholderText('Opcional'), { target: { value: '' } })
    fireEvent.click(screen.getByTitle('Guardar cambios'))

    await waitFor(() => expect(mocks.updateOrderItem).toHaveBeenCalledWith(1, 11, {
      quantity: 2,
      unit_cost: 10,
      supplier_sku: null,
    }))
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['order', 1] })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['orderItems', 1] })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['orders'] })
    expect(invalidateSpy).not.toHaveBeenCalledWith({ queryKey: ['products'] })
  })

  it('surfaces the backend detail in a role="alert" box', async () => {
    mocks.useOrder.mockReturnValue({ order: null, orderItems: [], isLoading: false, error: null })
    mocks.createOrder.mockRejectedValue(Object.assign(new Error('Request failed'), {
      isAxiosError: true,
      response: { data: { detail: 'Provider with id 2 not found' } },
    }))
    renderOrderForm('/orders/create')

    await chooseProvider()
    await addProduct()
    fireEvent.click(screen.getByRole('button', { name: /Guardar Pedido/ }))

    const alert = await screen.findByRole('alert')
    expect(alert.textContent).toBe('Provider with id 2 not found')
  })

  it('deletes an item without invalidating stock caches', async () => {
    const queryClient = renderOrderForm()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    fireEvent.click(screen.getAllByRole('button', { name: 'Editar' })[0])
    fireEvent.click(screen.getByTitle('Eliminar ítem'))

    await waitFor(() => expect(mocks.deleteOrderItem).toHaveBeenCalledWith(1, 11))
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['order', 1] })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['orderItems', 1] })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['orders'] })
    expect(invalidateSpy).not.toHaveBeenCalledWith({ queryKey: ['products'] })
    expect(invalidateSpy).not.toHaveBeenCalledWith({ queryKey: ['inventoryStats'] })
  })

  it('announces validation errors with role="alert"', () => {
    mocks.useOrder.mockReturnValue({ order: null, orderItems: [], isLoading: false, error: null })
    renderOrderForm('/orders/create')

    fireEvent.click(screen.getByRole('button', { name: /Guardar Pedido/ }))

    expect(screen.getByRole('alert').textContent).toBe('Selecciona un proveedor.')
  })
})
