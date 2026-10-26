import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import Clients from './Clients'

const mocks = vi.hoisted(() => ({ getClients: vi.fn() }))

vi.mock('@/lib/api/clients', () => ({
  clientsApi: {
    getClients: mocks.getClients,
    createClient: vi.fn(),
    updateClient: vi.fn(),
    deleteClient: vi.fn(),
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
}

describe('Clients search and pagination', () => {
  beforeEach(() => {
    mocks.getClients.mockReset()
    mocks.getClients.mockImplementation((params: { skip: number }) => Promise.resolve(clientPage(params.skip)))
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
})
