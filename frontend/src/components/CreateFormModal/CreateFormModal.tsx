import { useId, useState } from 'react'
import { createPortal } from 'react-dom'
import { Loader, X } from 'lucide-react'
import { z } from 'zod'
import styles from './CreateFormModal.module.css'
import Dropdown, { type DropdownOption } from '@/components/Dropdown/Dropdown'
import { useDialogFocus } from '@/hooks/useDialogFocus'
import { getAPIErrorMessage } from '@/lib/apiError'

export interface FieldConfig {
  key: string
  label: string
  placeholder?: string
  type?: 'text' | 'email' | 'tel' | 'url' | 'textarea' | 'password' | 'select'
  required?: boolean
  options?: { value: string; label: string }[]
}

interface CreateFormModalProps<T> {
  title: string
  fields: FieldConfig[]
  onSubmit: (values: Record<string, string>) => Promise<T>
  onClose: () => void
  onCreated: (result: T) => void
  initialValues?: Record<string, string>
  validationSchema?: z.ZodSchema
}

export default function CreateFormModal<T>({
  title,
  fields,
  onSubmit,
  onClose,
  onCreated,
  initialValues,
  validationSchema,
}: CreateFormModalProps<T>) {
  const [values, setValues] = useState<Record<string, string>>(() => {
    if (initialValues) return initialValues
    return Object.fromEntries(fields.map(f => [f.key, '']))
  })
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const titleId = useId()
  const fieldIdPrefix = useId()
  const dialogRef = useDialogFocus(onClose)

  function setValue(key: string, value: string) {
    setValues(prev => ({ ...prev, [key]: value }))
  }

  async function handleSave() {
    if (validationSchema) {
      const result = validationSchema.safeParse(values)
      if (!result.success) {
        setError(result.error.issues[0]?.message ?? 'Datos inválidos')
        return
      }
    }
    for (const field of fields) {
      if (field.required && !values[field.key].trim()) {
        setError(`El campo "${field.label}" es requerido`)
        return
      }
    }
    setLoading(true)
    setError(null)
    try {
      const result = await onSubmit(
        Object.fromEntries(Object.entries(values).map(([k, v]) => [k, v.trim()]))
      )
      onCreated(result)
    } catch (err) {
      setError(getAPIErrorMessage(err, 'Error al guardar'))
    } finally {
      setLoading(false)
    }
  }

  return createPortal(
    <div className={styles.modalBackdrop} data-scrim onClick={onClose}>
      <div
        ref={dialogRef}
        className={styles.modalContent}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        onClick={e => e.stopPropagation()}
      >
        <div className={styles.modalHeader}>
          <h2 id={titleId} className={styles.modalTitle}>{title}</h2>
          <button
            className={styles.modalCloseButton}
            onClick={onClose}
            type="button"
            aria-label="Cerrar"
          >
            <X size={20} />
          </button>
        </div>
        {error && <div className={styles.modalError} role="alert">{error}</div>}
        <div className={styles.modalForm}>
          {fields.map((field, i) => {
            const fieldId = `${fieldIdPrefix}-${field.key}`
            return (
              <div key={field.key} className={styles.inputGroup}>
                <label
                  className={styles.inputLabel}
                  htmlFor={field.type === 'select' ? undefined : fieldId}
                >
                  {field.label}
                </label>
                {field.type === 'textarea' ? (
                  <textarea
                    id={fieldId}
                    className={styles.inputField}
                    placeholder={field.placeholder}
                    value={values[field.key]}
                    onChange={e => setValue(field.key, e.target.value)}
                    data-autofocus={i === 0 ? '' : undefined}
                    rows={3}
                  />
                ) : field.type === 'select' ? (
                  <Dropdown
                    value={values[field.key]}
                    options={(field.options || []) as DropdownOption[]}
                    onChange={v => setValue(field.key, v)}
                    placeholder={field.placeholder}
                    ariaLabel={field.label}
                    autoFocus={i === 0}
                  />
                ) : (
                  <input
                    id={fieldId}
                    type={field.type ?? 'text'}
                    className={styles.inputField}
                    placeholder={field.placeholder}
                    value={values[field.key]}
                    onChange={e => setValue(field.key, e.target.value)}
                    data-autofocus={i === 0 ? '' : undefined}
                  />
                )}
              </div>
            )
          })}
        </div>
        <div className={styles.modalActions}>
          <button className={styles.modalCancelBtn} onClick={onClose} type="button">
            Cancelar
          </button>
          <button
            className={styles.modalSaveBtn}
            onClick={handleSave}
            disabled={loading}
            type="button"
          >
            {loading ? <><Loader size={14} className={styles.spinner} /> Guardando...</> : 'Guardar'}
          </button>
        </div>
      </div>
    </div>,
    document.body
  )
}
