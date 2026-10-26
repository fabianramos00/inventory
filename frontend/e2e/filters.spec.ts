import { expect, test } from '@playwright/test'
import {
  bootstrap,
  client,
  getCalls,
  mockApi,
  order,
  provider,
  sale,
  salesStats,
  user,
  type ApiCall,
  type Routes,
} from './support'

function lastParams(calls: ApiCall[], pathname: string) {
  const last = getCalls(calls, pathname).at(-1)
  return last ? Object.fromEntries(last.url.searchParams.entries()) : {}
}

const salesPath = '/api/v1/sales'

test('Ventas: los filtros envían los parámetros esperados y "Sin cliente" usa no_client', async ({ page }) => {
  await bootstrap(page)
  const routes: Routes = {
    'GET /api/v1/sales/stats': { body: salesStats },
    'GET /api/v1/sales': { body: { items: [sale(1)], has_next: false } },
    'GET /api/v1/users': { body: { items: [user(2, 'Ana Torres')], total: 1, page: 1, size: 10, pages: 1 } },
    'GET /api/v1/sales/clients': { body: { items: [client(3, 'Cliente Uno')], has_next: false } },
  }
  const calls = await mockApi(page, routes)
  await page.goto('/sales')
  await expect.poll(() => getCalls(calls, salesPath).length).toBeGreaterThan(0)

  await page.getByRole('button', { name: 'Usuario', exact: true }).click()
  await page.getByRole('option', { name: 'Ana Torres' }).click()
  await expect.poll(() => lastParams(calls, salesPath).user_id).toBe('2')

  await page.getByRole('button', { name: 'Cliente', exact: true }).click()
  await page.getByRole('option', { name: 'Cliente Uno' }).click()
  await expect.poll(() => lastParams(calls, salesPath).client_id).toBe('3')

  await page.getByRole('button', { name: 'Cliente Uno', exact: true }).click()
  await page.getByRole('option', { name: 'Sin cliente' }).click()
  await expect.poll(() => lastParams(calls, salesPath).no_client).toBe('true')

  await page.getByRole('button', { name: 'Método pago', exact: true }).click()
  await page.getByRole('option', { name: 'Crédito' }).click()
  await expect.poll(() => lastParams(calls, salesPath).payment_method).toBe('credit')

  await page.getByRole('button', { name: 'Entrega', exact: true }).click()
  await page.getByRole('option', { name: 'Entregado' }).click()
  await expect.poll(() => lastParams(calls, salesPath).delivery_status).toBe('delivered')

  await page.getByRole('button', { name: 'Estado pago', exact: true }).click()
  await page.getByRole('option', { name: 'Parcial' }).click()
  await expect.poll(() => lastParams(calls, salesPath).payment_status).toBe('partial')

  await page.getByTitle('Fecha inicio').fill('2026-01-01')
  await expect.poll(() => lastParams(calls, salesPath).start_date).toBe('2026-01-01')

  await page.getByTitle('Fecha fin').fill('2026-02-01')
  await expect.poll(() => lastParams(calls, salesPath).end_date).toBe('2026-02-01')

  const finalParams = lastParams(calls, salesPath)
  expect(finalParams).toMatchObject({
    skip: '0',
    limit: '10',
    user_id: '2',
    no_client: 'true',
    payment_method: 'credit',
    delivery_status: 'delivered',
    payment_status: 'partial',
    start_date: '2026-01-01',
    end_date: '2026-02-01',
  })
  expect(finalParams.client_id).toBeUndefined()
})

test('Pedidos: proveedor, estados y rango de fechas envían los parámetros esperados', async ({ page }) => {
  await bootstrap(page)
  const routes: Routes = {
    'GET /api/v1/supply-chain/purchase-orders': { body: { items: [order(1)], has_next: false } },
    'GET /api/v1/supply-chain/providers': { body: { items: [provider(4, 'Proveedor Central')], total: 1, page: 1, size: 10, pages: 1 } },
  }
  const calls = await mockApi(page, routes)
  await page.goto('/orders')
  await expect.poll(() => getCalls(calls, '/api/v1/supply-chain/purchase-orders').length).toBeGreaterThan(0)

  await page.getByRole('button', { name: 'Proveedor', exact: true }).click()
  await page.getByRole('option', { name: 'Proveedor Central' }).click()
  await expect.poll(() => lastParams(calls, '/api/v1/supply-chain/purchase-orders').provider_id).toBe('4')

  await page.getByRole('button', { name: 'Estado pago', exact: true }).click()
  await page.getByRole('option', { name: 'Pagado' }).click()
  await expect.poll(() => lastParams(calls, '/api/v1/supply-chain/purchase-orders').payment_status).toBe('paid')

  await page.getByRole('button', { name: 'Estado', exact: true }).click()
  await page.getByRole('option', { name: 'Recibido' }).click()
  await expect.poll(() => lastParams(calls, '/api/v1/supply-chain/purchase-orders').status).toBe('received')

  await page.getByTitle('Fecha inicio').fill('2026-01-01')
  await expect.poll(() => lastParams(calls, '/api/v1/supply-chain/purchase-orders').min_date).toBe('2026-01-01')

  await page.getByTitle('Fecha fin').fill('2026-03-01')
  await expect.poll(() => lastParams(calls, '/api/v1/supply-chain/purchase-orders').max_date).toBe('2026-03-01')

  expect(lastParams(calls, '/api/v1/supply-chain/purchase-orders')).toMatchObject({
    skip: '0',
    limit: '10',
    provider_id: '4',
    payment_status: 'paid',
    status: 'received',
    min_date: '2026-01-01',
    max_date: '2026-03-01',
  })
})
