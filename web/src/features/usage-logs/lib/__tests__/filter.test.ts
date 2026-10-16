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

import { describe, test, vi } from 'vitest'

import { getChannels, searchChannels } from '@/features/channels/api'
import { api } from '@/lib/api'

import {
  getAllLogs,
  getUserLogs,
  getAllMidjourneyLogs,
  getUserMidjourneyLogs,
  getAllTaskLogs,
  getUserTaskLogs,
  getLogStats,
  getUserLogStats,
} from '../../api'
import { getAuditLogs } from '../../audit/api'
import { buildQuickFilterSearch, buildSearchParams } from '../filter'

vi.mock('@/lib/api', () => ({ api: { get: vi.fn() } }))

describe('usage log filters', () => {
  test('all log and channel requests trim outer whitespace without altering names', async () => {
    const get = vi.mocked(api.get).mockResolvedValue({
      data: { success: true, data: { items: [], total: 0 } },
    })
    for (const fetch of [
      getAllLogs,
      getUserLogs,
      getLogStats,
      getUserLogStats,
    ]) {
      await fetch({ model_name: '  model name\t', token_name: '   ' })
      const url = new URL(
        String(get.mock.lastCall?.[0]),
        'https://test.invalid'
      )
      assert.equal(url.searchParams.get('model_name'), 'model name')
      assert.equal(url.searchParams.has('token_name'), false)
    }
    for (const fetch of [
      getAllMidjourneyLogs,
      getUserMidjourneyLogs,
      getAllTaskLogs,
      getUserTaskLogs,
    ]) {
      await fetch({
        channel_id: ' 325 ',
        task_id: '  task id\t',
        mj_id: '  drawing id ',
      })
      const url = new URL(
        String(get.mock.lastCall?.[0]),
        'https://test.invalid'
      )
      assert.equal(url.searchParams.get('channel_id'), '325')
      assert.equal(url.searchParams.get('task_id'), 'task id')
      assert.equal(url.searchParams.get('mj_id'), 'drawing id')
    }
    for (const scope of ['all', 'self'] as const) {
      await getAuditLogs(scope, {
        p: 1,
        page_size: 20,
        username: '  user name ',
        request_id: '\t ',
      })
      const params = get.mock.lastCall?.[1]?.params as Record<string, unknown>
      assert.equal(params.username, 'user name')
      assert.equal(params.request_id, undefined)
      assert.equal(params.page_size, 20)
    }
    await getChannels({
      group: '  group name ',
      status: '  enabled  ',
      tag_mode: false,
    })
    const channelParams = get.mock.lastCall?.[1]?.params as Record<
      string,
      unknown
    >
    assert.equal(channelParams.group, 'group name')
    assert.equal(channelParams.status, 'enabled')
    assert.equal(channelParams.tag_mode, false)
    await searchChannels({
      keyword: '  channel name ',
      model: ' model ',
      group: ' \t ',
      p: 2,
    })
    const searchParams = get.mock.lastCall?.[1]?.params as Record<
      string,
      unknown
    >
    assert.equal(searchParams.keyword, 'channel name')
    assert.equal(searchParams.model, 'model')
    assert.equal(searchParams.group, undefined)
    assert.equal(searchParams.p, 2)
  })

  test('trims text filters and merges request id filters', () => {
    assert.deepEqual(
      buildSearchParams(
        {
          startTime: new Date(1000),
          endTime: new Date(2000),
          channel: ' 325 ',
          model: ' claude-sonnet-5 ',
          token: ' ',
          group: ' relay ',
          username: ' 202 ',
          requestId: ' ',
          upstreamRequestId: ' upstream-request ',
        },
        'common'
      ),
      {
        startTime: 1000,
        endTime: 2000,
        channel: '325',
        model: 'claude-sonnet-5',
        group: 'relay',
        username: '202',
        requestId: 'upstream-request',
      }
    )
  })

  test('quick filters overwrite the selected field and reset pagination', () => {
    const first = buildQuickFilterSearch(
      { page: 9, group: 'old', startTime: 1000 },
      'group',
      'relay'
    )
    const second = buildQuickFilterSearch(first, 'group', 'vip')

    assert.deepEqual(second, {
      page: 1,
      group: 'vip',
      startTime: 1000,
    })
    assert.deepEqual(buildQuickFilterSearch(second, 'type', '2').type, ['2'])
  })
})
