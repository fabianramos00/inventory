import type { ReactNode } from 'react'
import { Search, XCircle } from 'lucide-react'
import styles from './CommandBar.module.css'

interface CommandBarProps {
  search?: string
  onSearchChange?: (val: string) => void
  placeholder?: string
  showSearch?: boolean
  hasActiveFilters?: boolean
  onClearFilters?: () => void
  children?: ReactNode
}

export default function CommandBar({ search = '', onSearchChange, placeholder = 'Buscar...', showSearch = true, hasActiveFilters, onClearFilters, children }: CommandBarProps) {
  return (
    <div className={styles.commandBar}>
      <div className={styles.controlsBar}>
        {showSearch && (
          <div className={styles.searchWrapper}>
            <Search size={14} className={styles.searchIcon} />
            <input
              type="text"
              className={styles.searchInput}
              placeholder={placeholder}
              value={search}
              onChange={e => onSearchChange?.(e.target.value)}
            />
          </div>
        )}
        <div className={styles.actions}>
          {children}
          {hasActiveFilters && onClearFilters && (
            <button className={styles.clearFiltersBtn} onClick={onClearFilters}>
              <XCircle size={13} />
              Limpiar filtros
            </button>
          )}
        </div>
      </div>
    </div>
  )
}
