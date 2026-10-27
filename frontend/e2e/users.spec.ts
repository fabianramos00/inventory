import { expect, test } from '@playwright/test'

const admin = {
  email: 'admin@example.com',
  password: 'admin12345',
  fullName: 'Admin User',
}

test('creates, searches, edits, toggles, and deletes a user against the real API', async ({ page }) => {
  const suffix = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`
  const name = `Usuario E2E ${suffix}`
  const editedName = `Usuario Editado ${suffix}`
  const email = `e2e.${suffix}@example.com`

  await page.goto('/#/login')
  await page.getByLabel('Correo electrónico').fill(admin.email)
  await page.getByLabel('Contraseña', { exact: true }).fill(admin.password)
  await page.getByRole('button', { name: 'Ingresar' }).click()
  await expect(page.getByRole('heading', { name: `Hola, ${admin.fullName}` })).toBeVisible()

  await page.getByRole('link', { name: 'Usuarios' }).click()
  await expect(page).toHaveURL(/#\/users$/)
  await expect(page.getByRole('heading', { name: 'Usuarios' })).toBeVisible()

  await page.getByRole('button', { name: 'Nuevo Usuario' }).click()
  const createDialog = page.getByRole('dialog', { name: 'Nuevo Usuario' })
  await expect(createDialog).toBeVisible()
  await createDialog.getByLabel('Nombre Completo').fill(name)
  await createDialog.getByLabel('Correo Electrónico').fill(email)
  await createDialog.getByLabel('Contraseña', { exact: true }).fill('password123')
  await createDialog.getByRole('button', { name: 'Rol' }).click()
  await createDialog.getByRole('button', { name: 'Usuario', exact: true }).click()
  await createDialog.getByRole('button', { name: 'Guardar' }).click()
  await expect(createDialog).toBeHidden()

  const search = page.getByPlaceholder('Buscar usuario...')
  await search.fill(suffix)
  const row = page.locator('tr').filter({ hasText: name })
  await expect(row).toBeVisible()
  await expect(row).toContainText(email)
  await expect(row).toContainText('Usuario')

  await row.getByTitle('Editar usuario').click()
  const editDialog = page.getByRole('dialog', { name: 'Editar Usuario' })
  await expect(editDialog).toBeVisible()
  await expect(editDialog.getByLabel('Correo Electrónico')).toHaveValue(email)
  await editDialog.getByLabel('Nombre Completo').fill(editedName)
  await editDialog.getByRole('button', { name: 'Guardar' }).click()
  await expect(editDialog).toBeHidden()

  const editedRow = page.locator('tr').filter({ hasText: editedName })
  await expect(editedRow).toBeVisible()
  await expect(editedRow).toContainText(email)

  await editedRow.getByTitle('Desactivar usuario').click()
  await expect(page.locator('tr').filter({ hasText: editedName }).getByTitle('Activar usuario')).toBeVisible()

  await page.locator('tr').filter({ hasText: editedName }).getByTitle('Eliminar usuario').click()
  const deleteDialog = page.getByRole('dialog', { name: 'Eliminar Usuario' })
  await expect(deleteDialog).toBeVisible()
  await expect(deleteDialog).toContainText(editedName)
  await deleteDialog.getByRole('button', { name: 'Eliminar', exact: true }).click()
  await expect(deleteDialog).toBeHidden()

  await expect(page.getByText('No se encontraron usuarios.', { exact: true })).toBeVisible()
})
