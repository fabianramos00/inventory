import { expect, test } from '@playwright/test'
import { ADMIN } from './helpers'

test('logs in against the real API and logs out', async ({ page }) => {
  await page.goto('/#/login')
  await expect(page.getByRole('heading', { name: 'Bienvenido' })).toBeVisible()

  await page.getByLabel('Correo electrónico').fill(ADMIN.email)
  await page.getByLabel('Contraseña', { exact: true }).fill(ADMIN.password)
  await page.getByRole('button', { name: 'Ingresar' }).click()

  await expect(page).toHaveURL(/#\/$/)
  await expect(page.getByRole('heading', { name: 'Dashboard' })).toBeVisible()

  await page.getByRole('button', { name: 'Cerrar sesión' }).click()

  await expect(page).toHaveURL(/#\/login$/)
  await expect(page.getByRole('heading', { name: 'Bienvenido' })).toBeVisible()
})
