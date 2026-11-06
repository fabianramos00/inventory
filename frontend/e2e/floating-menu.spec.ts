import { expect, test, type Page } from '@playwright/test'
import { AUTH_FILE, createApiClient, seedCatalog, seedProduct, uniqueSuffix } from './helpers'

test.use({ storageState: AUTH_FILE })

/**
 * The web fonts swap in after the first paint and shift the page by a couple
 * of pixels. Wait for them so the height comparison only measures the menu.
 */
async function settleLayout(page: Page) {
  await page.waitForLoadState('networkidle')
  await page.evaluate(() => document.fonts.ready)
}

function scrollHeight(page: Page) {
  return page.evaluate(() => document.scrollingElement?.scrollHeight ?? 0)
}

test.describe('Menú flotante (full stack)', () => {
  test('no agranda el documento y cabe en la ventana', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 720 })
    await page.goto('/#/inventory/create')
    await expect(page.getByRole('heading', { name: 'Registro de producto' })).toBeVisible()
    await settleLayout(page)

    const before = await scrollHeight(page)

    // Last attribute dropdown of the form: "Unidad de medida".
    await page.getByRole('button', { name: 'Seleccionar', exact: true }).last().click()

    const menu = page.locator('[data-floating-menu]')
    await expect(menu).toBeVisible()
    await expect(page.locator('body > [data-floating-menu]')).toHaveCount(1)

    expect(await scrollHeight(page)).toBe(before)

    const box = await menu.boundingBox()
    expect(box).not.toBeNull()
    expect(box!.y).toBeGreaterThanOrEqual(0)
    expect(box!.y + box!.height).toBeLessThanOrEqual(720)
    expect(box!.x).toBeGreaterThanOrEqual(0)
    expect(box!.x + box!.width).toBeLessThanOrEqual(1280)
  })

  test('la lista de resultados de productos de una venta no agranda el documento', async ({ page }) => {
    const api = await createApiClient()
    const suffix = uniqueSuffix()

    try {
      const catalog = await seedCatalog(api, suffix)
      await seedProduct(api, catalog, {
        name: `Producto flotante ${suffix}`,
        stock_quantity: 5,
      })

      await page.setViewportSize({ width: 1280, height: 720 })
      await page.goto('/#/sales/create')
      await expect(page.getByRole('heading', { name: 'Crear venta' })).toBeVisible()
      await settleLayout(page)

      const before = await scrollHeight(page)

      await page.getByPlaceholder('Buscar producto por nombre o SKU...').fill(suffix)

      const listbox = page.locator('#product-search-listbox')
      await expect(listbox).toBeVisible()
      await expect(listbox.getByRole('option').first()).toBeVisible()
      // The results are portaled to the body, so they stay out of the form flow.
      await expect(page.locator('body > [data-floating-menu]')).toHaveCount(1)

      expect(await scrollHeight(page)).toBe(before)

      const box = await listbox.boundingBox()
      expect(box).not.toBeNull()
      expect(box!.y).toBeGreaterThanOrEqual(0)
      expect(box!.y + box!.height).toBeLessThanOrEqual(720)
      expect(box!.x).toBeGreaterThanOrEqual(0)
      expect(box!.x + box!.width).toBeLessThanOrEqual(1280)
    } finally {
      await api.dispose()
    }
  })
})
