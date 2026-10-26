export function formatDate(iso: string) {
  return new Date(iso).toLocaleDateString('es-CO', { day: '2-digit', month: '2-digit', year: 'numeric' })
}

const numberFormatter = new Intl.NumberFormat('es-CO')
const currencyFormatter = new Intl.NumberFormat('es-CO', {
  minimumFractionDigits: 2,
  maximumFractionDigits: 2,
})

export function formatNumber(value: number) {
  return numberFormatter.format(Number.isFinite(value) ? value : 0)
}

export function formatCurrency(value: number) {
  return `$ ${currencyFormatter.format(Number.isFinite(value) ? value : 0)}`
}

export function safeNumber(value: string, fallback: number = 0): number {
  const n = parseFloat(value)
  return Number.isFinite(n) ? n : fallback
}
