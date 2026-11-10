import { useState } from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import ClientPaymentModal from './ClientPaymentModal'
import { formatCurrency } from '@/lib/utils'
import type { ClientPaymentResult } from '@/types'

function result(overrides: Partial<ClientPaymentResult> = {}): ClientPaymentResult {
  return {
    client_id: 12,
    amount: 150,
    remaining_debt: 0,
    allocations: [],
    ...overrides,
  }
}

function renderModal(options: {
  debt?: number
  onSubmit?: (data: { amount: number; payment_method: 'cash' | 'credit' | 'debit' }) => Promise<ClientPaymentResult>
  onSuccess?: (value: ClientPaymentResult) => void
  onClose?: () => void
} = {}) {
  const onSubmit = options.onSubmit ?? vi.fn().mockResolvedValue(result())
  const onSuccess = options.onSuccess ?? vi.fn()
  const onClose = options.onClose ?? vi.fn()
  const view = render(
    <ClientPaymentModal
      client={{ id: 12, name: 'Ana Pérez', debt_amount: options.debt ?? 150 }}
      onSubmit={onSubmit}
      onSuccess={onSuccess}
      onClose={onClose}
    />,
  )
  return { ...view, onSubmit, onSuccess, onClose }
}

function chooseMethod(label = 'Efectivo') {
  fireEvent.click(screen.getByRole('button', { name: 'Método de pago' }))
  fireEvent.click(screen.getByRole('option', { name: label }))
}

function submit() {
  fireEvent.click(screen.getByRole('button', { name: 'Registrar pago' }))
}

describe('ClientPaymentModal', () => {
  it('prefills Pagar todo with the full debt and shows the oldest-first note', () => {
    renderModal()

    expect((screen.getByLabelText('Monto') as HTMLInputElement).value).toBe('150.00')
    expect(screen.getByText('El pago se aplica a las ventas más antiguas primero.')).toBeTruthy()
    expect(screen.getByText('Saldo restante:')).toBeTruthy()
    expect(screen.getByText(formatCurrency(0))).toBeTruthy()
  })

  it('pays the whole debt with one method', async () => {
    const onSubmit = vi.fn().mockResolvedValue(result())
    const onSuccess = vi.fn()
    renderModal({ onSubmit, onSuccess })

    chooseMethod('Efectivo')
    submit()

    await waitFor(() => expect(onSubmit).toHaveBeenCalledWith({ amount: 150, payment_method: 'cash' }))
    await waitFor(() => expect(onSuccess).toHaveBeenCalledWith(result()))
  })

  it('validates zero, overpayment and more than two decimals in partial mode', async () => {
    const { onSubmit } = renderModal()
    fireEvent.click(screen.getByRole('radio', { name: 'Pago parcial' }))

    const amount = screen.getByLabelText('Monto')
    fireEvent.change(amount, { target: { value: '0' } })
    submit()
    expect(await screen.findByText('El monto debe ser mayor que cero.')).toBeTruthy()

    fireEvent.change(amount, { target: { value: '200' } })
    submit()
    expect(await screen.findByText('El monto no puede superar el saldo pendiente.')).toBeTruthy()

    fireEvent.change(amount, { target: { value: '10.001' } })
    submit()
    expect(await screen.findByText('Usa máximo dos decimales.')).toBeTruthy()
    expect(onSubmit).not.toHaveBeenCalled()
  })

  it('requires a payment method', async () => {
    const { onSubmit } = renderModal()

    submit()

    expect(await screen.findByText('Selecciona un método de pago.')).toBeTruthy()
    expect(onSubmit).not.toHaveBeenCalled()
  })

  it('disables submit and close while the payment is pending', async () => {
    let resolvePayment: (value: ClientPaymentResult) => void = () => undefined
    const pending = new Promise<ClientPaymentResult>(resolve => {
      resolvePayment = resolve
    })
    const onSubmit = vi.fn().mockReturnValue(pending)
    const onSuccess = vi.fn()
    renderModal({ onSubmit, onSuccess })

    chooseMethod('Crédito')
    submit()

    const button = screen.getByRole('button', { name: 'Registrando…' }) as HTMLButtonElement
    expect(button.disabled).toBe(true)
    expect((screen.getByRole('button', { name: 'Cerrar' }) as HTMLButtonElement).disabled).toBe(true)
    expect((screen.getByRole('button', { name: 'Cancelar' }) as HTMLButtonElement).disabled).toBe(true)

    resolvePayment(result({ remaining_debt: 25, allocations: [{ sale_id: 41, amount: 125 }] }))
    await waitFor(() => expect(onSuccess).toHaveBeenCalled())
  })

  it('shows an inline API error and re-enables submit', async () => {
    const onSubmit = vi.fn().mockRejectedValue(Object.assign(new Error('Request failed'), {
      isAxiosError: true,
      response: { data: { detail: 'El monto no puede superar la deuda.' } },
    }))
    renderModal({ onSubmit })

    chooseMethod('Débito')
    submit()

    const alert = await screen.findByRole('alert')
    expect(alert.textContent).toBe('El monto no puede superar la deuda.')
    expect((screen.getByRole('button', { name: 'Registrar pago' }) as HTMLButtonElement).disabled).toBe(false)
  })

  it('focuses the amount in partial mode and Escape restores focus to the opener', () => {
    function Harness() {
      const [open, setOpen] = useState(false)
      return (
        <div>
          <button type="button" onClick={() => setOpen(true)}>Abrir pago</button>
          {open && (
            <ClientPaymentModal
              client={{ id: 12, name: 'Ana Pérez', debt_amount: 150 }}
              onSubmit={vi.fn().mockResolvedValue(result())}
              onSuccess={vi.fn()}
              onClose={() => setOpen(false)}
            />
          )}
        </div>
      )
    }

    render(<Harness />)
    const opener = screen.getByRole('button', { name: 'Abrir pago' })
    opener.focus()
    fireEvent.click(opener)

    fireEvent.click(screen.getByRole('radio', { name: 'Pago parcial' }))
    expect(document.activeElement).toBe(screen.getByLabelText('Monto'))

    fireEvent.keyDown(document, { key: 'Escape' })

    expect(screen.queryByRole('dialog')).toBeNull()
    expect(document.activeElement).toBe(opener)
  })
})
