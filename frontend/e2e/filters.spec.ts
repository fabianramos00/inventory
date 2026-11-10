import { expect, test, type Page } from '@playwright/test'
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

// The field is a segmented DD/MM/YYYY control; only the calendar button opens
// the popover. The month/year dropdowns close the whole popover when an option
// is picked (filed as a product defect), so navigate with the month buttons
// instead.
async function pickDate(page: Page, label: string, iso: string) {
  await page
    .getByRole('button', { name: new RegExp(`^Seleccionar ${label.toLowerCase()}`) })
    .click()

  const dialog = page.getByRole('dialog', { name: label })
  await expect(dialog).toBeVisible()

  const target = new Date(`${iso}T00:00:00`)
  const today = new Date()
  const monthDelta =
    (today.getFullYear() - target.getFullYear()) * 12 + (today.getMonth() - target.getMonth())
  const nav = monthDelta >= 0 ? 'Ir al mes anterior' : 'Ir al mes siguiente'
  for (let step = 0; step < Math.abs(monthDelta); step += 1) {
    await dialog.getByRole('button', { name: nav }).click()
  }

  await dialog.locator(`[data-day="${iso}"] button`).click()
  await expect(dialog).toBeHidden()
}

const salesPath = '/api/v1/sales'

function salesRoutes(): Routes {
  return {
    'GET /api/v1/sales/stats': { body: salesStats },
    'GET /api/v1/sales': { body: { items: [sale(1)], has_next: false } },
    'GET /api/v1/users/options': { body: { items: [user(2, 'Ana Torres')], has_next: false, skip: 0, limit: 20 } },
    'GET /api/v1/sales/clients': { body: { items: [client(3, 'Cliente Uno')], has_next: false } },
  }
}

test('Ventas: el campo de fecha es editable y solo el botón abre el calendario', async ({ page }) => {
  await bootstrap(page)
  await mockApi(page, salesRoutes())
  await page.goto('/#/sales')

  const field = page.getByRole('textbox', { name: 'Fecha inicio día' })
  await expect(field).toBeEditable()
  await expect(field).toHaveValue('')

  await field.click()
  await expect(page.getByRole('dialog', { name: 'Fecha inicio' })).toBeHidden()

  await field.focus()
  await expect(page.getByRole('dialog', { name: 'Fecha inicio' })).toBeHidden()

  await page.getByRole('button', { name: 'Seleccionar fecha inicio' }).click()
  await expect(page.getByRole('dialog', { name: 'Fecha inicio' })).toBeVisible()
})

test('Ventas: escribir DD/MM/YYYY aplica el filtro y borrar el campo lo limpia', async ({ page }) => {
  await bootstrap(page)
  const calls = await mockApi(page, salesRoutes())
  await page.goto('/#/sales')
  await expect.poll(() => getCalls(calls, salesPath).length).toBeGreaterThan(0)

  const day = page.getByRole('textbox', { name: 'Fecha inicio día' })
  const month = page.getByRole('textbox', { name: 'Fecha inicio mes' })
  const year = page.getByRole('textbox', { name: 'Fecha inicio año' })
  await day.fill('15')
  await month.fill('01')
  await year.fill('2026')
  await expect(day).toHaveValue('15')
  await expect(month).toHaveValue('01')
  await expect(year).toHaveValue('2026')
  await expect(page.getByRole('button', { name: 'Limpiar filtros' })).toBeVisible()
  await expect.poll(() => lastParams(calls, salesPath).start_date).toBe('2026-01-15T05:00:00.000Z')

  await day.fill('')
  await expect(day).toHaveValue('')
  await expect(page.getByRole('button', { name: 'Limpiar filtros' })).toBeHidden()

  // Clearing returns the query key to the cached empty-filter key, so force a
  // fresh request with another filter and confirm start_date is gone.
  await page.getByRole('button', { name: 'Método pago', exact: true }).click()
  await page.getByRole('option', { name: 'Crédito' }).click()
  await expect.poll(() => lastParams(calls, salesPath).payment_method).toBe('credit')
  expect(lastParams(calls, salesPath).start_date).toBeUndefined()
})

