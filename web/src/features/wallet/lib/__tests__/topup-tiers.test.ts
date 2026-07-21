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

import { calculatePresetPricing } from '../format'
import {
  getTopupBonusRate,
  getTopupDiscountRate,
  mergePresetAmounts,
} from '../payment'

describe('top-up pricing tiers', () => {
  test('uses the highest threshold at or below the selected amount', () => {
    const discounts = { 10: 0.95, 100: 0.9 }
    const bonuses = { 20: 0.05, 200: 0.1 }

    assert.equal(getTopupDiscountRate(discounts, 150), 0.9)
    assert.equal(getTopupBonusRate(bonuses, 150), 0.05)
  })

  test('ignores legacy bonus values in the discount map', () => {
    assert.equal(getTopupDiscountRate({ 10: 0.2 }, 10), 1)
    assert.equal(getTopupBonusRate({ 10: 2 }, 10), 0)
  })

  test('adds the effective discount and bonus to every preset', () => {
    assert.deepEqual(
      mergePresetAmounts([50, 250], { 10: 0.95, 200: 0.9 }, { 20: 0.05 }),
      [
        { value: 50, discount: 0.95, bonus: 0.05 },
        { value: 250, discount: 0.9, bonus: 0.05 },
      ]
    )
  })

  test('shows the actual payment and credited amount for a preset', () => {
    const pricing = calculatePresetPricing(1, 6.9, 1, 1, 0.05)

    assert.equal(pricing.actualPrice, 6.9)
    assert.equal(pricing.receivedAmount, 1.05)
  })
})
