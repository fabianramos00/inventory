import { ChevronLeft, ChevronRight } from 'lucide-react'
import styles from './Pagination.module.css'

interface PaginationProps {
  currentPage: number
  onPageChange: (page: number) => void
  isLoading?: boolean
  hasNext?: boolean
}

export default function Pagination({
  currentPage,
  onPageChange,
  isLoading = false,
  hasNext = false,
}: PaginationProps) {
  const canGoPrev = currentPage > 1
  const canGoNext = hasNext

  if (!canGoPrev && !canGoNext) return null

  return (
    <nav className={styles.pagination} aria-label="Paginación">
      <button
        className={styles.paginationBtn}
        onClick={() => canGoPrev && onPageChange(currentPage - 1)}
        disabled={!canGoPrev || isLoading}
        aria-label="Ir a página anterior"
      >
        <ChevronLeft size={16} />
        <span className={styles.btnText}>Anterior</span>
      </button>

      <span className={styles.paginationMeta}>
        <span className={styles.paginationInfo}>Página {currentPage}</span>
      </span>

      <button
        className={styles.paginationBtn}
        onClick={() => canGoNext && onPageChange(currentPage + 1)}
        disabled={!canGoNext || isLoading}
        aria-label="Ir a página siguiente"
      >
        <span className={styles.btnText}>Siguiente</span>
        <ChevronRight size={16} />
      </button>
    </nav>
  )
}
