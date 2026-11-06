import { useState, useEffect, useId, useRef, type KeyboardEvent } from 'react'
import { createPortal } from 'react-dom'
import { Loader, ChevronDown, Plus } from 'lucide-react'
import { useClickOutside } from '@/hooks/useClickOutside'
import { useFloatingMenu } from '@/hooks/useFloatingMenu'
import { focusListOption } from '@/hooks/useListbox'
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
  const triggerRef = useRef<HTMLButtonElement>(null)
  const panelRef = useRef<HTMLDivElement>(null)
  const listRef = useRef<HTMLDivElement>(null)
  const searchRef = useRef<HTMLInputElement>(null)
  const listId = useId()
  const debounceRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)
  const fetchRef = useRef(fetchOptions)
  const onOpenChangeRef = useRef(onOpenChange)

  const { panelProps } = useFloatingMenu({
    open: isOpen,
    anchor: triggerRef,
    panel: panelRef,
    width: 'trigger',
  })

  useClickOutside(containerRef, () => onOpenChangeRef.current(null), isOpen, panelRef)

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
    searchRef.current?.focus({ preventScroll: true })
  }, [isOpen])

  const selectedOption =
    options.find(o => o.id === value) ??
    initialOptions?.find(o => o.id === value)
  const displayLabel = selectedOption
    ? (formatLabel ? formatLabel(selectedOption) : selectedOption.name)
    : 'Seleccionar'

  function closeMenu() {
    onOpenChangeRef.current(null)
    triggerRef.current?.focus({ preventScroll: true })
  }

  function handleListKeyDown(event: KeyboardEvent<HTMLElement>) {
    if (event.key === 'Escape') {
      event.preventDefault()
      // The menu may live inside a modal: only the menu closes.
      event.stopPropagation()
      closeMenu()
      return
    }
    if (event.key === 'Tab') {
      // The menu is portaled, so it is no longer a DOM sibling of the trigger.
      event.preventDefault()
      closeMenu()
      return
    }
    if (event.key === 'ArrowDown') {
      event.preventDefault()
      focusListOption(listRef.current, 1)
    } else if (event.key === 'ArrowUp') {
      event.preventDefault()
      focusListOption(listRef.current, -1)
    }
  }

  function handleSearchKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === 'Escape') {
      event.preventDefault()
      event.stopPropagation()
      closeMenu()
      return
    }
    if (event.key === 'ArrowDown') {
      event.preventDefault()
      focusListOption(listRef.current, 1)
    }
  }

  function handleTriggerKeyDown(event: KeyboardEvent<HTMLButtonElement>) {
    if (event.key === 'ArrowDown') {
      event.preventDefault()
      onOpenChangeRef.current(id)
      requestAnimationFrame(() => focusListOption(listRef.current, 1))
    } else if (event.key === 'Escape') {
      onOpenChangeRef.current(null)
    }
  }

  return (
    <div className={styles.container} ref={containerRef}>
      <div className={styles.row}>
        <div className={`${styles.dropdownWrapper} ${isOpen ? styles.dropdownWrapperOpen : ''}`}>
          <button
            ref={triggerRef}
            type="button"
            className={styles.dropdownButton}
            onClick={() => onOpenChange(isOpen ? null : id)}
            onKeyDown={handleTriggerKeyDown}
            disabled={disabled}
            aria-expanded={isOpen}
            aria-haspopup="listbox"
            aria-controls={listId}
          >
            <span>{displayLabel}</span>
            <ChevronDown size={16} />
          </button>

          {isOpen && createPortal(
            <div {...panelProps} ref={panelRef} className={styles.dropdownMenu}>
              <input
                ref={searchRef}
                type="text"
                className={styles.dropdownSearch}
                placeholder={`Buscar ${label}...`}
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                onKeyDown={handleSearchKeyDown}
              />
              {loading && (
                <div className={styles.dropdownLoading}>
                  <Loader size={16} className={styles.spinner} />
                </div>
              )}
              {!loading && options.length === 0 && (
                <div className={styles.dropdownEmpty}>Sin opciones</div>
              )}
              <div id={listId} role="listbox" ref={listRef}>
                {!loading && options.map(option => (
                  <button
                    key={option.id}
                    type="button"
                    role="option"
                    aria-selected={value === option.id}
                    tabIndex={-1}
                    className={`${styles.dropdownOption} ${value === option.id ? styles.dropdownOptionActive : ''}`}
                    onClick={() => {
                      onSelect(option)
                      onOpenChange(null)
                      setSearch('')
                    }}
                    onKeyDown={handleListKeyDown}
                  >
                    {formatLabel ? formatLabel(option) : option.name}
                  </button>
                ))}
              </div>
            </div>,
            document.body,
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
