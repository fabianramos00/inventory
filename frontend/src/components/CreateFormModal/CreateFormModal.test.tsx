import { useState } from 'react'
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import CreateFormModal, { type FieldConfig } from './CreateFormModal'

const FIELDS: FieldConfig[] = [
  { key: 'name', label: 'Nombre', placeholder: 'Nombre del proveedor', required: true },
  { key: 'email', label: 'Correo electrónico', type: 'email' },
  {
    key: 'category',
    label: 'Categoría',
    type: 'select',
    options: [{ value: 'a', label: 'Categoría A' }],
  },
]

function Harness() {
  const [open, setOpen] = useState(false)
  return (
    <>
      <button type="button" onClick={() => setOpen(true)}>Abrir</button>
      {open && (
        <CreateFormModal
          title="Nuevo Proveedor"
          fields={FIELDS}
          onSubmit={vi.fn().mockResolvedValue({})}
          onClose={() => setOpen(false)}
          onCreated={vi.fn()}
        />
      )}
    </>
  )
}

describe('CreateFormModal', () => {
  it('exposes dialog semantics and labels every field', () => {
    render(<Harness />)
    fireEvent.click(screen.getByRole('button', { name: 'Abrir' }))

    expect(screen.getByRole('dialog', { name: 'Nuevo Proveedor' })).toBeTruthy()
    expect(screen.getByLabelText('Nombre')).toBeTruthy()
    expect(screen.getByLabelText('Correo electrónico')).toBeTruthy()
    expect(screen.getByLabelText('Categoría')).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Categoría' })).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Cerrar' })).toBeTruthy()
    expect(document.activeElement).toBe(screen.getByLabelText('Nombre'))
  })

  it('closes with Escape and restores focus to the trigger', () => {
    render(<Harness />)
    const trigger = screen.getByRole('button', { name: 'Abrir' })
    trigger.focus()
    fireEvent.click(trigger)

    fireEvent.keyDown(document, { key: 'Escape' })

    expect(screen.queryByRole('dialog')).toBeNull()
    expect(document.activeElement).toBe(trigger)
  })

  it('wraps Tab and Shift+Tab inside the dialog', () => {
    render(<Harness />)
    fireEvent.click(screen.getByRole('button', { name: 'Abrir' }))

    const closeButton = screen.getByRole('button', { name: 'Cerrar' })
    const saveButton = screen.getByRole('button', { name: 'Guardar' })

    saveButton.focus()
    fireEvent.keyDown(document, { key: 'Tab' })
    expect(document.activeElement).toBe(closeButton)

    fireEvent.keyDown(document, { key: 'Tab', shiftKey: true })
    expect(document.activeElement).toBe(saveButton)
  })
})
