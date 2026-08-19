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

import { getImageTokenBreakdown } from '../format'

describe('image token breakdown', () => {
  test('uses explicit image input and output token fields', () => {
    assert.deepEqual(
      getImageTokenBreakdown({
        image_output: 999,
        image_input_tokens: 120,
        image_output_tokens: 80,
      }),
      { input: 120, output: 80 }
    )
  })

  test('treats legacy image_output as image input tokens', () => {
    assert.deepEqual(getImageTokenBreakdown({ image_output: 240 }), {
      input: 240,
      output: 0,
    })
  })

  test('keeps an explicit zero from falling back to the legacy field', () => {
    assert.deepEqual(
      getImageTokenBreakdown({
        image: true,
        image_output: 240,
        image_input_tokens: 0,
      }),
      { input: 0, output: 0 }
    )
  })
})
