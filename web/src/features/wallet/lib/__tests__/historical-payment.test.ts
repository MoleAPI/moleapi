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

import { formatQuota } from '@/lib/format'

import {
  formatHistoricalPaymentAmount,
  formatHistoricalTopUpCredit,
} from '../format'

describe('historical payment amount', () => {
  test('uses the recorded ISO currency with native fraction rules', () => {
    const expected = new Intl.NumberFormat('en-US', {
      style: 'currency',
      currency: 'USD',
    }).format(12.5)

    assert.equal(
      formatHistoricalPaymentAmount(12.5, ' usd ', 'en-US'),
      expected
    )
  })

  test('falls back to a symbol-free number for missing or invalid currency', () => {
    assert.equal(formatHistoricalPaymentAmount(12.5, '', 'en-US'), '12.5')
    assert.equal(formatHistoricalPaymentAmount(12.5, 'ZZZ', 'en-US'), '12.5')
  })

  test('keeps legacy Creem quota amounts in quota units', () => {
    const display = formatHistoricalTopUpCredit({
      amount: 500_000,
      credited_quota: 0,
      payment_method: 'creem',
      payment_provider: null,
    })

    assert.equal(display.value, formatQuota(500_000))
    assert.equal(display.hasCreditedFact, false)
  })

  test('prefers immutable credited quota for new records', () => {
    const display = formatHistoricalTopUpCredit({
      amount: 10,
      credited_quota: 500_000,
      payment_method: 'stripe',
      payment_provider: 'stripe',
    })

    assert.equal(display.value, formatQuota(500_000))
    assert.equal(display.hasCreditedFact, true)
  })
})
