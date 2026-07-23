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

import {
  FILTER_ALL,
  QUOTA_TYPES,
  ENDPOINT_TYPES,
  SORT_OPTIONS,
} from '../../constants'
import type { PricingModel } from '../../types'
import { filterAndSortModels } from '../filters'

const baseModel = {
  id: 1,
  model_name: 'base',
  quota_type: 0,
  model_ratio: 1,
  completion_ratio: 1,
  enable_groups: ['default'],
} satisfies PricingModel

function runFilter(
  models: PricingModel[],
  overrides: Partial<Parameters<typeof filterAndSortModels>[1]> = {}
): PricingModel[] {
  return filterAndSortModels(models, {
    search: '',
    vendor: FILTER_ALL,
    group: FILTER_ALL,
    quotaType: QUOTA_TYPES.ALL,
    endpointType: ENDPOINT_TYPES.ALL,
    tag: FILTER_ALL,
    sortBy: SORT_OPTIONS.POPULAR,
    ...overrides,
  })
}

describe('pricing filters', () => {
  test('popular sort puts recent 24h usage first and falls back to name', () => {
    const result = runFilter(
      [
        { ...baseModel, id: 1, model_name: 'charlie' },
        { ...baseModel, id: 2, model_name: 'alpha' },
        { ...baseModel, id: 3, model_name: 'bravo' },
      ],
      {
        popularModels: [
          { model_name: 'bravo', request_count: 4 },
          { model_name: 'charlie', request_count: 9 },
        ],
      }
    )

    assert.deepEqual(
      result.map((model) => model.model_name),
      ['charlie', 'bravo', 'alpha']
    )
  })

  test('search matches localized descriptions', () => {
    const result = runFilter(
      [
        {
          ...baseModel,
          id: 1,
          model_name: 'localized',
          description: 'English only',
          description_i18n: { zh: '中文介绍' },
        },
      ],
      { search: '中文' }
    )

    assert.equal(result.length, 1)
    assert.equal(result[0].model_name, 'localized')
  })
})
