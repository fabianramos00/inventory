import { expect, type Page } from '@playwright/test'

export interface Reply {
  status?: number
  body?: unknown
}

export type RouteHandler = (url: URL) => Reply
export type Routes = Record<string, Reply | RouteHandler>

export interface ApiCall {
  method: string
  url: URL
  at: number
}

export async function bootstrap(page: Page) {
  await page.addInitScript(() => {
    localStorage.setItem('token', 'e2e-access-token')
    localStorage.setItem('refresh_token', 'e2e-refresh-token')
    localStorage.setItem('user', JSON.stringify({ full_name: 'Admin E2E', is_superuser: true }))
  })
}

export async function mockApi(page: Page, routes: Routes) {
  const calls: ApiCall[] = []

  await page.route(/\/api\/v1\//, async route => {
    const request = route.request()
    const url = new URL(request.url())
    calls.push({ method: request.method(), url, at: Date.now() })

    const key = `${request.method()} ${url.pathname}`
    const entry = routes[key] ?? routes[url.pathname]
    if (!entry) {
      await route.fulfill({
        status: 404,
        contentType: 'application/json',
        body: JSON.stringify({ detail: `Unmocked request: ${key}` }),
      })
      return
    }

    const reply = typeof entry === 'function' ? entry(url) : entry
    await route.fulfill({
      status: reply.status ?? 200,
      contentType: 'application/json',
      body: JSON.stringify(reply.body ?? {}),
    })
  })

  return calls
}

export function getCalls(calls: ApiCall[], pathname: string) {
  return calls.filter(call => call.method === 'GET' && call.url.pathname === pathname)
}

export function deleteCalls(calls: ApiCall[], pathname: string) {
  return calls.filter(call => call.method === 'DELETE' && call.url.pathname === pathname)
}

export async function expectModalScrim(page: Page, visible: boolean) {
  const scrim = page.locator('[data-scrim]')
  if (visible) {
    await expect(page.getByRole('dialog')).toBeVisible()
    await expect(scrim).toBeVisible()
  } else {
    await expect(scrim).toHaveCount(0)
  }
}

export function deleteModal(page: Page) {
  return page.getByRole('dialog')
}

export const client = (id: number, name = `Cliente ${id}`) => ({
  id,
  name,
  identity_card: `CC-${id}`,
  email: `cliente${id}@test.pe`,
  phone: `300-000-${id}`,
  created_at: '2026-01-15T10:00:00',
  updated_at: null,
})

export const provider = (id: number, name = `Proveedor ${id}`) => ({
  id,
  name,
  contact_info: `Contacto ${id}`,
  email: `proveedor${id}@test.pe`,
  phone: `310-000-${id}`,
})

export const user = (id: number, fullName = `Usuario ${id}`) => ({
  id,
  full_name: fullName,
  email: `usuario${id}@test.pe`,
  is_superuser: id === 2,
  is_active: true,
})

export const sale = (id = 1) => ({
  id,
  total_amount: 125.5,
  debt_amount: 25.5,
  payment_status: 'partial',
  delivery_status: 'pending',
  payment_method: 'cash',
  amount_paid: 100,
  client_id: 3,
  created_at: '2026-02-10T09:00:00',
  updated_at: null,
  created_by_id: 2,
  created_by: { id: 2, full_name: 'Ana Torres', email: 'ana@test.pe', is_active: true, is_superuser: true },
  client: { id: 3, name: 'Cliente Uno' },
})

export const order = (id = 1) => ({
  id,
  provider_id: 4,
  status: 'received',
  payment_status: 'paid',
  order_date: '2026-02-12T09:00:00',
  total_amount: 480,
  provider: { id: 4, name: 'Proveedor Central', contact_info: null, email: null, phone: null },
})

export const product = (id = 1) => ({
  id,
  name: `Producto ${id}`,
  sku: `SKU-${id}`,
  category: { id: 1, code: 'HER', name: 'Herramientas' },
  stock: 4,
  min_stock: 5,
  unit: 'und',
  price: 19.9,
  stock_quantity: 4,
  low_stock_threshold: 5,
  size_value: '10',
  measurement_unit: { id: 1, name: 'Centímetros', abbreviation: 'cm' },
  material: { id: 1, name: 'Acero' },
  brand: { id: 1, name: 'Truper' },
})

export const salesStats = {
  unpaid_count: 1,
  undelivered_count: 2,
  total_amount_sum: 125.5,
  amount_paid_sum: 100,
}

export const inventoryStats = {
  total_products: 12,
  total_stock: 40,
  low_stock_count: 2,
  total_inventory_value: 950.4,
}
