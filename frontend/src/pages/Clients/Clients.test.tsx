import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import Clients from './Clients'
import { formatCurrency } from '@/lib/utils'

const mocks = vi.hoisted(() => ({
  getClients: vi.fn(),
  createClient: vi.fn(),
  updateClient: vi.fn(),
  deleteClient: vi.fn(),
  addClientPayment: vi.fn(),
}))

vi.mock('@/lib/api/clients', () => ({
  clientsApi: {
    getClients: mocks.getClients,
    createClient: mocks.createClient,
    updateClient: mocks.updateClient,
    deleteClient: mocks.deleteClient,
    addClientPayment: mocks.addClientPayment,
  },
}))

function clientPage(skip: number) {
  return {
    data: {
      items: [{
        id: skip + 1,
        name: `Cliente ${skip + 1}`,
        identity_card: null,
        email: null,
        phone: null,
        debt_amount: 0,
        created_at: '2026-01-01T00:00:00',
        updated_at: null,
      }],
      has_next: true,
      skip,
      limit: 10,
    },
  }
}

async function settle(ms = 0) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms)
  })
}

function renderClients() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <Clients />
    </QueryClientProvider>,
  )
  return queryClient
}

function client(overrides: Record<string, unknown> = {}) {
  return {
    id: 1,
    name: 'Cliente 1',
    identity_card: '1020304050',
    email: 'ana@example.com',
    phone: '3001234567',
    debt_amount: 0,
    created_at: '2026-01-01T00:00:00',
    updated_at: null,
    ...overrides,
  }
}

function paymentResult(overrides: Record<string, unknown> = {}) {
  return {
    client_id: 1,
    amount: 150,
    remaining_debt: 0,
    allocations: [],
    ...overrides,
  }
}

function pageOf(items: ReturnType<typeof client>[]) {
  return { data: { items, has_next: false, skip: 0, limit: 10 } }
}

function payWithCash(amount?: string) {
  const dialog = screen.getByRole('dialog', { name: 'Registrar pago' })
  if (amount !== undefined) {
    fireEvent.click(within(dialog).getByRole('radio', { name: 'Pago parcial' }))
    fireEvent.change(within(dialog).getByLabelText('Monto'), { target: { value: amount } })
  }
  fireEvent.click(within(dialog).getByRole('button', { name: 'Método de pago' }))
  fireEvent.click(screen.getByRole('option', { name: 'Efectivo' }))
  fireEvent.click(within(dialog).getByRole('button', { name: 'Registrar pago' }))
}

