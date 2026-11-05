import { expect, test, type Page } from '@playwright/test'
import {
  AUTH_FILE,
  createApiClient,
  seedCatalog,
  seedClient,
  seedProduct,
  seedProvider,
  uniqueSuffix,
} from './helpers'
import { mockHealth } from './support'

test.use({ storageState: AUTH_FILE })

const BANNER_HEIGHT_VARIABLE = '--offline-banner-height'

function bannerHeightVariable(page: Page) {
  return page.evaluate(
    variable => document.documentElement.style.getPropertyValue(variable).trim(),
    BANNER_HEIGHT_VARIABLE,
  )
}

async function expectBannerOffsetPublished(page: Page) {
  await expect
    .poll(() => bannerHeightVariable(page), {
      message: 'offline banner publishes --offline-banner-height',
    })
    .not.toBe('')
  expect(Number.parseFloat(await bannerHeightVariable(page))).toBeGreaterThan(0)
}

async function expectNoHorizontalOverflow(page: Page) {
  const { scrollWidth, clientWidth } = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    clientWidth: document.documentElement.clientWidth,
  }))
  expect(scrollWidth).toBeLessThanOrEqual(clientWidth)
}

async function expectChromeBelowBanner(page: Page, locator: ReturnType<Page['locator']>) {
  const bannerBox = await page.getByRole('status').filter({ hasText: 'Modo local' }).boundingBox()
  const chromeBox = await locator.boundingBox()
  expect(bannerBox).not.toBeNull()
  expect(chromeBox).not.toBeNull()
  expect(chromeBox!.y).toBeGreaterThanOrEqual(bannerBox!.y + bannerBox!.height - 1)
}

interface Summary {
  inventory_value: number
  total_sales_today: number
  pending_orders: number
  delivered_orders: number
  pending_payment_sales: number
}

interface TrendPoint {
  date: string
  sales_amount: number
  orders_cost: number
}

interface TopProduct {
  product_id: number
  product_name: string
  product_sku: string
  total_quantity_sold: number
  total_sales_amount: number
}

interface StatusCount {
  status: string
  count: number
  percentage: number
}

test('los agregados del dashboard reflejan una venta y un pedido sembrados', async () => {
  const api = await createApiClient()
  const suffix = uniqueSuffix()
  let saleId: number | undefined
  let orderId: number | undefined
  let productId: number | undefined
  let clientId: number | undefined
  let providerId: number | undefined

  try {
    const beforeSummary = await api.get<Summary>('/dashboard/summary')
    const beforeTrend = await api.get<{ data: TrendPoint[] }>('/dashboard/sales-trend')
    const beforeStatus = await api.get<{ data: StatusCount[] }>('/dashboard/order-status-distribution')
    const today = beforeTrend.data.at(-1)!.date
    const todayBefore = beforeTrend.data.at(-1)!.sales_amount
    const ordersCostTodayBefore = beforeTrend.data.at(-1)!.orders_cost
    const pendingOrdersBefore = beforeStatus.data.find(row => row.status === 'pending')?.count ?? 0

    const catalog = await seedCatalog(api, suffix)
    const product = await seedProduct(api, catalog, {
      name: `Producto dashboard ${suffix}`,
      price: 125.5,
      cost: 40,
      stock_quantity: 20,
    })
    const client = await seedClient(api, suffix)
    const provider = await seedProvider(api, suffix)
    productId = product.id
    clientId = client.id
    providerId = provider.id

    const sale = await api.post<{ id: number }>('/sales', {
      amount_paid: 0,
      client_id: client.id,
      items: [{ product_id: product.id, quantity: 2, delivered_quantity: 2, unit_price: 125.5 }],
    })
    saleId = sale.id
    const order = await api.post<{ id: number }>('/supply-chain/purchase-orders', {
      provider_id: provider.id,
      status: 'pending',
      payment_status: 'pending',
      items: [{ product_id: product.id, quantity: 3, unit_cost: 30 }],
    })
    orderId = order.id

    const afterSummary = await api.get<Summary>('/dashboard/summary')
    const afterTrend = await api.get<{ data: TrendPoint[] }>('/dashboard/sales-trend')
    const afterStatus = await api.get<{ data: StatusCount[] }>('/dashboard/order-status-distribution')
    const topProducts = await api.get<{ products: TopProduct[] }>('/dashboard/top-products', { limit: 100 })

    expect(afterSummary.total_sales_today).toBeGreaterThanOrEqual(beforeSummary.total_sales_today + 251)
    expect(afterSummary.pending_payment_sales).toBeGreaterThanOrEqual(beforeSummary.pending_payment_sales + 251)
    expect(afterSummary.pending_orders).toBeGreaterThanOrEqual(beforeSummary.pending_orders + 1)

    expect(afterTrend.data).toHaveLength(7)
    expect(afterTrend.data.at(-1)!.date).toBe(today)
    expect(afterTrend.data.at(-1)!.sales_amount).toBeGreaterThanOrEqual(todayBefore + 251)
    expect(afterTrend.data.at(-1)!.orders_cost).toBeGreaterThanOrEqual(ordersCostTodayBefore + 90)

    const top = topProducts.products.find(item => item.product_id === product.id)
    expect(top).toBeTruthy()
    expect(top!.total_quantity_sold).toBe(2)
    expect(top!.total_sales_amount).toBe(251)

    const pendingAfter = afterStatus.data.find(row => row.status === 'pending')?.count ?? 0
    expect(pendingAfter).toBeGreaterThanOrEqual(pendingOrdersBefore + 1)
  } finally {
    if (orderId) await api.delete(`/supply-chain/purchase-orders/${orderId}`).catch(() => undefined)
    if (saleId) await api.delete(`/sales/${saleId}`).catch(() => undefined)
    if (productId) await api.delete(`/inventory/products/${productId}`).catch(() => undefined)
    if (clientId) await api.delete(`/sales/clients/${clientId}`).catch(() => undefined)
    if (providerId) await api.delete(`/supply-chain/providers/${providerId}`).catch(() => undefined)
    await api.dispose()
  }
})

