import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import SaleForm from './SaleForm'
import saleStyles from './SaleForm.module.css'
import formStyles from '../formPage.module.css'
import { formatDateTime } from '@/lib/utils'

const saleFormCss = import.meta.glob('./SaleForm.module.css', {
  query: '?raw',
  import: 'default',
  eager: true,
})['./SaleForm.module.css']

const mocks = vi.hoisted(() => ({
  useSale: vi.fn(),
  createSale: vi.fn(),
  updateSale: vi.fn(),
  deleteSale: vi.fn(),
  addSaleItem: vi.fn(),
  updateSaleItem: vi.fn(),
  deleteSaleItem: vi.fn(),
  getSaleItems: vi.fn(),
  getSalePayments: vi.fn(),
  addSalePayment: vi.fn(),
  deleteSalePayment: vi.fn(),
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
    getSalePayments: mocks.getSalePayments,
    addSalePayment: mocks.addSalePayment,
    deleteSalePayment: mocks.deleteSalePayment,
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
    client_id: null,
    created_at: '2026-01-02T03:04:05Z',
    updated_at: null,
    created_by_id: 2,
    created_by: { id: 2, full_name: 'Ana Pérez', email: 'ana@example.com', is_active: true, is_superuser: false },
    client: null,
    ...overrides,
  }
}

function payment(overrides: Record<string, unknown> = {}) {
  return {
    id: 31,
    sale_id: 1,
    amount: 5.25,
    payment_method: 'cash',
    created_at: '2026-01-03T15:07:00Z',
    created_by_id: 2,
    created_by: { id: 2, full_name: 'Ana Pérez', email: 'ana@example.com', is_active: true, is_superuser: false },
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
  return render(
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

async function addProduct() {
  fireEvent.change(
    screen.getByPlaceholderText('Buscar producto por nombre o SKU...'),
    { target: { value: 'tal' } },
  )
  await waitFor(
    () => expect(mocks.getProducts).toHaveBeenCalledWith({ search: 'tal', skip: 0, limit: 8 }),
    { timeout: 2000 },
  )
  fireEvent.click(await screen.findByText('Taladro'))
}

function cssBlock(css: string, marker: string): string {
  const start = css.indexOf(marker)
  if (start < 0) return ''
  let depth = 0
  for (let i = start; i < css.length; i++) {
    if (css[i] === '{') depth++
    else if (css[i] === '}') {
      depth--
      if (depth === 0) return css.slice(start, i + 1)
    }
  }
  return ''
}

describe('SaleForm payload wiring', () => {
  beforeEach(() => {
    mocks.useSale.mockReset().mockReturnValue({
      sale: sale(),
      saleItems: [saleItem()],
      payments: [],
      paymentsHasNext: false,
      paymentsLoading: false,
      paymentsFetching: false,
      paymentsError: null,
      loadMorePayments: vi.fn(),
      retryPayments: vi.fn(),
      isLoading: false,
      error: null,
    })
    mocks.updateSale.mockReset().mockResolvedValue({ data: sale() })
    mocks.deleteSale.mockReset().mockResolvedValue({ data: sale() })
    mocks.addSaleItem.mockReset().mockResolvedValue({ data: sale() })
    mocks.updateSaleItem.mockReset().mockResolvedValue({ data: sale() })
    mocks.deleteSaleItem.mockReset().mockResolvedValue({ data: sale() })
    mocks.getSalePayments.mockReset()
    mocks.addSalePayment.mockReset()
    mocks.deleteSalePayment.mockReset()
    mocks.getSaleItems.mockReset().mockResolvedValue({
      data: { items: [saleItem({ id: 12, product_id: 5 })], has_next: false, skip: 0, limit: 100 },
    })
    mocks.getProducts.mockReset().mockResolvedValue({ data: { items: [product()], has_next: false, skip: 0, limit: 8 } })
    mocks.getClients.mockReset().mockResolvedValue({ data: { items: [], has_next: false, skip: 0, limit: 10 } })
    mocks.createClient.mockReset()
  })

  it('renders the detail view when the sale creator is null', () => {
    mocks.useSale.mockReturnValue({
      sale: sale({ created_by: null }),
      saleItems: [saleItem()],
      payments: [],
      paymentsHasNext: false,
      paymentsLoading: false,
      paymentsFetching: false,
      paymentsError: null,
      loadMorePayments: vi.fn(),
      retryPayments: vi.fn(),
      isLoading: false,
      error: null,
    })
    renderSale()

    expect(screen.getByText('Registrado por')).toBeTruthy()
    expect(screen.getAllByText('Sin usuario').length).toBeGreaterThan(0)
    expect(screen.getByText('Martillo')).toBeTruthy()
  })

  it('sends only client_id when saving sale details', async () => {
    renderSale()
    fireEvent.click(editButtons()[1])

    fireEvent.click(screen.getByRole('button', { name: /Guardar/ }))

    await waitFor(() => expect(mocks.updateSale).toHaveBeenCalledWith(1, {
      client_id: null,
    }))
  })

  it('does not expose amount paid or payment method in sale edit mode', () => {
    renderSale()
    fireEvent.click(editButtons()[1])

    expect(document.getElementById('edit-amount-paid')).toBeNull()
    expect(screen.queryByText('Método pago')).toBeNull()
    expect(screen.queryByRole('button', { name: 'Efectivo' })).toBeNull()
    expect(screen.getByText('Los pagos se registran con «Pagar».')).toBeTruthy()
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

describe('SaleForm payments', () => {
  function renderWithPayments(overrides: Record<string, unknown> = {}) {
    mocks.useSale.mockReturnValue({
      sale: sale(),
      saleItems: [saleItem()],
      payments: [payment()],
      paymentsHasNext: false,
      paymentsLoading: false,
      paymentsFetching: false,
      paymentsError: null,
      loadMorePayments: vi.fn(),
      retryPayments: vi.fn(),
      isLoading: false,
      error: null,
      ...overrides,
    })
    renderSale()
  }

  it('renders the payment history newest first with method, user and amount', () => {
    renderWithPayments()

    expect(screen.getByRole('button', { name: /^Pagar$/ })).toBeTruthy()
    expect(screen.getByText('Historial de pagos')).toBeTruthy()
    expect(screen.getByText('Fecha y hora')).toBeTruthy()
    expect(screen.getByText('Método')).toBeTruthy()
    expect(screen.getAllByText('Registrado por').length).toBeGreaterThan(0)
    expect(screen.getByText('Monto')).toBeTruthy()
    expect(screen.getByText('Efectivo')).toBeTruthy()
    expect(screen.getAllByText('Ana Pérez').length).toBeGreaterThan(0)
    expect(screen.getByText('$ 5,25')).toBeTruthy()
    expect(screen.getByText(formatDateTime(payment().created_at))).toBeTruthy()
  })

  it('falls back for legacy payments without method or user', () => {
    renderWithPayments({
      payments: [payment({ payment_method: null, created_by_id: null, created_by: null })],
    })

    expect(screen.getByText('Sin especificar')).toBeTruthy()
    expect(screen.getByText('Sin usuario')).toBeTruthy()
  })

  it('shows the empty history with the Pagar hint', () => {
    renderWithPayments({ payments: [] })

    expect(screen.getByText('Sin pagos registrados')).toBeTruthy()
    expect(screen.getByText('Registra el primer pago con «Pagar».')).toBeTruthy()
  })

  it('registers a payment from the modal with the required method', async () => {
    renderWithPayments()

    fireEvent.click(screen.getByRole('button', { name: /^Pagar$/ }))
    expect(screen.getByRole('dialog')).toBeTruthy()
    const amount = screen.getByLabelText('Monto del pago') as HTMLInputElement
    expect(amount.value).toBe('10.00')

    fireEvent.click(screen.getByRole('button', { name: 'Registrar pago' }))
    await waitFor(() => expect(screen.getByText('Selecciona un método de pago.')).toBeTruthy())
    expect(mocks.addSalePayment).not.toHaveBeenCalled()

    fireEvent.click(screen.getByRole('button', { name: 'Método de pago' }))
    fireEvent.click(screen.getByRole('option', { name: 'Efectivo' }))
    mocks.addSalePayment.mockResolvedValueOnce({
      data: sale({ debt_amount: 0, amount_paid: 20, payment_status: 'paid' }),
    })
    fireEvent.click(screen.getByRole('button', { name: 'Registrar pago' }))

    await waitFor(() => expect(mocks.addSalePayment).toHaveBeenCalledWith(1, {
      amount: 10,
      payment_method: 'cash',
    }))
    await waitFor(() => expect(screen.getByRole('status').textContent).toContain('Pago de $ 10,00 registrado. Saldo pendiente: $ 0,00.'))
  })

  it('rejects zero and above-debt amounts before calling the API', async () => {
    renderWithPayments()
    fireEvent.click(screen.getByRole('button', { name: /^Pagar$/ }))

    const amount = screen.getByLabelText('Monto del pago') as HTMLInputElement
    fireEvent.change(amount, { target: { value: '0' } })
    fireEvent.click(screen.getByRole('button', { name: 'Registrar pago' }))
    await waitFor(() => expect(screen.getByText('Ingresa un monto mayor a 0.')).toBeTruthy())

    fireEvent.change(amount, { target: { value: '10.01' } })
    fireEvent.click(screen.getByRole('button', { name: 'Registrar pago' }))
    await waitFor(() => expect(screen.getByText(/no puede superar el saldo pendiente/)).toBeTruthy())

    expect(mocks.addSalePayment).not.toHaveBeenCalled()
  })

  it('loads older payments with Ver más', () => {
    const loadMorePayments = vi.fn()
    renderWithPayments({ paymentsHasNext: true, loadMorePayments })

    fireEvent.click(screen.getByRole('button', { name: 'Ver más' }))

    expect(loadMorePayments).toHaveBeenCalled()
  })

  it('keeps the history frame and retries after a load error', () => {
    const retryPayments = vi.fn()
    renderWithPayments({ payments: [], paymentsError: new Error('failed'), retryPayments })

    expect(screen.getByText('No se pudieron cargar los pagos.')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Reintentar' }))
    expect(retryPayments).toHaveBeenCalled()
  })

  it('deletes a payment after a confirmation naming the resulting balance', async () => {
    renderWithPayments()
    mocks.deleteSalePayment.mockResolvedValueOnce({ data: sale({ debt_amount: 15.25, amount_paid: 4.75 }) })

    fireEvent.click(screen.getByRole('button', { name: 'Eliminar pago de $ 5,25' }))

    const dialog = screen.getByRole('dialog')
    expect(within(dialog).getByText(/Saldo resultante: \$ 15,25/)).toBeTruthy()
    fireEvent.click(within(dialog).getByRole('button', { name: 'Eliminar' }))

    await waitFor(() => expect(mocks.deleteSalePayment).toHaveBeenCalledWith(1, 31))
    await waitFor(() => expect(screen.getByRole('status').textContent).toContain('Pago eliminado. Saldo pendiente: $ 15,25.'))
  })

  it('names an unspecified method in the delete confirmation', () => {
    renderWithPayments({ payments: [payment({ payment_method: null })] })

    fireEvent.click(screen.getByRole('button', { name: 'Eliminar pago de $ 5,25' }))

    const dialog = screen.getByRole('dialog')
    expect(within(dialog).getByText(/Sin especificar/)).toBeTruthy()
  })

  it('keeps the confirmation open and shows an error when deleting fails', async () => {
    renderWithPayments()
    mocks.deleteSalePayment.mockRejectedValueOnce(new Error('No se pudo eliminar el pago.'))

    fireEvent.click(screen.getByRole('button', { name: 'Eliminar pago de $ 5,25' }))
    const dialog = screen.getByRole('dialog')
    fireEvent.click(within(dialog).getByRole('button', { name: 'Eliminar' }))

    await waitFor(() => expect(mocks.deleteSalePayment).toHaveBeenCalledWith(1, 31))
    await waitFor(() => expect(within(dialog).getByRole('alert').textContent).toContain('No se pudo eliminar el pago.'))
    expect(within(dialog).getByRole('button', { name: 'Eliminar' })).toBeTruthy()
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

  it('always shows the payment method and keeps the payload null at amount zero', async () => {
    mocks.useSale.mockReturnValue({ sale: null, saleItems: [], isLoading: false, error: null })
    mocks.createSale.mockReset().mockResolvedValue({ data: sale() })
    renderSale('/sales/create')

    expect(screen.getByText('Método de pago')).toBeTruthy()
    expect((screen.getByRole('button', { name: 'Método de pago' }) as HTMLButtonElement).disabled).toBe(false)
    expect((screen.getByRole('button', { name: /Guardar Venta/ }) as HTMLButtonElement).disabled).toBe(false)

    await addProduct()

    fireEvent.click(screen.getByRole('button', { name: /Guardar Venta/ }))

    await waitFor(() => expect(mocks.createSale).toHaveBeenCalledTimes(1))
    const payload = mocks.createSale.mock.calls[0][0] as Record<string, unknown>
    expect(payload.payment_method).toBeNull()
    expect(payload.amount_paid).toBe(0)
  })

  it('shows the amount paid before the payment method', () => {
    mocks.useSale.mockReturnValue({ sale: null, saleItems: [], isLoading: false, error: null })
    renderSale('/sales/create')

    const amount = screen.getByText('Monto pagado')
    const method = screen.getByText('Método de pago')
    expect(amount.compareDocumentPosition(method) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })

  it('shows the local payment status with no manual dropdown', async () => {
    mocks.useSale.mockReturnValue({ sale: null, saleItems: [], isLoading: false, error: null })
    renderSale('/sales/create')

    expect(screen.getByText(/Se calcula al guardar\./)).toBeTruthy()
    expect(screen.queryByRole('button', { name: 'Pendiente' })).toBeNull()

    await addProduct()
    fireEvent.change(document.getElementById('create-amount-paid')!, { target: { value: '50' } })

    await waitFor(() => expect(screen.getByText('Pagado')).toBeTruthy())
  })

  it('shows the delivered-over-quantity error next to the field', async () => {
    mocks.useSale.mockReturnValue({ sale: null, saleItems: [], isLoading: false, error: null })
    renderSale('/sales/create')

    await addProduct()

    fireEvent.change(document.getElementById('create-item-5-delivered')!, { target: { value: '2' } })

    expect(screen.getByText('máx. 1')).toBeTruthy()
    expect((screen.getByRole('button', { name: /Guardar Venta/ }) as HTMLButtonElement).disabled).toBe(false)
  })

  it('shows the empty-sale error inline and moves focus to the product search', () => {
    mocks.useSale.mockReturnValue({ sale: null, saleItems: [], isLoading: false, error: null })
    renderSale('/sales/create')

    fireEvent.click(screen.getByRole('button', { name: /Guardar Venta/ }))

    const error = screen.getByText('Agrega al menos un producto a la venta.')
    expect(error).toBeTruthy()
    expect(document.getElementById('sale-product-search')).toBe(document.activeElement)
    expect(mocks.createSale).not.toHaveBeenCalled()
  })
})

describe('SaleForm create validation focus', () => {
  beforeEach(() => {
    mocks.useSale.mockReturnValue({ sale: null, saleItems: [], isLoading: false, error: null })
    mocks.createSale.mockReset().mockResolvedValue({ data: sale() })
  })

  it('focuses the delivered quantity of the offending item', async () => {
    renderSale('/sales/create')
    await addProduct()

    fireEvent.change(document.getElementById('create-item-5-delivered')!, { target: { value: '2' } })
    fireEvent.click(screen.getByRole('button', { name: /Guardar Venta/ }))

    expect(document.getElementById('create-item-5-delivered')).toBe(document.activeElement)
    expect(screen.getByText('máx. 1')).toBeTruthy()
    expect(mocks.createSale).not.toHaveBeenCalled()
  })

  it('focuses the amount paid when it exceeds the total', async () => {
    renderSale('/sales/create')
    await addProduct()

    fireEvent.change(document.getElementById('create-amount-paid')!, { target: { value: '100' } })
    fireEvent.click(screen.getByRole('button', { name: /Guardar Venta/ }))

    expect(document.getElementById('create-amount-paid')).toBe(document.activeElement)
    expect(screen.getByText('máx. $ 50.00')).toBeTruthy()
    expect(mocks.createSale).not.toHaveBeenCalled()
  })

  it('focuses the payment method when the amount is positive and no method is chosen', async () => {
    renderSale('/sales/create')
    await addProduct()

    fireEvent.change(document.getElementById('create-amount-paid')!, { target: { value: '10' } })
    fireEvent.click(screen.getByRole('button', { name: /Guardar Venta/ }))

    expect(screen.getByRole('button', { name: 'Método de pago' })).toBe(document.activeElement)
    expect(screen.getByText('Selecciona un método de pago.')).toBeTruthy()
    expect(mocks.createSale).not.toHaveBeenCalled()
  })
})

describe('SaleForm create layout', () => {
  beforeEach(() => {
    mocks.useSale.mockReturnValue({ sale: null, saleItems: [], isLoading: false, error: null })
  })

  it('renders a single save button in the rail', () => {
    const { container } = renderSale('/sales/create')

    expect(screen.getAllByRole('button', { name: /Guardar Venta/ })).toHaveLength(1)
    const save = screen.getByRole('button', { name: /Guardar Venta/ })
    expect(container.querySelector(`.${saleStyles.saleGrid}`)).toBeTruthy()
    expect(save.closest(`.${formStyles.rightColumn}`)).toBeTruthy()
  })

  it('announces the receipt total through a live region', () => {
    const { container } = renderSale('/sales/create')

    const total = container.querySelector('[aria-live="polite"]')
    expect(total).toBeTruthy()
    expect(total!.textContent).toBe('$ 0,00')
    expect(total!.parentElement!.className).toContain(saleStyles.receiptTotal)
  })

  it('keeps the rail at least 320px wide and stacks the receipt total', () => {
    expect(cssBlock(saleFormCss, '@media (min-width: 1024px)')).toContain(
      'grid-template-columns: minmax(0, 1fr) 320px;',
    )
    expect(cssBlock(saleFormCss, '@media (min-width: 1280px)')).toContain(
      'grid-template-columns: minmax(0, 1fr) 360px;',
    )

    const total = cssBlock(saleFormCss, '.receiptTotal {')
    expect(total).toContain('flex-direction: column;')
    expect(total).toContain('align-items: flex-end;')
  })
})

describe('SaleForm edit layout', () => {
  beforeEach(() => {
    mocks.useSale.mockReturnValue({
      sale: sale(),
      saleItems: [saleItem()],
      payments: [],
      paymentsHasNext: false,
      paymentsLoading: false,
      paymentsFetching: false,
      paymentsError: null,
      loadMorePayments: vi.fn(),
      retryPayments: vi.fn(),
      isLoading: false,
      error: null,
    })
  })

  it('drops the duplicated payment status and amount rows from the details card', () => {
    renderSale()

    expect(screen.queryByText('Estado pago')).toBeNull()
    expect(screen.queryByText('Monto pagado')).toBeNull()
  })

  it('shows the paid amount and the balance in the receipt', () => {
    renderSale()

    expect(screen.getByText('Pagado')).toBeTruthy()
    expect(screen.getByText('Saldo')).toBeTruthy()
    expect(screen.queryByText('Deuda')).toBeNull()
  })
})