describe('Clients', () => {
  beforeEach(() => {
    mocks.getClients.mockReset()
    mocks.getClients.mockImplementation((params: { skip: number }) => Promise.resolve(clientPage(params.skip)))
    mocks.createClient.mockReset()
    mocks.updateClient.mockReset()
    mocks.deleteClient.mockReset()
    mocks.deleteClient.mockResolvedValue({ data: client() })
    mocks.addClientPayment.mockReset()
    mocks.addClientPayment.mockResolvedValue({ data: paymentResult() })
  })

  it('resets to page 1 and only requests the debounced search', async () => {
    vi.useFakeTimers()
    try {
      renderClients()
      await settle()

      expect(mocks.getClients).toHaveBeenCalledWith({ skip: 0, limit: 10 })
      expect(screen.getByText('Cliente 1')).toBeTruthy()

      fireEvent.click(screen.getByRole('button', { name: 'Ir a página siguiente' }))
      await settle()

      expect(mocks.getClients).toHaveBeenCalledWith({ skip: 10, limit: 10 })
      expect(screen.getByText('Página 2')).toBeTruthy()

      mocks.getClients.mockClear()
      fireEvent.change(
        screen.getByPlaceholderText('Buscar cliente por nombre, cédula o contacto...'),
        { target: { value: 'ana' } },
      )

      await settle(50)
      expect(mocks.getClients).not.toHaveBeenCalled()

      await settle(250)
      expect(mocks.getClients).toHaveBeenCalledWith({ skip: 0, limit: 10, search: 'ana' })
      expect(mocks.getClients).toHaveBeenCalledTimes(1)
    } finally {
      vi.useRealTimers()
    }
  })

  it('shows debt in red and zero debt as Sin deuda', async () => {
    mocks.getClients.mockResolvedValue({
      data: {
        items: [
          client({ id: 1, name: 'Cliente con deuda', debt_amount: 150.5 }),
          client({ id: 2, name: 'Cliente al día', debt_amount: 0 }),
        ],
        has_next: false,
        skip: 0,
        limit: 10,
      },
    })
    renderClients()

    await screen.findByText('Cliente con deuda')
    expect(screen.getByText(formatCurrency(150.5)).className).toContain('debtAmount')
    expect(screen.getByText('Sin deuda').className).toContain('debtSettled')
  })

  it('shows a retryable error instead of the empty state when the query fails', async () => {
    mocks.getClients.mockRejectedValueOnce(new Error('API down'))
    renderClients()

    await screen.findByText('No se pudieron cargar los clientes.')
    expect(screen.queryByText('No se encontraron clientes.')).toBeNull()

    mocks.getClients.mockResolvedValue(clientPage(0))
    fireEvent.click(screen.getByRole('button', { name: 'Reintentar' }))

    await screen.findByText('Cliente 1')
    expect(screen.queryByText('No se pudieron cargar los clientes.')).toBeNull()
  })

  it('creates a client with the name only and omits empty optional fields', async () => {
    mocks.createClient.mockResolvedValue({ data: client({ id: 9, name: 'Ana' }) })
    renderClients()
    await screen.findByText('Cliente 1')

    fireEvent.click(screen.getByRole('button', { name: /Nuevo Cliente/ }))
    fireEvent.change(screen.getByLabelText('Nombre del Cliente'), { target: { value: 'Ana' } })
    fireEvent.click(screen.getByRole('button', { name: 'Guardar' }))

    await waitFor(() => expect(mocks.createClient).toHaveBeenCalledWith({ name: 'Ana' }))
  })

  it('invalidates the client list after creating a client', async () => {
    mocks.createClient.mockResolvedValue({ data: client({ id: 9, name: 'Ana' }) })
    const queryClient = renderClients()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')
    await screen.findByText('Cliente 1')

    fireEvent.click(screen.getByRole('button', { name: /Nuevo Cliente/ }))
    fireEvent.change(screen.getByLabelText('Nombre del Cliente'), { target: { value: 'Ana' } })
    fireEvent.click(screen.getByRole('button', { name: 'Guardar' }))

    await waitFor(() => expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['clients'] }))
  })

  it('deletes a client after confirming and invalidates the client list', async () => {
    const queryClient = renderClients()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')
    await screen.findByText('Cliente 1')

    fireEvent.click(screen.getByTitle('Eliminar'))
    fireEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Eliminar' }))

    await waitFor(() => expect(mocks.deleteClient).toHaveBeenCalledWith(1))
    await waitFor(() => expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['clients'] }))
  })

  it('clears an optional field when the edit form submits it empty', async () => {
    mocks.getClients.mockResolvedValue({
      data: { items: [client()], has_next: false, skip: 0, limit: 10 },
    })
    mocks.updateClient.mockResolvedValue({ data: client({ email: null }) })
    renderClients()
    await screen.findByText('Cliente 1')

    fireEvent.click(screen.getByTitle('Editar'))
    fireEvent.change(screen.getByLabelText('Correo Electrónico'), { target: { value: '' } })
    fireEvent.click(screen.getByRole('button', { name: 'Guardar' }))

    await waitFor(() => expect(mocks.updateClient).toHaveBeenCalledWith(1, {
      name: 'Cliente 1',
      identity_card: '1020304050',
      email: null,
      phone: '3001234567',
    }))
  })

  it('shows the readable duplicate identity card error', async () => {
    mocks.createClient.mockRejectedValue(Object.assign(new Error('Request failed with status code 400'), {
      isAxiosError: true,
      response: {
        status: 400,
        data: {
          detail: 'A client with this identity card already exists.',
          code: 'UNIQUE_VIOLATION',
        },
      },
    }))
    renderClients()
    await screen.findByText('Cliente 1')

    fireEvent.click(screen.getByRole('button', { name: /Nuevo Cliente/ }))
    fireEvent.change(screen.getByLabelText('Nombre del Cliente'), { target: { value: 'Ana' } })
    fireEvent.change(screen.getByLabelText('Cédula'), { target: { value: '1020304050' } })
    fireEvent.click(screen.getByRole('button', { name: 'Guardar' }))

    const alert = await screen.findByRole('alert')
    expect(alert.textContent).toBe('A client with this identity card already exists.')
  })

  it('shows Pagar only for clients with debt and names the action after the client', async () => {
    mocks.getClients.mockResolvedValue(pageOf([
      client({ id: 1, name: 'Cliente con deuda', debt_amount: 150.5 }),
      client({ id: 2, name: 'Cliente al día', debt_amount: 0 }),
    ]))
    renderClients()

    await screen.findByText('Cliente con deuda')
    expect(screen.getByRole('button', { name: 'Pagar deuda de Cliente con deuda' })).toBeTruthy()
    expect(screen.getByText('Sin deuda')).toBeTruthy()
    expect(screen.queryByRole('button', { name: 'Pagar deuda de Cliente al día' })).toBeNull()
  })

  it('opens the payment modal for the clicked client', async () => {
    mocks.getClients.mockResolvedValue(pageOf([
      client({ id: 1, name: 'Cliente con deuda', debt_amount: 150.5 }),
      client({ id: 2, name: 'Otro cliente con deuda', debt_amount: 80 }),
    ]))
    renderClients()
    await screen.findByText('Otro cliente con deuda')

    fireEvent.click(screen.getByRole('button', { name: 'Pagar deuda de Otro cliente con deuda' }))

    const dialog = await screen.findByRole('dialog', { name: 'Registrar pago' })
    expect(within(dialog).getByText('Otro cliente con deuda')).toBeTruthy()
    expect((within(dialog).getByLabelText('Monto') as HTMLInputElement).value).toBe('80.00')
  })

  it('records a payment and invalidates the client list and detail', async () => {
    mocks.getClients.mockResolvedValue(pageOf([
      client({ id: 7, name: 'Cliente con deuda', debt_amount: 150 }),
    ]))
    mocks.addClientPayment.mockResolvedValue({
      data: paymentResult({ client_id: 7, amount: 150, remaining_debt: 0 }),
    })
    const queryClient = renderClients()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')
    await screen.findByText('Cliente con deuda')

    fireEvent.click(screen.getByRole('button', { name: 'Pagar deuda de Cliente con deuda' }))
    payWithCash()

    await waitFor(() => expect(mocks.addClientPayment).toHaveBeenCalledWith(7, {
      amount: 150,
      payment_method: 'cash',
    }))
    await waitFor(() => expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['clients'] }))
    await waitFor(() => expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['client', 7] }))
  })

  it('keeps Pagar while the remaining debt is greater than zero', async () => {
    let debt = 150
    mocks.getClients.mockImplementation(() => Promise.resolve(pageOf([
      client({ id: 3, name: 'Cliente con deuda', debt_amount: debt }),
    ])))
    mocks.addClientPayment.mockResolvedValue({
      data: paymentResult({ client_id: 3, amount: 75, remaining_debt: 75 }),
    })
    renderClients()
    await screen.findByText('Cliente con deuda')

    fireEvent.click(screen.getByRole('button', { name: 'Pagar deuda de Cliente con deuda' }))
    debt = 75
    payWithCash('75')

    await screen.findByText(`Pago registrado. Saldo pendiente: ${formatCurrency(75)}.`)
    await screen.findByText(formatCurrency(75))
    expect(screen.getByRole('button', { name: 'Pagar deuda de Cliente con deuda' })).toBeTruthy()
  })

  it('removes Pagar once the debt reaches zero', async () => {
    let debt = 150
    mocks.getClients.mockImplementation(() => Promise.resolve(pageOf([
      client({ id: 4, name: 'Cliente con deuda', debt_amount: debt }),
    ])))
    mocks.addClientPayment.mockResolvedValue({
      data: paymentResult({ client_id: 4, amount: 150, remaining_debt: 0 }),
    })
    renderClients()
    await screen.findByText('Cliente con deuda')

    fireEvent.click(screen.getByRole('button', { name: 'Pagar deuda de Cliente con deuda' }))
    debt = 0
    payWithCash()

    await screen.findByText(`Pago registrado. Saldo pendiente: ${formatCurrency(0)}.`)
    await waitFor(() => expect(
      screen.queryByRole('button', { name: 'Pagar deuda de Cliente con deuda' }),
    ).toBeNull())
    expect(screen.getByText('Sin deuda')).toBeTruthy()
  })
})
