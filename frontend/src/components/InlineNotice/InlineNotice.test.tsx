import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import InlineNotice from './InlineNotice'
import type { NoticeTone } from './InlineNotice'

function renderNotice(options: {
  tone?: NoticeTone
  autoCloseMs?: number
  onClose?: () => void
  children?: string
} = {}) {
  const onClose = options.onClose ?? vi.fn()
  const view = render(
    <InlineNotice
      tone={options.tone ?? 'success'}
      onClose={onClose}
      {...(options.autoCloseMs !== undefined ? { autoCloseMs: options.autoCloseMs } : {})}
    >
      {options.children ?? 'Guardado exitosamente.'}
    </InlineNotice>,
  )
  return { ...view, onClose }
}

function advance(ms: number) {
  act(() => {
    vi.advanceTimersByTime(ms)
  })
}

describe('InlineNotice', () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => vi.useRealTimers())

  it('auto-dismisses a success notice at 5000 ms, not before', () => {
    const { onClose } = renderNotice()

    advance(4999)
    expect(onClose).not.toHaveBeenCalled()

    advance(1)
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('does not auto-dismiss an error notice', () => {
    const { onClose } = renderNotice({ tone: 'error' })

    advance(60_000)

    expect(onClose).not.toHaveBeenCalled()
  })

  it('closes manually and does not fire a second time after the window', () => {
    const { onClose } = renderNotice()

    fireEvent.click(screen.getByRole('button', { name: 'Cerrar notificación' }))
    expect(onClose).toHaveBeenCalledTimes(1)

    advance(5000)
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('clears the timer on unmount', () => {
    const { unmount, onClose } = renderNotice()

    unmount()
    advance(5000)

    expect(onClose).not.toHaveBeenCalled()
  })

  it('pauses on hover and restarts the window on leave', () => {
    const { onClose } = renderNotice()

    advance(4000)
    fireEvent.mouseEnter(screen.getByRole('status'))

    advance(10_000)
    expect(onClose).not.toHaveBeenCalled()

    fireEvent.mouseLeave(screen.getByRole('status'))
    advance(4999)
    expect(onClose).not.toHaveBeenCalled()

    advance(1)
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('pauses on focus and restarts the window on blur', () => {
    const { onClose } = renderNotice()
    const close = screen.getByRole('button', { name: 'Cerrar notificación' })

    advance(4000)
    fireEvent.focusIn(close)

    advance(10_000)
    expect(onClose).not.toHaveBeenCalled()

    fireEvent.focusOut(close, { relatedTarget: document.body })
    advance(4999)
    expect(onClose).not.toHaveBeenCalled()

    advance(1)
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('uses a polite status for success and info and an alert for errors', () => {
    const success = renderNotice({ tone: 'success' })
    expect(screen.getByRole('status').getAttribute('aria-live')).toBe('polite')
    expect(screen.getByRole('status').getAttribute('aria-atomic')).toBe('true')
    success.unmount()

    const info = renderNotice({ tone: 'info' })
    expect(screen.getByRole('status').getAttribute('aria-live')).toBe('polite')
    info.unmount()

    renderNotice({ tone: 'error' })
    expect(screen.getByRole('alert')).toBeTruthy()
    expect(screen.queryByRole('status')).toBeNull()
    expect(screen.getByRole('alert').getAttribute('aria-live')).toBeNull()
  })

  it('exposes the close button as Cerrar notificación', () => {
    renderNotice()

    expect(screen.getByRole('button', { name: 'Cerrar notificación' })).toBeTruthy()
  })
})
