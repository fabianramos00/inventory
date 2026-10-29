import type { ReactNode } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import Dashboard from './Dashboard'

const mocks = vi.hoisted(() => ({
  getSummary: vi.fn(),
  getSalesTrend: vi.fn(),
  getTopProducts: vi.fn(),
  getOrderStatusDistribution: vi.fn(),
  getProducts: vi.fn(),
  getStats: vi.fn(),
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
    getStats: mocks.getStats,
  },
}))

vi.mock('recharts', () => ({
  ResponsiveContainer: ({ children }: { children?: ReactNode }) => <>{children}</>,
  LineChart: ({ data }: { data?: Array<{ dia: string; ventas: number }> }) => (
    <div data-testid="trend-chart" data-points={JSON.stringify(data ?? [])} />
  ),
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

const topProduct = {
  product_id: 1,
  product_name: 'Martillo',
  product_sku: 'SKU-1',
  brand_name: 'Marca',
  category_name: 'Herramientas',
  material_name: 'Acero',
  total_quantity_sold: 3,
  total_sales_amount: 21,
}

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

describe('Dashboard', () => {
  beforeEach(() => {
    mocks.getSummary.mockReset().mockResolvedValue({ data: summary })
    mocks.getSalesTrend.mockReset().mockResolvedValue({ data: { data: [trendItem] } })
    mocks.getTopProducts.mockReset().mockResolvedValue({ data: { products: [] } })
    mocks.getOrderStatusDistribution.mockReset().mockResolvedValue({ data: { data: [] } })
    mocks.getProducts.mockReset().mockResolvedValue({ data: { items: [], total: 0 } })
    mocks.getStats.mockReset().mockResolvedValue({
      data: { total_products: 0, total_stock: 0, low_stock_count: 0, total_inventory_value: 0 },
    })
  })

  it('renders pending payment sales as COP currency, never as a count', async () => {
    mocks.getSummary.mockResolvedValue({
      data: { ...summary, total_sales_today: 2500, pending_payment_sales: 1234.5 },
    })
    renderDashboard()

    const unpaidLabel = await screen.findByText('Ventas sin pagar')
    expect(unpaidLabel.parentElement?.textContent).toBe('Ventas sin pagar$ 1.234,50')
    expect(screen.getByText('Ventas de hoy').parentElement?.textContent).toBe('Ventas de hoy$ 2.500,00')
    expect(screen.getByText('Pedidos pendientes').parentElement?.textContent).toBe('Pedidos pendientes2')
    expect(screen.getByText('Hay $ 1.234,50 en ventas sin pagar.')).toBeTruthy()
  })

  it('renders the order status distribution with counts and percentages', async () => {
    mocks.getOrderStatusDistribution.mockResolvedValue({
      data: {
        data: [
          { status: 'pending', count: 2, percentage: 50 },
          { status: 'received', count: 2, percentage: 50 },
        ],
      },
    })
    renderDashboard()

    expect(await screen.findByText('Pendiente')).toBeTruthy()
    expect(screen.getByText('Recibido')).toBeTruthy()
    expect(screen.getAllByText('50%')).toHaveLength(2)
  })

  it('renders all seven zero-filled trend days', async () => {
    const zeroTrend = [
      '2026-10-22',
      '2026-10-23',
      '2026-10-24',
      '2026-10-25',
      '2026-10-26',
      '2026-10-27',
      '2026-10-28',
    ].map((date) => ({ date, sales_amount: 0, orders_cost: 0 }))
    mocks.getSalesTrend.mockResolvedValue({ data: { data: zeroTrend } })
    renderDashboard()

    const chart = await screen.findByTestId('trend-chart')
    const points = JSON.parse(chart.dataset.points ?? '[]') as Array<{ dia: string; ventas: number }>
    expect(points).toHaveLength(7)
    expect(points.map((point) => point.dia)).toEqual(['Jue', 'Vie', 'Sáb', 'Dom', 'Lun', 'Mar', 'Mié'])
    expect(points.every((point) => point.ventas === 0)).toBe(true)
    expect(screen.getByText('Total 7 días').parentElement?.textContent).toBe('Total 7 días$ 0,00')
  })

  it('keeps the trend calendar day when the host timezone is ahead of UTC', async () => {
    vi.stubEnv('TZ', 'Pacific/Kiritimati')
    try {
      mocks.getSalesTrend.mockResolvedValue({
        data: { data: [{ date: '2026-10-24', sales_amount: 0, orders_cost: 0 }] },
      })
      renderDashboard()

      const chart = await screen.findByTestId('trend-chart')
      const points = JSON.parse(chart.dataset.points ?? '[]') as Array<{ dia: string; ventas: number }>
      expect(points[0]).toEqual({ dia: 'Sáb', ventas: 0 })
    } finally {
      vi.unstubAllEnvs()
    }
  })

  it('renders top products with nullable brand, category, and material names', async () => {
    mocks.getTopProducts.mockResolvedValue({
      data: {
        products: [
          { ...topProduct, brand_name: null, category_name: null, material_name: null },
        ],
      },
    })
    renderDashboard()

    const row = (await screen.findByText('Martillo')).closest('tr')
    expect(row).not.toBeNull()
    expect(within(row as HTMLElement).getByText('SKU-1')).toBeTruthy()
    expect(within(row as HTMLElement).getByText('-')).toBeTruthy()
    expect(within(row as HTMLElement).getByText('$ 21,00')).toBeTruthy()
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

  it('retries only the failed order-status section', async () => {
    mocks.getOrderStatusDistribution.mockRejectedValueOnce(new Error('orders down'))
    renderDashboard()

    await screen.findByText('No se pudo cargar el estado de pedidos.')
    mocks.getOrderStatusDistribution.mockResolvedValue({
      data: { data: [{ status: 'pending', count: 2, percentage: 100 }] },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Reintentar' }))

    await waitFor(() => expect(mocks.getOrderStatusDistribution).toHaveBeenCalledTimes(2))
    expect(await screen.findByText('Pendiente')).toBeTruthy()
    expect(mocks.getSummary).toHaveBeenCalledTimes(1)
    expect(mocks.getSalesTrend).toHaveBeenCalledTimes(1)
  })

  it('retries only the failed top-products section', async () => {
    mocks.getTopProducts.mockRejectedValueOnce(new Error('products down'))
    renderDashboard()

    await screen.findByText('No se pudieron cargar los productos vendidos.')
    mocks.getTopProducts.mockResolvedValue({ data: { products: [topProduct] } })
    fireEvent.click(screen.getByRole('button', { name: 'Reintentar' }))

    await waitFor(() => expect(mocks.getTopProducts).toHaveBeenCalledTimes(2))
    expect(await screen.findByText('Martillo')).toBeTruthy()
    expect(mocks.getSummary).toHaveBeenCalledTimes(1)
    expect(mocks.getOrderStatusDistribution).toHaveBeenCalledTimes(1)
  })

  it('retries only the failed low-stock section and its stats', async () => {
    mocks.getProducts.mockRejectedValueOnce(new Error('stock down'))
    renderDashboard()

    await screen.findByText('No se pudo cargar el stock bajo.')
    mocks.getProducts.mockResolvedValue({
      data: {
        items: [{ id: 1, name: 'Martillo', sku: 'SKU-1', stock_quantity: 2, low_stock_threshold: 5 }],
        total: 1,
        has_next: false,
      },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Reintentar' }))

    await waitFor(() => expect(mocks.getProducts).toHaveBeenCalledTimes(2))
    expect(await screen.findByText('Martillo')).toBeTruthy()
    expect(mocks.getStats).toHaveBeenCalledTimes(2)
    expect(mocks.getSummary).toHaveBeenCalledTimes(1)
  })

  it('uses the exact low-stock count from stats instead of the returned page length', async () => {
    mocks.getSummary.mockResolvedValue({ data: { ...summary, pending_payment_sales: 0 } })
    mocks.getStats.mockResolvedValue({
      data: { total_products: 100, total_stock: 500, low_stock_count: 25, total_inventory_value: 1000 },
    })
    mocks.getProducts.mockResolvedValue({
      data: {
        items: Array.from({ length: 10 }, (_, i) => ({
          id: i + 1,
          name: `Producto ${i + 1}`,
          sku: `SKU-${i + 1}`,
          stock_quantity: 1,
          low_stock_threshold: 5,
        })),
        total: 10,
        has_next: true,
      },
    })
    renderDashboard()

    expect(await screen.findByText('25 productos bajo el mínimo.')).toBeTruthy()
    expect(screen.getAllByText('Stock bajo')[0].parentElement?.textContent).toContain('25')
    expect(mocks.getProducts).toHaveBeenCalledWith({ skip: 0, limit: 10, stock_status: 'low_stock' })
  })

  it('falls back to the returned page length when stats fail', async () => {
    mocks.getSummary.mockResolvedValue({ data: { ...summary, pending_payment_sales: 0 } })
    mocks.getStats.mockRejectedValue(new Error('stats down'))
    mocks.getProducts.mockResolvedValue({
      data: {
        items: [
          { id: 1, name: 'Martillo', sku: 'SKU-1', stock_quantity: 2, low_stock_threshold: 5 },
          { id: 2, name: 'Cinta', sku: 'SKU-2', stock_quantity: 0, low_stock_threshold: 3 },
        ],
        total: 2,
        has_next: false,
      },
    })
    renderDashboard()

    expect(await screen.findByText('2 productos bajo el mínimo.')).toBeTruthy()
    expect(screen.getAllByText('Stock bajo')[0].parentElement?.textContent).toContain('2')
    expect(screen.getByText('Martillo')).toBeTruthy()
    expect(screen.getByText('SKU-1')).toBeTruthy()
    expect(screen.getByText('Cinta')).toBeTruthy()
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

  it('renders zero money KPIs as COP and zero pending orders as a count', async () => {
    mocks.getSummary.mockResolvedValue({
      data: {
        inventory_value: 0,
        total_sales_today: 0,
        pending_orders: 0,
        delivered_orders: 0,
        pending_payment_sales: 0,
      },
    })
    renderDashboard()

    const today = await screen.findByText('Ventas de hoy')
    expect(today.parentElement?.textContent).toBe('Ventas de hoy$ 0,00')
    expect(screen.getByText('Ventas sin pagar').parentElement?.textContent).toBe('Ventas sin pagar$ 0,00')
    expect(screen.getByText('Pedidos pendientes').parentElement?.textContent).toBe('Pedidos pendientes0')
    expect(screen.queryByText('Hay $ 0,00 en ventas sin pagar.')).toBeNull()
  })

  it('keeps the trend calendar day when the host timezone is behind UTC', async () => {
    vi.stubEnv('TZ', 'America/Los_Angeles')
    try {
      mocks.getSalesTrend.mockResolvedValue({
        data: { data: [{ date: '2026-10-24', sales_amount: 1, orders_cost: 0 }] },
      })
      renderDashboard()

      const chart = await screen.findByTestId('trend-chart')
      const points = JSON.parse(chart.dataset.points ?? '[]') as Array<{ dia: string; ventas: number }>
      expect(points[0]).toEqual({ dia: 'Sáb', ventas: 1 })
    } finally {
      vi.unstubAllEnvs()
    }
  })
})
