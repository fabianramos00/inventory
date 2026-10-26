import { useQuery } from '@tanstack/react-query'
import { inventoryApi } from '@/lib/api/inventory'
import type { Product } from '@/types'

export function useProduct(id: string | undefined) {
  const productId = Number(id)
  if (id !== undefined && isNaN(productId)) throw new Error('ID de producto inválido')

  const productQuery = useQuery<Product>({
    queryKey: ['product', productId],
    queryFn: () => inventoryApi.getProduct(productId).then(res => res.data),
    enabled: !!id,
  })

  return {
    product: productQuery.data ?? null,
    isLoading: productQuery.isLoading,
    error: productQuery.error,
  }
}
