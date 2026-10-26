import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import FilterSelect from './FilterSelect'

const options = [
  { value: 'a', label: 'Categoría A' },
  { value: 'b', label: 'Categoría B' },
]

describe('FilterSelect listbox behavior', () => {
  it('opens with ArrowDown and clamps focus while moving through options', () => {
    render(<FilterSelect label="Filtro" value="" options={options} onChange={vi.fn()} />)

    const trigger = screen.getByRole('button', { name: 'Filtro' })
    trigger.focus()

    fireEvent.keyDown(trigger, { key: 'ArrowDown' })
    expect(screen.getByRole('listbox')).toBeTruthy()

    fireEvent.keyDown(trigger, { key: 'ArrowDown' })
    expect(document.activeElement).toBe(screen.getByRole('option', { name: 'Todos' }))

    fireEvent.keyDown(document.activeElement!, { key: 'ArrowUp' })
    expect(document.activeElement).toBe(screen.getByRole('option', { name: 'Todos' }))

    fireEvent.keyDown(document.activeElement!, { key: 'ArrowDown' })
    expect(document.activeElement).toBe(screen.getByRole('option', { name: 'Categoría A' }))

    fireEvent.keyDown(document.activeElement!, { key: 'ArrowDown' })
    fireEvent.keyDown(document.activeElement!, { key: 'ArrowDown' })
    expect(document.activeElement).toBe(screen.getByRole('option', { name: 'Categoría B' }))

    fireEvent.keyDown(document.activeElement!, { key: 'Escape' })
    expect(screen.queryByRole('listbox')).toBeNull()
    expect(document.activeElement).toBe(trigger)
  })

  it('closes on outside click without moving focus back to the trigger', () => {
    render(
      <div>
        <FilterSelect label="Filtro" value="" options={options} onChange={vi.fn()} />
        <button type="button">Fuera</button>
      </div>,
    )

    const trigger = screen.getByRole('button', { name: 'Filtro' })
    fireEvent.click(trigger)
    expect(screen.getByRole('listbox')).toBeTruthy()

    const outside = screen.getByRole('button', { name: 'Fuera' })
    outside.focus()
    fireEvent.mouseDown(outside)

    expect(screen.queryByRole('listbox')).toBeNull()
    expect(document.activeElement).toBe(outside)
  })
})
