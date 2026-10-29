import { expect, test } from '@playwright/test'
import {
  AUTH_FILE,
  createApiClient,
  escapeRegExp,
  seedCatalog,
  seedProduct,
  seedProvider,
  uniqueSuffix,
  type Order,
  type Product,
} from './helpers'

test.use({ storageState: AUTH_FILE })

test('crea un pedido, lo recibe una sola vez y no revierte stock al eliminarlo', async ({ page }) => {
  const api = await createApiClient()
  const suffix = uniqueSuffix()

  try {
    const catalog = await seedCatalog(api, suffix)
    const product = await seedProduct(api, catalog, {
      name: `Cemento pedido ${suffix}`,
      cost: 30,
      price: 80,
      stock_quantity: 4,
    })
    const provider = await seedProvider(api, suffix)
    const productPath = `/inventory/products/${product.id}`

    await page.goto('/#/orders/create')
    await expect(page.getByRole('heading', { name: 'Crear pedido' })).toBeVisible()

    await page.getByRole('button', { name: /Proveedor:/ }).click()
    await page.getByPlaceholder('Buscar proveedor...').fill(provider.name)
    await page.getByRole('option', { name: provider.name, exact: true }).click()

    await page.getByPlaceholder('Buscar producto por nombre o SKU...').fill(suffix)
    await page
      .getByRole('button', { name: new RegExp(escapeRegExp(product.name)) })
      .click()
    await page.locator('input[type=number]').nth(0).fill('3')
    await page.locator('input[type=number]').nth(1).fill('30')

    await page.getByRole('button', { name: 'Guardar Pedido' }).click()
    await expect(page).toHaveURL(/#\/orders$/)

    const orders = await api.get<{ items: Order[] }>('/supply-chain/purchase-orders', {
      provider_id: provider.id,
      limit: 10,
    })
    const order = orders.items.find(item => item.provider_id === provider.id)
    expect(order).toBeTruthy()
    const orderPath = `/supply-chain/purchase-orders/${order!.id}`

    await expect.poll(async () => (await api.get<Product>(productPath)).stock_quantity).toBe(4)

    await page.goto(`/#/orders/${order!.id}`)
    await expect(page.getByRole('heading', { name: provider.name })).toBeVisible()
    await page.getByRole('button', { name: 'Editar' }).nth(1).click()
    await page.getByRole('button', { name: /Estado: Pendiente/ }).click()
    await page.getByRole('option', { name: 'Recibido' }).click()
    await page.getByRole('button', { name: 'Guardar', exact: true }).click()

    await expect.poll(async () => (await api.get<Order>(orderPath)).status).toBe('received')
    await expect.poll(async () => (await api.get<Product>(productPath)).stock_quantity).toBe(7)

    await api.put(orderPath, { status: 'received' })
    await expect.poll(async () => (await api.get<Product>(productPath)).stock_quantity).toBe(7)

    await page.getByRole('button', { name: 'Eliminar', exact: true }).click()
    const dialog = page.getByRole('dialog', { name: 'Eliminar Pedido' })
    await expect(dialog).toBeVisible()
    await dialog.getByRole('button', { name: 'Eliminar', exact: true }).click()
    await expect(dialog).toBeHidden()

    await expect(page).toHaveURL(/#\/orders$/)
    await expect.poll(() => api.status('GET', orderPath)).toBe(404)
    await expect.poll(async () => (await api.get<Product>(productPath)).stock_quantity).toBe(7)
  } finally {
    await api.dispose()
  }
})
