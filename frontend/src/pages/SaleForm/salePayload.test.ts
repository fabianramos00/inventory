import { describe, expect, it } from 'vitest'
import { buildCreateSalePayload } from './salePayload'

describe('buildCreateSalePayload', () => {
  it('does not send payment_status because the server derives it from amount_paid', () => {
    const payload = buildCreateSalePayload({
      paymentMethod: 'cash',
      amountPaid: '0',
      clientId: '',
      items: [{ product_id: 7, quantity: '2', delivered_quantity: '0', unit_price: '' }],
    })

    expect(payload).not.toHaveProperty('payment_status')
    expect(Object.keys(payload)).toEqual(['payment_method', 'amount_paid', 'client_id', 'items'])
  })

  it('keeps low or null optional values intact', () => {
    const payload = buildCreateSalePayload({
      paymentMethod: '',
      amountPaid: '0',
      clientId: '',
      items: [
        { product_id: 7, quantity: '1', delivered_quantity: '0', unit_price: '' },
        { product_id: 9, quantity: '3', delivered_quantity: '3', unit_price: '12.50' },
      ],
    })

    expect(payload.payment_method).toBeNull()
    expect(payload.amount_paid).toBe(0)
    expect(payload.client_id).toBeNull()
    expect(payload.items).toEqual([
      { product_id: 7, quantity: 1, delivered_quantity: 0, unit_price: null },
      { product_id: 9, quantity: 3, delivered_quantity: 3, unit_price: 12.5 },
    ])
  })
})
