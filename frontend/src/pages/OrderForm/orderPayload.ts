import type { CreateOrderInput, OrderPaymentStatus, OrderStatus } from '@/types'

interface CreateOrderPayloadInput {
  providerId: number
  status: OrderStatus
  paymentStatus: OrderPaymentStatus
  items: {
    product_id: number
    quantity: string
    unit_cost: string
    supplier_sku: string
  }[]
}

interface UpdateOrderPayloadInput {
  providerId: number | ''
  status: OrderStatus
  paymentStatus: OrderPaymentStatus
}

interface OrderItemPayloadInput {
  product_id: number
  quantity: string
  unit_cost: string
  supplier_sku: string
}

function toQuantity(value: string) {
  return parseFloat(value) || 1
}

function toMoney(value: string) {
  return value !== '' ? parseFloat(value) || 0 : 0
}

export function buildCreateOrderPayload({
  providerId,
  status,
  paymentStatus,
  items,
}: CreateOrderPayloadInput): CreateOrderInput {
  return {
    provider_id: providerId,
    status,
    payment_status: paymentStatus,
    items: items.map(item => ({
      product_id: item.product_id,
      quantity: toQuantity(item.quantity),
      unit_cost: toMoney(item.unit_cost),
      ...(item.supplier_sku ? { supplier_sku: item.supplier_sku } : {}),
    })),
  }
}

export function buildUpdateOrderPayload({ providerId, status, paymentStatus }: UpdateOrderPayloadInput) {
  return {
    ...(providerId ? { provider_id: providerId } : {}),
    status,
    payment_status: paymentStatus,
  }
}

export function buildAddOrderItemPayload(item: OrderItemPayloadInput) {
  return {
    product_id: item.product_id,
    quantity: toQuantity(item.quantity),
    unit_cost: toMoney(item.unit_cost),
    ...(item.supplier_sku ? { supplier_sku: item.supplier_sku } : {}),
  }
}

export function buildUpdateOrderItemPayload(item: Omit<OrderItemPayloadInput, 'product_id'>) {
  return {
    quantity: toQuantity(item.quantity),
    unit_cost: toMoney(item.unit_cost),
    supplier_sku: item.supplier_sku || null,
  }
}
