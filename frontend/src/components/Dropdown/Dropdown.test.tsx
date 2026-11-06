import { fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { ReactNode } from 'react'
import Dropdown from './Dropdown'
import { useDialogFocus } from '@/hooks/useDialogFocus'

const options = [
  { value: 'cash', label: 'Efectivo' },
  { value: 'debit', label: 'Débito' },
]

function Dialog({ children }: { children: ReactNode }) {
  const dialogRef = useDialogFocus(() => {})
  return (
    <div ref={dialogRef} role="dialog">
      <button type="button">Antes</button>
      {children}
    </div>
  )
}

describe('Dropdown floating menu', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('renders the menu in the document body while it is open', () => {
    render(<Dropdown value="" options={options} onChange={vi.fn()} ariaLabel="Método de pago" />)

    expect(screen.queryByRole('listbox')).toBeNull()

    fireEvent.click(screen.getByRole('button', { name: 'Método de pago' }))

    const listbox = screen.getByRole('listbox')
    expect(listbox.parentElement?.parentElement).toBe(document.body)
  })

  it('selects a portaled option without losing the click', () => {
    const onChange = vi.fn()
    render(<Dropdown value="" options={options} onChange={onChange} ariaLabel="Método de pago" />)

    fireEvent.click(screen.getByRole('button', { name: 'Método de pago' }))
    const option = screen.getByRole('option', { name: 'Efectivo' })
    fireEvent.mouseDown(option)
    fireEvent.click(option)

    expect(onChange).toHaveBeenCalledWith('cash')
    expect(screen.queryByRole('listbox')).toBeNull()
  })

  it('keeps Escape inside the menu so a modal stays open', () => {
    const onDocumentEscape = vi.fn()
    document.addEventListener('keydown', onDocumentEscape)

    try {
      render(<Dropdown value="" options={options} onChange={vi.fn()} ariaLabel="Método de pago" />)
      fireEvent.click(screen.getByRole('button', { name: 'Método de pago' }))
      fireEvent.keyDown(screen.getByRole('option', { name: 'Efectivo' }), { key: 'Escape' })

      expect(onDocumentEscape).not.toHaveBeenCalled()
      expect(screen.queryByRole('listbox')).toBeNull()
      expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Método de pago' }))
    } finally {
      document.removeEventListener('keydown', onDocumentEscape)
    }
  })

  it('leaves no floating markup behind when it is closed', () => {
    render(<Dropdown value="" options={options} onChange={vi.fn()} ariaLabel="Método de pago" />)

    expect(document.querySelector('[data-floating-menu]')).toBeNull()
    expect(screen.queryByRole('listbox')).toBeNull()
  })

  it('does not pull the focus out of a portaled menu that belongs to the dialog', () => {
    render(
      <Dialog>
        <Dropdown value="" options={options} onChange={vi.fn()} ariaLabel="Método de pago" />
      </Dialog>,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Método de pago' }))
    const option = screen.getByRole('option', { name: 'Efectivo' })
    option.focus()

    fireEvent.keyDown(document, { key: 'Tab' })

    expect(document.activeElement).toBe(option)
  })
})
