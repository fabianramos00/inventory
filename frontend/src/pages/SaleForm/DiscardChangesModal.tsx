import { useState } from 'react'
import { createPortal } from 'react-dom'
import { AlertTriangle, X, Trash2 } from 'lucide-react'
import { useDialogFocus } from '@/hooks/useDialogFocus'
import styles from './DiscardChangesModal.module.css'

interface DiscardChangesModalProps {
  open: boolean
  itemCount: number
  totalAmount: string
  onStay: () => void
  onDiscard: () => void
}

export default function DiscardChangesModal(props: DiscardChangesModalProps) {
  if (!props.open) return null
  return <DiscardChangesDialog {...props} />
}

function DiscardChangesDialog({ itemCount, totalAmount, onStay, onDiscard }: DiscardChangesModalProps) {
  const [isClosing, setIsClosing] = useState(false)
  const dialogRef = useDialogFocus(handleSafeClose)

  function handleSafeClose() {
    setIsClosing(true)
    setTimeout(() => {
      onStay()
    }, 150)
  }

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
        ref={dialogRef}
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
            className={styles.safeBtn}
            onClick={handleSafeClose}
            type="button"
            data-autofocus
          >
            Continuar editando
          </button>
          <button
            className={styles.discardBtn}
            onClick={onDiscard}
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
