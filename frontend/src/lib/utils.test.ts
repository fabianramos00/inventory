import { describe, expect, it } from 'vitest'
import { formatCurrency, formatDate, formatNumber, getProductStatus } from './utils'

describe('formatCurrency', () => {
  it('formats COP with Colombian separators and two decimals', () => {
    expect(formatCurrency(1234.56)).toBe('$ 1.234,56')
    expect(formatCurrency(0)).toBe('$ 0,00')
  })

  it('falls back to zero for non-finite values', () => {
    expect(formatCurrency(Number.NaN)).toBe('$ 0,00')
  })
})

describe('formatNumber', () => {
  it('formats counts with Colombian thousands separators and no decimals', () => {
    expect(formatNumber(1234)).toBe('1.234')
    expect(formatNumber(0)).toBe('0')
  })
})

describe('formatDate', () => {
  it('formats dates as DD/MM/YYYY', () => {
    expect(formatDate('2026-10-24T12:00:00Z')).toBe('24/10/2026')
  })
})

describe('getProductStatus', () => {
  it('classifies stock against the threshold', () => {
    expect(getProductStatus(0, 5)).toBe('out_of_stock')
    expect(getProductStatus(5, 5)).toBe('low_stock')
    expect(getProductStatus(6, 5)).toBe('in_stock')
  })
})
