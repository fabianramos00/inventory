import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import Dashboard from './Dashboard'

const mocks = vi.hoisted(() => ({
  getSummary: vi.fn(),
  getSalesTrend: vi.fn(),
  getTopProducts: vi.fn(),
  getOrderStatusDistribution: vi.fn(),
  getProducts: vi.fn(),
}))

vi.mock('@/lib/api/dashboard', () => ({
  dashboardApi: {
    getSummary: mocks.getSummary,
    getSalesTrend: mocks.getSalesTrend,
    getTopProducts: mocks.getTopProducts,
    getOrderStatusDistribution: mocks.getOrderStatusDistribution,
  },
}))

vi.mock('@/lib/api/inventory', () => ({
  inventoryApi: {
    getProducts: mocks.getProducts,
  },
}))

vi.mock('recharts', () => ({
  ResponsiveContainer: () => null,
  LineChart: () => null,
  Line: () => null,
  XAxis: () => null,
  YAxis: () => null,
  CartesianGrid: () => null,
  Tooltip: () => null,
}))

const summary = {
  inventory_value: 1000,
  total_sales_today: 100,
  pending_orders: 2,
  delivered_orders: 3,
  pending_payment_sales: 50,
}

const trendItem = { date: '2026-01-01', sales_amount: 10, orders_cost: 5 }

function renderDashboard() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <Dashboard />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('Dashboard error states', () => {
  beforeEach(() => {
    mocks.getSummary.mockReset().mockResolvedValue({ data: summary })
    mocks.getSalesTrend.mockReset().mockResolvedValue({ data: { data: [trendItem] } })
    mocks.getTopProducts.mockReset().mockResolvedValue({ data: { products: [] } })
    mocks.getOrderStatusDistribution.mockReset().mockResolvedValue({ data: { data: [] } })
    mocks.getProducts.mockReset().mockResolvedValue({ data: { items: [], total: 0 } })
  })

  it('distinguishes a failed section from a successful empty one and retries only that section', async () => {
    mocks.getSalesTrend.mockRejectedValueOnce(new Error('trend down'))
    renderDashboard()

    await screen.findByText('No se pudo cargar la tendencia de ventas.')
    expect(screen.getByText('Sin productos vendidos.')).toBeTruthy()
    expect(screen.queryByText('No se pudieron cargar los productos vendidos.')).toBeNull()

    mocks.getSalesTrend.mockResolvedValue({ data: { data: [trendItem] } })
    fireEvent.click(screen.getByRole('button', { name: 'Reintentar' }))

    await waitFor(() => expect(mocks.getSalesTrend).toHaveBeenCalledTimes(2))
    await waitFor(() => expect(screen.queryByText('No se pudo cargar la tendencia de ventas.')).toBeNull())
    expect(mocks.getTopProducts).toHaveBeenCalledTimes(1)
  })

  it('shows successful empty messages instead of errors', async () => {
    mocks.getSalesTrend.mockResolvedValue({ data: { data: [] } })
    renderDashboard()

    await screen.findByText('Sin datos de tendencia de ventas.')
    expect(screen.getByText('Sin datos de estado de pedidos.')).toBeTruthy()
    expect(screen.getByText('Nada bajo el mínimo')).toBeTruthy()
    expect(screen.queryByText('No se pudo cargar la tendencia de ventas.')).toBeNull()
    expect(screen.queryByRole('button', { name: 'Reintentar' })).toBeNull()
  })

  it('shows a summary error instead of the KPIs and retries only the summary', async () => {
    mocks.getSummary.mockRejectedValueOnce(new Error('summary down'))
    renderDashboard()

    await screen.findByText('No se pudieron cargar los indicadores.')
    expect(screen.queryByText('Ventas de hoy')).toBeNull()

    mocks.getSummary.mockResolvedValue({ data: summary })
    fireEvent.click(screen.getByRole('button', { name: 'Reintentar' }))

    await waitFor(() => expect(mocks.getSummary).toHaveBeenCalledTimes(2))
    expect(mocks.getSalesTrend).toHaveBeenCalledTimes(1)
    expect(await screen.findByText('Ventas de hoy')).toBeTruthy()
  })
})
