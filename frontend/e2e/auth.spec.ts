import { expect, test } from '@playwright/test'

const admin = {
  email: 'admin@example.com',
  password: 'admin12345',
  fullName: 'Admin User',
}

test('logs in against the real API and logs out', async ({ page }) => {
  await page.goto('/#/login')
  await expect(page.getByRole('heading', { name: 'Bienvenido' })).toBeVisible()

  await page.getByLabel('Correo electrónico').fill(admin.email)
  await page.getByLabel('Contraseña', { exact: true }).fill(admin.password)
  await page.getByRole('button', { name: 'Ingresar' }).click()

  await expect(page).toHaveURL(/#\/$/)
  await expect(page.getByText('Sesión iniciada')).toBeVisible()
  await expect(page.getByRole('heading', { name: `Hola, ${admin.fullName}` })).toBeVisible()

  const homeCard = page.getByRole('region', { name: `Hola, ${admin.fullName}` })
  await expect(homeCard).toBeVisible()
  await homeCard.getByRole('button', { name: 'Cerrar sesión' }).click()

  await expect(page).toHaveURL(/#\/login$/)
  await expect(page.getByRole('heading', { name: 'Bienvenido' })).toBeVisible()
})
