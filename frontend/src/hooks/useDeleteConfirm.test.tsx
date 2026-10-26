import { act, renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useDeleteConfirm } from './useDeleteConfirm'

type Target = { id: number; label: string }

describe('useDeleteConfirm', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('deletes the selected target and clears it on success', async () => {
    const deleteItem = vi.fn().mockResolvedValue(undefined)
    const { result } = renderHook(() => useDeleteConfirm<Target>(deleteItem))

    act(() => result.current.requestDelete({ id: 4, label: 'Venta #4' }))
    expect(result.current.target).toEqual({ id: 4, label: 'Venta #4' })

    await act(async () => {
      await result.current.confirm()
    })

    expect(deleteItem).toHaveBeenCalledWith(4)
    expect(result.current.target).toBeNull()
  })

  it('keeps the target when deletion fails so the modal can show the error', async () => {
    const deleteItem = vi.fn().mockRejectedValue(new Error('falló'))
    const { result } = renderHook(() => useDeleteConfirm<Target>(deleteItem))

    act(() => result.current.requestDelete({ id: 1, label: 'Pedido #1' }))

    await act(async () => {
      await expect(result.current.confirm()).rejects.toThrow('falló')
    })

    expect(result.current.target).toEqual({ id: 1, label: 'Pedido #1' })
  })

  it('closes without deleting and ignores confirm when there is no target', async () => {
    const deleteItem = vi.fn()
    const { result } = renderHook(() => useDeleteConfirm<Target>(deleteItem))

    act(() => result.current.requestDelete({ id: 1, label: 'Pedido #1' }))
    act(() => result.current.close())
    expect(result.current.target).toBeNull()

    await act(async () => {
      await result.current.confirm()
    })

    expect(deleteItem).not.toHaveBeenCalled()
  })
})
