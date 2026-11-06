import { useId, useRef } from 'react'
import { createPortal } from 'react-dom'
import { ChevronDown } from 'lucide-react'
import { useClickOutside } from '@/hooks/useClickOutside'
import { useFloatingMenu } from '@/hooks/useFloatingMenu'
import { useListbox } from '@/hooks/useListbox'
import styles from './FilterSelect.module.css'

interface FilterOption {
  value: string
  label: string
}

interface FilterSelectProps {
  label: string
  value: string
  options: FilterOption[]
  onChange: (value: string) => void
  emptyLabel?: string
}

export default function FilterSelect({
  label,
  value,
  options,
  onChange,
  emptyLabel = 'Todos',
}: FilterSelectProps) {
  const wrapperRef = useRef<HTMLDivElement>(null)
  const listId = useId()
  const { isOpen, setIsOpen, triggerRef, listRef, close, handleKeyDown, handleTriggerKeyDown } = useListbox()
  const { panelProps } = useFloatingMenu({ open: isOpen, anchor: triggerRef, panel: listRef, width: 'content' })

  useClickOutside(wrapperRef, () => close(), isOpen, listRef)

  const selectedLabel = options.find(option => option.value === value)?.label || label

  function select(next: string) {
    onChange(next)
    close(true)
  }

  return (
    <div className={styles.wrapper} ref={wrapperRef}>
      <button
        ref={triggerRef}
        type="button"
        className={`${styles.trigger} ${value ? styles.active : ''}`}
        onClick={() => (isOpen ? close() : setIsOpen(true))}
        onKeyDown={handleTriggerKeyDown}
        aria-expanded={isOpen}
        aria-haspopup="listbox"
        aria-controls={listId}
      >
        <span>{selectedLabel}</span>
        <ChevronDown
          size={14}
          className={`${styles.chevron} ${isOpen ? styles.chevronOpen : ''}`}
        />
      </button>

      {isOpen && createPortal(
        <div {...panelProps} ref={listRef} id={listId} role="listbox" className={styles.dropdown}>
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
          {options.map(option => (
            <button
              key={option.value}
              type="button"
              role="option"
              aria-selected={value === option.value}
              tabIndex={-1}
              className={`${styles.option} ${value === option.value ? styles.optionActive : ''}`}
              onClick={() => select(option.value)}
              onKeyDown={handleKeyDown}
            >
              {option.label}
            </button>
          ))}
        </div>,
        document.body,
      )}
    </div>
  )
}
