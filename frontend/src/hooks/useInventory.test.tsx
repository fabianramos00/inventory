import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useInventory } from './useInventory'

const mocks = vi.hoisted(() => ({
  getProducts: vi.fn(),
  getStats: vi.fn(),
  deleteProduct: vi.fn(),
}))

vi.mock('@/lib/api/inventory', () => ({
  inventoryApi: {
    getProducts: mocks.getProducts,
    getStats: mocks.getStats,
    deleteProduct: mocks.deleteProduct,
  },
}))

function wrapperFor(queryClient: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  }
}

describe('useInventory', () => {
  beforeEach(() => {
    mocks.getProducts.mockReset().mockResolvedValue({
      data: { items: [{ id: 1 }], has_next: true, skip: 0, limit: 10 },
    })
    mocks.getStats.mockReset().mockResolvedValue({
      data: { total_products: 0, total_stock: 0, low_stock_count: 0, total_inventory_value: 0 },
    })
    mocks.deleteProduct.mockReset().mockResolvedValue(undefined)
  })

  it('returns hasNext and sends skip/limit for the requested page', async () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const { result } = renderHook(
      () => useInventory({ page: 1, search: '', filters: {} }),
      { wrapper: wrapperFor(queryClient) },
    )

    await waitFor(() => expect(result.current.isLoading).toBe(false))

    expect(mocks.getProducts).toHaveBeenCalledWith({ skip: 0, limit: 10 })
    expect(result.current.products).toEqual([{ id: 1 }])
    expect(result.current.hasNext).toBe(true)
  })

  it('sends skip/limit for page 2 and reports hasNext false', async () => {
    mocks.getProducts.mockResolvedValue({
      data: { items: [], has_next: false, skip: 10, limit: 10 },
    })
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const { result } = renderHook(
      () => useInventory({ page: 2, search: '', filters: {} }),
      { wrapper: wrapperFor(queryClient) },
    )

    await waitFor(() => expect(result.current.isLoading).toBe(false))

    expect(mocks.getProducts).toHaveBeenCalledWith({ skip: 10, limit: 10 })
    expect(result.current.hasNext).toBe(false)
  })

  it('passes search and filters through to the API', async () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    renderHook(
      () => useInventory({
        page: 1,
        search: 'mart',
        filters: { category_id: 2, material_id: 3, brand_id: 4, stock_status: 'low_stock' },
      }),
      { wrapper: wrapperFor(queryClient) },
    )

    await waitFor(() => expect(mocks.getProducts).toHaveBeenCalledWith({
      skip: 0,
      limit: 10,
      search: 'mart',
      category_id: 2,
      material_id: 3,
      brand_id: 4,
      stock_status: 'low_stock',
    }))
  })

  it('does not derive hasNext from legacy total or pages fields', async () => {
    mocks.getProducts.mockResolvedValue({
      data: { items: [{ id: 1 }], total: 30, page: 1, size: 10, pages: 3 },
    })
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const { result } = renderHook(
      () => useInventory({ page: 1, search: '', filters: {} }),
      { wrapper: wrapperFor(queryClient) },
    )

    await waitFor(() => expect(result.current.isLoading).toBe(false))

    expect(result.current.hasNext).toBe(false)
  })
})
