import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import Sales from './Sales'

const mocks = vi.hoisted(() => ({
  useSales: vi.fn(),
}))

vi.mock('@/hooks/useSales', () => ({
  useSales: mocks.useSales,
}))

vi.mock('@/lib/api/users', () => ({
  usersApi: { getUsers: vi.fn().mockResolvedValue({ data: { items: [], has_next: false } }) },
}))

vi.mock('@/lib/api/clients', () => ({
  clientsApi: { getClients: vi.fn().mockResolvedValue({ data: { items: [], has_next: false } }) },
}))

function sale(overrides: Record<string, unknown> = {}) {
  return {
    id: 1,
    total_amount: 100,
    debt_amount: 40,
    amount_paid: 60,
    payment_status: 'partial',
    delivery_status: 'pending',
    payment_method: 'cash',
    client_id: 5,
    created_at: '2026-01-02T03:04:05Z',
    updated_at: null,
    created_by_id: 2,
    created_by: { id: 2, full_name: 'Ana Pérez', email: 'ana@example.com', is_active: true, is_superuser: false },
    client: { id: 5, name: 'Carlos Ruiz' },
    ...overrides,
  }
}

function baseResult(overrides: Record<string, unknown> = {}) {
  return {
    sales: [sale()],
    hasNext: false,
    stats: { unpaidCount: 1, undeliveredCount: 2, totalAmountSum: 100, amountPaidSum: 60 },
    isLoading: false,
    isFetching: false,
    isError: false,
    refetch: vi.fn(),
    deleteMutation: { mutateAsync: vi.fn() },
    ...overrides,
  }
}

function renderSales() {
  render(
    <MemoryRouter initialEntries={['/sales']}>
      <Sales />
    </MemoryRouter>,
  )
}

describe('Sales list', () => {
  beforeEach(() => {
    mocks.useSales.mockReset().mockImplementation(() => baseResult())
  })

  it('renders the stats strip and the debt amount returned by the API', () => {
    renderSales()

    expect(screen.getByText('Total vendido')).toBeTruthy()
    expect(screen.getByText('Por cobrar')).toBeTruthy()
    expect(screen.getAllByText('$ 100,00').length).toBeGreaterThan(0)
    expect(screen.getAllByText('$ 60,00').length).toBeGreaterThan(0)
    expect(screen.getAllByText('$ 40,00').length).toBeGreaterThan(0)
    expect(screen.getByText('Debe $ 40,00')).toBeTruthy()
    expect(screen.getByText('Carlos Ruiz')).toBeTruthy()
    expect(screen.getByText('Ana Pérez')).toBeTruthy()
    expect(screen.getByText('Parcial')).toBeTruthy()
    expect(screen.getByText('Entrega: Pendiente')).toBeTruthy()
  })

  it('moves to the next page only while the cursor reports has_next', async () => {
    mocks.useSales.mockImplementation(({ page }: { page: number }) => baseResult({
      hasNext: page === 1,
      sales: [sale({ id: page })],
    }))
    renderSales()

    fireEvent.click(screen.getByRole('button', { name: 'Ir a página siguiente' }))

    await waitFor(() => expect(mocks.useSales).toHaveBeenLastCalledWith(expect.objectContaining({ page: 2 })))
    const next = screen.getByRole('button', { name: 'Ir a página siguiente' })
    expect((next as HTMLButtonElement).disabled).toBe(true)

    fireEvent.click(next)
    expect(mocks.useSales).toHaveBeenLastCalledWith(expect.objectContaining({ page: 2 }))
  })

  it('does not render pagination controls when the first page has no next page', () => {
    renderSales()

    expect(screen.queryByRole('button', { name: 'Ir a página siguiente' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Ir a página anterior' })).toBeNull()
  })
})
