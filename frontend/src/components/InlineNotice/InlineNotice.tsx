import { useEffect, useRef, useState } from 'react'
import { X } from 'lucide-react'
import type { FocusEvent, ReactNode, Ref } from 'react'
import styles from './InlineNotice.module.css'

export type NoticeTone = 'success' | 'error' | 'info'

interface InlineNoticeProps {
  tone: NoticeTone
  onClose: () => void
  autoCloseMs?: number
  children: ReactNode
  ref?: Ref<HTMLDivElement>
}

const DEFAULT_AUTO_CLOSE_MS = 5000

export default function InlineNotice({
  tone,
  onClose,
  autoCloseMs,
  children,
  ref,
}: InlineNoticeProps) {
  const [hovered, setHovered] = useState(false)
  const [focused, setFocused] = useState(false)
  const paused = hovered || focused
  const closedRef = useRef(false)
  const timerRef = useRef<number | null>(null)
  const onCloseRef = useRef(onClose)
  const resolvedAutoCloseMs =
    autoCloseMs ?? (tone === 'success' ? DEFAULT_AUTO_CLOSE_MS : 0)

  useEffect(() => {
    onCloseRef.current = onClose
  }, [onClose])

  useEffect(() => {
    if (paused || resolvedAutoCloseMs <= 0 || closedRef.current) return
    const timer = window.setTimeout(() => {
      if (closedRef.current) return
      closedRef.current = true
      onCloseRef.current()
    }, resolvedAutoCloseMs)
    timerRef.current = timer
    return () => {
      window.clearTimeout(timer)
      if (timerRef.current === timer) timerRef.current = null
    }
  }, [paused, resolvedAutoCloseMs])

  function handleClose() {
    if (closedRef.current) return
    closedRef.current = true
    if (timerRef.current !== null) {
      window.clearTimeout(timerRef.current)
      timerRef.current = null
    }
    onCloseRef.current()
  }

  function handleFocus(event: FocusEvent<HTMLDivElement>) {
    // Programmatic focus on the container itself must not freeze the timer;
    // only focus inside the notice (e.g. the close button) pauses it.
    if (event.target !== event.currentTarget) setFocused(true)
  }

  function handleBlur(event: FocusEvent<HTMLDivElement>) {
    if (!event.currentTarget.contains(event.relatedTarget as Node | null)) {
      setFocused(false)
    }
  }

  return (
    <div
      ref={ref}
      className={`${styles.notice} ${styles[tone]}`}
      role={tone === 'error' ? 'alert' : 'status'}
      aria-live={tone === 'error' ? undefined : 'polite'}
      aria-atomic="true"
      tabIndex={-1}
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
      onFocus={handleFocus}
      onBlur={handleBlur}
    >
      <span className={styles.message}>{children}</span>
      <button
        type="button"
        className={styles.close}
        onClick={handleClose}
        aria-label="Cerrar notificación"
      >
        <X size={16} aria-hidden="true" />
      </button>
    </div>
  )
}
