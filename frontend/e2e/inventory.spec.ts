import { expect, test, type Page } from '@playwright/test'
import {
  AUTH_FILE,
  createApiClient,
  seedCatalog,
  seedProduct,
  uniqueSuffix,
  type Product,
} from './helpers'

test.use({ storageState: AUTH_FILE })

async function selectProductOption(page: Page, field: string, option: string) {
  const row = page.locator('dt').filter({ hasText: new RegExp(`^${field}$`) }).first().locator('..')
  await row.getByRole('button', { name: 'Seleccionar' }).click()
  await page.getByPlaceholder(`Buscar ${field.toLowerCase()}...`).fill(option)
  await page.getByRole('button', { name: option, exact: true }).click()
}

async function fillNumberField(page: Page, index: number, value: string) {
  await page.getByRole('spinbutton').nth(index).fill(value)
}

test.describe('Inventario (full stack)', () => {
  test('lista y busca un producto creado por la API', async ({ page }) => {
    const api = await createApiClient()
    try {
      const suffix = uniqueSuffix()
      const catalog = await seedCatalog(api, suffix)
      const product = await seedProduct(api, catalog, {
        name: `Martillo E2E ${suffix}`,
        sku: `MART-${suffix}`,
        stock_quantity: 7,
      })

      await page.goto('/#/inventory')
      await expect(page.getByRole('heading', { name: 'Inventario Principal' })).toBeVisible()

      await page.getByPlaceholder('Nombre o ID...').fill(suffix)
      const row = page.locator('tr').filter({ hasText: product.name })
      await expect(row).toBeVisible()
      await expect(row).toContainText(product.sku)
      await expect(row).toContainText(catalog.category.name)
      await expect(row).toContainText('7')
    } finally {
      await api.dispose()
    }
  })

  test('crea un producto desde la UI y persiste los valores en la API', async ({ page }) => {
    const api = await createApiClient()
    const suffix = uniqueSuffix()
    const name = `Producto UI ${suffix}`
    let productId: number | undefined

    try {
      const catalog = await seedCatalog(api, suffix)

      await page.goto('/#/inventory/create')
      await expect(page.getByRole('heading', { name: 'Registro de producto' })).toBeVisible()
      await page.getByPlaceholder('Nombre del producto').fill(name)

      await selectProductOption(page, 'Categoría', catalog.category.name)
      await selectProductOption(page, 'Marca', catalog.brand.name)
      await selectProductOption(page, 'Material', catalog.material.name)

      await fillNumberField(page, 0, '125.50')
      await fillNumberField(page, 1, '80.25')
      await fillNumberField(page, 2, '6')
      await fillNumberField(page, 3, '2')

      await page.getByRole('button', { name: 'Guardar Producto' }).click()
      await expect(page).toHaveURL(/#\/inventory$/)
      await expect(page.getByRole('heading', { name: 'Inventario Principal' })).toBeVisible()

      await page.getByPlaceholder('Nombre o ID...').fill(name)
      const row = page.locator('tr').filter({ hasText: name })
      await expect(row).toBeVisible()
      await row.getByRole('button', { name, exact: true }).click()

      await expect(page).toHaveURL(/#\/inventory\/product\/\d+$/)
      productId = Number(page.url().split('/').pop())
      const created = await api.get<Product>(`/inventory/products/${productId}`)

      expect(created.name).toBe(name)
      expect(created.sku).toMatch(/^SKU-[A-Z0-9]{8}$/)
      expect(created.price).toBe(125.5)
      expect(created.cost).toBe(80.25)
      expect(created.stock_quantity).toBe(6)
      expect(created.low_stock_threshold).toBe(2)
      expect(created.category?.id).toBe(catalog.category.id)
      expect(created.brand?.id).toBe(catalog.brand.id)
      expect(created.material?.id).toBe(catalog.material.id)
    } finally {
      if (productId) await api.delete(`/inventory/products/${productId}`).catch(() => undefined)
      await api.dispose()
    }
  })

  test('edita y elimina un producto desde la UI con verificación en la API', async ({ page }) => {
    const api = await createApiClient()
    const suffix = uniqueSuffix()
    const editedName = `Taladro editado ${suffix}`

    try {
      const catalog = await seedCatalog(api, suffix)
      const product = await seedProduct(api, catalog, {
        name: `Taladro E2E ${suffix}`,
        stock_quantity: 8,
      })
      const productPath = `/inventory/products/${product.id}`

      await page.goto('/#/inventory')
      await page.getByPlaceholder('Nombre o ID...').fill(suffix)
      const row = page.locator('tr').filter({ hasText: product.name })
      await expect(row).toBeVisible()
      await row.getByRole('button', { name: product.name, exact: true }).click()

      await expect(page.getByRole('heading', { name: product.name })).toBeVisible()
      await page.getByRole('button', { name: 'Editar' }).click()
      await page.getByPlaceholder('Nombre del producto').fill(editedName)
      await fillNumberField(page, 2, '3')
      await page.getByRole('button', { name: 'Guardar', exact: true }).click()

      await expect(page.getByRole('heading', { name: editedName })).toBeVisible()
      await expect.poll(async () => (await api.get<Product>(productPath)).name).toBe(editedName)
      await expect.poll(async () => (await api.get<Product>(productPath)).stock_quantity).toBe(3)

      await page.getByRole('button', { name: 'Eliminar', exact: true }).click()
      const dialog = page.getByRole('dialog', { name: 'Eliminar Producto' })
      await expect(dialog).toBeVisible()
      await dialog.getByRole('button', { name: 'Eliminar', exact: true }).click()
      await expect(dialog).toBeHidden()

      await expect(page).toHaveURL(/#\/inventory$/)
      await expect.poll(() => api.status('GET', productPath)).toBe(404)
    } finally {
      await api.dispose()
    }
  })
})
