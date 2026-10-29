import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import Inventory from './Inventory'

const mocks = vi.hoisted(() => ({
  useInventory: vi.fn(),
}))

vi.mock('@/hooks/useInventory', () => ({
  useInventory: mocks.useInventory,
}))

const product = (overrides: Partial<Record<string, unknown>> = {}) => ({
  id: 1,
  name: 'Martillo',
  sku: 'SKU-1',
  category: { id: 1, code: 'HER', name: 'Herramientas' },
  brand: { id: 1, name: 'Truper' },
  material: { id: 1, name: 'Acero' },
  measurement_unit: { id: 1, name: 'Centímetros', abbreviation: 'cm' },
  stock_quantity: 4,
  low_stock_threshold: 5,
  size_value: '10',
  price: 19.9,
  ...overrides,
})

const deleteMutation = { mutateAsync: vi.fn() }

function baseResult(overrides: Record<string, unknown> = {}) {
  return {
    products: [product()],
    hasNext: false,
    stats: { totalProducts: 1, totalStock: 4, lowStockCount: 1, totalValue: 19.9 },
    isLoading: false,
    isFetching: false,
    isError: false,
    refetch: vi.fn(),
    deleteMutation,
    ...overrides,
  }
}

function renderInventory() {
  render(
    <MemoryRouter initialEntries={['/inventory']}>
      <Inventory />
    </MemoryRouter>,
  )
}

describe('Inventory products', () => {
  beforeEach(() => {
    mocks.useInventory.mockReset().mockImplementation(() => baseResult())
  })

  it('renders stock, threshold, relations and the measurement-unit fallback', async () => {
    mocks.useInventory.mockImplementation(() => baseResult({
      products: [
        product(),
        product({
          id: 2,
          name: 'Cinta',
          sku: 'SKU-2',
          category: null,
          brand: null,
          material: null,
          measurement_unit: null,
          unit: 'und',
          stock_quantity: 0,
          low_stock_threshold: 0,
        }),
      ],
    }))

    renderInventory()

    expect(screen.getByText('Martillo')).toBeTruthy()
    expect(screen.getByText('Herramientas')).toBeTruthy()
    expect(screen.getByText('Truper')).toBeTruthy()
    expect(screen.getByText('Acero')).toBeTruthy()
    expect(screen.getByLabelText('Martillo: 4 cm de mínimo 5')).toBeTruthy()
    expect(screen.getByText('Mín. 5 cm')).toBeTruthy()

    expect(screen.getByText('Cinta')).toBeTruthy()
    expect(screen.getByLabelText('Cinta: 0 und de mínimo 0')).toBeTruthy()
    expect(screen.getByText('Mín. 0 und')).toBeTruthy()
  })

  it('increments the page only while hasNext is true', async () => {
    mocks.useInventory.mockImplementation(({ page }: { page: number }) => baseResult({
      products: page === 1 ? [product()] : [product({ id: 2, name: 'Cinta', sku: 'SKU-2' })],
      hasNext: page === 1,
    }))

    renderInventory()
    expect(screen.getByText('Martillo')).toBeTruthy()

    fireEvent.click(screen.getByRole('button', { name: 'Ir a página siguiente' }))

    await waitFor(() => expect(mocks.useInventory).toHaveBeenLastCalledWith(expect.objectContaining({ page: 2 })))
    expect(await screen.findByText('Cinta')).toBeTruthy()
    const next = screen.getByRole('button', { name: 'Ir a página siguiente' })
    expect((next as HTMLButtonElement).disabled).toBe(true)

    fireEvent.click(next)
    expect(mocks.useInventory).toHaveBeenLastCalledWith(expect.objectContaining({ page: 2 }))
  })

  it('hides pagination when the first page has no next page', () => {
    renderInventory()

    expect(screen.queryByRole('button', { name: 'Ir a página siguiente' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Ir a página anterior' })).toBeNull()
  })

  it('resets to page 1 when the availability filter or the search changes', async () => {
    mocks.useInventory.mockImplementation(({ page }: { page: number }) => baseResult({
      hasNext: true,
      products: page === 1 ? [product()] : [product({ id: 2, name: 'Cinta', sku: 'SKU-2' })],
    }))

    renderInventory()
    fireEvent.click(screen.getByRole('button', { name: 'Ir a página siguiente' }))
    await waitFor(() => expect(mocks.useInventory).toHaveBeenLastCalledWith(expect.objectContaining({ page: 2 })))

    fireEvent.click(screen.getByRole('button', { name: 'Disponibilidad' }))
    fireEvent.click(screen.getByRole('option', { name: 'Stock Bajo' }))
    await waitFor(() => expect(mocks.useInventory).toHaveBeenLastCalledWith(expect.objectContaining({
      page: 1,
      filters: expect.objectContaining({ stock_status: 'low_stock' }),
    })))

    fireEvent.click(screen.getByRole('button', { name: 'Ir a página siguiente' }))
    await waitFor(() => expect(mocks.useInventory).toHaveBeenLastCalledWith(expect.objectContaining({ page: 2 })))

    fireEvent.change(screen.getByPlaceholderText('Nombre o ID...'), { target: { value: 'mart' } })
    await waitFor(
      () => expect(mocks.useInventory).toHaveBeenLastCalledWith(expect.objectContaining({ page: 1, search: 'mart' })),
      { timeout: 2000 },
    )
  })

  it('shows the empty state when there are no products', () => {
    mocks.useInventory.mockImplementation(() => baseResult({ products: [] }))

    renderInventory()

    expect(screen.getByText('No se encontraron productos.')).toBeTruthy()
    expect(screen.queryByRole('alert')).toBeNull()
  })

  it('shows the error state and retries', () => {
    const refetch = vi.fn()
    mocks.useInventory.mockImplementation(() => baseResult({ products: [], isError: true, refetch }))

    renderInventory()

    expect(screen.getByRole('alert').textContent).toContain('No se pudieron cargar los datos.')
    fireEvent.click(screen.getByRole('button', { name: 'Reintentar' }))
    expect(refetch).toHaveBeenCalledTimes(1)
  })

  it('shows skeleton rows while loading', () => {
    mocks.useInventory.mockImplementation(() => baseResult({ products: [], isLoading: true }))

    renderInventory()

    expect(screen.getByRole('table')).toBeTruthy()
    expect(screen.getAllByRole('row')).toHaveLength(6)
    expect(screen.queryByText('No se encontraron productos.')).toBeNull()
  })
})
