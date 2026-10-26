import { useState, useEffect, useRef } from 'react'
import { Loader, ChevronDown, Plus } from 'lucide-react'
import type { FilterOption } from '@/types'
import styles from './ProductDropdown.module.css'

interface ProductDropdownProps {
  id: string
  label: string
  value: number | null | undefined
  openDropdownId: string | null
  onOpenChange: (id: string | null) => void
  fetchOptions: (search: string) => Promise<FilterOption[]>
  onSelect: (option: FilterOption) => void
  onAddNew: () => void
  formatLabel?: (option: FilterOption) => string
  initialOptions?: FilterOption[]
  error?: string
  disabled?: boolean
}

export default function ProductDropdown({
  id,
  label,
  value,
  openDropdownId,
  onOpenChange,
  fetchOptions,
  onSelect,
  onAddNew,
  formatLabel,
  initialOptions,
  error,
  disabled,
}: ProductDropdownProps) {
  const isOpen = openDropdownId === id
  const [options, setOptions] = useState<FilterOption[]>(initialOptions ?? [])
  const [search, setSearch] = useState('')
  const [loading, setLoading] = useState(false)
  const containerRef = useRef<HTMLDivElement>(null)
  const debounceRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)
  const fetchRef = useRef(fetchOptions)
  const onOpenChangeRef = useRef(onOpenChange)

  useEffect(() => {
    fetchRef.current = fetchOptions
    onOpenChangeRef.current = onOpenChange
  })

  useEffect(() => {
    if (!initialOptions?.length) return
    // eslint-disable-next-line react-hooks/set-state-in-effect -- merge prop-provided options without dropping fetched ones
    setOptions(prev => {
      const prevIds = new Set(prev.map(o => o.id))
      const newItems = initialOptions.filter(o => !prevIds.has(o.id))
      return newItems.length > 0 ? [...prev, ...newItems] : prev
    })
  }, [initialOptions])

  useEffect(() => {
    if (!isOpen) return
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(async () => {
      setLoading(true)
      try {
        const results = await fetchRef.current(search)
        setOptions(results)
      } catch {
        setOptions([])
      } finally {
        setLoading(false)
      }
    }, 300)
    return () => { if (debounceRef.current) clearTimeout(debounceRef.current) }
  }, [isOpen, search])

  useEffect(() => {
    if (!isOpen) return
    const handleMouseDown = (e: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        onOpenChangeRef.current(null)
      }
    }
    document.addEventListener('mousedown', handleMouseDown)
    return () => document.removeEventListener('mousedown', handleMouseDown)
  }, [isOpen])

  const selectedOption =
    options.find(o => o.id === value) ??
    initialOptions?.find(o => o.id === value)
  const displayLabel = selectedOption
    ? (formatLabel ? formatLabel(selectedOption) : selectedOption.name)
    : 'Seleccionar'

  return (
    <div className={styles.container} ref={containerRef}>
      <div className={styles.row}>
        <div className={`${styles.dropdownWrapper} ${isOpen ? styles.dropdownWrapperOpen : ''}`}>
          <button
            type="button"
            className={styles.dropdownButton}
            onClick={() => onOpenChange(isOpen ? null : id)}
            disabled={disabled}
          >
            <span>{displayLabel}</span>
            <ChevronDown size={16} />
          </button>
          
          {isOpen && (
            <div className={styles.dropdownMenu}>
              <input
                type="text"
                className={styles.dropdownSearch}
                placeholder={`Buscar ${label}...`}
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                autoFocus
              />
              {loading && (
                <div className={styles.dropdownLoading}>
                  <Loader size={16} className={styles.spinner} />
                </div>
              )}
              {!loading && options.length === 0 && (
                <div className={styles.dropdownEmpty}>Sin opciones</div>
              )}
              {!loading && options.map(option => (
                <button
                  key={option.id}
                  type="button"
                  className={`${styles.dropdownOption} ${value === option.id ? styles.dropdownOptionActive : ''}`}
                  onClick={() => {
                    onSelect(option)
                    onOpenChange(null)
                    setSearch('')
                  }}
                >
                  {formatLabel ? formatLabel(option) : option.name}
                </button>
              ))}
            </div>
          )}
        </div>

        <button
          type="button"
          className={styles.addButton}
          onClick={onAddNew}
          disabled={disabled}
          title={`Agregar nuevo ${label}`}
        >
          <Plus size={16} />
        </button>
      </div>
      {error && <p className={styles.errorMessage}>{error}</p>}
    </div>
  )
}
