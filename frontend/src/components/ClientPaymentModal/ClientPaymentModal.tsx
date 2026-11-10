import { useEffect, useId, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { Loader, X } from 'lucide-react'
import styles from './ClientPaymentModal.module.css'
import Dropdown from '@/components/Dropdown/Dropdown'
import { useDialogFocus } from '@/hooks/useDialogFocus'
import { getAPIErrorMessage } from '@/lib/apiError'
import { formatCurrency } from '@/lib/utils'
import type { Client, ClientPaymentResult, PaymentMethod } from '@/types'

const PAYMENT_METHOD_OPTIONS = [
  { value: 'cash', label: 'Efectivo' },
  { value: 'debit', label: 'Débito' },
  { value: 'credit', label: 'Crédito' },
]

interface ClientPaymentModalProps {
  client: Pick<Client, 'id' | 'name' | 'debt_amount'>
  onClose: () => void
  onSubmit: (data: { amount: number; payment_method: Exclude<PaymentMethod, null> }) => Promise<ClientPaymentResult>
  onSuccess: (result: ClientPaymentResult) => void
}

function parseAmount(raw: string): number {
  const normalized = raw.trim().replace(',', '.')
  return normalized === '' ? Number.NaN : Number(normalized)
}

function validateAmount(raw: string, debt: number): string | null {
  if (!/^\d*[.,]?\d*$/.test(raw.trim())) return 'Ingresa un monto válido.'
  const [, decimals = ''] = raw.trim().replace(',', '.').split('.')
  if (decimals.length > 2) return 'Usa máximo dos decimales.'
  const amount = parseAmount(raw)
  if (!Number.isFinite(amount) || amount <= 0) return 'El monto debe ser mayor que cero.'
  if (amount > debt) return 'El monto no puede superar el saldo pendiente.'
  return null
}

export default function ClientPaymentModal({ client, onClose, onSubmit, onSuccess }: ClientPaymentModalProps) {
  const debt = Math.max(client.debt_amount, 0)
  const [mode, setMode] = useState<'full' | 'partial'>('full')
  const [amount, setAmount] = useState(debt.toFixed(2))
  const [paymentMethod, setPaymentMethod] = useState('')
  const [amountError, setAmountError] = useState<string | null>(null)
  const [methodError, setMethodError] = useState<string | null>(null)
  const [apiError, setApiError] = useState<string | null>(null)
  const [isSaving, setIsSaving] = useState(false)
  const titleId = useId()
  const modeName = useId()
  const amountId = useId()
  const amountHelpId = useId()
  const amountErrorId = useId()
  const methodErrorId = useId()
  const amountRef = useRef<HTMLInputElement>(null)

  function handleClose() {
    if (!isSaving) onClose()
  }

  const dialogRef = useDialogFocus(handleClose)

  useEffect(() => {
    if (mode === 'partial') amountRef.current?.focus()
  }, [mode])

  const parsedAmount = mode === 'full' ? debt : parseAmount(amount)
  const remaining = Math.max(debt - (Number.isFinite(parsedAmount) ? parsedAmount : 0), 0)

  function selectMode(next: 'full' | 'partial') {
    setMode(next)
    setAmountError(null)
    setApiError(null)
    if (next === 'full') setAmount(debt.toFixed(2))
    else if (amount === debt.toFixed(2)) setAmount('')
  }

  async function handleSubmit(event: React.FormEvent) {
    event.preventDefault()
    if (isSaving) return

    const nextAmountError = mode === 'full' ? null : validateAmount(amount, debt)
    const nextMethodError = paymentMethod ? null : 'Selecciona un método de pago.'
    setAmountError(nextAmountError)
    setMethodError(nextMethodError)
    setApiError(null)
    if (nextAmountError || nextMethodError) return

    setIsSaving(true)
    try {
      const result = await onSubmit({
        amount: mode === 'full' ? debt : parseAmount(amount),
        payment_method: paymentMethod as Exclude<PaymentMethod, null>,
      })
      onSuccess(result)
    } catch (err) {
      setApiError(getAPIErrorMessage(err, 'No se pudo registrar el pago. Intente de nuevo.'))
      setIsSaving(false)
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
        onClick={event => event.stopPropagation()}
      >
        <div className={styles.header}>
          <div>
            <h2 id={titleId} className={styles.title}>Registrar pago</h2>
            <p className={styles.subline}>{client.name}</p>
          </div>
          <button
            type="button"
            className={styles.closeButton}
            onClick={handleClose}
            aria-label="Cerrar"
            disabled={isSaving}
          >
            <X size={20} />
          </button>
        </div>

        <dl className={styles.context}>
          <div>
            <dt>Saldo pendiente</dt>
            <dd>{formatCurrency(debt)}</dd>
          </div>
        </dl>

        {apiError && <div className={styles.apiError} role="alert">{apiError}</div>}

        <form onSubmit={handleSubmit} className={styles.form}>
          <fieldset className={styles.modeGroup} disabled={isSaving}>
            <legend className={styles.label}>Tipo de pago</legend>
            <label className={styles.modeOption}>
              <input
                type="radio"
                name={modeName}
                value="full"
                checked={mode === 'full'}
                onChange={() => selectMode('full')}
                data-autofocus
              />
              Pagar todo
            </label>
            <label className={styles.modeOption}>
              <input
                type="radio"
                name={modeName}
                value="partial"
                checked={mode === 'partial'}
                onChange={() => selectMode('partial')}
              />
              Pago parcial
            </label>
          </fieldset>

          <div className={styles.field}>
            <label htmlFor={amountId} className={styles.label}>Monto</label>
            <input
              id={amountId}
              ref={amountRef}
              type="text"
              inputMode="decimal"
              className={`form-input ${styles.amountInput} ${amountError ? styles.inputError : ''}`}
              value={mode === 'full' ? debt.toFixed(2) : amount}
              onChange={event => {
                if (!/^\d*[.,]?\d*$/.test(event.target.value)) return
                setAmount(event.target.value)
                setAmountError(null)
                setApiError(null)
              }}
              onBlur={() => mode === 'partial' && setAmountError(validateAmount(amount, debt))}
              readOnly={mode === 'full'}
              disabled={isSaving}
              aria-invalid={!!amountError}
              aria-describedby={`${amountHelpId}${amountError ? ` ${amountErrorId}` : ''}`}
            />
            <span id={amountHelpId} className={styles.helper}>Máx. {formatCurrency(debt)}</span>
            {amountError && <span id={amountErrorId} className={styles.fieldError} role="alert">{amountError}</span>}
          </div>

          <div className={`${styles.field} ${isSaving ? styles.disabled : ''}`}>
            <span className={styles.label}>Método de pago</span>
            <Dropdown
              value={paymentMethod}
              options={PAYMENT_METHOD_OPTIONS}
              onChange={value => {
                setPaymentMethod(value)
                setMethodError(null)
                setApiError(null)
              }}
              placeholder="Selecciona un método"
              ariaLabel="Método de pago"
            />
            {methodError && <span id={methodErrorId} className={styles.fieldError} role="alert">{methodError}</span>}
          </div>

          <p className={styles.remaining}>
            Saldo restante: <strong>{formatCurrency(remaining)}</strong>
          </p>
          <p className={styles.note}>El pago se aplica a las ventas más antiguas primero.</p>

          <div className={styles.actions}>
            <button type="button" className="btn-ghost" onClick={handleClose} disabled={isSaving}>
              Cancelar
            </button>
            <button type="submit" className="btn-primary" disabled={isSaving}>
              {isSaving ? <><Loader size={15} className={styles.spinner} /> Registrando…</> : 'Registrar pago'}
            </button>
          </div>
        </form>
      </div>
    </div>,
    document.body,
  )
}
