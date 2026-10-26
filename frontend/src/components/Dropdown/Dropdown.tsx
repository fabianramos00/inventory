import { useState, useRef, useEffect } from 'react'
import { ChevronDown } from 'lucide-react'
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
  const [isOpen, setIsOpen] = useState(false)
  const dropdownRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    function handleClickOutside(event: MouseEvent) {
      if (dropdownRef.current && !dropdownRef.current.contains(event.target as Node)) {
        setIsOpen(false)
      }
    }

    document.addEventListener('mousedown', handleClickOutside)
    return () => document.removeEventListener('mousedown', handleClickOutside)
  }, [])

  const selectedOption = options.find(opt => opt.value === value)
  const displayLabel = selectedOption?.label || placeholder

  function handleSelectOption(optionValue: string) {
    onChange(optionValue)
    setIsOpen(false)
  }

  return (
    <div className={styles.selectDropdown} ref={dropdownRef}>
      <button
        type="button"
        className={`${styles.selectTrigger} ${value ? styles.selectTriggerActive : ''}`}
        onClick={() => setIsOpen(!isOpen)}
        aria-label={ariaLabel}
        data-autofocus={autoFocus ? '' : undefined}
      >
        <span>{displayLabel}</span>
        <ChevronDown size={16} />
      </button>

      {isOpen && (
        <div className={styles.selectContent}>
          <div className={styles.selectOptions}>
            {options.length === 0 ? (
              <div className={styles.emptyOption}>No hay opciones disponibles</div>
            ) : (
              options.map(option => (
                <button
                  key={option.value}
                  type="button"
                  className={`${styles.selectOption} ${value === option.value ? styles.selectOptionActive : ''}`}
                  onClick={() => handleSelectOption(option.value)}
                >
                  {option.label}
                </button>
              ))
            )}
          </div>
        </div>
      )}
    </div>
  )
}
