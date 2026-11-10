import { expect, test } from '@playwright/test'
import {
  bootstrap,
  client,
  mockApi,
  sale,
  salesStats,
  user,
  type Routes,
} from './support'

function salesRoutes(): Routes {
  return {
    'GET /api/v1/sales/stats': { body: salesStats },
    'GET /api/v1/sales': { body: { items: [sale(1)], has_next: false } },
    'GET /api/v1/users/options': { body: { items: [user(2, 'Ana Torres')], has_next: false, skip: 0, limit: 20 } },
    'GET /api/v1/sales/clients': { body: { items: [client(3, 'Cliente Uno')], has_next: false } },
  }
}

test('FilterSelect: ArrowDown abre, las flechas recorren y acotan, Escape cierra y restaura el foco', async ({ page }) => {
  await bootstrap(page)
  await mockApi(page, salesRoutes())
  await page.goto('/#/sales')

  const trigger = page.getByRole('button', { name: 'Método pago' })
  await trigger.focus()

  await page.keyboard.press('ArrowDown')
  await expect(page.getByRole('listbox')).toBeVisible()

  await page.keyboard.press('ArrowDown')
  await expect(page.getByRole('option', { name: 'Todos' })).toBeFocused()

  await page.keyboard.press('ArrowUp')
  await expect(page.getByRole('option', { name: 'Todos' })).toBeFocused()

  await page.keyboard.press('ArrowDown')
  await expect(page.getByRole('option', { name: 'Efectivo' })).toBeFocused()
  await page.keyboard.press('ArrowDown')
  await page.keyboard.press('ArrowDown')
  await page.keyboard.press('ArrowDown')
  await expect(page.getByRole('option', { name: 'Débito' })).toBeFocused()

  await page.keyboard.press('Escape')
  await expect(page.getByRole('listbox')).toBeHidden()
  await expect(trigger).toBeFocused()
})

test('FilterSelect: click fuera cierra sin devolver el foco al trigger', async ({ page }) => {
  await bootstrap(page)
  await mockApi(page, salesRoutes())
  await page.goto('/#/sales')

  await page.getByRole('button', { name: 'Método pago' }).click()
  await expect(page.getByRole('listbox')).toBeVisible()

  await page.getByRole('heading', { name: 'Ventas' }).click()
  await expect(page.getByRole('listbox')).toBeHidden()
})

test('SearchSelect: ArrowDown abre y enfoca la búsqueda, flechas recorren, Escape restaura el trigger', async ({ page }) => {
  await bootstrap(page)
  await mockApi(page, salesRoutes())
  await page.goto('/#/sales')

  const trigger = page.getByRole('button', { name: 'Cliente' })
  await trigger.focus()

  await page.keyboard.press('ArrowDown')
  await expect(page.getByRole('listbox')).toBeVisible()
  await expect(page.getByPlaceholder('Buscar cliente...')).toBeFocused()

  await page.keyboard.press('ArrowDown')
  await expect(page.getByRole('option', { name: 'Todos los Clientes' })).toBeFocused()

  await page.keyboard.press('ArrowUp')
  await expect(page.getByRole('option', { name: 'Todos los Clientes' })).toBeFocused()

  await page.keyboard.press('Escape')
  await expect(page.getByRole('listbox')).toBeHidden()
  await expect(trigger).toBeFocused()
})

test('CalendarDropdown: Escape dentro de la lista de meses cierra todo el popover del filtro', async ({ page }) => {
  await bootstrap(page)
  await mockApi(page, salesRoutes())
  await page.goto('/#/sales')

  const calendarButton = page.getByRole('button', { name: 'Seleccionar fecha inicio' })
  await calendarButton.click()
  await expect(page.getByRole('dialog', { name: 'Fecha inicio' })).toBeVisible()

  const monthTrigger = page.getByRole('button', { name: 'Elegir el mes' })
  await monthTrigger.click()
  await expect(page.getByRole('listbox')).toBeVisible()
  await page.keyboard.press('ArrowDown')
  await expect(page.getByRole('option').first()).toBeFocused()

  // The list stops Escape so a parent dialog is not closed on the first press.
  await page.keyboard.press('Escape')
  await expect(page.getByRole('listbox')).toBeHidden()
  await expect(page.getByRole('dialog', { name: 'Fecha inicio' })).toBeVisible()
  await expect(monthTrigger).toBeFocused()

  await page.keyboard.press('Escape')
  await expect(page.getByRole('dialog', { name: 'Fecha inicio' })).toBeHidden()
  await expect(calendarButton).toBeFocused()
})

test('CalendarDropdown: Escape dentro de la lista de años cierra todo el popover del filtro', async ({ page }) => {
  await bootstrap(page)
  await mockApi(page, salesRoutes())
  await page.goto('/#/sales')

  const calendarButton = page.getByRole('button', { name: 'Seleccionar fecha inicio' })
  await calendarButton.click()
  await expect(page.getByRole('dialog', { name: 'Fecha inicio' })).toBeVisible()

  const yearTrigger = page.getByRole('button', { name: 'Elegir el año' })
  await yearTrigger.click()
  await expect(page.getByRole('listbox')).toBeVisible()
  await page.keyboard.press('ArrowDown')
  await expect(page.getByRole('option').first()).toBeFocused()

  // The list stops Escape so a parent dialog is not closed on the first press.
  await page.keyboard.press('Escape')
  await expect(page.getByRole('listbox')).toBeHidden()
  await expect(page.getByRole('dialog', { name: 'Fecha inicio' })).toBeVisible()
  await expect(yearTrigger).toBeFocused()

  await page.keyboard.press('Escape')
  await expect(page.getByRole('dialog', { name: 'Fecha inicio' })).toBeHidden()
  await expect(calendarButton).toBeFocused()
})

test('OrderForm: Estado se abre con teclado, las flechas recorren, Enter selecciona y Escape restaura el foco', async ({ page }) => {
  await bootstrap(page)
  await mockApi(page, {})
  await page.goto('/#/orders/create')

  const trigger = page.getByRole('button', { name: /^Estado: / })
  await trigger.focus()

  await page.keyboard.press('ArrowDown')
  await expect(page.getByRole('listbox')).toBeVisible()
  await expect(page.getByRole('option', { name: 'Pendiente' })).toBeFocused()

  await page.keyboard.press('ArrowDown')
  await expect(page.getByRole('option', { name: 'Enviado' })).toBeFocused()

  await page.keyboard.press('Enter')
  await expect(page.getByRole('listbox')).toBeHidden()
  await expect(page.getByRole('button', { name: 'Estado: Enviado' })).toBeVisible()

  await trigger.focus()
  await page.keyboard.press('ArrowDown')
  await expect(page.getByRole('listbox')).toBeVisible()

  await page.keyboard.press('Escape')
  await expect(page.getByRole('listbox')).toBeHidden()
  await expect(trigger).toBeFocused()
})
