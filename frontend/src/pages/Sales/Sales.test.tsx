import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
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
    created_at: '2026-01-15T12:00:00Z',
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
  return render(
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
  })

  it('renders the split columns with independent payment and delivery states', () => {
    const { container } = renderSales()

    const headers = screen.getAllByRole('columnheader').map(th => th.textContent)
    expect(headers).toEqual(['Venta', 'Fecha', 'Cliente', 'Total / Cobro', 'Estado pago', 'Entrega', 'Usuario', 'Acciones'])

    const row = screen.getByText('Carlos Ruiz').closest('tr')!
    expect(within(row).getByRole('button', { name: 'Ver venta #1' })).toBeTruthy()
    expect(within(row).getByText('15/01/2026')).toBeTruthy()
    expect(within(row).getByText('Parcial')).toBeTruthy()
    expect(within(row).getByText('Pendiente')).toBeTruthy()

    const moneyTexts = within(row).getAllByText(/\$/).map(node => node.textContent)
    expect(moneyTexts).toEqual(['$ 100,00', 'Debe $ 40,00'])

    expect(container.querySelector('progress')).toBeNull()
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
