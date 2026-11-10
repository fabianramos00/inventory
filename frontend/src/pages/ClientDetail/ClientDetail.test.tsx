import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ClientDetail from './ClientDetail'
import { formatCurrency, formatDate } from '@/lib/utils'

const mocks = vi.hoisted(() => ({
  useClient: vi.fn(),
  useSales: vi.fn(),
  updateClient: vi.fn(),
}))

vi.mock('@/hooks/useClient', () => ({ useClient: mocks.useClient }))
vi.mock('@/hooks/useSales', () => ({ useSales: mocks.useSales }))
vi.mock('@/lib/api/clients', () => ({
  clientsApi: { updateClient: mocks.updateClient },
}))

function client(debt = 150.5) {
  return {
    id: 12,
    name: 'Ana Pérez',
    identity_card: '1020304050',
    email: 'ana@example.com',
    phone: '3001234567',
    debt_amount: debt,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: null,
  }
}

function sale(overrides: Record<string, unknown> = {}) {
  return {
    id: 41,
    total_amount: 120,
    amount_paid: 20,
    debt_amount: 100,
    payment_status: 'partial',
    delivery_status: 'pending',
    client_id: 12,
    created_at: '2026-01-02T03:04:05Z',
    updated_at: null,
    created_by_id: 1,
    created_by: null,
    client: null,
    ...overrides,
  }
}

function defaultSales(overrides: Record<string, unknown> = {}) {
  return {
    sales: [],
    hasNext: false,
    isLoading: false,
    isFetching: false,
    isError: false,
    refetch: vi.fn(),
    ...overrides,
  }
}

function renderDetail() {
  return render(
    <MemoryRouter initialEntries={['/clients/12']}>
      <Routes>
        <Route path="/clients" element={<div>clients-list</div>} />
        <Route path="/clients/:id" element={<ClientDetail />} />
        <Route path="/sales/:id" element={<div>sale-detail</div>} />
      </Routes>
    </MemoryRouter>,
  )
}

function setup(clientValue = client(), salesValue = defaultSales()) {
  mocks.useClient.mockReturnValue({
    client: clientValue,
    isLoading: false,
    isFetching: false,
    error: null,
    refetch: vi.fn(),
    invalidate: vi.fn(),
    paymentMutation: { mutateAsync: vi.fn() },
  })
  mocks.useSales.mockReturnValue(salesValue)
  mocks.updateClient.mockReset()
}

function lastSalesParam() {
  const calls = mocks.useSales.mock.calls
  return calls[calls.length - 1][0] as {
    page: number
    filters: Record<string, unknown>
  }
}

