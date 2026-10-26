import { AlertCircle, RefreshCw } from 'lucide-react'
import styles from './ErrorState.module.css'

interface ErrorStateProps {
  message: string
  onRetry?: () => void
  className?: string
}

export default function ErrorState({ message, onRetry, className }: ErrorStateProps) {
  return (
    <div className={`${styles.errorState} ${className ?? ''}`} role="alert">
      <AlertCircle size={18} className={styles.errorIcon} />
      <p className={styles.message}>{message}</p>
      {onRetry && (
        <button type="button" className={styles.retryButton} onClick={onRetry}>
          <RefreshCw size={14} />
          Reintentar
        </button>
      )}
    </div>
  )
}
