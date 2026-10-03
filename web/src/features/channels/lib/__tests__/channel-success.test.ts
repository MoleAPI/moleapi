import { expect, test } from 'vitest'

import { channelSchema } from '../../types'
import {
  getChannelSuccessStats,
  type ChannelSuccessMetric,
} from '../channel-success'
import { aggregateChannelsByTag } from '../channel-utils'

const base = channelSchema.parse({
  id: 1,
  key: '',
  name: 'First',
  type: 1,
  status: 1,
  created_time: 0,
  test_time: 0,
  response_time: 0,
  balance_updated_time: 0,
  tag: 'shared',
})

test('aggregates success rate using request counts instead of averaging percentages', () => {
  const channels = aggregateChannelsByTag([
    base,
    { ...base, id: 2, name: 'Second' },
  ])
  const metrics = new Map<number, ChannelSuccessMetric>([
    [
      1,
      { channel_id: 1, request_count: 2, success_count: 1, success_rate: 50 },
    ],
    [
      2,
      { channel_id: 2, request_count: 8, success_count: 8, success_rate: 100 },
    ],
  ])
  expect(getChannelSuccessStats(channels[0], metrics)).toMatchObject({
    request_count: 10,
    success_count: 9,
    success_rate: 90,
  })
})
