import { useEffect, useId, useRef, type ChangeEvent, type KeyboardEvent } from 'react'
import { ChevronDown } from 'lucide-react'
import type { DropdownProps } from 'react-day-picker'
import { useClickOutside } from '@/hooks/useClickOutside'
import { useListbox } from '@/hooks/useListbox'
import styles from './CalendarDropdown.module.css'

export default function CalendarDropdown({
  options = [],
  value,
  onChange,
  disabled,
  className,
  style,
  reverse = false,
  ...rest
}: DropdownProps & { reverse?: boolean }) {
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

  useClickOutside(wrapperRef, () => close(), isOpen)

  useEffect(() => {
    if (!isOpen) return
    listRef.current?.querySelector<HTMLElement>('[aria-selected="true"]')?.scrollIntoView({ block: 'nearest' })
  }, [isOpen, listRef])

  const selected = options.find(option => String(option.value) === String(value))
  const visibleOptions = reverse ? [...options].reverse() : options

  function select(option: (typeof options)[number]) {
    onChange?.({ target: { value: String(option.value) } } as ChangeEvent<HTMLSelectElement>)
    close(true)
  }

  function handleKeyDown(event: KeyboardEvent<HTMLElement>) {
    if (event.key === 'Escape') {
      event.preventDefault()
      close(true)
      return
    }
    handleListKeyDown(event)
  }

  function handleTriggerKeyDown(event: KeyboardEvent<HTMLButtonElement>) {
    if (event.key === 'Escape') {
      close(true)
      return
    }
    handleTriggerListKeyDown(event)
  }

  return (
    <div ref={wrapperRef} className={`${styles.root} ${className ?? ''}`} style={style}>
      <button
        ref={triggerRef}
        type="button"
        className={styles.trigger}
        onClick={() => (isOpen ? close() : setIsOpen(true))}
        onKeyDown={handleTriggerKeyDown}
        disabled={disabled}
        aria-expanded={isOpen}
        aria-haspopup="listbox"
        aria-controls={listId}
        aria-label={rest['aria-label']}
      >
        <span>{selected?.label ?? value}</span>
        <ChevronDown
          size={12}
          className={`${styles.chevron} ${isOpen ? styles.chevronOpen : ''}`}
        />
      </button>

      {isOpen && (
        <div className={styles.list} id={listId} role="listbox" ref={listRef}>
          {visibleOptions.map(option => (
            <button
              key={option.value}
              type="button"
              role="option"
              aria-selected={String(option.value) === String(value)}
              tabIndex={-1}
              disabled={option.disabled}
              className={`${styles.option} ${String(option.value) === String(value) ? styles.optionActive : ''}`}
              onClick={() => select(option)}
              onKeyDown={handleKeyDown}
            >
              {option.label}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