test('Ventas: seleccionar un día en el calendario actualiza el campo', async ({ page }) => {
  await bootstrap(page)
  const calls = await mockApi(page, salesRoutes())
  await page.goto('/#/sales')
  await expect.poll(() => getCalls(calls, salesPath).length).toBeGreaterThan(0)

  await pickDate(page, 'Fecha inicio', '2026-01-01')

  await expect(page.getByRole('textbox', { name: 'Fecha inicio día' })).toHaveValue('01')
  await expect(page.getByRole('textbox', { name: 'Fecha inicio mes' })).toHaveValue('01')
  await expect(page.getByRole('textbox', { name: 'Fecha inicio año' })).toHaveValue('2026')
  await expect.poll(() => lastParams(calls, salesPath).start_date).toBe('2026-01-01T05:00:00.000Z')
})

test('Ventas: los filtros envían los parámetros esperados y "Sin cliente" usa no_client', async ({ page }) => {
  await bootstrap(page)
  const calls = await mockApi(page, salesRoutes())
  await page.goto('/#/sales')
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

  // Date filters travel as Bogotá day bounds (UTC), matching lib/dateBounds.ts.
  await pickDate(page, 'Fecha inicio', '2026-01-01')
  await expect(page.getByRole('textbox', { name: 'Fecha inicio día' })).toHaveValue('01')
  await expect(page.getByRole('textbox', { name: 'Fecha inicio mes' })).toHaveValue('01')
  await expect(page.getByRole('textbox', { name: 'Fecha inicio año' })).toHaveValue('2026')
  await expect.poll(() => lastParams(calls, salesPath).start_date).toBe('2026-01-01T05:00:00.000Z')

  await pickDate(page, 'Fecha fin', '2026-02-01')
  await expect(page.getByRole('textbox', { name: 'Fecha fin día' })).toHaveValue('01')
  await expect(page.getByRole('textbox', { name: 'Fecha fin mes' })).toHaveValue('02')
  await expect(page.getByRole('textbox', { name: 'Fecha fin año' })).toHaveValue('2026')
  await expect.poll(() => lastParams(calls, salesPath).end_date).toBe('2026-02-02T04:59:59.999Z')

  const finalParams = lastParams(calls, salesPath)
  expect(finalParams).toMatchObject({
    skip: '0',
    limit: '10',
    user_id: '2',
    no_client: 'true',
    payment_method: 'credit',
    delivery_status: 'delivered',
    payment_status: 'partial',
    start_date: '2026-01-01T05:00:00.000Z',
    end_date: '2026-02-02T04:59:59.999Z',
  })
  expect(finalParams.client_id).toBeUndefined()
  await expect(page.getByRole('table').getByText('Cliente Uno')).toBeVisible()
})

test('Pedidos: proveedor, estados y rango de fechas envían los parámetros esperados', async ({ page }) => {
  await bootstrap(page)
  const routes: Routes = {
    'GET /api/v1/supply-chain/purchase-orders': { body: { items: [order(1)], has_next: false } },
    'GET /api/v1/supply-chain/providers': { body: { items: [provider(4, 'Proveedor Central')], total: 1, page: 1, size: 10, pages: 1 } },
  }
  const calls = await mockApi(page, routes)
  await page.goto('/#/orders')
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

  await pickDate(page, 'Fecha inicio', '2026-01-01')
  await expect(page.getByRole('textbox', { name: 'Fecha inicio día' })).toHaveValue('01')
  await expect(page.getByRole('textbox', { name: 'Fecha inicio mes' })).toHaveValue('01')
  await expect(page.getByRole('textbox', { name: 'Fecha inicio año' })).toHaveValue('2026')
  await expect.poll(() => lastParams(calls, '/api/v1/supply-chain/purchase-orders').min_date).toBe('2026-01-01T05:00:00.000Z')

  await pickDate(page, 'Fecha fin', '2026-03-01')
  await expect(page.getByRole('textbox', { name: 'Fecha fin día' })).toHaveValue('01')
  await expect(page.getByRole('textbox', { name: 'Fecha fin mes' })).toHaveValue('03')
  await expect(page.getByRole('textbox', { name: 'Fecha fin año' })).toHaveValue('2026')
  await expect.poll(() => lastParams(calls, '/api/v1/supply-chain/purchase-orders').max_date).toBe('2026-03-02T04:59:59.999Z')

  expect(lastParams(calls, '/api/v1/supply-chain/purchase-orders')).toMatchObject({
    skip: '0',
    limit: '10',
    provider_id: '4',
    payment_status: 'paid',
    status: 'received',
    min_date: '2026-01-01T05:00:00.000Z',
    max_date: '2026-03-02T04:59:59.999Z',
  })
  await expect(page.getByRole('table').getByText('Proveedor Central')).toBeVisible()
})