test('renderiza los KPIs y gráficos del dashboard en /#/', async ({ page }) => {
  await page.goto('/#/')
  await expect(page.getByRole('heading', { name: 'Dashboard' })).toBeVisible()
  await expect(page.getByText('Ventas de hoy', { exact: true })).toBeVisible()
  await expect(page.getByText('Ventas sin pagar', { exact: true })).toBeVisible()
  await expect(page.getByText('Pedidos pendientes', { exact: true })).toBeVisible()
  await expect(page.getByText('Ventas de los últimos 7 días')).toBeVisible()
  await expect(page.getByText('Top productos')).toBeVisible()
})

test.describe('banner de base de datos local', () => {
  test('desktop: publica su altura y el sidebar fijo queda debajo', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 })
    await mockHealth(page, 'sqlite')
    await page.goto('/#/')
    await expect(page.getByRole('heading', { name: 'Dashboard' })).toBeVisible()

    const banner = page.getByRole('status').filter({ hasText: 'Modo local' })
    await expect(banner).toBeVisible()
    await expectBannerOffsetPublished(page)

    await expectChromeBelowBanner(page, page.locator('#app-sidebar'))
    await expectNoHorizontalOverflow(page)
  })

  test('mobile: publica su altura y el header fijo queda debajo', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await mockHealth(page, 'sqlite')
    await page.goto('/#/')
    await expect(page.getByRole('heading', { name: 'Dashboard' })).toBeVisible()

    const banner = page.getByRole('status').filter({ hasText: 'Modo local' })
    await expect(banner).toBeVisible()
    await expectBannerOffsetPublished(page)

    await expectChromeBelowBanner(page, page.getByRole('banner'))
    await expectNoHorizontalOverflow(page)
  })

  test('postgres: no renderiza el banner ni define la variable', async ({ page }) => {
    await mockHealth(page, 'postgres')
    const healthResponse = page.waitForResponse(
      response => new URL(response.url()).pathname === '/healthz',
    )
    await page.goto('/#/')
    await healthResponse
    await expect(page.getByRole('heading', { name: 'Dashboard' })).toBeVisible()

    await expect(page.getByRole('status').filter({ hasText: 'Modo local' })).toHaveCount(0)
    await expect.poll(() => bannerHeightVariable(page)).toBe('')
  })
})
