import { useQuery } from '@tanstack/react-query'
import { salesApi } from '@/lib/api/sales'
import type { Sale, SaleItem } from '@/types'

export interface SaleItemView extends SaleItem {
  product_category: string
  product_brand: string
  product_material: string
}

export function useSale(id: string | undefined) {
  const saleId = Number(id)

  const saleQuery = useQuery<Sale>({
    queryKey: ['sale', saleId],
    queryFn: () => salesApi.getSale(saleId).then(res => res.data),
    enabled: !!id,
  })

  const itemsQuery = useQuery<SaleItemView[]>({
    queryKey: ['saleItems', saleId],
    queryFn: () =>
      salesApi
        .getSaleItems(saleId, { skip: 0, limit: 100 })
        .then(res =>
          res.data.items.map(item => ({
            ...item,
            product_category: item.category_name ?? '',
            product_brand: item.brand_name ?? '',
            product_material: item.material_name ?? '',
          }))
        ),
    enabled: !!id,
  })

  return {
    sale: saleQuery.data ?? null,
    saleItems: itemsQuery.data ?? [],
    isLoading: saleQuery.isLoading || itemsQuery.isLoading,
    error: saleQuery.error ?? itemsQuery.error,
  }
}
