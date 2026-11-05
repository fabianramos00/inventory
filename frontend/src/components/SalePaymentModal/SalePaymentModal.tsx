import { useId, useState } from 'react'
import { createPortal } from 'react-dom'
import { Loader, X } from 'lucide-react'
import styles from './SalePaymentModal.module.css'
import Dropdown from '@/components/Dropdown/Dropdown'
import { salesApi } from '@/lib/api/sales'
import { useDialogFocus } from '@/hooks/useDialogFocus'
import { getAPIErrorMessage } from '@/lib/apiError'
import { formatCurrency } from '@/lib/utils'
import type { PaymentMethod, Sale } from '@/types'

const PAYMENT_METHOD_OPTIONS = [
  { value: 'cash', label: 'Efectivo' },
  { value: 'debit', label: 'Débito' },
  { value: 'credit', label: 'Crédito' },
]

interface SalePaymentModalProps {
  sale: Sale
  onClose: () => void
  onSuccess: (updatedSale: Sale, amount: number) => void
}

function parsePaymentAmount(raw: string): number {
  const normalized = raw.trim().replace(',', '.')
  return normalized === '' ? Number.NaN : Number(normalized)
}

function validatePaymentAmount(raw: string, debt: number): string | null {
  if (!/^\d*[.,]?\d*$/.test(raw.trim())) return 'Ingresa un monto válido.'
  const [, decimals = ''] = raw.trim().replace(',', '.').split('.')
  if (decimals.length > 2) return 'Usa máximo dos decimales.'
  const amount = parsePaymentAmount(raw)
  if (!Number.isFinite(amount) || amount <= 0) return 'Ingresa un monto mayor a 0.'
  if (amount > debt) return `El monto no puede superar el saldo pendiente de ${formatCurrency(debt)}.`
  return null
}

export default function SalePaymentModal({ sale, onClose, onSuccess }: SalePaymentModalProps) {
  const debt = Math.max(sale.debt_amount, 0)
  const [amount, setAmount] = useState(() => debt.toFixed(2))
  const [paymentMethod, setPaymentMethod] = useState('')
  const [amountError, setAmountError] = useState<string | null>(null)
  const [methodError, setMethodError] = useState<string | null>(null)
  const [apiError, setApiError] = useState<string | null>(null)
  const [isSaving, setIsSaving] = useState(false)
  const titleId = useId()
  const amountId = useId()
  const amountHelpId = useId()
  const amountErrorId = useId()
  const methodErrorId = useId()

  function handleClose() {
    if (!isSaving) onClose()
  }

  const dialogRef = useDialogFocus(handleClose)
  const parsedAmount = parsePaymentAmount(amount)
  const remaining = Math.max(debt - (Number.isFinite(parsedAmount) ? parsedAmount : 0), 0)

  function handleAmountChange(value: string) {
    if (!/^\d*[.,]?\d*$/.test(value)) return
    setAmount(value)
    setAmountError(null)
    setApiError(null)
  }

  async function handleSubmit(event: React.FormEvent) {
    event.preventDefault()
    if (isSaving) return

    const nextAmountError = validatePaymentAmount(amount, debt)
    const nextMethodError = paymentMethod ? null : 'Selecciona un método de pago.'
    setAmountError(nextAmountError)
    setMethodError(nextMethodError)
    setApiError(null)
    if (nextAmountError || nextMethodError) return

    setIsSaving(true)
    try {
      const response = await salesApi.addSalePayment(sale.id, {
        amount: parsePaymentAmount(amount),
        payment_method: paymentMethod as Exclude<PaymentMethod, null>,
      })
      onSuccess(response.data, parsePaymentAmount(amount))
    } catch (err) {
      setApiError(getAPIErrorMessage(err, 'No se pudo registrar el pago. Intenta de nuevo.'))
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
            <p className={styles.subline}>
              Venta #{sale.id} · {sale.client?.name ?? 'Sin cliente'}
            </p>
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
          <div>
            <dt>Total</dt>
            <dd>{formatCurrency(sale.total_amount)}</dd>
          </div>
        </dl>

        {apiError && <div className={styles.apiError} role="alert">{apiError}</div>}

        <form onSubmit={handleSubmit} className={styles.form}>
          <div className={styles.field}>
            <label htmlFor={amountId} className={styles.label}>Monto del pago</label>
            <input
              id={amountId}
              type="text"
              inputMode="decimal"
              className={`form-input ${styles.amountInput} ${amountError ? styles.inputError : ''}`}
              value={amount}
              onChange={event => handleAmountChange(event.target.value)}
              onFocus={event => event.currentTarget.select()}
              onBlur={() => setAmountError(validatePaymentAmount(amount, debt))}
              disabled={isSaving}
              aria-invalid={!!amountError}
              aria-describedby={`${amountHelpId}${amountError ? ` ${amountErrorId}` : ''}`}
              data-autofocus
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
            Saldo después del pago: <strong>{formatCurrency(remaining)}</strong>
          </p>

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
