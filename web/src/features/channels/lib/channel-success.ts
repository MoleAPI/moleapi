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
import type { Channel } from '../types'
import { isTagAggregateRow } from './channel-utils'

export type ChannelSuccessMetric = {
  channel_id: number
  request_count: number
  success_count: number
  success_rate: number
}

export type ChannelSuccessStats = {
  request_count: number
  success_count: number
  success_rate: number
}

export type ChannelProbeMetric = {
  channel_id: number
  channel_name: string
  model: string
  level?: 'basic' | 'standard' | 'advanced' | 'custom'
  status: 'pending' | 'healthy' | 'degraded'
  recent_pass: number
  recent_total: number
  last_test_at?: number
}

export type ChannelProbeStats = {
  status: 'pending' | 'healthy' | 'degraded'
  items: ChannelProbeMetric[]
}

export function getChannelSuccessStats(
  channel: Channel,
  metrics?: ReadonlyMap<number, ChannelSuccessMetric>
): ChannelSuccessStats | undefined {
  if (!metrics) return undefined

  if (!isTagAggregateRow(channel)) {
    const metric = metrics.get(channel.id)
    if (!metric || metric.request_count <= 0) return undefined
    return {
      request_count: metric.request_count,
      success_count: metric.success_count,
      success_rate: metric.success_rate,
    }
  }

  let requestCount = 0
  let successCount = 0
  for (const child of channel.children) {
    const metric = metrics.get(child.id)
    if (!metric || metric.request_count <= 0) continue
    requestCount += metric.request_count
    successCount += metric.success_count
  }

  if (requestCount <= 0) return undefined

  return {
    request_count: requestCount,
    success_count: successCount,
    success_rate: (successCount / requestCount) * 100,
  }
}

export function getChannelProbeStats(
  channel: Channel,
  metrics?: ReadonlyMap<number, ChannelProbeMetric[]>
): ChannelProbeStats | undefined {
  if (!metrics) return undefined
  const items = isTagAggregateRow(channel)
    ? channel.children.flatMap((child) => metrics.get(child.id) ?? [])
    : (metrics.get(channel.id) ?? [])
  if (items.length === 0) return undefined

  let status: ChannelProbeStats['status'] = 'pending'
  if (items.some((item) => item.status === 'degraded')) {
    status = 'degraded'
  } else if (items.every((item) => item.status === 'healthy')) {
    status = 'healthy'
  }
  return { status, items }
}
