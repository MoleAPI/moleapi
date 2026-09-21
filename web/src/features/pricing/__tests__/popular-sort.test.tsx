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
import { renderHook } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { SORT_OPTIONS } from '../constants'
import { useFilters } from '../hooks/use-filters'
import { sortModels } from '../lib/filters'
import type { PricingModel } from '../types'

vi.mock('@tanstack/react-router', () => ({
  useSearch: () => ({}),
}))

const models = [
  { id: 1, model_name: 'zeta' },
  { id: 2, model_name: 'glm-4.5' },
  { id: 3, model_name: 'claude-sonnet-4' },
  { id: 4, model_name: 'gpt-5.1' },
  { id: 5, model_name: 'alpha' },
] as PricingModel[]

const popularity = [
  {
    model_name: 'glm-4.5',
    request_count: 40,
    avg_latency_ms: 0,
    success_rate: 0,
    avg_tps: 0,
  },
  {
    model_name: 'claude-sonnet-4',
    request_count: 4,
    avg_latency_ms: 0,
    success_rate: 0,
    avg_tps: 0,
  },
  {
    model_name: 'gpt-5.1',
    request_count: 9,
    avg_latency_ms: 0,
    success_rate: 0,
    avg_tps: 0,
  },
]

describe('popular model sorting', () => {
  it('uses popular sorting by default', () => {
    const { result } = renderHook(() => useFilters(models, popularity))

    expect(result.current.sortBy).toBe(SORT_OPTIONS.POPULAR)
  })

  it('puts recently used model families first with a stable fallback', () => {
    expect(
      sortModels(models, SORT_OPTIONS.POPULAR, popularity).map(
        (model) => model.model_name
      )
    ).toEqual(['gpt-5.1', 'claude-sonnet-4', 'glm-4.5', 'alpha', 'zeta'])
  })
})
