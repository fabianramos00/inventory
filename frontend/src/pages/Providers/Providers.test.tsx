import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import Providers from './Providers'

const mocks = vi.hoisted(() => ({
  getProviders: vi.fn(),
  createProvider: vi.fn(),
  updateProvider: vi.fn(),
  deleteProvider: vi.fn(),
}))

vi.mock('@/lib/api/providers', () => ({
  providersApi: {
    getProviders: mocks.getProviders,
    createProvider: mocks.createProvider,
    updateProvider: mocks.updateProvider,
    deleteProvider: mocks.deleteProvider,
  },
}))

function providerPage(skip: number) {
  return {
    data: {
      items: [{
        id: skip + 1,
        name: `Proveedor ${skip + 1}`,
        contact_info: null,
        email: null,
        phone: null,
      }],
      has_next: true,
      skip,
      limit: 10,
    },
  }
}

function renderProviders() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <Providers />
    </QueryClientProvider>,
  )
  return queryClient
}

function provider(overrides: Record<string, unknown> = {}) {
  return {
    id: 1,
    name: 'Proveedor 1',
    contact_info: 'Juan Pérez',
    email: 'ventas@example.com',
    phone: '3001234567',
    ...overrides,
  }
}

describe('Providers search and pagination', () => {
  beforeEach(() => {
    mocks.getProviders.mockReset()
    mocks.getProviders.mockImplementation((params: { skip: number }) => Promise.resolve(providerPage(params.skip)))
    mocks.createProvider.mockReset()
    mocks.updateProvider.mockReset()
    mocks.deleteProvider.mockReset()
    mocks.deleteProvider.mockResolvedValue({ data: provider() })
  })

  it('uses has_next for cursor pagination and resets to page 1 on search', async () => {
    renderProviders()

    await waitFor(() => expect(mocks.getProviders).toHaveBeenCalledWith({ skip: 0, limit: 10 }))
    await screen.findByText('Proveedor 1')
    expect(screen.queryByText(/de \d+$/)).toBeNull()

    fireEvent.click(screen.getByRole('button', { name: 'Ir a página siguiente' }))
    await waitFor(() => expect(mocks.getProviders).toHaveBeenCalledWith({ skip: 10, limit: 10 }))
    await screen.findByText('Página 2')

    mocks.getProviders.mockClear()
    fireEvent.change(
      screen.getByPlaceholderText('Buscar proveedor por nombre, correo o teléfono...'),
      { target: { value: 'ana' } },
    )

    await waitFor(() => expect(mocks.getProviders).toHaveBeenCalledWith({ skip: 0, limit: 10, search: 'ana' }))
    expect(mocks.getProviders).toHaveBeenCalledTimes(1)
  })

  it('ignores total/pages and hides pagination when has_next is false', async () => {
    mocks.getProviders.mockResolvedValueOnce({
      data: { items: [provider()], has_next: false, skip: 0, limit: 10, total: 100, pages: 10 },
    })
    renderProviders()

    await screen.findByText('Proveedor 1')
    expect(screen.queryByRole('button', { name: 'Ir a página siguiente' })).toBeNull()
    expect(screen.queryByText(/de \d+$/)).toBeNull()
  })

  it('shows a retryable error instead of the empty state when the query fails', async () => {
    mocks.getProviders.mockRejectedValueOnce(new Error('API down'))
    renderProviders()

    await screen.findByText('No se pudieron cargar los proveedores.')
    expect(screen.queryByText('No se encontraron proveedores.')).toBeNull()

    mocks.getProviders.mockResolvedValue(providerPage(0))
    fireEvent.click(screen.getByRole('button', { name: 'Reintentar' }))

    await screen.findByText('Proveedor 1')
    expect(screen.queryByText('No se pudieron cargar los proveedores.')).toBeNull()
  })

  it('creates a provider with the name only and omits empty optional fields', async () => {
    mocks.createProvider.mockResolvedValue({ data: provider({ id: 9, name: 'Distribuidora' }) })
    renderProviders()
    await screen.findByText('Proveedor 1')

    fireEvent.click(screen.getByRole('button', { name: /Nuevo Proveedor/ }))
    fireEvent.change(screen.getByLabelText('Nombre del Proveedor'), { target: { value: 'Distribuidora' } })
    fireEvent.click(screen.getByRole('button', { name: 'Guardar' }))

    await waitFor(() => expect(mocks.createProvider).toHaveBeenCalledWith({ name: 'Distribuidora' }))
  })

  it('invalidates the provider list after creating a provider', async () => {
    mocks.createProvider.mockResolvedValue({ data: provider({ id: 9, name: 'Distribuidora' }) })
    const queryClient = renderProviders()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')
    await screen.findByText('Proveedor 1')

    fireEvent.click(screen.getByRole('button', { name: /Nuevo Proveedor/ }))
    fireEvent.change(screen.getByLabelText('Nombre del Proveedor'), { target: { value: 'Distribuidora' } })
    fireEvent.click(screen.getByRole('button', { name: 'Guardar' }))

    await waitFor(() => expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['providers'] }))
  })

  it('deletes a provider after confirming and invalidates the provider list', async () => {
    const queryClient = renderProviders()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')
    await screen.findByText('Proveedor 1')

    fireEvent.click(screen.getByTitle('Eliminar'))
    fireEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Eliminar' }))

    await waitFor(() => expect(mocks.deleteProvider).toHaveBeenCalledWith(1))
    await waitFor(() => expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['providers'] }))
  })

  it('clears optional fields when the edit form submits them empty', async () => {
    mocks.getProviders.mockResolvedValue({
      data: { items: [provider()], has_next: false, skip: 0, limit: 10 },
    })
    mocks.updateProvider.mockResolvedValue({ data: provider({ email: null }) })
    renderProviders()
    await screen.findByText('Proveedor 1')

    fireEvent.click(screen.getByTitle('Editar'))
    fireEvent.change(screen.getByLabelText('Correo Electrónico'), { target: { value: '' } })
    fireEvent.click(screen.getByRole('button', { name: 'Guardar' }))

    await waitFor(() => expect(mocks.updateProvider).toHaveBeenCalledWith(1, {
      name: 'Proveedor 1',
      contact_info: 'Juan Pérez',
      email: null,
      phone: '3001234567',
    }))
  })

  it('shows the readable foreign key error on delete conflict', async () => {
    mocks.deleteProvider.mockRejectedValue(Object.assign(new Error('Request failed with status code 400'), {
      isAxiosError: true,
      response: {
        status: 400,
        data: { detail: 'Provider is referenced by an order', code: 'FOREIGN_KEY_VIOLATION' },
      },
    }))
    renderProviders()
    await screen.findByText('Proveedor 1')

    fireEvent.click(screen.getByTitle('Eliminar'))
    fireEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Eliminar' }))

    const alert = await screen.findByRole('alert')
    expect(alert.textContent).toBe('Provider is referenced by an order')
  })
})
