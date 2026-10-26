import { expect, test } from '@playwright/test'
import {
  bootstrap,
  client,
  getCalls,
  inventoryStats,
  mockApi,
  product,
  provider,
  user,
  type Routes,
} from './support'

function clientRoutes(): Routes {
  return {
    'GET /api/v1/sales/clients': url => {
      const skip = Number(url.searchParams.get('skip') ?? 0)
      const search = url.searchParams.get('search')
      if (search) {
        return { body: { items: [client(99, `Ana ${search}`)], has_next: true, skip, limit: 10 } }
      }
      const items = Array.from({ length: 10 }, (_, index) => client(skip + index + 1))
      return { body: { items, has_next: skip + 10 < 30, skip, limit: 10 } }
    },
  }
}

function providerRoutes(): Routes {
  return {
    'GET /api/v1/supply-chain/providers': url => {
      const skip = Number(url.searchParams.get('skip') ?? 0)
      const search = url.searchParams.get('search')
      const items = search
        ? [provider(99, `Ana ${search}`)]
        : Array.from({ length: 10 }, (_, index) => provider(skip + index + 1))
      return { body: { items, total: 30, page: skip / 10 + 1, size: 10, pages: 3 } }
    },
  }
}

function userRoutes(): Routes {
  return {
    'GET /api/v1/users': url => {
      const skip = Number(url.searchParams.get('skip') ?? 0)
      const search = url.searchParams.get('search')
      const items = search
        ? [user(99, `Ana ${search}`)]
        : Array.from({ length: 10 }, (_, index) => user(skip + index + 1))
      return { body: { items, total: 30, page: skip / 10 + 1, size: 10, pages: 3 } }
    },
  }
}

function inventoryRoutes(): Routes {
  return {
    'GET /api/v1/inventory/products/stats/overview': { body: inventoryStats },
    'GET /api/v1/inventory/products': url => {
      const skip = Number(url.searchParams.get('skip') ?? 0)
      const search = url.searchParams.get('search')
      const items = search
        ? [{ ...product(99), name: `Ana ${search}` }]
        : Array.from({ length: 10 }, (_, index) => product(skip + index + 1))
      return { body: { items, total: 30, page: skip / 10 + 1, size: 10, pages: 3 } }
    },
  }
}

const cases = [
  {
    name: 'Clientes',
    path: '/clients',
    listPath: '/api/v1/sales/clients',
    placeholder: 'Buscar cliente por nombre, cédula o contacto...',
    firstRow: 'Cliente 1',
    secondRow: 'Cliente 11',
    pageTwo: 'Página 2',
    pageOne: 'Página 1',
    routes: clientRoutes(),
  },
  {
    name: 'Proveedores',
    path: '/providers',
    listPath: '/api/v1/supply-chain/providers',
    placeholder: 'Buscar proveedor por nombre o contacto...',
    firstRow: 'Proveedor 1',
    secondRow: 'Proveedor 11',
    pageTwo: 'Página 2 de 3',
    pageOne: 'Página 1 de 3',
    routes: providerRoutes(),
  },
  {
    name: 'Usuarios',
    path: '/users',
    listPath: '/api/v1/users',
    placeholder: 'Buscar usuario...',
    firstRow: 'Usuario 1',
    secondRow: 'Usuario 11',
    pageTwo: 'Página 2 de 3',
    pageOne: 'Página 1 de 3',
    routes: userRoutes(),
  },
  {
    name: 'Inventario',
    path: '/inventory',
    listPath: '/api/v1/inventory/products',
    placeholder: 'Nombre o ID...',
    firstRow: 'Producto 1',
    secondRow: 'Producto 11',
    pageTwo: 'Página 2 de 3',
    pageOne: 'Página 1 de 3',
    routes: inventoryRoutes(),
  },
]

for (const testCase of cases) {
  test(`${testCase.name}: debounced search resets to page 1 with a single request`, async ({ page }) => {
    await bootstrap(page)
    const calls = await mockApi(page, testCase.routes)

    await page.goto(testCase.path)
    await expect(page.getByText(testCase.firstRow, { exact: true })).toBeVisible()

    await page.getByRole('button', { name: 'Ir a página siguiente' }).click()
    await expect.poll(() =>
      getCalls(calls, testCase.listPath).some(call => call.url.searchParams.get('skip') === '10'),
    ).toBe(true)
    await expect(page.getByText(testCase.pageTwo, { exact: true })).toBeVisible()
    await expect(page.getByText(testCase.secondRow, { exact: true })).toBeVisible()

    const requestsBeforeTyping = getCalls(calls, testCase.listPath).length

    const typedAt = Date.now()
    await page.getByPlaceholder(testCase.placeholder).fill('ana')

    await expect.poll(() =>
      getCalls(calls, testCase.listPath).length,
    { timeout: 2000 }).toBe(requestsBeforeTyping + 1)

    const lastRequest = getCalls(calls, testCase.listPath).at(-1)!
    expect(lastRequest.at - typedAt).toBeGreaterThanOrEqual(280)
    expect(lastRequest.url.searchParams.get('skip')).toBe('0')
    expect(lastRequest.url.searchParams.get('search')).toBe('ana')
    expect(getCalls(calls, testCase.listPath).filter(call => call.url.searchParams.get('search') === 'ana')).toHaveLength(1)

    await expect(page.getByText(`Ana ana`, { exact: true })).toBeVisible()
    await expect(page.getByText(testCase.pageOne, { exact: true })).toBeVisible()
  })
}

test('Paginación: cada página pide skip = 10 * (página - 1) y actualiza las filas', async ({ page }) => {
  await bootstrap(page)
  const calls = await mockApi(page, clientRoutes())

  await page.goto('/clients')
  await expect(page.getByText('Cliente 1', { exact: true })).toBeVisible()
  expect(getCalls(calls, '/api/v1/sales/clients').at(-1)!.url.searchParams.get('skip')).toBe('0')

  await page.getByRole('button', { name: 'Ir a página siguiente' }).click()
  await expect(page.getByText('Cliente 11', { exact: true })).toBeVisible()
  expect(getCalls(calls, '/api/v1/sales/clients').at(-1)!.url.searchParams.get('skip')).toBe('10')

  await page.getByRole('button', { name: 'Ir a página siguiente' }).click()
  await expect(page.getByText('Cliente 21', { exact: true })).toBeVisible()
  expect(getCalls(calls, '/api/v1/sales/clients').at(-1)!.url.searchParams.get('skip')).toBe('20')
})
