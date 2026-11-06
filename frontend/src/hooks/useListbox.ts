import { useRef, useState, type KeyboardEvent } from 'react'

/** Moves focus between the buttons of a listbox without scrolling the page. */
export function focusListOption(list: HTMLElement | null, delta: number): HTMLElement | null {
  if (!list) return null
  const buttons = list.querySelectorAll<HTMLButtonElement>('button')
  if (!buttons.length) return null
  const index = Array.from(buttons).indexOf(document.activeElement as HTMLButtonElement)
  const next = index === -1
    ? (delta > 0 ? 0 : buttons.length - 1)
    : Math.min(Math.max(index + delta, 0), buttons.length - 1)
  const target = buttons[next]
  target?.focus({ preventScroll: true })
  if (target && typeof target.scrollIntoView === 'function') {
    target.scrollIntoView({ block: 'nearest' })
  }
  return target ?? null
}

export function useListbox() {
  const [isOpen, setIsOpen] = useState(false)
  const triggerRef = useRef<HTMLButtonElement>(null)
  const listRef = useRef<HTMLDivElement>(null)

  function close(restoreFocus = false) {
    setIsOpen(false)
    if (restoreFocus) triggerRef.current?.focus({ preventScroll: true })
  }

  function handleKeyDown(event: KeyboardEvent<HTMLElement>) {
    if (event.key === 'Escape') {
      event.preventDefault()
      // The menu may live inside a modal: only the menu closes.
      event.stopPropagation()
      close(true)
    } else if (event.key === 'Tab') {
      // The menu is portaled, so it is no longer a DOM sibling of the trigger.
      event.preventDefault()
      close(true)
    } else if (event.key === 'ArrowDown') {
      event.preventDefault()
      focusListOption(listRef.current, 1)
    } else if (event.key === 'ArrowUp') {
      event.preventDefault()
      focusListOption(listRef.current, -1)
    }
  }

  function handleTriggerKeyDown(event: KeyboardEvent<HTMLButtonElement>) {
    if (event.key === 'ArrowDown') {
      event.preventDefault()
      if (!isOpen) setIsOpen(true)
      else focusListOption(listRef.current, 1)
    } else if (event.key === 'Escape') {
      close(true)
    }
  }

  return { isOpen, setIsOpen, triggerRef, listRef, close, handleKeyDown, handleTriggerKeyDown }
}
