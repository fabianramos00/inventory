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
    await page.getByRole('button', { name: 'Método de pago' }).click()
    await page.getByRole('option', { name: 'Efectivo' }).click()
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

test('registra, lista y elimina pagos desde el detalle de la venta', async ({ page }) => {
  const api = await createApiClient()
  const suffix = uniqueSuffix()

  try {
    const catalog = await seedCatalog(api, suffix)
    const product = await seedProduct(api, catalog, {
      name: `Taladro pagos ${suffix}`,
      price: 50,
      cost: 30,
      stock_quantity: 5,
    })
    const client = await seedClient(api, suffix)
    const sale = await api.post<Sale>('/sales', {
      amount_paid: 0,
      client_id: client.id,
      items: [{ product_id: product.id, quantity: 1, delivered_quantity: 0, unit_price: 50 }],
    })

    await page.goto(`/#/sales/${sale.id}`)
    await expect(page.getByRole('heading', { name: client.name })).toBeVisible()
    await expect(page.getByText('Sin pagos registrados')).toBeVisible()

    await page.getByRole('button', { name: /^Pagar$/ }).click()
    const dialog = page.getByRole('dialog', { name: 'Registrar pago' })
    await expect(dialog).toBeVisible()
    await dialog.getByLabel('Monto del pago').fill('20')
    await dialog.getByRole('button', { name: 'Método de pago' }).click()
    // The menu is portaled to the body, so the option is outside the dialog.
    await page.getByRole('option', { name: 'Efectivo' }).click()
    await dialog.getByRole('button', { name: 'Registrar pago' }).click()
    await expect(dialog).toBeHidden()

    await expect(page.getByRole('status')).toContainText('Pago de $ 20,00 registrado. Saldo pendiente: $ 30,00.')
    await expect(page.getByRole('cell', { name: 'Efectivo' })).toBeVisible()
    await expect(page.getByRole('cell', { name: 'Admin User' })).toBeVisible()
    await expect(page.getByRole('cell', { name: '$ 20,00', exact: true })).toBeVisible()

    const afterPayment = await api.get<Sale>(`/sales/${sale.id}`)
    expect(afterPayment.amount_paid).toBe(20)
    const payments = await api.get<{ items: { amount: number; payment_method: string }[] }>(
      `/sales/${sale.id}/payments`,
    )
    expect(payments.items).toHaveLength(1)
    expect(payments.items[0].amount).toBe(20)
    expect(payments.items[0].payment_method).toBe('cash')

    await page.getByRole('button', { name: /Eliminar pago de/ }).click()
    const deleteDialog = page.getByRole('dialog', { name: 'Eliminar pago' })
    await expect(deleteDialog).toBeVisible()
    await expect(deleteDialog).toContainText('Saldo resultante: $ 50,00')
    await deleteDialog.getByRole('button', { name: 'Eliminar', exact: true }).click()
    await expect(deleteDialog).toBeHidden()

    await expect(page.getByRole('status')).toContainText('Pago eliminado. Saldo pendiente: $ 50,00.')
    await expect(page.getByText('Sin pagos registrados')).toBeVisible()

    const afterDelete = await api.get<Sale>(`/sales/${sale.id}`)
    expect(afterDelete.amount_paid).toBe(0)
    const remaining = await api.get<{ items: unknown[] }>(`/sales/${sale.id}/payments`)
    expect(remaining.items).toHaveLength(0)
  } finally {
    await api.dispose()
  }
})
