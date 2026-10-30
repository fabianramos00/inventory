import { useId, useRef, useState } from 'react'
import { DayPicker, type DropdownProps } from 'react-day-picker'
import { es } from 'react-day-picker/locale'
import { CalendarDays } from 'lucide-react'
import { useClickOutside } from '@/hooks/useClickOutside'
import CalendarDropdown from './CalendarDropdown'
import 'react-day-picker/style.css'
import styles from './DateFilterInput.module.css'

interface DateFilterInputProps {
  value: string
  onChange: (value: string) => void
  label: string
}

function toDate(value: string) {
  return value ? new Date(`${value}T00:00:00`) : undefined
}

function YearDropdown(props: DropdownProps) {
  return <CalendarDropdown {...props} reverse />
}

function toIso(date: Date) {
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${date.getFullYear()}-${month}-${day}`
}

export default function DateFilterInput({ value, onChange, label }: DateFilterInputProps) {
  const [isOpen, setIsOpen] = useState(false)
  const wrapperRef = useRef<HTMLDivElement>(null)
  const calendarButtonRef = useRef<HTMLButtonElement>(null)
  const dialogId = useId()

  function close(restoreFocus = false) {
    setIsOpen(false)
    if (restoreFocus) calendarButtonRef.current?.focus()
  }

  useClickOutside(wrapperRef, () => close(), isOpen)

  function handleSelect(day: Date | undefined) {
    if (!day) return
    onChange(toIso(day))
    close(true)
  }

  return (
    <div className={`${styles.dateFilterInput} ${value ? styles.filterActive : ''}`} ref={wrapperRef}>
      <span className={styles.labelText}>{label}</span>
      <input
        type="date"
        className={styles.dateInput}
        value={value}
        onChange={e => onChange(e.target.value)}
        title={label}
      />
      <button
        ref={calendarButtonRef}
        type="button"
        className={styles.calendarButton}
        onClick={() => (isOpen ? close() : setIsOpen(true))}
        aria-expanded={isOpen}
        aria-haspopup="dialog"
        aria-controls={dialogId}
        aria-label={`Seleccionar ${label.toLowerCase()}`}
      >
        <CalendarDays size={14} />
      </button>

      {isOpen && (
        <div
          id={dialogId}
          className={styles.calendarPopover}
          role="dialog"
          aria-label={label}
          onKeyDown={event => {
            if (event.key === 'Escape') {
              event.preventDefault()
              close(true)
            }
          }}
        >
          <DayPicker
            mode="single"
            autoFocus
            selected={toDate(value)}
            onSelect={handleSelect}
            defaultMonth={toDate(value) ?? new Date()}
            locale={es}
            captionLayout="dropdown"
            components={{ Dropdown: CalendarDropdown, YearsDropdown: YearDropdown }}
            className={styles.calendar}
            showOutsideDays
          />
        </div>
      )}
    </div>
  )
}
