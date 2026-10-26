import { useEffect, useRef, useState } from 'react'

export function useDebouncedValue<T>(value: T, delay = 300, onDebounced?: () => void) {
  const [debounced, setDebounced] = useState(value)
  const onDebouncedRef = useRef(onDebounced)
  const initialValue = useRef(value)
  const hasChanged = useRef(false)

  useEffect(() => {
    onDebouncedRef.current = onDebounced
  })

  useEffect(() => {
    if (value !== initialValue.current) hasChanged.current = true
    const timeout = setTimeout(() => {
      setDebounced(value)
      if (hasChanged.current) onDebouncedRef.current?.()
    }, delay)
    return () => clearTimeout(timeout)
  }, [value, delay])

  return debounced
}
