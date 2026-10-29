import { useId, useState } from 'react'
import { createPortal } from 'react-dom'
import { Loader, Trash2, X } from 'lucide-react'
import styles from './ConfirmDeleteModal.module.css'
import { useDialogFocus } from '@/hooks/useDialogFocus'
import { getApiErrorDetail } from '@/lib/apiError'

interface ConfirmDeleteModalProps {
  productName: string
  onConfirm: () => Promise<void>
  onClose: () => void
  title: string
}

export default function ConfirmDeleteModal({ productName, onConfirm, onClose, title }: ConfirmDeleteModalProps) {
  const [isDeleting, setIsDeleting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const titleId = useId()

  function handleClose() {
    if (!isDeleting) onClose()
  }

  const dialogRef = useDialogFocus(handleClose)

  async function handleConfirm() {
    setIsDeleting(true)
    setError(null)
    try {
      await onConfirm()
    } catch (err: unknown) {
      setError(
        getApiErrorDetail(err)
        || (err instanceof Error ? err.message : '')
        || 'Error al eliminar. Intenta nuevamente.'
      )
      setIsDeleting(false)
    }
  }

  return createPortal(
    <div className={styles.backdrop} data-scrim onClick={handleClose}>
      <div
        ref={dialogRef}
        className={styles.modal}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        onClick={(e) => e.stopPropagation()}
      >
        <div className={styles.header}>
          <div className={styles.titleGroup}>
            <Trash2 size={18} className={styles.titleIcon} />
            <h2 id={titleId} className={styles.title}>{title}</h2>
          </div>
          <button
            className={styles.closeButton}
            onClick={handleClose}
            type="button"
            aria-label="Cerrar"
            disabled={isDeleting}
          >
            <X size={20} />
          </button>
        </div>

        <div className={styles.body}>
          <p className={styles.message}>
            ¿Estás seguro que deseas eliminar <strong>"{productName}"</strong>?
          </p>
          <p className={styles.warning}>Esta acción no puede deshacerse.</p>
        </div>

        {error && <div className={styles.error} role="alert">{error}</div>}

        <div className={styles.actions}>
          <button
            className={styles.cancelBtn}
            onClick={handleClose}
            type="button"
            data-autofocus
            disabled={isDeleting}
          >
            Cancelar
          </button>
          <button className={styles.deleteBtn} onClick={handleConfirm} type="button" disabled={isDeleting}>
            {isDeleting ? (
              <><Loader size={14} className={styles.spinner} /> Eliminando...</>
            ) : (
              <><Trash2 size={14} /> Eliminar</>
            )}
          </button>
        </div>
      </div>
    </div>,
    document.body
  )
}
