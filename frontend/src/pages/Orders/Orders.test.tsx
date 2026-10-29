import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import Orders from './Orders'

const mocks = vi.hoisted(() => ({
  getOrders: vi.fn(),
  deleteOrder: vi.fn(),
}))

vi.mock('@/lib/api/orders', () => ({
  ordersApi: {
    getOrders: mocks.getOrders,
    deleteOrder: mocks.deleteOrder,
  },
}))

vi.mock('@/lib/api/providers', () => ({
  providersApi: {
    getProviders: vi.fn().mockResolvedValue({ data: { items: [], has_next: false } }),
  },
}))

function orderPage() {
  return {
    data: {
      items: [{
        id: 1,
        provider_id: 2,
        status: 'pending',
        payment_status: 'pending',
        order_date: '2026-01-01T00:00:00',
        total_amount: 120,
        provider: { id: 2, name: 'Ferretería Central', contact_info: null, email: null, phone: null },
      }],
      has_next: false,
      skip: 0,
      limit: 10,
    },
  }
}

function renderOrders() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <Orders />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('Orders list', () => {
  beforeEach(() => {
    mocks.getOrders.mockReset().mockResolvedValue(orderPage())
    mocks.deleteOrder.mockReset().mockResolvedValue(undefined)
  })

  it('requests the first page without empty filter params', async () => {
    renderOrders()

    await screen.findByText('Ferretería Central')
    expect(mocks.getOrders).toHaveBeenCalledWith({ skip: 0, limit: 10 })
  })

  it('shows a retryable error instead of the empty state when the query fails', async () => {
    mocks.getOrders.mockRejectedValueOnce(new Error('API down'))
    renderOrders()

    await screen.findByText('No se pudieron cargar los datos.')
    expect(screen.queryByText('No se encontraron pedidos.')).toBeNull()

    mocks.getOrders.mockResolvedValue(orderPage())
    fireEvent.click(screen.getByRole('button', { name: 'Reintentar' }))

    await screen.findByText('Ferretería Central')
    expect(mocks.getOrders).toHaveBeenCalledTimes(2)
    expect(screen.queryByText('No se pudieron cargar los datos.')).toBeNull()
  })

  it('deletes the selected order after confirming', async () => {
    renderOrders()

    await screen.findByText('Ferretería Central')
    fireEvent.click(screen.getByTitle('Eliminar pedido'))

    expect(screen.getByText(/Pedido #1 — Ferretería Central/)).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Eliminar' }))

    await waitFor(() => expect(mocks.deleteOrder).toHaveBeenCalledWith(1))
  })

  it('uses has_next to enable cursor pagination', async () => {
    mocks.getOrders.mockResolvedValue({
      data: { ...orderPage().data, has_next: true },
    })
    renderOrders()
    await screen.findByText('Ferretería Central')

    const next = screen.getByRole('button', { name: 'Ir a página siguiente' }) as HTMLButtonElement
    expect(next.disabled).toBe(false)
    fireEvent.click(next)

    await waitFor(() => expect(mocks.getOrders).toHaveBeenLastCalledWith({ skip: 10, limit: 10 }))
  })

  it('sends Bogota day bounds and resets the page when a date filter changes', async () => {
    mocks.getOrders.mockResolvedValue({
      data: { ...orderPage().data, has_next: true },
    })
    renderOrders()
    await screen.findByText('Ferretería Central')

    fireEvent.click(screen.getByRole('button', { name: 'Ir a página siguiente' }))
    await waitFor(() => expect(mocks.getOrders).toHaveBeenLastCalledWith({ skip: 10, limit: 10 }))

    fireEvent.change(screen.getByTitle('Fecha inicio'), { target: { value: '2026-10-24' } })

    await waitFor(() => expect(mocks.getOrders).toHaveBeenLastCalledWith({
      skip: 0,
      limit: 10,
      min_date: '2026-10-24T05:00:00.000Z',
    }))
  })

  it('sends the inclusive Bogota end-of-day bound', async () => {
    renderOrders()
    await screen.findByText('Ferretería Central')

    fireEvent.change(screen.getByTitle('Fecha fin'), { target: { value: '2026-10-25' } })

    await waitFor(() => expect(mocks.getOrders).toHaveBeenLastCalledWith({
      skip: 0,
      limit: 10,
      max_date: '2026-10-26T04:59:59.999Z',
    }))
  })

  it('resets the page when a status filter changes', async () => {
    mocks.getOrders.mockResolvedValue({
      data: { ...orderPage().data, has_next: true },
    })
    renderOrders()
    await screen.findByText('Ferretería Central')

    fireEvent.click(screen.getByRole('button', { name: 'Ir a página siguiente' }))
    await waitFor(() => expect(mocks.getOrders).toHaveBeenLastCalledWith({ skip: 10, limit: 10 }))

    fireEvent.click(screen.getByRole('button', { name: 'Estado' }))
    fireEvent.click(screen.getByRole('option', { name: 'Enviado' }))

    await waitFor(() => expect(mocks.getOrders).toHaveBeenLastCalledWith({
      skip: 0,
      limit: 10,
      status: 'sent',
    }))
  })
})
