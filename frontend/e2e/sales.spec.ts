import { expect, test } from '@playwright/test'
import {
  AUTH_FILE,
  createApiClient,
  escapeRegExp,
  seedCatalog,
  seedClient,
  seedProduct,
  uniqueSuffix,
  type Product,
  type Sale,
} from './helpers'

test.use({ storageState: AUTH_FILE })

test('crea una venta desde la UI, descuenta stock y al eliminarla lo restaura', async ({ page }) => {
  const api = await createApiClient()
  const suffix = uniqueSuffix()

  try {
    const catalog = await seedCatalog(api, suffix)
    const product = await seedProduct(api, catalog, {
      name: `Taladro venta ${suffix}`,
      price: 125.5,
      cost: 70,
      stock_quantity: 10,
    })
    const client = await seedClient(api, suffix)
    const productPath = `/inventory/products/${product.id}`

    await page.goto('/#/sales/create')
    await expect(page.getByRole('heading', { name: 'Crear venta' })).toBeVisible()

    await page.getByPlaceholder('Buscar producto por nombre o SKU...').fill(suffix)
    await page
      .getByRole('option', { name: new RegExp(escapeRegExp(product.name)) })
      .click()
    await page.locator(`#create-item-${product.id}-quantity`).fill('2')

    await page.getByRole('button', { name: 'Sin cliente' }).click()
    await page.getByPlaceholder('Buscar cliente...').fill(client.name)
    await page.getByRole('option', { name: client.name, exact: true }).click()

    await page.locator('#create-amount-paid').fill('50')
    await page.getByRole('button', { name: 'Guardar Venta' }).click()
    await expect(page).toHaveURL(/#\/sales$/)

    const sales = await api.get<{ items: Sale[] }>('/sales', { client_id: client.id, limit: 10 })
    const sale = sales.items.find(item => item.client_id === client.id)
    expect(sale).toBeTruthy()
    expect(sale!.total_amount).toBe(251)
    expect(sale!.amount_paid).toBe(50)

    await expect.poll(async () => (await api.get<Product>(productPath)).stock_quantity).toBe(8)

    const row = page.locator('tr').filter({ hasText: client.name })
    await expect(row).toBeVisible()
    await row.getByTitle('Eliminar venta').click()
    const dialog = page.getByRole('dialog', { name: 'Eliminar Venta' })
    await expect(dialog).toBeVisible()
    await dialog.getByRole('button', { name: 'Eliminar', exact: true }).click()
    await expect(dialog).toBeHidden()

    await expect.poll(() => api.status('GET', `/sales/${sale!.id}`)).toBe(404)
    await expect.poll(async () => (await api.get<Product>(productPath)).stock_quantity).toBe(10)
  } finally {
    await api.dispose()
  }
})
