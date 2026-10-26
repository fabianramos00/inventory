import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import StockMeter from './StockMeter'

describe('StockMeter', () => {
  it('clamps the value and exposes a labelled meter', () => {
    const { container } = render(<StockMeter value={12} max={5} tone="warning" label="Stock de tornillos" />)
    const meter = screen.getByRole('progressbar', { name: 'Stock de tornillos' })
    expect(meter.getAttribute('value')).toBe('5')
    expect(meter.getAttribute('max')).toBe('5')
    expect(container.firstElementChild?.className).toContain('warning')
  })

  it('handles a zero maximum without an invalid range', () => {
    render(<StockMeter value={0} max={0} />)
    expect(screen.getByRole('progressbar').getAttribute('max')).toBe('1')
  })
})
