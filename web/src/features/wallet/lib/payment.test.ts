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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { PAYMENT_TYPES } from '../constants'
import {
  dispatchSelectedPayment,
  getEpayPaymentMethods,
  getStandardPaymentMethods,
  isLanTuPayment,
  isSafeHttpPaymentUrl,
  isStripePayment,
  isWaffoPayment,
  isWaffoPancakePayment,
} from './payment'

describe('payment type classification', () => {
  test('keeps Waffo and Waffo Pancake on their dedicated flows', () => {
    assert.equal(isWaffoPayment(PAYMENT_TYPES.WAFFO), true)
    assert.equal(isWaffoPayment(PAYMENT_TYPES.WAFFO_PANCAKE), false)
    assert.equal(isWaffoPancakePayment(PAYMENT_TYPES.WAFFO_PANCAKE), true)
    assert.equal(isWaffoPancakePayment(PAYMENT_TYPES.WAFFO), false)
    assert.equal(isStripePayment(PAYMENT_TYPES.STRIPE), true)
    assert.equal(isLanTuPayment(PAYMENT_TYPES.LANTU), true)
  })

  test('renders Waffo only in its dedicated method list', () => {
    assert.deepEqual(
      getStandardPaymentMethods([
        { name: 'Alipay', type: PAYMENT_TYPES.ALIPAY },
        { name: 'Waffo', type: PAYMENT_TYPES.WAFFO },
        { name: 'LanTu', type: PAYMENT_TYPES.LANTU },
      ]),
      [
        { name: 'Alipay', type: PAYMENT_TYPES.ALIPAY },
        { name: 'LanTu', type: PAYMENT_TYPES.LANTU },
      ]
    )
  })

  test('accepts only absolute HTTP payment URLs without credentials', () => {
    assert.equal(isSafeHttpPaymentUrl('https://pay.example.com/order'), true)
    assert.equal(isSafeHttpPaymentUrl('http://pay.example.com/order'), true)
    assert.equal(isSafeHttpPaymentUrl('javascript:alert(1)'), false)
    assert.equal(isSafeHttpPaymentUrl('/relative'), false)
    assert.equal(isSafeHttpPaymentUrl('https://user:pass@example.com'), false)
  })

  test('keeps dedicated gateways out of Epay subscription methods', () => {
    assert.deepEqual(
      getEpayPaymentMethods([
        { name: 'Alipay', type: 'alipay' },
        { name: 'Stripe', type: PAYMENT_TYPES.STRIPE },
        { name: 'Creem', type: PAYMENT_TYPES.CREEM },
        { name: 'Waffo', type: PAYMENT_TYPES.WAFFO },
        { name: 'Pancake', type: PAYMENT_TYPES.WAFFO_PANCAKE },
        { name: 'LanTu', type: 'lantu' },
        { name: 'WeChat', type: 'wxpay' },
      ]),
      [
        { name: 'Alipay', type: 'alipay' },
        { name: 'WeChat', type: 'wxpay' },
      ]
    )
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
      }
    )

    assert.equal(success, true)
    assert.deepEqual(calls, ['waffo:120:3'])
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
      }
    )

    assert.equal(success, false)
    assert.equal(called, false)
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
      }
    )

    assert.equal(success, true)
    assert.deepEqual(calls, ['lantu:50'])
  })
})
