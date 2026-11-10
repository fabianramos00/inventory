import { expect, test, type Locator, type Page } from '@playwright/test'
import {
  AUTH_FILE,
  createApiClient,
  seedCatalog,
  seedClient,
  seedProduct,
  uniqueSuffix,
  type Sale,
} from './helpers'

test.use({ storageState: AUTH_FILE })

interface ClientDebtSale extends Sale {
  payment_status: string
  debt_amount: number
}

function headerDebt(page: Page) {
  return page
    .getByText('Saldo pendiente', { exact: true })
    .locator('xpath=following-sibling::span')
}

function saleRow(page: Page, saleId: number) {
  return page
    .locator('tbody tr')
    .filter({ has: page.getByRole('button', { name: `Ver venta #${saleId}` }) })
}

function clientCard(page: Page, name: string) {
  return page
    .getByRole('link', { name, exact: true })
    .locator('xpath=ancestor::div[.//span[normalize-space()="Saldo pendiente"]][1]')
}

function cardDebt(card: Locator) {
  return card
    .getByText('Saldo pendiente', { exact: true })
    .locator('xpath=following-sibling::span')
}

async function pay(page: Page, amount?: string) {
  const dialog = page.getByRole('dialog', { name: 'Registrar pago' })
  await expect(dialog).toBeVisible()

  if (amount !== undefined) {
    await dialog.getByRole('radio', { name: 'Pago parcial' }).click()
    await dialog.getByLabel('Monto').fill(amount)
  }

  await dialog.getByRole('button', { name: 'Método de pago' }).click()
  // The menu is portaled to the body, so the option is outside the dialog.
  await page.getByRole('option', { name: 'Efectivo' }).click()
  await dialog.getByRole('button', { name: 'Registrar pago' }).click()
  await expect(dialog).toBeHidden()
}

test('paga una venta desde su detalle y refresca el saldo del cliente al volver', async ({ page }) => {
  const api = await createApiClient()
  const suffix = uniqueSuffix()
  let saleId: number | undefined
  let productId: number | undefined
  let clientId: number | undefined

  try {
    const catalog = await seedCatalog(api, suffix)
    const product = await seedProduct(api, catalog, {
      name: `Producto deuda venta ${suffix}`,
      price: 150,
      cost: 60,
      stock_quantity: 5,
    })
    productId = product.id
    const client = await seedClient(api, suffix)
    clientId = client.id

    const sale = await api.post<ClientDebtSale>('/sales', {
      amount_paid: 0,
      client_id: client.id,
      items: [{ product_id: product.id, quantity: 1, delivered_quantity: 0, unit_price: 150 }],
    })
    saleId = sale.id

    // Load the client first so its queries are cached while the sale is paid.
    await page.goto(`/#/clients/${client.id}`)
    await expect(page.getByRole('heading', { name: client.name })).toBeVisible()
    await expect(headerDebt(page)).toHaveText('$ 150,00')

    await page.getByRole('button', { name: `Ver venta #${sale.id}` }).click()
    await expect(page).toHaveURL(new RegExp(`#/sales/${sale.id}$`))

    await page.getByRole('button', { name: /^Pagar$/ }).click()
    const dialog = page.getByRole('dialog', { name: 'Registrar pago' })
    await expect(dialog).toBeVisible()
    await dialog.getByLabel('Monto del pago').fill('50')
    await dialog.getByRole('button', { name: 'Método de pago' }).click()
    await page.getByRole('option', { name: 'Efectivo' }).click()
    await dialog.getByRole('button', { name: 'Registrar pago' }).click()
    await expect(dialog).toBeHidden()
    await expect(page.getByRole('status')).toContainText('Saldo pendiente: $ 100,00.')

    // Browser back keeps the SPA cache: the client header only updates if the
    // sale write invalidated the client list/detail queries.
    await page.goBack()
    await expect(page).toHaveURL(new RegExp(`#/clients/${client.id}$`))
    await expect(page.getByRole('heading', { name: client.name })).toBeVisible()
    await expect(headerDebt(page)).toHaveText('$ 100,00')

    const clientAfterPayment = await api.get<{ debt_amount: number }>(`/sales/clients/${client.id}`)
    expect(clientAfterPayment.debt_amount).toBe(100)
  } finally {
    if (saleId) await api.delete(`/sales/${saleId}`).catch(() => undefined)
    if (productId) await api.delete(`/inventory/products/${productId}`).catch(() => undefined)
    if (clientId) await api.delete(`/sales/clients/${clientId}`).catch(() => undefined)
    await api.dispose()
  }
})

