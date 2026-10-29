import { render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import App from './App'

vi.mock('@/pages/Login/Login', () => ({ default: () => <div>login-page</div> }))
vi.mock('@/pages/Inventory/Inventory', () => ({ default: () => <div>inventory-page</div> }))
vi.mock('@/pages/ProductForm/ProductForm', () => ({ default: () => <div>product-form-page</div> }))
vi.mock('@/pages/Sales/Sales', () => ({ default: () => <div>sales-page</div> }))
vi.mock('@/pages/SaleForm/SaleForm', () => ({ default: () => <div>sale-form-page</div> }))
vi.mock('@/pages/Clients/Clients', () => ({ default: () => <div>clients-page</div> }))
vi.mock('@/pages/Providers/Providers', () => ({ default: () => <div>providers-page</div> }))

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
    ['#/sales', 'sales-page', '#/sales'],
    ['#/sales/create', 'sale-form-page', '#/sales/create'],
    ['#/sales/12', 'sale-form-page', '#/sales/12'],
    ['#/clients', 'clients-page', '#/clients'],
    ['#/providers', 'providers-page', '#/providers'],
  ])('renders %s for an authenticated user', async (hash, text, expectedHash) => {
    authenticate()
    renderAt(hash)

    expect(await screen.findByText(text)).toBeTruthy()
    expect(window.location.hash).toBe(expectedHash)
  })

  it.each([
    '#/inventory',
    '#/inventory/create',
    '#/inventory/product/1',
    '#/sales',
    '#/sales/create',
    '#/sales/12',
    '#/clients',
    '#/providers',
  ])(
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

  it.each([
    ['#/sales', 'sales-page'],
    ['#/sales/create', 'sale-form-page'],
    ['#/sales/12', 'sale-form-page'],
  ])('keeps the Ventas sidebar link active for %s', async (hash, text) => {
    authenticate()
    renderAt(hash)

    expect(await screen.findByText(text)).toBeTruthy()
    expect(screen.getByRole('link', { name: 'Ventas' }).getAttribute('aria-current')).toBe('page')
    expect(screen.getByRole('link', { name: 'Dashboard' }).getAttribute('aria-current')).toBeNull()
  })

  it('marks the Clientes sidebar link as active', async () => {
    authenticate()
    renderAt('#/clients')

    expect(await screen.findByText('clients-page')).toBeTruthy()
    expect(screen.getByRole('link', { name: 'Clientes' }).getAttribute('aria-current')).toBe('page')
    expect(screen.getByRole('link', { name: 'Dashboard' }).getAttribute('aria-current')).toBeNull()
  })

  it('marks the Proveedores sidebar link as active', async () => {
    authenticate()
    renderAt('#/providers')

    expect(await screen.findByText('providers-page')).toBeTruthy()
    expect(screen.getByRole('link', { name: 'Proveedores' }).getAttribute('aria-current')).toBe('page')
    expect(screen.getByRole('link', { name: 'Dashboard' }).getAttribute('aria-current')).toBeNull()
  })
})
