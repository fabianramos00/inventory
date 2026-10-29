import { expect, request, type APIResponse, type Page } from '@playwright/test'
import path from 'node:path'
import { readFile } from 'node:fs/promises'

export const API_ORIGIN = 'http://127.0.0.1:8787'
export const API_URL = `${API_ORIGIN}/api/v1`
export const APP_URL = 'http://127.0.0.1:4173'
export const AUTH_FILE = path.resolve(process.cwd(), 'test-results/.auth/admin.json')

export const ADMIN = {
  email: 'admin@example.com',
  password: 'admin12345',
  fullName: 'Admin User',
}

type Query = Record<string, string | number | boolean | undefined>
type Json = Record<string, unknown>

export interface ApiClient {
  get<T = Json>(path: string, params?: Query): Promise<T>
  post<T = Json>(path: string, body?: unknown): Promise<T>
  put<T = Json>(path: string, body?: unknown): Promise<T>
  delete<T = Json>(path: string): Promise<T>
  status(method: 'GET' | 'POST' | 'PUT' | 'DELETE', path: string, body?: unknown): Promise<number>
  dispose(): Promise<void>
}

export interface Named {
  id: number
  name: string
}

export interface Catalog {
  suffix: string
  category: Named & { code: string }
  brand: Named
  material: Named
  unit: Named & { abbreviation: string }
}

export interface Product extends Named {
  sku: string
  price: number
  cost: number
  stock_quantity: number
  low_stock_threshold: number
  category: Named | null
  brand: Named | null
  material: Named | null
}

export interface Client extends Named {
  identity_card: string | null
  email: string | null
  phone: string | null
}

export interface Provider extends Named {
  contact_info: string | null
  email: string | null
  phone: string | null
}

export interface Sale {
  id: number
  total_amount: number
  amount_paid: number
  client_id: number | null
  client: Named | null
}

export interface Order {
  id: number
  provider_id: number
  status: 'pending' | 'sent' | 'received' | 'cancelled'
  payment_status: 'pending' | 'paid'
  provider: Named
}

function cleanParams(params?: Query) {
  return Object.fromEntries(
    Object.entries(params ?? {}).filter(([, value]) => value !== undefined),
  ) as Record<string, string | number | boolean>
}

async function assertOk(response: APIResponse, label: string) {
  if (!response.ok()) {
    throw new Error(`${label} failed with ${response.status()}: ${await response.text()}`)
  }
}

async function storedAccessToken(): Promise<string | null> {
  try {
    const state = JSON.parse(await readFile(AUTH_FILE, 'utf8')) as {
      origins?: { localStorage?: { name: string; value: string }[] }[]
    }
    return state.origins
      ?.flatMap(origin => origin.localStorage ?? [])
      .find(entry => entry.name === 'token')?.value ?? null
  } catch {
    return null
  }
}

export async function createApiClient(): Promise<ApiClient> {
  const context = await request.newContext({ baseURL: API_ORIGIN })
  let accessToken = await storedAccessToken()
  if (!accessToken) {
    const login = await context.post('/api/v1/auth/login', {
      form: { username: ADMIN.email, password: ADMIN.password },
    })
    await assertOk(login, 'API login')
    accessToken = (await login.json() as { access_token: string }).access_token
  }
  const headers = { Authorization: `Bearer ${accessToken}` }

  async function call<T>(
    method: 'GET' | 'POST' | 'PUT' | 'DELETE',
    path: string,
    body?: unknown,
    params?: Query,
  ): Promise<T> {
    const url = path.startsWith('/api/') ? path : `/api/v1${path}`
    const response = await context.fetch(url, {
      method,
      headers,
      ...(params ? { params: cleanParams(params) } : {}),
      ...(body === undefined ? {} : { data: body }),
    })
    await assertOk(response, `${method} ${path}`)
    if (response.status() === 204) return undefined as T
    return await response.json() as T
  }

  return {
    get: (path, params) => call('GET', path, undefined, params),
    post: (path, body) => call('POST', path, body),
    put: (path, body) => call('PUT', path, body),
    delete: path => call('DELETE', path),
    status: async (method, path, body) => {
      const url = path.startsWith('/api/') ? path : `/api/v1${path}`
      const response = await context.fetch(url, {
        method,
        headers,
        ...(body === undefined ? {} : { data: body }),
      })
      return response.status()
    },
    dispose: () => context.dispose(),
  }
}

export function uniqueSuffix() {
  return `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`
}

export async function seedCatalog(api: ApiClient, suffix = uniqueSuffix()): Promise<Catalog> {
  const category = await api.post<Named & { code: string }>('/inventory/categories', {
    code: `E${suffix.slice(-8).toUpperCase()}`,
    name: `Categoria E2E ${suffix}`,
  })
  const brand = await api.post<Named>('/inventory/brands', { name: `Marca E2E ${suffix}` })
  const material = await api.post<Named>('/inventory/materials', { name: `Material E2E ${suffix}` })
  const unit = await api.post<Named & { abbreviation: string }>('/inventory/measurement-units', {
    name: `Unidad E2E ${suffix}`,
    abbreviation: `u${suffix.slice(-2)}`,
  })
  return { suffix, category, brand, material, unit }
}

export async function seedProduct(
  api: ApiClient,
  catalog: Catalog,
  overrides: Partial<Pick<Product, 'name' | 'sku' | 'price' | 'cost' | 'stock_quantity' | 'low_stock_threshold'>> = {},
): Promise<Product> {
  return api.post<Product>('/inventory/products', {
    name: overrides.name ?? `Producto E2E ${catalog.suffix}`,
    sku: overrides.sku ?? `SKU-${catalog.suffix}`,
    brand_id: catalog.brand.id,
    category_id: catalog.category.id,
    material_id: catalog.material.id,
    measurement_unit_id: catalog.unit.id,
    size_value: '10',
    price: overrides.price ?? 100,
    cost: overrides.cost ?? 50,
    stock_quantity: overrides.stock_quantity ?? 10,
    low_stock_threshold: overrides.low_stock_threshold ?? 2,
  })
}

export async function seedClient(api: ApiClient, suffix = uniqueSuffix()): Promise<Client> {
  return api.post<Client>('/sales/clients', {
    name: `Cliente E2E ${suffix}`,
    identity_card: `CC-${suffix}`,
    email: `cliente.${suffix}@example.com`,
    phone: '3000000000',
  })
}

export async function seedProvider(api: ApiClient, suffix = uniqueSuffix()): Promise<Provider> {
  return api.post<Provider>('/supply-chain/providers', {
    name: `Proveedor E2E ${suffix}`,
    contact_info: `Contacto ${suffix}`,
    email: `proveedor.${suffix}@example.com`,
    phone: '3100000000',
  })
}

export async function loginThroughUi(page: Page) {
  await page.goto('/#/login')
  await expect(page.getByRole('heading', { name: 'Bienvenido' })).toBeVisible()
  await page.getByLabel('Correo electrónico').fill(ADMIN.email)
  await page.getByLabel('Contraseña', { exact: true }).fill(ADMIN.password)
  await page.getByRole('button', { name: 'Ingresar' }).click()
  await expect(page.getByRole('heading', { name: `Hola, ${ADMIN.fullName}` })).toBeVisible()
}

export function escapeRegExp(value: string) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}
