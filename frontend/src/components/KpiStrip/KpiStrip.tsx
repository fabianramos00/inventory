import styles from './KpiStrip.module.css'
import { kpiShape, kpiTemplateStyle, type KpiCardConfig } from './kpiShapes'

export type { KpiCardConfig, KpiUnit } from './kpiShapes'

export default function KpiStrip({ cards }: { cards: KpiCardConfig[] }) {
  const kinds = cards.map((card) => card.kind)

  return (
    <div
      className={styles.kpiStrip}
      data-lead={cards[0]?.lead ? 'true' : undefined}
      data-shape={kpiShape(kinds)}
      style={kpiTemplateStyle(kinds)}
    >
      {cards.map((card) => (
        <div key={card.label} className={`${styles.kpiItem} ${card.lead ? styles.kpiItemLead : ''}`} data-kind={card.kind}>
          <span className={`${styles.kpiLabel} ${card.lead ? styles.kpiLabelLead : ''}`}>{card.label}</span>
          <span
            className={`${styles.kpiValue} ${card.lead ? styles.kpiLead : ''}`}
            data-overflow="ellipsis"
            data-tone={card.tone}
            title={String(card.value)}
          >
            {card.value}
          </span>
        </div>
      ))}
    </div>
  )
}
