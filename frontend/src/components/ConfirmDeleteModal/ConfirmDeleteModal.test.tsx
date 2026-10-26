import { useState } from 'react'
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import ConfirmDeleteModal from './ConfirmDeleteModal'

function Harness({ onConfirm }: { onConfirm: () => Promise<void> }) {
  const [open, setOpen] = useState(false)
  return (
    <>
      <button type="button" onClick={() => setOpen(true)}>Abrir</button>
      {open && (
        <ConfirmDeleteModal
          title="Eliminar Cliente"
          productName="Cliente Uno"
          onConfirm={onConfirm}
          onClose={() => setOpen(false)}
        />
      )}
    </>
  )
}

describe('ConfirmDeleteModal', () => {
  it('shows the title, focuses Cancelar and restores focus on Escape', () => {
    render(<Harness onConfirm={vi.fn().mockResolvedValue(undefined)} />)
    const trigger = screen.getByRole('button', { name: 'Abrir' })
    trigger.focus()
    fireEvent.click(trigger)

    expect(screen.getByRole('dialog', { name: 'Eliminar Cliente' })).toBeTruthy()
    expect(screen.getByText(/Cliente Uno/)).toBeTruthy()
    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Cancelar' }))

    fireEvent.keyDown(document, { key: 'Escape' })

    expect(screen.queryByRole('dialog')).toBeNull()
    expect(document.activeElement).toBe(trigger)
  })

  it('ignores Escape while deleting', async () => {
    const onConfirm = vi.fn(() => new Promise<void>(() => {}))
    render(<Harness onConfirm={onConfirm} />)
    fireEvent.click(screen.getByRole('button', { name: 'Abrir' }))

    fireEvent.click(screen.getByRole('button', { name: 'Eliminar' }))
    await screen.findByRole('button', { name: 'Eliminando...' })

    fireEvent.keyDown(document, { key: 'Escape' })

    expect(screen.getByRole('dialog', { name: 'Eliminar Cliente' })).toBeTruthy()
  })
})
