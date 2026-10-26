import { useState, useEffect, useRef, useCallback } from 'react'
import { createPortal } from 'react-dom'
import { AlertTriangle, X, Trash2 } from 'lucide-react'
import styles from './DiscardChangesModal.module.css'

interface DiscardChangesModalProps {
  open: boolean
  itemCount: number
  totalAmount: string
  onStay: () => void
  onDiscard: () => void
}

export default function DiscardChangesModal({
  open,
  itemCount,
  totalAmount,
  onStay,
  onDiscard,
}: DiscardChangesModalProps) {
  const [isClosing, setIsClosing] = useState(false)
  const safeBtnRef = useRef<HTMLButtonElement>(null)
  const discardBtnRef = useRef<HTMLButtonElement>(null)
  const closeBtnRef = useRef<HTMLButtonElement>(null)
  const triggerRef = useRef<Element | null>(null)

  const focusableRefs = useCallback(() => {
    return [
      safeBtnRef.current,
      discardBtnRef.current,
      closeBtnRef.current,
    ].filter(Boolean) as HTMLElement[]
  }, [])

  useEffect(() => {
    if (!open) return

    triggerRef.current = document.activeElement

    const raf = requestAnimationFrame(() => {
      safeBtnRef.current?.focus()
    })

    function handleKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape') {
        e.preventDefault()
        handleSafeClose()
        return
      }
      if (e.key === 'Tab') {
        const focusable = focusableRefs()
        if (focusable.length === 0) return

        const currentIndex = focusable.indexOf(document.activeElement as HTMLElement)
        if (e.shiftKey) {
          if (currentIndex <= 0) {
            e.preventDefault()
            focusable[focusable.length - 1]?.focus()
          }
        } else {
          if (currentIndex >= focusable.length - 1) {
            e.preventDefault()
            focusable[0]?.focus()
          }
        }
      }
    }

    document.addEventListener('keydown', handleKeyDown)
    return () => {
      document.removeEventListener('keydown', handleKeyDown)
      cancelAnimationFrame(raf)
    }
  }, [open, focusableRefs])

  useEffect(() => {
    if (open || isClosing) return
    if (triggerRef.current instanceof HTMLElement) {
      triggerRef.current.focus()
    }
  }, [open, isClosing])

  function handleSafeClose() {
    setIsClosing(true)
    setTimeout(() => {
      onStay()
    }, 150)
  }

  function handleDiscard() {
    onDiscard()
  }

  if (!open) return null

  const bodyText =
    itemCount === 1
      ? `Tienes 1 producto agregado por un total de ${totalAmount}. Todos los cambios se perderán.`
      : `Tienes ${itemCount} productos agregados por un total de ${totalAmount}. Todos los cambios se perderán.`

  return createPortal(
    <div
      className={`${styles.backdrop} ${isClosing ? styles.backdropClosing : ''}`}
      data-scrim
      onClick={isClosing ? undefined : handleSafeClose}
    >
      <div
        className={`${styles.modal} ${isClosing ? styles.modalClosing : ''}`}
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
        aria-labelledby="discard-changes-title"
        aria-describedby="discard-changes-body"
      >
        <div className={styles.header}>
          <div className={styles.titleGroup}>
            <AlertTriangle size={18} className={styles.warningIcon} aria-hidden="true" />
            <h2 className={styles.title} id="discard-changes-title">
              ¿Descartar la venta?
            </h2>
          </div>
          <button
            ref={closeBtnRef}
            className={styles.closeButton}
            onClick={handleSafeClose}
            type="button"
            aria-label="Cerrar"
          >
            <X size={20} />
          </button>
        </div>

        <p className={styles.body} id="discard-changes-body">
          <span>{bodyText.split(totalAmount)[0]}</span>
          <span className={styles.monoAmount}>{totalAmount}</span>
          <span>{bodyText.split(totalAmount)[1]}</span>
        </p>

        <div className={styles.actions}>
          <button
            ref={safeBtnRef}
            className={styles.safeBtn}
            onClick={handleSafeClose}
            type="button"
          >
            Continuar editando
          </button>
          <button
            ref={discardBtnRef}
            className={styles.discardBtn}
            onClick={handleDiscard}
            type="button"
          >
            <Trash2 size={14} aria-hidden="true" />
            Descartar venta
          </button>
        </div>
      </div>
    </div>,
    document.body
  )
}
