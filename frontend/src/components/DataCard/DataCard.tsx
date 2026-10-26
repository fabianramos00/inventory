import type { ReactNode } from 'react'
import styles from './DataCard.module.css'

interface DataCardProps {
  title?: string
  action?: ReactNode
  children: ReactNode
  className?: string
}

export default function DataCard({ title, action, children, className }: DataCardProps) {
  return (
    <div className={`${styles.card} ${className ?? ''}`}>
      {(title || action) && (
        <div className={styles.header}>
          {title && <span className={styles.title}>{title}</span>}
          {action && <div>{action}</div>}
        </div>
      )}
      {children}
    </div>
  )
}
