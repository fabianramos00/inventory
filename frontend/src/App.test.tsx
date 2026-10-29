import { render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import App from './App'

vi.mock('@/pages/Login/Login', () => ({ default: () => <div>login-page</div> }))
vi.mock('@/pages/Inventory/Inventory', () => ({ default: () => <div>inventory-page</div> }))
vi.mock('@/pages/ProductForm/ProductForm', () => ({ default: () => <div>product-form-page</div> }))

function authenticate() {
  localStorage.setItem('token', 'access-token')
  localStorage.setItem('refresh_token', 'refresh-token')
  localStorage.setItem('user', JSON.stringify({
    id: 1,
    full_name: 'Admin User',
    email: 'admin@example.com',
    is_active: true,
    is_superuser: true,
  }))
}

function renderAt(hash: string) {
  window.location.hash = hash
  render(<App />)
}

describe('App routes', () => {
  beforeEach(() => {
    localStorage.clear()
    window.location.hash = ''
  })

  it.each([
    ['#/inventory', 'inventory-page', '#/inventory'],
    ['#/inventory/create', 'product-form-page', '#/inventory/create'],
    ['#/inventory/product/1', 'product-form-page', '#/inventory/product/1'],
  ])('renders %s for an authenticated user', async (hash, text, expectedHash) => {
    authenticate()
    renderAt(hash)

    expect(await screen.findByText(text)).toBeTruthy()
    expect(window.location.hash).toBe(expectedHash)
  })

  it.each(['#/inventory', '#/inventory/create', '#/inventory/product/1'])(
    'redirects %s to login when unauthenticated',
    async (hash) => {
      renderAt(hash)

      expect(await screen.findByText('login-page')).toBeTruthy()
      await waitFor(() => expect(window.location.hash).toBe('#/login'))
    },
  )

  it('marks the inventory sidebar link as active', async () => {
    authenticate()
    renderAt('#/inventory')

    expect(await screen.findByText('inventory-page')).toBeTruthy()
    const inventoryLink = screen.getByRole('link', { name: 'Inventario' })
    const dashboardLink = screen.getByRole('link', { name: 'Dashboard' })
    expect(inventoryLink.getAttribute('aria-current')).toBe('page')
    expect(dashboardLink.getAttribute('aria-current')).toBeNull()
  })
})
