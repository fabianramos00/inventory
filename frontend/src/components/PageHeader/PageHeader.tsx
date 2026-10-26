import type { ReactNode } from 'react'
import styles from './PageHeader.module.css'

interface PageHeaderProps {
  activeLabel: string
  title: string
  subtitle?: string
  finding?: string
  action?: ReactNode
}

export default function PageHeader({ activeLabel, title, subtitle, finding, action }: PageHeaderProps) {
  return (
    <div className={styles.pageHeader}>
      <div className={styles.heading}>
        <div className={styles.breadcrumb}>{activeLabel}</div>
        <h1 className={styles.pageTitle}>{title}</h1>
        {finding && <p className={styles.finding}>{finding}</p>}
        {subtitle && <p className={styles.subtitle}>{subtitle}</p>}
      </div>
      {action && <div className={styles.action}>{action}</div>}
    </div>
  )
}
