import { useState } from 'react'

interface DeleteTarget {
  id: number
}

export function useDeleteConfirm<T extends DeleteTarget>(deleteItem: (id: number) => Promise<unknown>) {
  const [target, setTarget] = useState<T | null>(null)

  async function confirm() {
    if (!target) return
    await deleteItem(target.id)
    setTarget(null)
  }

  return { target, requestDelete: setTarget, close: () => setTarget(null), confirm }
}
