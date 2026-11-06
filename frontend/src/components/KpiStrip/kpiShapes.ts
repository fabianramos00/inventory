import type { CSSProperties } from 'react'

export type KpiUnit = 'money' | 'count'

export interface KpiCardConfig {
  label: string
  value: string | number
  kind: KpiUnit
  lead?: boolean
  tone?: 'negative'
}

export const KPI_SHAPE_TEMPLATES: Record<string, string> = {
  'money-money-count-count': 'minmax(0, 1.45fr) minmax(0, 1.3fr) minmax(0, 0.65fr) minmax(0, 0.65fr)',
  'money-money-money-count': 'minmax(0, 1.25fr) minmax(0, 1.25fr) minmax(0, 1.25fr) minmax(0, 0.75fr)',
  'count-count-count-money': 'minmax(0, 0.75fr) minmax(0, 0.75fr) minmax(0, 0.75fr) minmax(0, 1.25fr)',
}

export function kpiShape(kinds: KpiUnit[]): string {
  return kinds.join('-')
}

export function kpiTemplate(kinds: KpiUnit[]): string {
  const shape = kpiShape(kinds)
  return KPI_SHAPE_TEMPLATES[shape] ?? `repeat(${kinds.length}, minmax(0, 1fr))`
}

export function kpiTemplateStyle(kinds: KpiUnit[]): CSSProperties {
  return { '--kpi-template': kpiTemplate(kinds) } as CSSProperties
}
