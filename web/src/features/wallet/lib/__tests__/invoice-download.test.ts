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

import { getTopUpInvoiceDownloadUrl, getTopUpInvoiceUrl } from '../billing'

describe('top-up invoice download', () => {
  test('links the owner to a completed top-up invoice', () => {
    assert.equal(
      getTopUpInvoiceUrl({ id: 42, user_id: 7, status: 'success' }, 7),
      '/api/user/topup/42/invoice'
    )
    assert.equal(
      getTopUpInvoiceUrl(
        { id: 42, user_id: 7, status: 'success' },
        7,
        false,
        true
      ),
      '/api/user/topup/42/invoice?download=1'
    )
    assert.equal(
      getTopUpInvoiceDownloadUrl({ id: 42, user_id: 7, status: 'success' }, 7),
      '/api/user/topup/42/invoice?download=1'
    )
  })

  test('allows an admin to view another users completed invoice', () => {
    assert.equal(
      getTopUpInvoiceUrl({ id: 42, user_id: 7, status: 'success' }, 99, true),
      '/api/user/topup/42/invoice'
    )
  })

  test('does not expose invoice links for incomplete or another users records', () => {
    assert.equal(
      getTopUpInvoiceUrl({ id: 42, user_id: 7, status: 'pending' }, 7),
      null
    )
    assert.equal(
      getTopUpInvoiceUrl({ id: 42, user_id: 8, status: 'success' }, 7),
      null
    )
  })
})
