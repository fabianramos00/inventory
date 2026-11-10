import {
  useEffect,
  useId,
  useRef,
  useState,
  type ChangeEvent,
  type FocusEvent,
  type KeyboardEvent,
} from 'react'
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

type SegmentKey = 'day' | 'month' | 'year'

interface Segments {
  day: string
  month: string
  year: string
}

const EMPTY_SEGMENTS: Segments = { day: '', month: '', year: '' }

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

function formatIso(value: string) {
  if (!value) return ''
  const [year, month, day] = value.split('-')
  return `${day}/${month}/${year}`
}

function segmentsFromIso(value: string): Segments {
  if (!value) return EMPTY_SEGMENTS
  const [year, month, day] = value.split('-')
  return { day: day ?? '', month: month ?? '', year: year ?? '' }
}

function isoFromSegments({ day, month, year }: Segments) {
  if (!day || !month || year.length !== 4) return null
  const dayNumber = Number(day)
  const monthNumber = Number(month)
  const yearNumber = Number(year)
  const date = new Date(yearNumber, monthNumber - 1, dayNumber)
  if (
    date.getFullYear() !== yearNumber ||
    date.getMonth() !== monthNumber - 1 ||
    date.getDate() !== dayNumber
  ) {
    return null
  }
  return toIso(date)
}

export default function DateFilterInput({ value, onChange, label }: DateFilterInputProps) {
  const [isOpen, setIsOpen] = useState(false)
  const [segments, setSegments] = useState<Segments>(() => segmentsFromIso(value))
  const [isInvalid, setIsInvalid] = useState(false)
  const wrapperRef = useRef<HTMLDivElement>(null)
  const dayRef = useRef<HTMLInputElement>(null)
  const monthRef = useRef<HTMLInputElement>(null)
  const yearRef = useRef<HTMLInputElement>(null)
  const calendarButtonRef = useRef<HTMLButtonElement>(null)
  const dialogId = useId()

  useEffect(() => {
    if (wrapperRef.current?.contains(document.activeElement)) return
    setSegments(segmentsFromIso(value))
    setIsInvalid(false)
  }, [value])

  const displayValue = formatIso(value)

  function close(restoreFocus = false) {
    setIsOpen(false)
    if (restoreFocus) calendarButtonRef.current?.focus()
  }

  useClickOutside(wrapperRef, () => close(), isOpen)

  const segmentRefs = { day: dayRef, month: monthRef, year: yearRef }

  function focusSegment(key: SegmentKey) {
    segmentRefs[key].current?.focus()
  }

  function commit(next: Segments) {
    setSegments(next)

    if (!next.day || !next.month || next.year.length !== 4) {
      setIsInvalid(false)
      onChange('')
      return
    }

    const iso = isoFromSegments(next)
    if (!iso) {
      setIsInvalid(true)
      return
    }

    setIsInvalid(false)
    if (iso !== value) onChange(iso)
  }

  function handleSelect(day: Date | undefined) {
    if (!day) return
    const iso = toIso(day)
    setSegments(segmentsFromIso(iso))
    setIsInvalid(false)
    onChange(iso)
    close(true)
  }

  function handleSegmentChange(key: SegmentKey, event: ChangeEvent<HTMLInputElement>) {
    const maxLength = key === 'year' ? 4 : 2
    const digits = event.target.value.replace(/\D/g, '').slice(0, maxLength)
    const next = { ...segments, [key]: digits }
    commit(next)

    if (key !== 'year' && digits.length === maxLength) {
      focusSegment(key === 'day' ? 'month' : 'year')
    }
  }

  function handleSegmentKeyDown(key: SegmentKey, event: KeyboardEvent<HTMLInputElement>) {
    if (key !== 'day' && event.key === 'Backspace' && segments[key] === '') {
      event.preventDefault()
      focusSegment(key === 'month' ? 'day' : 'month')
      return
    }

    if (event.key === 'ArrowLeft' && key !== 'day') {
      event.preventDefault()
      focusSegment(key === 'month' ? 'day' : 'month')
      return
    }

    if (event.key === 'ArrowRight' && key !== 'year') {
      event.preventDefault()
      focusSegment(key === 'day' ? 'month' : 'year')
    }
  }

  function handleSegmentBlur(event: FocusEvent<HTMLInputElement>) {
    const next = event.relatedTarget as Node | null
    if (
      next === dayRef.current ||
      next === monthRef.current ||
      next === yearRef.current
    ) {
      return
    }

    if (!segments.day || !segments.month || segments.year.length !== 4) {
      setIsInvalid(false)
      return
    }

    const iso = isoFromSegments(segments)
    if (iso) {
      setIsInvalid(false)
      setSegments({
        day: segments.day.padStart(2, '0'),
        month: segments.month.padStart(2, '0'),
        year: segments.year.padStart(4, '0'),
      })
      return
    }

    const restored = segmentsFromIso(value)
    setIsInvalid(false)
    setSegments(isoFromSegments(restored) ? restored : EMPTY_SEGMENTS)
  }

  return (
    <div
      className={`${styles.dateFilterInput} ${value ? styles.filterActive : ''}`}
      ref={wrapperRef}
      aria-invalid={isInvalid}
    >
      <span className={styles.labelText}>{label}</span>
      <div className={styles.segments}>
        <input
          ref={dayRef}
          type="text"
          className={styles.segmentInput}
          value={segments.day}
          onChange={event => handleSegmentChange('day', event)}
          onKeyDown={event => handleSegmentKeyDown('day', event)}
          onBlur={handleSegmentBlur}
          placeholder="dd"
          aria-label={`${label} día`}
          inputMode="numeric"
          autoComplete="off"
          maxLength={2}
        />
        <span className={styles.separator} aria-hidden="true">/</span>
        <input
          ref={monthRef}
          type="text"
          className={styles.segmentInput}
          value={segments.month}
          onChange={event => handleSegmentChange('month', event)}
          onKeyDown={event => handleSegmentKeyDown('month', event)}
          onBlur={handleSegmentBlur}
          placeholder="mm"
          aria-label={`${label} mes`}
          inputMode="numeric"
          autoComplete="off"
          maxLength={2}
        />
        <span className={styles.separator} aria-hidden="true">/</span>
        <input
          ref={yearRef}
          type="text"
          className={`${styles.segmentInput} ${styles.segmentYear}`}
          value={segments.year}
          onChange={event => handleSegmentChange('year', event)}
          onKeyDown={event => handleSegmentKeyDown('year', event)}
          onBlur={handleSegmentBlur}
          placeholder="aaaa"
          aria-label={`${label} año`}
          inputMode="numeric"
          autoComplete="off"
          maxLength={4}
        />
      </div>
      <button
        ref={calendarButtonRef}
        type="button"
        className={styles.calendarButton}
        onClick={() => (isOpen ? close() : setIsOpen(true))}
        aria-expanded={isOpen}
        aria-haspopup="dialog"
        aria-controls={dialogId}
        aria-label={`Seleccionar ${label.toLowerCase()}${displayValue ? `: ${displayValue}` : ''}`}
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
            required
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
