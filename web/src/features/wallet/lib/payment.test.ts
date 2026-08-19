/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { describe, expect, test } from 'vitest'

import { PAYMENT_TYPES } from '../constants'
import {
  dispatchSelectedPayment,
  findNearestCreemProduct,
  getEpayPaymentMethods,
  getStandardPaymentMethods,
  isLanTuPayment,
  isNowPaymentsPayment,
  isSafeHttpPaymentUrl,
  isStripePayment,
  isWaffoPayment,
  isWaffoPancakePayment,
} from './payment'

describe('payment type classification', () => {
  test('keeps Waffo and Waffo Pancake on their dedicated flows', () => {
    expect(isWaffoPayment(PAYMENT_TYPES.WAFFO)).toBe(true)
    expect(isWaffoPayment(PAYMENT_TYPES.WAFFO_PANCAKE)).toBe(false)
    expect(isWaffoPancakePayment(PAYMENT_TYPES.WAFFO_PANCAKE)).toBe(true)
    expect(isWaffoPancakePayment(PAYMENT_TYPES.WAFFO)).toBe(false)
    expect(isStripePayment(PAYMENT_TYPES.STRIPE)).toBe(true)
    expect(isLanTuPayment(PAYMENT_TYPES.LANTU)).toBe(true)
    expect(isNowPaymentsPayment(PAYMENT_TYPES.NOWPAYMENTS)).toBe(true)
  })

  test('renders Waffo only in its dedicated method list', () => {
    expect(
      getStandardPaymentMethods([
        { name: 'Alipay', type: PAYMENT_TYPES.ALIPAY },
        { name: 'Waffo', type: PAYMENT_TYPES.WAFFO },
        { name: 'LanTu', type: PAYMENT_TYPES.LANTU },
      ])
    ).toEqual([
      { name: 'Alipay', type: PAYMENT_TYPES.ALIPAY },
      { name: 'LanTu', type: PAYMENT_TYPES.LANTU },
    ])
  })

  test('accepts only absolute HTTP payment URLs without credentials', () => {
    expect(isSafeHttpPaymentUrl('https://pay.example.com/order')).toBe(true)
    expect(isSafeHttpPaymentUrl('http://pay.example.com/order')).toBe(true)
    expect(isSafeHttpPaymentUrl('javascript:alert(1)')).toBe(false)
    expect(isSafeHttpPaymentUrl('/relative')).toBe(false)
    expect(isSafeHttpPaymentUrl('https://user:pass@example.com')).toBe(false)
  })

  test('keeps dedicated gateways out of Epay subscription methods', () => {
    expect(
      getEpayPaymentMethods([
        { name: 'Alipay', type: 'alipay' },
        { name: 'Stripe', type: PAYMENT_TYPES.STRIPE },
        { name: 'Creem', type: PAYMENT_TYPES.CREEM },
        { name: 'Waffo', type: PAYMENT_TYPES.WAFFO },
        { name: 'Pancake', type: PAYMENT_TYPES.WAFFO_PANCAKE },
        { name: 'LanTu', type: 'lantu' },
        { name: 'Crypto Pay', type: PAYMENT_TYPES.NOWPAYMENTS },
        { name: 'WeChat', type: 'wxpay' },
      ])
    ).toEqual([
      { name: 'Alipay', type: 'alipay' },
      { name: 'WeChat', type: 'wxpay' },
    ])
  })

  test('selects the nearest Creem product by configured price', () => {
    const products = [
      { name: '$1', productId: 'prod_1', price: 1, quota: 1, currency: 'USD' },
      { name: '$3', productId: 'prod_3', price: 3, quota: 3, currency: 'USD' },
      { name: '$7', productId: 'prod_7', price: 7, quota: 7, currency: 'USD' },
    ] as const

    expect(findNearestCreemProduct([...products], 3)).toEqual({
      product: products[1],
      adjusted: false,
    })
    expect(findNearestCreemProduct([...products], 4)).toEqual({
      product: products[1],
      adjusted: true,
    })
    expect(findNearestCreemProduct([...products], 5)).toEqual({
      product: products[1],
      adjusted: true,
    })
  })
})

describe('payment dispatch', () => {
  test('keeps the selected Waffo method index through confirmation', async () => {
    const calls: string[] = []
    const success = await dispatchSelectedPayment(
      { name: 'Waffo Card', type: PAYMENT_TYPES.WAFFO },
      120,
      3,
      {
        regular: async () => {
          calls.push('regular')
          return false
        },
        waffo: async (amount, index) => {
          calls.push(`waffo:${amount}:${index}`)
          return true
        },
        waffoPancake: async () => {
          calls.push('pancake')
          return false
        },
        lantu: async () => false,
        nowPayments: async () => false,
      }
    )

    expect(success).toBe(true)
    expect(calls).toEqual(['waffo:120:3'])
  })

  test('does not create a Waffo order without a selected method index', async () => {
    let called = false
    const success = await dispatchSelectedPayment(
      { name: 'Waffo Card', type: PAYMENT_TYPES.WAFFO },
      120,
      null,
      {
        regular: async () => false,
        waffo: async () => {
          called = true
          return true
        },
        waffoPancake: async () => false,
        lantu: async () => false,
        nowPayments: async () => false,
      }
    )

    expect(success).toBe(false)
    expect(called).toBe(false)
  })

  test('dispatches LanTu to its own checkout flow', async () => {
    const calls: string[] = []
    const success = await dispatchSelectedPayment(
      { name: 'LanTu', type: PAYMENT_TYPES.LANTU },
      50,
      null,
      {
        regular: async () => false,
        waffo: async () => false,
        waffoPancake: async () => false,
        lantu: async (amount) => {
          calls.push(`lantu:${amount}`)
          return true
        },
        nowPayments: async () => false,
      }
    )

    expect(success).toBe(true)
    expect(calls).toEqual(['lantu:50'])
  })

  test('dispatches NOWPayments to its hosted checkout flow', async () => {
    const calls: string[] = []
    const success = await dispatchSelectedPayment(
      { name: 'Crypto Pay', type: PAYMENT_TYPES.NOWPAYMENTS },
      25,
      null,
      {
        regular: async () => false,
        waffo: async () => false,
        waffoPancake: async () => false,
        lantu: async () => false,
        nowPayments: async (amount) => {
          calls.push(`nowpayments:${amount}`)
          return true
        },
      }
    )

    expect(success).toBe(true)
    expect(calls).toEqual(['nowpayments:25'])
  })
})
