import { useId, useRef, useState, type KeyboardEvent } from 'react'
import { ChevronDown, Loader } from 'lucide-react'
import { useClickOutside } from '@/hooks/useClickOutside'
import { useDebouncedFetch } from '@/hooks/useDebouncedFetch'
import { useListbox } from '@/hooks/useListbox'
import type { FilterOption } from '@/types'
import styles from './SearchSelect.module.css'

interface SearchSelectProps {
  label: string
  value: number | ''
  onChange: (value: number | '', option?: FilterOption) => void
  fetchOptions: (search: string) => Promise<FilterOption[]>
  emptyLabel: string
  specialOption?: { value: number; label: string }
  selectedOption?: FilterOption
  placeholder?: string
  className?: string
  ariaLabel?: string
}

export default function SearchSelect({
  label,
  value,
  onChange,
  fetchOptions,
  emptyLabel,
  specialOption,
  selectedOption: selectedOptionProp,
  placeholder,
  className,
  ariaLabel,
}: SearchSelectProps) {
  const [search, setSearch] = useState('')
  const [cachedSelected, setCachedSelected] = useState<FilterOption | null>(null)
  const wrapperRef = useRef<HTMLDivElement>(null)
  const listId = useId()
  const {
    isOpen,
    setIsOpen,
    triggerRef,
    listRef,
    close,
    handleKeyDown: handleListKeyDown,
    handleTriggerKeyDown: handleTriggerListKeyDown,
  } = useListbox()

  const { data: options, loading } = useDebouncedFetch(
    () => fetchOptions(search),
    [search, isOpen],
    300,
    isOpen,
  )

  function closeSearch(restoreFocus = false) {
    setSearch('')
    close(restoreFocus)
  }

  useClickOutside(wrapperRef, () => closeSearch(), isOpen)

  const selectedOption = cachedSelected?.id === value
    ? cachedSelected
    : selectedOptionProp?.id === value
      ? selectedOptionProp
      : options.find(option => option.id === value)
  const selectedLabel = specialOption && value === specialOption.value
    ? specialOption.label
    : selectedOption?.name ?? (value === '' ? placeholder ?? label : label)

  function select(next: number | '') {
    const chosen = options.find(item => item.id === next)
      ?? (specialOption && next === specialOption.value
        ? { id: specialOption.value, name: specialOption.label }
        : null)
    setCachedSelected(chosen)
    onChange(next, chosen ?? undefined)
    closeSearch(true)
  }

  function handleKeyDown(event: KeyboardEvent<HTMLElement>) {
    if (event.key === 'Escape') {
      event.preventDefault()
      closeSearch(true)
      return
    }
    handleListKeyDown(event)
  }

  function handleTriggerKeyDown(event: KeyboardEvent<HTMLButtonElement>) {
    if (event.key === 'Escape') {
      closeSearch(true)
      return
    }
    handleTriggerListKeyDown(event)
  }

  return (
    <div className={`${styles.wrapper} ${className ?? ''}`} ref={wrapperRef}>
      <button
        ref={triggerRef}
        type="button"
        className={`${styles.trigger} ${value !== '' ? styles.active : ''}`}
        onClick={() => (isOpen ? closeSearch() : setIsOpen(true))}
        onKeyDown={handleTriggerKeyDown}
        aria-expanded={isOpen}
        aria-haspopup="listbox"
        aria-controls={listId}
        aria-label={ariaLabel}
      >
        <span>{selectedLabel}</span>
        <ChevronDown
          size={14}
          className={`${styles.chevron} ${isOpen ? styles.chevronOpen : ''}`}
        />
      </button>

      {isOpen && (
        <div className={styles.dropdown}>
          <input
            type="text"
            className={styles.search}
            placeholder={`Buscar ${label.toLowerCase()}...`}
            value={search}
            onChange={event => setSearch(event.target.value)}
            onKeyDown={handleKeyDown}
            size={1}
            autoFocus
          />
          <div className={styles.options} ref={listRef} id={listId} role="listbox">
            <button
              type="button"
              role="option"
              aria-selected={value === ''}
              tabIndex={-1}
              className={`${styles.option} ${value === '' ? styles.optionActive : ''}`}
              onClick={() => select('')}
              onKeyDown={handleKeyDown}
            >
              {emptyLabel}
            </button>
            {specialOption && (
              <button
                type="button"
                role="option"
                aria-selected={value === specialOption.value}
                tabIndex={-1}
                className={`${styles.option} ${value === specialOption.value ? styles.optionActive : ''}`}
                onClick={() => select(specialOption.value)}
                onKeyDown={handleKeyDown}
              >
                {specialOption.label}
              </button>
            )}
            {loading ? (
              <div className={styles.loading}>
                <Loader size={14} className={styles.spinner} />
              </div>
            ) : options.length === 0 ? (
              <div className={styles.empty}>Sin resultados</div>
            ) : (
              options.map(option => (
                <button
                  key={option.id}
                  type="button"
                  role="option"
                  aria-selected={value === option.id}
                  tabIndex={-1}
                  className={`${styles.option} ${value === option.id ? styles.optionActive : ''}`}
                  onClick={() => select(option.id)}
                  onKeyDown={handleKeyDown}
                >
                  {option.name}
                </button>
              ))
            )}
          </div>
        </div>
      )}
    </div>
  )
}