describe('ClientDetail', () => {
  beforeEach(() => {
    mocks.useClient.mockReset()
    mocks.useSales.mockReset()
    setup()
  })

  it('shows the header, contact data and debt in red, or Sin deuda at zero', () => {
    const { unmount } = renderDetail()

    expect(screen.getByRole('heading', { name: 'Ana Pérez' })).toBeTruthy()
    expect(screen.getByText('CC 1020304050')).toBeTruthy()
    expect(screen.getByText('ana@example.com')).toBeTruthy()
    expect(screen.getByText('3001234567')).toBeTruthy()
    expect(screen.getByText('Saldo pendiente')).toBeTruthy()
    expect(screen.getByText(formatCurrency(150.5)).className).toContain('debtAmount')
    expect(screen.getByRole('button', { name: /Pagar deuda/ })).toBeTruthy()

    unmount()
    setup(client(0))
    renderDetail()

    expect(screen.getByText('Sin deuda')).toBeTruthy()
    expect(screen.queryByRole('button', { name: /Pagar deuda/ })).toBeNull()
  })

  it('renders the sales columns and forwards each filter with client_id', async () => {
    setup(client(), defaultSales({ sales: [sale()] }))
    renderDetail()

    for (const heading of ['Fecha', 'Total', 'Pagado', 'Saldo', 'Estado pago', 'Estado entrega', 'Ver']) {
      expect(screen.getByRole('columnheader', { name: heading })).toBeTruthy()
    }
    expect(screen.getByText(formatDate('2026-01-02T03:04:05Z'))).toBeTruthy()
    expect(screen.getByText(formatCurrency(120))).toBeTruthy()

    fireEvent.click(screen.getByRole('button', { name: /Estado de pago/ }))
    fireEvent.click(screen.getByRole('option', { name: 'Parcial' }))
    await waitFor(() => expect(lastSalesParam().filters.payment_status).toBe('partial'))

    fireEvent.click(screen.getByRole('button', { name: /Estado de entrega/ }))
    fireEvent.click(screen.getByRole('option', { name: 'Entregado' }))
    await waitFor(() => expect(lastSalesParam().filters.delivery_status).toBe('delivered'))

    fireEvent.change(screen.getByLabelText('Desde día'), { target: { value: '02' } })
    fireEvent.change(screen.getByLabelText('Desde mes'), { target: { value: '01' } })
    fireEvent.change(screen.getByLabelText('Desde año'), { target: { value: '2026' } })
    await waitFor(() => expect(lastSalesParam().filters.start_date).toBe('2026-01-02'))

    fireEvent.change(screen.getByLabelText('Hasta día'), { target: { value: '05' } })
    fireEvent.change(screen.getByLabelText('Hasta mes'), { target: { value: '01' } })
    fireEvent.change(screen.getByLabelText('Hasta año'), { target: { value: '2026' } })
    await waitFor(() => expect(lastSalesParam().filters.end_date).toBe('2026-01-05'))

    const filters = lastSalesParam().filters
    expect(filters.client_id).toBe(12)
    expect(filters.payment_status).toBe('partial')
    expect(filters.delivery_status).toBe('delivered')

    fireEvent.click(screen.getByRole('button', { name: 'Limpiar filtros' }))
    await waitFor(() => {
      const cleared = lastSalesParam()
      expect(cleared.page).toBe(1)
      expect(cleared.filters.payment_status).toBe('')
      expect(cleared.filters.delivery_status).toBe('')
      expect(cleared.filters.start_date).toBe('')
      expect(cleared.filters.end_date).toBe('')
    })
  })

  it('pages through client sales', async () => {
    setup(client(), defaultSales({ sales: [sale()], hasNext: true }))
    renderDetail()

    fireEvent.click(screen.getByRole('button', { name: 'Ir a página siguiente' }))

    await waitFor(() => expect(lastSalesParam().page).toBe(2))
  })

  it('opens sale detail from the row and from Ver', async () => {
    setup(client(), defaultSales({ sales: [sale()] }))
    const { unmount } = renderDetail()

    fireEvent.click(screen.getByRole('button', { name: 'Ver venta #41' }))
    expect(await screen.findByText('sale-detail')).toBeTruthy()

    unmount()
    setup(client(), defaultSales({ sales: [sale()] }))
    renderDetail()

    fireEvent.click(screen.getByText(formatDate('2026-01-02T03:04:05Z')))
    expect(await screen.findByText('sale-detail')).toBeTruthy()
  })

  it('submits a client payment and announces success', async () => {
    const mutateAsync = vi.fn().mockResolvedValue({
      data: { client_id: 12, amount: 150, remaining_debt: 0, allocations: [] },
    })
    mocks.useClient.mockReturnValue({
      client: client(),
      isLoading: false,
      isFetching: false,
      error: null,
      refetch: vi.fn(),
      invalidate: vi.fn(),
      paymentMutation: { mutateAsync },
    })
    renderDetail()

    fireEvent.click(screen.getByRole('button', { name: /Pagar deuda/ }))
    fireEvent.click(screen.getByRole('button', { name: 'Método de pago' }))
    fireEvent.click(screen.getByRole('option', { name: 'Efectivo' }))
    fireEvent.click(screen.getByRole('button', { name: 'Registrar pago' }))

    await waitFor(() => expect(mutateAsync).toHaveBeenCalledWith({ amount: 150.5, payment_method: 'cash' }))
    expect(await screen.findByText(/Pago registrado\./)).toBeTruthy()

    fireEvent.click(screen.getByRole('button', { name: 'Cerrar notificación' }))
    expect(screen.queryByText(/Pago registrado\./)).toBeNull()
  })

  it('auto-dismisses the payment confirmation after five seconds', async () => {
    vi.useFakeTimers()
    try {
      const mutateAsync = vi.fn().mockResolvedValue({
        data: { client_id: 12, amount: 150, remaining_debt: 0, allocations: [] },
      })
      mocks.useClient.mockReturnValue({
        client: client(),
        isLoading: false,
        isFetching: false,
        error: null,
        refetch: vi.fn(),
        invalidate: vi.fn(),
        paymentMutation: { mutateAsync },
      })
      renderDetail()

      fireEvent.click(screen.getByRole('button', { name: /Pagar deuda/ }))
      fireEvent.click(screen.getByRole('button', { name: 'Método de pago' }))
      fireEvent.click(screen.getByRole('option', { name: 'Efectivo' }))
      fireEvent.click(screen.getByRole('button', { name: 'Registrar pago' }))

      await act(async () => {
        await vi.advanceTimersByTimeAsync(0)
      })
      expect(screen.getByRole('status').textContent).toMatch(/Pago registrado\./)

      act(() => vi.advanceTimersByTime(4999))
      expect(screen.getByRole('status')).toBeTruthy()

      act(() => vi.advanceTimersByTime(1))
      expect(screen.queryByRole('status')).toBeNull()
    } finally {
      vi.useRealTimers()
    }
  })

  it('shows Cliente no encontrado for a missing client', () => {
    mocks.useClient.mockReturnValue({
      client: null,
      isLoading: false,
      isFetching: false,
      error: Object.assign(new Error('Request failed with status code 404'), {
        isAxiosError: true,
        response: { status: 404, data: { detail: 'Client not found' } },
      }),
      refetch: vi.fn(),
      invalidate: vi.fn(),
      paymentMutation: { mutateAsync: vi.fn() },
    })
    renderDetail()

    expect(screen.getByText('Cliente no encontrado')).toBeTruthy()
  })
})
