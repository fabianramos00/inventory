import { useState } from 'react'
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import DateFilterInput from './DateFilterInput'

function Harness({ initial = '' }: { initial?: string }) {
  const [value, setValue] = useState(initial)
  return <DateFilterInput value={value} onChange={setValue} label="Fecha inicio" />
}

function getSegments() {
  return {
    day: screen.getByRole('textbox', { name: 'Fecha inicio día' }) as HTMLInputElement,
    month: screen.getByRole('textbox', { name: 'Fecha inicio mes' }) as HTMLInputElement,
    year: screen.getByRole('textbox', { name: 'Fecha inicio año' }) as HTMLInputElement,
  }
}

describe('DateFilterInput', () => {
  it('emits an ISO date when all three segments are filled', () => {
    const onChange = vi.fn()
    render(<DateFilterInput value="" onChange={onChange} label="Fecha inicio" />)
    const { day, month, year } = getSegments()

    fireEvent.change(day, { target: { value: '01' } })
    fireEvent.change(month, { target: { value: '01' } })
    fireEvent.change(year, { target: { value: '2026' } })

    expect(onChange).toHaveBeenCalledWith('2026-01-01')
    expect(day.value).toBe('01')
    expect(month.value).toBe('01')
    expect(year.value).toBe('2026')
    expect(screen.getAllByText('/')).toHaveLength(2)
  })

  it('keeps the separators static in the DOM', () => {
    const { container } = render(
      <DateFilterInput value="2026-01-15" onChange={vi.fn()} label="Fecha inicio" />,
    )
    const field = container.firstElementChild as HTMLElement

    expect(field.textContent).toContain('/')
    expect(screen.getAllByText('/')).toHaveLength(2)

    fireEvent.change(getSegments().day, { target: { value: '09' } })

    expect(screen.getAllByText('/')).toHaveLength(2)
    expect(field.textContent).toContain('/')
  })

  it('does not open the calendar when a segment is clicked, only the button does', () => {
    render(<DateFilterInput value="2026-01-15" onChange={vi.fn()} label="Fecha inicio" />)
    const { day } = getSegments()

    fireEvent.focus(day)
    fireEvent.click(day)

    expect(screen.queryByRole('dialog')).toBeNull()

    fireEvent.click(screen.getByRole('button', { name: 'Seleccionar fecha inicio: 15/01/2026' }))

    expect(screen.getByRole('dialog')).toBeTruthy()
  })

  it('normalizes a one-digit day and month on blur', () => {
    const onChange = vi.fn()
    render(<DateFilterInput value="" onChange={onChange} label="Fecha inicio" />)
    const { day, month, year } = getSegments()

    fireEvent.change(day, { target: { value: '1' } })
    fireEvent.change(month, { target: { value: '1' } })
    fireEvent.change(year, { target: { value: '2026' } })
    fireEvent.blur(year)

    expect(day.value).toBe('01')
    expect(month.value).toBe('01')
    expect(year.value).toBe('2026')
    expect(onChange).toHaveBeenCalledWith('2026-01-01')
  })

  it('does not emit an invalid date and restores the last valid value on blur', () => {
    const onChange = vi.fn()
    const { container } = render(
      <DateFilterInput value="2026-01-15" onChange={onChange} label="Fecha inicio" />,
    )
    const field = container.firstElementChild as HTMLElement
    const { day, month } = getSegments()

    // 31/01/2026 is valid and updates the field.
    fireEvent.change(day, { target: { value: '31' } })
    onChange.mockClear()

    // 31/02/2026 is not a real date: no emission.
    fireEvent.change(month, { target: { value: '02' } })
    expect(onChange).not.toHaveBeenCalled()
    expect(field.getAttribute('aria-invalid')).toBe('true')

    fireEvent.blur(month)

    expect(day.value).toBe('15')
    expect(month.value).toBe('01')
    expect(field.getAttribute('aria-invalid')).toBe('false')
  })

  it('clears the filter when a segment is emptied', () => {
    const onChange = vi.fn()
    render(<DateFilterInput value="2026-01-15" onChange={onChange} label="Fecha inicio" />)
    const { day } = getSegments()

    fireEvent.change(day, { target: { value: '' } })

    expect(onChange).toHaveBeenCalledWith('')
    expect(day.value).toBe('')
  })

  it('fills the segments and closes the calendar when a day is selected', () => {
    const { container } = render(<Harness initial="2026-01-15" />)

    fireEvent.click(screen.getByRole('button', { name: 'Seleccionar fecha inicio: 15/01/2026' }))
    fireEvent.click(container.ownerDocument.querySelector('[data-day="2026-01-20"] button')!)

    expect(screen.queryByRole('dialog')).toBeNull()
    const { day, month, year } = getSegments()
    expect(day.value).toBe('20')
    expect(month.value).toBe('01')
    expect(year.value).toBe('2026')
  })

  it('closes on Escape and restores focus to the calendar button', () => {
    render(<DateFilterInput value="2026-01-15" onChange={vi.fn()} label="Fecha inicio" />)
    const button = screen.getByRole('button', { name: 'Seleccionar fecha inicio: 15/01/2026' })

    fireEvent.click(button)
    fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape' })

    expect(screen.queryByRole('dialog')).toBeNull()
    expect(document.activeElement).toBe(button)
  })
})
