import { expect, test } from '@playwright/test'
import {
  bootstrap,
  client,
  deleteCalls,
  deleteModal,
  expectModalScrim,
  getCalls,
  inventoryStats,
  mockApi,
  order,
  product,
  provider,
  sale,
  salesStats,
  user,
  type Routes,
} from './support'

interface DeleteCase {
  name: string
  path: string
  action: string
  listPath: string
  deletePath: string
  modalText: RegExp
  rowText: string
  emptyText: string
  createRoutes: () => Routes
}

const cases: DeleteCase[] = [
  {
    name: 'Ventas',
    path: '/sales',
    action: 'Eliminar venta',
    listPath: '/api/v1/sales',
    deletePath: '/api/v1/sales/1',
    modalText: /Venta #1/,
    rowText: '#1',
    emptyText: 'No se encontraron ventas.',
    createRoutes: () => {
      let deleted = false
      return {
        'GET /api/v1/sales/stats': { body: salesStats },
        'GET /api/v1/sales': () => ({ body: { items: deleted ? [] : [sale(1)], has_next: false } }),
        'DELETE /api/v1/sales/1': () => { deleted = true; return { body: {} } },
      }
    },
  },
  {
    name: 'Pedidos',
    path: '/orders',
    action: 'Eliminar pedido',
    listPath: '/api/v1/supply-chain/purchase-orders',
    deletePath: '/api/v1/supply-chain/purchase-orders/1',
    modalText: /Pedido #1/,
    rowText: 'Proveedor Central',
    emptyText: 'No se encontraron pedidos.',
    createRoutes: () => {
      let deleted = false
      return {
        'GET /api/v1/supply-chain/purchase-orders': () => ({ body: { items: deleted ? [] : [order(1)], has_next: false } }),
        'DELETE /api/v1/supply-chain/purchase-orders/1': () => { deleted = true; return { body: {} } },
      }
    },
  },
  {
    name: 'Inventario',
    path: '/inventory',
    action: 'Eliminar',
    listPath: '/api/v1/inventory/products',
    deletePath: '/api/v1/inventory/products/1',
    modalText: /Producto 1 10 cm/,
    rowText: 'Producto 1',
    emptyText: 'No se encontraron productos.',
    createRoutes: () => {
      let deleted = false
      return {
        'GET /api/v1/inventory/products/stats/overview': { body: inventoryStats },
        'GET /api/v1/inventory/products': () => ({ body: { items: deleted ? [] : [product(1)], total: deleted ? 0 : 1, page: 1, size: 10, pages: 1 } }),
        'DELETE /api/v1/inventory/products/1': () => { deleted = true; return { body: {} } },
      }
    },
  },
  {
    name: 'Clientes',
    path: '/clients',
    action: 'Eliminar',
    listPath: '/api/v1/sales/clients',
    deletePath: '/api/v1/sales/clients/1',
    modalText: /Cliente 1/,
    rowText: 'Cliente 1',
    emptyText: 'No se encontraron clientes.',
    createRoutes: () => {
      let deleted = false
      return {
        'GET /api/v1/sales/clients': () => ({ body: { items: deleted ? [] : [client(1)], has_next: false } }),
        'DELETE /api/v1/sales/clients/1': () => { deleted = true; return { body: {} } },
      }
    },
  },
  {
    name: 'Proveedores',
    path: '/providers',
    action: 'Eliminar',
    listPath: '/api/v1/supply-chain/providers',
    deletePath: '/api/v1/supply-chain/providers/1',
    modalText: /Proveedor 1/,
    rowText: 'Proveedor 1',
    emptyText: 'No se encontraron proveedores.',
    createRoutes: () => {
      let deleted = false
      return {
        'GET /api/v1/supply-chain/providers': () => ({ body: { items: deleted ? [] : [provider(1)], total: deleted ? 0 : 1, page: 1, size: 10, pages: 1 } }),
        'DELETE /api/v1/supply-chain/providers/1': () => { deleted = true; return { body: {} } },
      }
    },
  },
  {
    name: 'Usuarios',
    path: '/users',
    action: 'Eliminar usuario',
    listPath: '/api/v1/users',
    deletePath: '/api/v1/users/1',
    modalText: /Usuario 1/,
    rowText: 'Usuario 1',
    emptyText: 'No se encontraron usuarios.',
    createRoutes: () => {
      let deleted = false
      return {
        'GET /api/v1/users': () => ({ body: { items: deleted ? [] : [user(1)], total: deleted ? 0 : 1, page: 1, size: 10, pages: 1 } }),
        'DELETE /api/v1/users/1': () => { deleted = true; return { body: {} } },
      }
    },
  },
]

for (const testCase of cases) {
  test(`${testCase.name}: scrim + modal, cancelar no borra, confirmar borra y refresca`, async ({ page }) => {
    await bootstrap(page)
    const calls = await mockApi(page, testCase.createRoutes())

    await page.goto(testCase.path)
    await expect(page.getByText(testCase.rowText, { exact: true }).first()).toBeVisible()

    await page.getByTitle(testCase.action).click()
    const modal = deleteModal(page)
    await expect(modal).toBeVisible()
    await expect(modal.getByText(testCase.modalText)).toBeVisible()
    await expectModalScrim(page, true)

    const listRequestsBefore = getCalls(calls, testCase.listPath).length

    await modal.getByRole('button', { name: 'Cancelar' }).click()
    await expect(modal).toBeHidden()
    await expectModalScrim(page, false)
    expect(deleteCalls(calls, testCase.deletePath)).toHaveLength(0)
    expect(getCalls(calls, testCase.listPath)).toHaveLength(listRequestsBefore)

    await page.getByTitle(testCase.action).click()
    await expect(modal).toBeVisible()
    await modal.getByRole('button', { name: 'Eliminar', exact: true }).click()

    await expect.poll(() => deleteCalls(calls, testCase.deletePath).length).toBeGreaterThan(0)
    await expect(modal).toBeHidden()
    await expect.poll(() => getCalls(calls, testCase.listPath).length).toBeGreaterThan(listRequestsBefore)
    await expect(page.getByText(testCase.emptyText, { exact: true })).toBeVisible()
    await expectModalScrim(page, false)
  })
}

test('Usuarios: FOREIGN_KEY_VIOLATION muestra el mensaje en español y mantiene el modal', async ({ page }) => {
  await bootstrap(page)
  const calls = await mockApi(page, {
    'GET /api/v1/users': { body: { items: [user(1)], total: 1, page: 1, size: 10, pages: 1 } },
    'DELETE /api/v1/users/1': { status: 400, body: { code: 'FOREIGN_KEY_VIOLATION' } },
  })

  await page.goto('/users')
  await expect(page.getByText('Usuario 1', { exact: true })).toBeVisible()

  await page.getByTitle('Eliminar usuario').click()
  const modal = deleteModal(page)
  await modal.getByRole('button', { name: 'Eliminar', exact: true }).click()

  await expect(modal.getByText('No se puede eliminar el usuario porque está asociado a una venta.')).toBeVisible()
  await expect(modal).toBeVisible()
  await expectModalScrim(page, true)
  expect(getCalls(calls, '/api/v1/users')).toHaveLength(1)
})
