import { describe, expect, it } from 'vitest'
import {
  buildAddOrderItemPayload,
  buildCreateOrderPayload,
  buildUpdateOrderItemPayload,
  buildUpdateOrderPayload,
} from './orderPayload'

describe('order payloads', () => {
  it('builds the create payload with only the contract fields', () => {
    const payload = buildCreateOrderPayload({
      providerId: 3,
      status: 'sent',
      paymentStatus: 'pending',
      items: [{
        product_id: 5,
        quantity: '2',
        unit_cost: '10.25',
        supplier_sku: 'PROV-001',
      }],
    })

    expect(payload).toEqual({
      provider_id: 3,
      status: 'sent',
      payment_status: 'pending',
      items: [{
        product_id: 5,
        quantity: 2,
        unit_cost: 10.25,
        supplier_sku: 'PROV-001',
      }],
    })
    expect(Object.keys(payload)).toEqual(['provider_id', 'status', 'payment_status', 'items'])
  })

  it('omits supplier_sku from the create payload when empty', () => {
    const payload = buildCreateOrderPayload({
      providerId: 3,
      status: 'pending',
      paymentStatus: 'pending',
      items: [{ product_id: 5, quantity: '1', unit_cost: '', supplier_sku: '' }],
    })

    expect(payload.items[0]).toEqual({ product_id: 5, quantity: 1, unit_cost: 0 })
    expect(payload.items[0]).not.toHaveProperty('supplier_sku')
  })

  it('builds a partial details update without empty provider_id', () => {
    expect(buildUpdateOrderPayload({ providerId: 7, status: 'received', paymentStatus: 'paid' })).toEqual({
      provider_id: 7,
      status: 'received',
      payment_status: 'paid',
    })
    expect(buildUpdateOrderPayload({ providerId: '', status: 'sent', paymentStatus: 'pending' })).toEqual({
      status: 'sent',
      payment_status: 'pending',
    })
  })

  it('builds add and update item payloads with the documented fields', () => {
    const item = { product_id: 5, quantity: '3', unit_cost: '12.50', supplier_sku: 'SKU-1' }

    expect(buildAddOrderItemPayload(item)).toEqual({
      product_id: 5,
      quantity: 3,
      unit_cost: 12.5,
      supplier_sku: 'SKU-1',
    })
    expect(buildUpdateOrderItemPayload(item)).toEqual({
      quantity: 3,
      unit_cost: 12.5,
      supplier_sku: 'SKU-1',
    })
  })

  it('sends null when clearing supplier_sku on an item update', () => {
    const payload = buildUpdateOrderItemPayload({
      quantity: '2',
      unit_cost: '8',
      supplier_sku: '',
    })

    expect(payload).toEqual({ quantity: 2, unit_cost: 8, supplier_sku: null })
  })
})