test('paga la deuda de un cliente en parciales y refleja saldo y estado de la venta', async ({ page }) => {
  const api = await createApiClient()
  const suffix = uniqueSuffix()
  let saleId: number | undefined
  let productId: number | undefined
  let clientId: number | undefined

  try {
    const catalog = await seedCatalog(api, suffix)
    const product = await seedProduct(api, catalog, {
      name: `Producto deuda ${suffix}`,
      price: 150,
      cost: 60,
      stock_quantity: 5,
    })
    productId = product.id
    const client = await seedClient(api, suffix)
    clientId = client.id

    const sale = await api.post<ClientDebtSale>('/sales', {
      amount_paid: 0,
      client_id: client.id,
      items: [{ product_id: product.id, quantity: 1, delivered_quantity: 0, unit_price: 150 }],
    })
    saleId = sale.id

    await page.goto(`/#/clients/${client.id}`)
    await expect(page.getByRole('heading', { name: client.name })).toBeVisible()
    await expect(headerDebt(page)).toHaveText('$ 150,00')
    const row = saleRow(page, sale.id)
    await expect(row.locator('.badge')).toHaveText('Pendiente')

    await page.getByRole('button', { name: 'Pagar deuda' }).click()
    await pay(page, '50')

    await expect(page.getByRole('status').filter({ hasText: 'Pago registrado' }))
      .toContainText('Saldo pendiente: $ 100,00.')
    await expect(headerDebt(page)).toHaveText('$ 100,00')
    await expect(row.locator('.badge')).toHaveText('Parcial')

    const afterPartial = await api.get<ClientDebtSale>(`/sales/${sale.id}`)
    expect(afterPartial.amount_paid).toBe(50)
    expect(afterPartial.payment_status).toBe('partial')
    const clientAfterPartial = await api.get<{ debt_amount: number }>(`/sales/clients/${client.id}`)
    expect(clientAfterPartial.debt_amount).toBe(100)

    await page.getByRole('button', { name: 'Pagar deuda' }).click()
    await pay(page)

    await expect(page.getByRole('status').filter({ hasText: 'Pago registrado' }))
      .toContainText('Saldo pendiente: $ 0,00.')
    await expect(page.getByText('Sin deuda', { exact: true })).toBeVisible()
    await expect(page.getByRole('button', { name: 'Pagar deuda' })).toHaveCount(0)
    await expect(row.locator('.badge')).toHaveText('Pagado')

    const afterFull = await api.get<ClientDebtSale>(`/sales/${sale.id}`)
    expect(afterFull.amount_paid).toBe(150)
    expect(afterFull.payment_status).toBe('paid')
    const clientAfterFull = await api.get<{ debt_amount: number }>(`/sales/clients/${client.id}`)
    expect(clientAfterFull.debt_amount).toBe(0)
  } finally {
    if (saleId) await api.delete(`/sales/${saleId}`).catch(() => undefined)
    if (productId) await api.delete(`/inventory/products/${productId}`).catch(() => undefined)
    if (clientId) await api.delete(`/sales/clients/${clientId}`).catch(() => undefined)
    await api.dispose()
  }
})

test('paga desde la tarjeta del cliente en /clients y refresca su saldo pendiente', async ({ page }) => {
  const api = await createApiClient()
  const suffix = uniqueSuffix()
  let saleId: number | undefined
  let productId: number | undefined
  let clientId: number | undefined

  try {
    const catalog = await seedCatalog(api, suffix)
    const product = await seedProduct(api, catalog, {
      name: `Producto deuda tarjeta ${suffix}`,
      price: 150,
      cost: 60,
      stock_quantity: 5,
    })
    productId = product.id
    const client = await seedClient(api, suffix)
    clientId = client.id

    const sale = await api.post<ClientDebtSale>('/sales', {
      amount_paid: 0,
      client_id: client.id,
      items: [{ product_id: product.id, quantity: 1, delivered_quantity: 0, unit_price: 150 }],
    })
    saleId = sale.id

    await page.goto('/#/clients')
    const search = page.getByPlaceholder('Buscar cliente por nombre, cédula o contacto...')
    await expect(search).toBeVisible()
    await search.fill(client.name)

    const card = clientCard(page, client.name)
    await expect(card).toBeVisible()
    await expect(cardDebt(card)).toHaveText('$ 150,00')

    await card.getByRole('button', { name: `Pagar deuda de ${client.name}` }).click()
    await pay(page, '50')

    await expect(page.getByRole('status').filter({ hasText: 'Pago registrado' }))
      .toContainText('Saldo pendiente: $ 100,00.')
    await expect(cardDebt(card)).toHaveText('$ 100,00')

    const clientAfterPayment = await api.get<{ debt_amount: number }>(`/sales/clients/${client.id}`)
    expect(clientAfterPayment.debt_amount).toBe(100)
  } finally {
    if (saleId) await api.delete(`/sales/${saleId}`).catch(() => undefined)
    if (productId) await api.delete(`/inventory/products/${productId}`).catch(() => undefined)
    if (clientId) await api.delete(`/sales/clients/${clientId}`).catch(() => undefined)
    await api.dispose()
  }
})
