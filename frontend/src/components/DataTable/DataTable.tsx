import type { ReactNode } from 'react'
import { Loader, Inbox } from 'lucide-react'
import styles from './DataTable.module.css'
import Pagination from '@/components/Pagination'
import ErrorState from '@/components/ErrorState/ErrorState'

interface DataTableProps {
  children: ReactNode
  loading: boolean
  empty: boolean
  error?: boolean
  errorText?: string
  onRetry?: () => void
  loadingText?: string
  emptyText?: string
  emptyAction?: ReactNode
  minWidth?: string
  currentPage?: number
  hasNext?: boolean
  onPageChange?: (page: number) => void
  isPaginationLoading?: boolean
  skeletonRows?: number
  skeletonColumns?: number
  stickyFirstColumn?: boolean
}

export default function DataTable({
  children,
  loading,
  empty,
  error = false,
  errorText = 'No se pudieron cargar los datos.',
  onRetry,
  loadingText = 'Cargando datos...',
  emptyText = 'No se encontraron registros.',
  emptyAction,
  minWidth,
  currentPage,
  hasNext,
  onPageChange,
  isPaginationLoading,
  skeletonRows,
  skeletonColumns = 5,
  stickyFirstColumn = false,
}: DataTableProps) {
  if (loading) {
    if (skeletonRows && skeletonRows > 0) {
      return (
        <div className={styles.skeletonContainer}>
          <table
            className={`data-table ${stickyFirstColumn ? 'sticky-first' : ''}`}
            style={minWidth ? { minWidth } : undefined}
          >
            <thead>
              <tr>
                {Array.from({ length: skeletonColumns }).map((_, i) => (
                  <th scope="col" key={i}><div className={styles.skeletonHead} /></th>
                ))}
              </tr>
            </thead>
            <tbody>
              {Array.from({ length: skeletonRows }).map((_, row) => (
                <tr key={row}>
                  {Array.from({ length: skeletonColumns }).map((_, col) => (
                    <td key={col}>
                      <div className={styles.skeletonCell} />
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )
    }
    return (
      <div className={styles.loadingState}>
        <Loader size={20} className={styles.spinner} />
        <p>{loadingText}</p>
      </div>
    )
  }

  if (error) {
    return <ErrorState message={errorText} onRetry={onRetry} />
  }

  if (empty) {
    return (
      <div className={styles.emptyState}>
        <Inbox size={24} className={styles.emptyIcon} />
        <p>{emptyText}</p>
        {emptyAction && <div className={styles.emptyAction}>{emptyAction}</div>}
      </div>
    )
  }

  const hasPagination =
    currentPage !== undefined &&
    onPageChange !== undefined &&
    hasNext !== undefined

  return (
    <>
      <div className={styles.tableScroll}>
        <table
          className={`data-table ${stickyFirstColumn ? 'sticky-first' : ''}`}
          style={minWidth ? { minWidth } : undefined}
        >
          {children}
        </table>
      </div>
      {hasPagination && (
        <Pagination
          currentPage={currentPage}
          hasNext={hasNext}
          onPageChange={onPageChange}
          isLoading={isPaginationLoading}
        />
      )}
    </>
  )
}
