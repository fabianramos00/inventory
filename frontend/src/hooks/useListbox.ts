import { useRef, useState, type KeyboardEvent } from 'react'

export function useListbox() {
  const [isOpen, setIsOpen] = useState(false)
  const triggerRef = useRef<HTMLButtonElement>(null)
  const listRef = useRef<HTMLDivElement>(null)

  function close(restoreFocus = false) {
    setIsOpen(false)
    if (restoreFocus) triggerRef.current?.focus()
  }

  function focusOption(delta: number) {
    const buttons = listRef.current?.querySelectorAll<HTMLButtonElement>('button')
    if (!buttons?.length) return
    const index = Array.from(buttons).indexOf(document.activeElement as HTMLButtonElement)
    const next = index === -1
      ? (delta > 0 ? 0 : buttons.length - 1)
      : Math.min(Math.max(index + delta, 0), buttons.length - 1)
    buttons[next]?.focus()
  }

  function handleKeyDown(event: KeyboardEvent<HTMLElement>) {
    if (event.key === 'Escape') {
      event.preventDefault()
      close(true)
    } else if (event.key === 'ArrowDown') {
      event.preventDefault()
      focusOption(1)
    } else if (event.key === 'ArrowUp') {
      event.preventDefault()
      focusOption(-1)
    }
  }

  function handleTriggerKeyDown(event: KeyboardEvent<HTMLButtonElement>) {
    if (event.key === 'ArrowDown') {
      event.preventDefault()
      if (!isOpen) setIsOpen(true)
      else focusOption(1)
    } else if (event.key === 'Escape') {
      close(true)
    }
  }

  return { isOpen, setIsOpen, triggerRef, listRef, close, handleKeyDown, handleTriggerKeyDown }
}
