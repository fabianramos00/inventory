import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import KpiStrip, { type KpiCardConfig } from './KpiStrip'
import { KPI_SHAPE_TEMPLATES, kpiShape, kpiTemplate } from './kpiShapes'

const DASHBOARD_CARDS: KpiCardConfig[] = [
  { label: 'Ventas de hoy', value: '$ 12.345.678,00', kind: 'money', lead: true },
  { label: 'Saldo por cobrar', value: '$ 1.234,50', kind: 'money' },
  { label: 'Pedidos pendientes', value: '3', kind: 'count' },
  { label: 'Stock bajo', value: '7', kind: 'count' },
]

function stripOf(container: HTMLElement) {
  const strip = container.querySelector('[data-shape]')
  if (!strip) throw new Error('KPI strip not rendered')
  return strip as HTMLElement
}

describe('KpiStrip', () => {
  it('derives the column shape from the unit of every card', () => {
    expect(kpiShape(['money', 'money', 'count', 'count'])).toBe('money-money-count-count')
    expect(kpiShape(['money', 'money', 'money', 'count'])).toBe('money-money-money-count')
    expect(kpiShape(['count', 'count', 'count', 'money'])).toBe('count-count-count-money')

    const { container } = render(<KpiStrip cards={DASHBOARD_CARDS} />)

    expect(stripOf(container).getAttribute('data-shape')).toBe('money-money-count-count')
    expect(container.querySelectorAll('[data-kind]').length).toBe(4)
    expect(container.querySelector('[data-kind="money"]')).not.toBeNull()
    expect(container.querySelector('[data-kind="count"]')).not.toBeNull()
  })

  it('maps every shape to its weighted column template', () => {
    expect(KPI_SHAPE_TEMPLATES).toEqual({
      'money-money-count-count': 'minmax(0, 1.45fr) minmax(0, 1.3fr) minmax(0, 0.65fr) minmax(0, 0.65fr)',
      'money-money-money-count': 'minmax(0, 1.25fr) minmax(0, 1.25fr) minmax(0, 1.25fr) minmax(0, 0.75fr)',
      'count-count-count-money': 'minmax(0, 0.75fr) minmax(0, 0.75fr) minmax(0, 0.75fr) minmax(0, 1.25fr)',
    })

    expect(kpiTemplate(['money', 'money', 'count', 'count'])).toBe(KPI_SHAPE_TEMPLATES['money-money-count-count'])
    expect(kpiTemplate(['count', 'count', 'count', 'money'])).toBe(KPI_SHAPE_TEMPLATES['count-count-count-money'])
  })

  it('falls back to equal columns for a shape without a template', () => {
    expect(kpiTemplate(['money', 'count'])).toBe('repeat(2, minmax(0, 1fr))')
  })

  it('applies the weighted template of its shape to the strip', () => {
    const { container } = render(<KpiStrip cards={DASHBOARD_CARDS} />)

    expect(stripOf(container).style.getPropertyValue('--kpi-template')).toBe(
      KPI_SHAPE_TEMPLATES['money-money-count-count'],
    )
  })

  it('renders a large amount intact and keeps it available as a title', () => {
    render(<KpiStrip cards={DASHBOARD_CARDS} />)

    const value = screen.getByText('$ 12.345.678,00')
    expect(value.textContent).toBe('$ 12.345.678,00')
    expect(value.getAttribute('title')).toBe('$ 12.345.678,00')
    expect(value.getAttribute('data-overflow')).toBe('ellipsis')
    expect(value.getAttribute('aria-label')).toBeNull()
  })

  it('marks the debt tone only on the card that asks for it', () => {
    const { container } = render(<KpiStrip cards={[
      { label: 'Total vendido', value: '$ 100,00', kind: 'money' },
      { label: 'Por cobrar', value: '$ 40,00', kind: 'money', tone: 'negative' },
      { label: 'Sin entregar', value: '2', kind: 'count' },
    ]} />)

    const values = Array.from(container.querySelectorAll('[data-overflow]'))
    expect(values.map((value) => value.getAttribute('data-tone'))).toEqual([null, 'negative', null])
  })
})
