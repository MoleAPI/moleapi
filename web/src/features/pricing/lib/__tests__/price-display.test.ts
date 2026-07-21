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

import type { PricingModel } from '../../types'
import { getModelPriceDisplay } from '../model-helpers'

const model = {
  id: 1,
  model_name: 'example-model',
  quota_type: 0,
  model_ratio: 1,
  completion_ratio: 1,
  enable_groups: ['standard', 'discount'],
  group_ratio: { standard: 1, discount: 0.7 },
} satisfies PricingModel

describe('model card price display', () => {
  test('marks the lowest group price as starting at and shows its official discount', () => {
    assert.deepEqual(getModelPriceDisplay(model), {
      isStartingAt: true,
      discountPercent: 30,
    })
  })

  test('uses the selected group price without a starting-at label', () => {
    assert.deepEqual(getModelPriceDisplay(model, 'standard'), {
      isStartingAt: false,
      discountPercent: null,
    })
  })
})
