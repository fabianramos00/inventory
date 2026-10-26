import styles from './KpiStrip.module.css'

export interface KpiCardConfig {
  label: string
  value: string | number
  lead?: boolean
  tone?: 'negative'
}

export default function KpiStrip({ cards }: { cards: KpiCardConfig[] }) {
  return (
    <div className={styles.kpiStrip} data-lead={cards[0]?.lead ? 'true' : undefined}>
      {cards.map((card) => (
        <div key={card.label} className={`${styles.kpiItem} ${card.lead ? styles.kpiItemLead : ''}`}>
          <span className={`${styles.kpiLabel} ${card.lead ? styles.kpiLabelLead : ''}`}>{card.label}</span>
          <span className={`${styles.kpiValue} ${card.lead ? styles.kpiLead : ''}`}>
            {card.value}
          </span>
        </div>
      ))}
    </div>
  )
}
