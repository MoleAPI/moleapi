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

import { describe, test } from 'vitest'

import {
  canRequestTopUpInvoice,
  getInvoiceFilename,
  getTopUpInvoiceDownloadUrl,
  getTopUpInvoiceUrl,
} from '../billing'

describe('top-up invoice download', () => {
  test('links the owner to a completed top-up invoice', () => {
    assert.equal(
      getTopUpInvoiceUrl(
        {
          id: 42,
          user_id: 7,
          status: 'success',
          payment_method: 'alipay',
          payment_provider: 'epay',
        },
        7
      ),
      '/api/user/topup/42/invoice'
    )
    assert.equal(
      getTopUpInvoiceUrl(
        {
          id: 42,
          user_id: 7,
          status: 'success',
          payment_method: 'alipay',
          payment_provider: 'epay',
        },
        7,
        false,
        true
      ),
      '/api/user/topup/42/invoice?download=1'
    )
    assert.equal(
      getTopUpInvoiceDownloadUrl(
        {
          id: 42,
          user_id: 7,
          status: 'success',
          payment_method: 'alipay',
          payment_provider: 'epay',
        },
        7
      ),
      '/api/user/topup/42/invoice?download=1'
    )
  })

  test('allows an admin to view another users completed invoice', () => {
    assert.equal(
      getTopUpInvoiceUrl(
        {
          id: 42,
          user_id: 7,
          status: 'success',
          payment_method: 'nowpayments',
          payment_provider: 'nowpayments',
        },
        99,
        true
      ),
      '/api/user/topup/42/invoice'
    )
  })

  test('parses invoice filenames from content disposition headers', () => {
    assert.equal(
      getInvoiceFilename(
        'attachment; filename="receipt-USR20260722010101.pdf"',
        'fallback.pdf'
      ),
      'receipt-USR20260722010101.pdf'
    )
    assert.equal(getInvoiceFilename(undefined, 'fallback.pdf'), 'fallback.pdf')
  })

  test('does not expose invoice links for incomplete or another users records', () => {
    assert.equal(
      getTopUpInvoiceUrl(
        {
          id: 42,
          user_id: 7,
          status: 'pending',
          payment_method: 'alipay',
        },
        7
      ),
      null
    )
    assert.equal(
      getTopUpInvoiceUrl(
        {
          id: 42,
          user_id: 8,
          status: 'success',
          payment_method: 'alipay',
        },
        7
      ),
      null
    )
  })

  test('uses Waffo Pancake official invoices instead of local receipts', () => {
    assert.equal(
      getTopUpInvoiceUrl(
        {
          id: 42,
          user_id: 7,
          status: 'success',
          payment_method: 'waffo_pancake',
          payment_provider: 'waffo_pancake',
        },
        7
      ),
      null
    )
  })

  test('only allows supported fiat records to request an invoice', () => {
    for (const payment_method of ['alipay', 'wxpay', 'lantu']) {
      assert.equal(
        canRequestTopUpInvoice({
          id: 42,
          status: 'success',
          payment_method,
          money: 12.34,
        }),
        true
      )
    }
    for (const payment_method of ['nowpayments', 'waffo_pancake', 'stripe']) {
      assert.equal(
        canRequestTopUpInvoice({
          id: 42,
          status: 'success',
          payment_method,
          money: 12.34,
        }),
        false
      )
    }
  })
})
