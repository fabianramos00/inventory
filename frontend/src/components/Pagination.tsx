import { ChevronLeft, ChevronRight } from 'lucide-react'
import styles from './Pagination.module.css'

interface PaginationProps {
  currentPage: number
  totalPages?: number
  totalItems?: number
  pageSize?: number
  onPageChange: (page: number) => void
  isLoading?: boolean
  hasNext?: boolean
}

export default function Pagination({
  currentPage,
  totalPages,
  totalItems,
  pageSize = 10,
  onPageChange,
  isLoading = false,
  hasNext,
}: PaginationProps) {
  const usingCursor = hasNext !== undefined
  const canGoPrev = currentPage > 1
  const canGoNext = usingCursor ? hasNext : currentPage < (totalPages ?? 0)

  if (usingCursor ? !canGoPrev && !canGoNext : (totalPages ?? 0) <= 1) return null

  const rangeStart = (currentPage - 1) * pageSize + 1
  const rangeEnd = totalItems !== undefined
    ? Math.min(currentPage * pageSize, totalItems)
    : rangeStart + pageSize - 1
  const range = totalItems !== undefined && totalItems > 0
    ? `${rangeStart}–${rangeEnd} de ${totalItems}`
    : null

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
        <span className={styles.paginationInfo}>
          {usingCursor ? `Página ${currentPage}` : `Página ${currentPage} de ${totalPages}`}
        </span>
        {range && <span className={styles.paginationRange}>{range}</span>}
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
