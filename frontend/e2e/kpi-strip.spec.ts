import { expect, test } from '@playwright/test'
import { AUTH_FILE, createApiClient, seedCatalog, seedClient, seedProduct, uniqueSuffix } from './helpers'

test.use({ storageState: AUTH_FILE })

/** Big enough to overflow the lead KPI column at 1280px. */
const SEEDED_AMOUNT = 12_345_678

function parseCurrency(text: string) {
  const match = text.replace(/\s/g, '').match(/^\$([\d.]+),(\d{2})$/)
  if (!match) throw new Error(`unexpected currency text: "${text}"`)
  return Number(`${match[1].replace(/\./g, '')}.${match[2]}`)
}

test('los cuatro KPI caben en una línea y el monto grande no se parte', async ({ page }) => {
  const api = await createApiClient()
  const suffix = uniqueSuffix()
  let productId: number | undefined
  let saleId: number | undefined
  let clientId: number | undefined

  try {
    const catalog = await seedCatalog(api, suffix)
    const product = await seedProduct(api, catalog, {
      name: `Producto KPI ${suffix}`,
      price: SEEDED_AMOUNT,
      cost: 1,
      stock_quantity: 5,
    })
    productId = product.id
    const client = await seedClient(api, suffix)
    clientId = client.id

    const sale = await api.post<{ id: number }>('/sales', {
      amount_paid: 0,
      client_id: client.id,
      items: [{ product_id: product.id, quantity: 1, delivered_quantity: 1, unit_price: SEEDED_AMOUNT }],
    })
    saleId = sale.id

    await page.setViewportSize({ width: 1280, height: 720 })
    await page.goto('/#/')
    await expect(page.getByRole('heading', { name: 'Dashboard' })).toBeVisible()

    const cards = page.locator('[data-shape] > [data-kind]')
    await expect(cards).toHaveCount(4)

    // The four cards share one row.
    const boxes = await Promise.all((await cards.all()).map(card => card.boundingBox()))
    expect(boxes.every(box => box !== null)).toBe(true)
    const tops = boxes.map(box => box!.y)
    expect(Math.max(...tops) - Math.min(...tops)).toBeLessThanOrEqual(1)

    // The lead amount stays whole: one line, with the ellipsis contract.
    const leadValue = cards.first().locator('[data-overflow]')
    await expect(leadValue).toHaveAttribute('data-overflow', 'ellipsis')
    const text = (await leadValue.textContent())?.trim() ?? ''
    expect(parseCurrency(text)).toBeGreaterThanOrEqual(SEEDED_AMOUNT)
    await expect(leadValue).toHaveAttribute('title', text)

    const styles = await leadValue.evaluate(element => {
      const style = getComputedStyle(element)
      return {
        whiteSpace: style.whiteSpace,
        textOverflow: style.textOverflow,
        overflow: style.overflow,
        clientHeight: element.clientHeight,
        scrollHeight: element.scrollHeight,
        lineHeight: Number.parseFloat(style.lineHeight),
      }
    })
    expect(styles.whiteSpace).toBe('nowrap')
    expect(styles.textOverflow).toBe('ellipsis')
    expect(styles.overflow).toBe('hidden')
    expect(styles.clientHeight).toBeLessThanOrEqual(styles.lineHeight * 1.5)
    expect(styles.scrollHeight).toBeLessThanOrEqual(styles.clientHeight + styles.lineHeight * 0.5)

    // The count columns are narrower than the money columns.
    const money = await page.locator('[data-kind="money"]').all()
    const counts = await page.locator('[data-kind="count"]').all()
    expect(money).toHaveLength(2)
    expect(counts).toHaveLength(2)
    const moneyWidths = await Promise.all(money.map(card => card.boundingBox().then(box => box!.width)))
    const countWidths = await Promise.all(counts.map(card => card.boundingBox().then(box => box!.width)))
    expect(Math.max(...countWidths)).toBeLessThan(Math.min(...moneyWidths))
  } finally {
    if (saleId) await api.delete(`/sales/${saleId}`).catch(() => undefined)
    if (productId) await api.delete(`/inventory/products/${productId}`).catch(() => undefined)
    if (clientId) await api.delete(`/sales/clients/${clientId}`).catch(() => undefined)
    await api.dispose()
  }
})
