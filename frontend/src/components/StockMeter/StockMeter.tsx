import styles from './StockMeter.module.css'

export type StockTone = 'neutral' | 'warning' | 'danger'

interface StockMeterProps {
  value: number
  max: number
  tone?: StockTone
  label?: string
}

export default function StockMeter({ value, max, tone = 'neutral', label }: StockMeterProps) {
  const safeMax = Math.max(max, 1)
  const safeValue = Math.min(Math.max(value, 0), safeMax)

  return (
    <progress
      className={`${styles.meter} ${styles[tone]}`}
      value={safeValue}
      max={safeMax}
      aria-label={label ?? `Stock ${value} de ${safeMax}`}
    />
  )
}
