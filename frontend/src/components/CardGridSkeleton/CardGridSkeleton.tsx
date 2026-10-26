import styles from './CardGridSkeleton.module.css'

export default function CardGridSkeleton({ label }: { label: string }) {
  return (
    <div className={styles.grid} role="status" aria-label={label}>
      {Array.from({ length: 6 }).map((_, index) => (
        <div key={index} className={styles.card}>
          <div className={styles.identity}>
            <span className={styles.monogram} />
            <span className={styles.name} />
          </div>
          <span className={styles.line} />
          <span className={styles.lineShort} />
        </div>
      ))}
    </div>
  )
}
