import { useState } from 'react'
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import ProductDropdown from './ProductDropdown'
import type { FilterOption } from '@/types'

const options: FilterOption[] = [
  { id: 1, name: 'Cemento' },
  { id: 2, name: 'Arena' },
]

function Harness({ onSelect = vi.fn() }: { onSelect?: (option: FilterOption) => void }) {
  const [openDropdownId, setOpenDropdownId] = useState<string | null>(null)

  return (
    <ProductDropdown
      id="category"
      label="Categoría"
      value={null}
      openDropdownId={openDropdownId}
      onOpenChange={setOpenDropdownId}
      fetchOptions={vi.fn().mockResolvedValue(options)}
      onSelect={onSelect}
      onAddNew={vi.fn()}
      initialOptions={options}
    />
  )
}

function openMenu() {
  fireEvent.click(screen.getByRole('button', { name: 'Seleccionar' }))
  return screen.getByRole('listbox')
}

describe('ProductDropdown', () => {
  it('exposes the listbox semantics of the other menus', () => {
    render(<Harness />)

    const trigger = screen.getByRole('button', { name: 'Seleccionar' })
    expect(trigger.getAttribute('aria-expanded')).toBe('false')
    expect(trigger.getAttribute('aria-haspopup')).toBe('listbox')
    expect(trigger.getAttribute('aria-controls')).toBeTruthy()
    expect(screen.queryByRole('listbox')).toBeNull()
  })

  it('portals the menu to the document body and links it to the trigger', () => {
    render(<Harness />)

    const trigger = screen.getByRole('button', { name: 'Seleccionar' })
    const listbox = openMenu()

    expect(listbox.getAttribute('id')).toBe(trigger.getAttribute('aria-controls'))
    expect(listbox.closest('[data-floating-menu]')?.parentElement).toBe(document.body)
    expect(trigger.getAttribute('aria-expanded')).toBe('true')
    expect(screen.getAllByRole('option').map(option => option.textContent)).toEqual([
      'Cemento',
      'Arena',
    ])
  })

  it('selects a portaled option without losing the click', () => {
    const onSelect = vi.fn()
    render(<Harness onSelect={onSelect} />)

    openMenu()
    const option = screen.getByRole('option', { name: 'Cemento' })
    fireEvent.mouseDown(option)
    fireEvent.click(option)

    expect(onSelect).toHaveBeenCalledWith(options[0])
    expect(screen.queryByRole('listbox')).toBeNull()
  })

  it('moves the focus between the options with the arrow keys', () => {
    render(<Harness />)

    openMenu()
    const [first, second] = screen.getAllByRole('option')
    first.focus()

    fireEvent.keyDown(first, { key: 'ArrowDown' })
    expect(document.activeElement).toBe(second)

    fireEvent.keyDown(second, { key: 'ArrowUp' })
    expect(document.activeElement).toBe(first)
  })

  it('keeps Escape inside the menu so a modal stays open', () => {
    const onDocumentEscape = vi.fn()
    document.addEventListener('keydown', onDocumentEscape)

    try {
      render(<Harness />)
      openMenu()
      fireEvent.keyDown(screen.getByRole('option', { name: 'Cemento' }), { key: 'Escape' })

      expect(onDocumentEscape).not.toHaveBeenCalled()
      expect(screen.queryByRole('listbox')).toBeNull()
    } finally {
      document.removeEventListener('keydown', onDocumentEscape)
    }
  })
})
