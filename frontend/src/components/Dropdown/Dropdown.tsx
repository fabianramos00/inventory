import { useEffect, useId, useRef } from 'react'
import { createPortal } from 'react-dom'
import { ChevronDown } from 'lucide-react'
import { useClickOutside } from '@/hooks/useClickOutside'
import { useFloatingMenu } from '@/hooks/useFloatingMenu'
import { useListbox } from '@/hooks/useListbox'
import styles from './Dropdown.module.css'

export interface DropdownOption {
  value: string
  label: string
}

interface DropdownProps {
  value: string
  options: DropdownOption[]
  onChange: (value: string) => void
  placeholder?: string
  ariaLabel?: string
  autoFocus?: boolean
}

export default function Dropdown({
  value,
  options,
  onChange,
  placeholder = 'Seleccionar...',
  ariaLabel,
  autoFocus,
}: DropdownProps) {
  const wrapperRef = useRef<HTMLDivElement>(null)
  const panelRef = useRef<HTMLDivElement>(null)
  const listId = useId()
  const { isOpen, setIsOpen, triggerRef, listRef, close, handleKeyDown, handleTriggerKeyDown } = useListbox()
  const { panelProps } = useFloatingMenu({ open: isOpen, anchor: triggerRef, panel: panelRef, width: 'trigger' })

  useClickOutside(wrapperRef, () => close(), isOpen, panelRef)

  useEffect(() => {
    if (!isOpen) return
    const option = listRef.current?.querySelector<HTMLElement>('[role="option"]')
    option?.focus({ preventScroll: true })
    if (option && typeof option.scrollIntoView === 'function') {
      option.scrollIntoView({ block: 'nearest' })
    }
  }, [isOpen, listRef])

  const selectedOption = options.find(opt => opt.value === value)
  const displayLabel = selectedOption?.label || placeholder

  function handleSelectOption(optionValue: string) {
    onChange(optionValue)
    close(true)
  }

  return (
    <div className={styles.selectDropdown} ref={wrapperRef}>
      <button
        ref={triggerRef}
        type="button"
        className={`${styles.selectTrigger} ${value ? styles.selectTriggerActive : ''}`}
        onClick={() => (isOpen ? close() : setIsOpen(true))}
        onKeyDown={handleTriggerKeyDown}
        aria-label={ariaLabel}
        aria-expanded={isOpen}
        aria-haspopup="listbox"
        aria-controls={listId}
        data-autofocus={autoFocus ? '' : undefined}
      >
        <span>{displayLabel}</span>
        <ChevronDown size={16} />
      </button>

      {isOpen && createPortal(
        <div {...panelProps} ref={panelRef} className={styles.selectContent}>
          <div className={styles.selectOptions} id={listId} role="listbox" ref={listRef}>
            {options.length === 0 ? (
              <div className={styles.emptyOption}>No hay opciones disponibles</div>
            ) : (
              options.map(option => (
                <button
                  key={option.value}
                  type="button"
                  role="option"
                  aria-selected={value === option.value}
                  tabIndex={-1}
                  className={`${styles.selectOption} ${value === option.value ? styles.selectOptionActive : ''}`}
                  onClick={() => handleSelectOption(option.value)}
                  onKeyDown={handleKeyDown}
                >
                  {option.label}
                </button>
              ))
            )}
          </div>
        </div>,
        document.body,
      )}
    </div>
  )
}
