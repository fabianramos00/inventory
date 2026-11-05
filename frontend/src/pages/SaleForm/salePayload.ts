import type { CreateSaleInput, PaymentMethod } from '@/types'

export interface SaleItemPayloadInput {
  product_id: number
  quantity: string
  delivered_quantity: string
  unit_price: string
}

interface CreateSalePayloadInput {
  paymentMethod: string
  amountPaid: string
  clientId: number | ''
  items: SaleItemPayloadInput[]
}

export function buildCreateSalePayload({
  paymentMethod,
  amountPaid,
  clientId,
  items,
}: CreateSalePayloadInput): CreateSaleInput {
  const paid = parseFloat(amountPaid) || 0
  return {
    payment_method: paid > 0 ? (paymentMethod as PaymentMethod) || null : null,
    amount_paid: paid,
    client_id: clientId || null,
    items: items.map(item => ({
      product_id: item.product_id,
      quantity: Number(item.quantity),
      delivered_quantity: Number(item.delivered_quantity),
      unit_price: item.unit_price !== '' ? parseFloat(item.unit_price) || null : null,
    })),
  }
}
