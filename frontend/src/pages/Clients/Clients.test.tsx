import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import Clients from './Clients'

const mocks = vi.hoisted(() => ({
  getClients: vi.fn(),
  createClient: vi.fn(),
  updateClient: vi.fn(),
  deleteClient: vi.fn(),
}))

vi.mock('@/lib/api/clients', () => ({
  clientsApi: {
    getClients: mocks.getClients,
    createClient: mocks.createClient,
    updateClient: mocks.updateClient,
    deleteClient: mocks.deleteClient,
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
        created_at: '2026-01-01T00:00:00',
        updated_at: null,
      }],
      has_next: true,
      skip,
      limit: 10,
    },
  }
}

function sleep(ms: number) {
  return act(async () => {
    await new Promise(resolve => setTimeout(resolve, ms))
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
    created_at: '2026-01-01T00:00:00',
    updated_at: null,
    ...overrides,
  }
}

describe('Clients search and pagination', () => {
  beforeEach(() => {
    mocks.getClients.mockReset()
    mocks.getClients.mockImplementation((params: { skip: number }) => Promise.resolve(clientPage(params.skip)))
    mocks.createClient.mockReset()
    mocks.updateClient.mockReset()
    mocks.deleteClient.mockReset()
    mocks.deleteClient.mockResolvedValue({ data: client() })
  })

  it('resets to page 1 and only requests the debounced search', async () => {
    renderClients()

    await waitFor(() => expect(mocks.getClients).toHaveBeenCalledWith({ skip: 0, limit: 10 }))
    await screen.findByText('Cliente 1')
    await sleep(350)

    fireEvent.click(screen.getByRole('button', { name: 'Ir a página siguiente' }))
    await waitFor(() => expect(mocks.getClients).toHaveBeenCalledWith({ skip: 10, limit: 10 }))
    await screen.findByText('Página 2')

    mocks.getClients.mockClear()
    fireEvent.change(
      screen.getByPlaceholderText('Buscar cliente por nombre, cédula o contacto...'),
      { target: { value: 'ana' } },
    )

    await sleep(50)
    expect(mocks.getClients).not.toHaveBeenCalled()

    await sleep(300)
    await waitFor(() => expect(mocks.getClients).toHaveBeenCalledWith({ skip: 0, limit: 10, search: 'ana' }))
    expect(mocks.getClients).toHaveBeenCalledTimes(1)
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
})
