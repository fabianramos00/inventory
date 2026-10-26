import { fireEvent, render, screen } from '@testing-library/react'
import { beforeAll, describe, expect, it, vi } from 'vitest'
import CalendarDropdown from './CalendarDropdown'

beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn()
})

const options = [
  { value: 1, label: 'Enero', disabled: false },
  { value: 2, label: 'Febrero', disabled: false },
]

describe('CalendarDropdown listbox behavior', () => {
  it('opens with ArrowDown and moves focus into the options', () => {
    render(<CalendarDropdown options={options} value={1} onChange={vi.fn()} />)

    const trigger = screen.getByRole('button', { name: 'Enero' })
    trigger.focus()

    fireEvent.keyDown(trigger, { key: 'ArrowDown' })
    expect(screen.getByRole('listbox')).toBeTruthy()

    fireEvent.keyDown(trigger, { key: 'ArrowDown' })
    expect(document.activeElement).toBe(screen.getByRole('option', { name: 'Enero' }))

    fireEvent.keyDown(document.activeElement!, { key: 'ArrowDown' })
    expect(document.activeElement).toBe(screen.getByRole('option', { name: 'Febrero' }))

    fireEvent.keyDown(document.activeElement!, { key: 'ArrowDown' })
    expect(document.activeElement).toBe(screen.getByRole('option', { name: 'Febrero' }))

    fireEvent.keyDown(document.activeElement!, { key: 'Escape' })
    expect(screen.queryByRole('listbox')).toBeNull()
    expect(document.activeElement).toBe(trigger)
  })

  it('lets trigger Escape bubble to ancestor handlers, as before', () => {
    const onAncestorKeyDown = vi.fn()
    render(
      <div onKeyDown={onAncestorKeyDown}>
        <CalendarDropdown options={options} value={1} onChange={vi.fn()} />
      </div>,
    )

    const trigger = screen.getByRole('button', { name: 'Enero' })
    fireEvent.click(trigger)
    expect(screen.getByRole('listbox')).toBeTruthy()

    fireEvent.keyDown(trigger, { key: 'Escape' })

    expect(screen.queryByRole('listbox')).toBeNull()
    expect(document.activeElement).toBe(trigger)
    expect(onAncestorKeyDown).toHaveBeenCalled()
  })
})
