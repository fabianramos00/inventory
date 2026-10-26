import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useDebouncedValue } from './useDebouncedValue'

describe('useDebouncedValue', () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => vi.useRealTimers())

  it('keeps the initial value and emits the new value after 300ms', () => {
    const { result, rerender } = renderHook(
      ({ value }) => useDebouncedValue(value),
      { initialProps: { value: 'inicial' } },
    )

    expect(result.current).toBe('inicial')

    rerender({ value: 'nuevo' })
    act(() => vi.advanceTimersByTime(299))
    expect(result.current).toBe('inicial')

    act(() => vi.advanceTimersByTime(1))
    expect(result.current).toBe('nuevo')
  })

  it('does not fire the callback on the initial mount settle', () => {
    const onDebounced = vi.fn()
    renderHook(() => useDebouncedValue('', 300, onDebounced))

    act(() => vi.advanceTimersByTime(300))
    expect(onDebounced).not.toHaveBeenCalled()
  })

  it('only settles on the last value after rapid changes', () => {
    const onDebounced = vi.fn()
    const { result, rerender } = renderHook(
      ({ value }) => useDebouncedValue(value, 300, onDebounced),
      { initialProps: { value: '' } },
    )

    act(() => vi.advanceTimersByTime(300))
    expect(onDebounced).not.toHaveBeenCalled()

    rerender({ value: 'a' })
    act(() => vi.advanceTimersByTime(100))
    rerender({ value: 'ab' })
    act(() => vi.advanceTimersByTime(100))
    rerender({ value: 'abc' })

    act(() => vi.advanceTimersByTime(299))
    expect(result.current).toBe('')
    expect(onDebounced).not.toHaveBeenCalled()

    act(() => vi.advanceTimersByTime(1))
    expect(result.current).toBe('abc')
    expect(onDebounced).toHaveBeenCalledTimes(1)
  })

  it('reports a settle even when the value did not change', () => {
    const onDebounced = vi.fn()
    const { rerender } = renderHook(
      ({ value }) => useDebouncedValue(value, 300, onDebounced),
      { initialProps: { value: 'igual' } },
    )

    act(() => vi.advanceTimersByTime(300))
    onDebounced.mockClear()

    rerender({ value: 'distinto' })
    rerender({ value: 'igual' })
    act(() => vi.advanceTimersByTime(300))

    expect(onDebounced).toHaveBeenCalledTimes(1)
  })
})
