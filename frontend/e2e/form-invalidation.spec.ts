import { expect, test } from '@playwright/test'
import {
  bootstrap,
  deleteModal,
  getCalls,
  mockApi,
  order,
  sale,
  salesStats,
} from './support'

test('Venta: eliminar desde el detalle refresca la lista al volver', async ({ page }) => {
  await bootstrap(page)
  let deleted = false
  const calls = await mockApi(page, {
    'GET /api/v1/sales': () => ({ body: { items: deleted ? [] : [sale(1)], has_next: false, skip: 0, limit: 10 } }),
    'GET /api/v1/sales/stats': { body: salesStats },
    'GET /api/v1/sales/1': { body: sale(1) },
    'GET /api/v1/sales/1/items': { body: { items: [], has_next: false, skip: 0, limit: 100 } },
    'DELETE /api/v1/sales/1': () => { deleted = true; return { body: {} } },
  })

  await page.goto('/sales')
  await expect(page.getByText('#1', { exact: true })).toBeVisible()
  const listRequestsBefore = getCalls(calls, '/api/v1/sales').length

  await page.getByTitle('Ver venta').first().click()
  await expect(page).toHaveURL(/\/sales\/1$/)

  await page.getByRole('button', { name: 'Eliminar', exact: true }).click()
  await deleteModal(page).getByRole('button', { name: 'Eliminar', exact: true }).click()

  await expect(page).toHaveURL(/\/sales$/)
  await expect.poll(() => getCalls(calls, '/api/v1/sales').length).toBeGreaterThan(listRequestsBefore)
  await expect(page.getByText('No se encontraron ventas.', { exact: true })).toBeVisible()
})

test('Pedido: eliminar desde el detalle refresca la lista al volver', async ({ page }) => {
  await bootstrap(page)
  let deleted = false
  const calls = await mockApi(page, {
    'GET /api/v1/supply-chain/purchase-orders': () => ({ body: { items: deleted ? [] : [order(1)], has_next: false, skip: 0, limit: 10 } }),
    'GET /api/v1/supply-chain/purchase-orders/1': { body: order(1) },
    'GET /api/v1/supply-chain/purchase-orders/1/items': { body: { items: [], has_next: false, skip: 0, limit: 100 } },
    'DELETE /api/v1/supply-chain/purchase-orders/1': () => { deleted = true; return { body: {} } },
  })

  await page.goto('/orders')
  await expect(page.getByText('Proveedor Central', { exact: true })).toBeVisible()
  const listRequestsBefore = getCalls(calls, '/api/v1/supply-chain/purchase-orders').length

  await page.getByTitle('Ver pedido').first().click()
  await expect(page).toHaveURL(/\/orders\/1$/)

  await page.getByRole('button', { name: 'Eliminar', exact: true }).click()
  await deleteModal(page).getByRole('button', { name: 'Eliminar', exact: true }).click()

  await expect(page).toHaveURL(/\/orders$/)
  await expect.poll(() => getCalls(calls, '/api/v1/supply-chain/purchase-orders').length).toBeGreaterThan(listRequestsBefore)
  await expect(page.getByText('No se encontraron pedidos.', { exact: true })).toBeVisible()
})
