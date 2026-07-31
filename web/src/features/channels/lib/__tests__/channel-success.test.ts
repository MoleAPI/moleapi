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

import type { Channel } from '../../types'
import { getChannelSuccessStats } from '../channel-success'

describe('channel success stats', () => {
  test('returns a single channel success rate', () => {
    const stats = getChannelSuccessStats(
      { id: 7 } as Channel,
      new Map([
        [
          7,
          {
            channel_id: 7,
            request_count: 20,
            success_count: 19,
            success_rate: 95,
          },
        ],
      ])
    )

    assert.deepEqual(stats, {
      request_count: 20,
      success_count: 19,
      success_rate: 95,
    })
  })

  test('aggregates tag rows by request count', () => {
    const stats = getChannelSuccessStats(
      {
        children: [{ id: 1 }, { id: 2 }, { id: 3 }],
      } as unknown as Channel,
      new Map([
        [
          1,
          {
            channel_id: 1,
            request_count: 10,
            success_count: 10,
            success_rate: 100,
          },
        ],
        [
          2,
          {
            channel_id: 2,
            request_count: 30,
            success_count: 15,
            success_rate: 50,
          },
        ],
      ])
    )

    assert.deepEqual(stats, {
      request_count: 40,
      success_count: 25,
      success_rate: 62.5,
    })
  })
})
