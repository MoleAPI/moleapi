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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { PluginsTable } from '../components/plugins-table'
import type { TaskPluginListItem } from '../types'

vi.mock('@/lib/lobe-icon', () => ({ getLobeIcon: () => null }))

const clients: QueryClient[] = []

afterEach(() => {
  for (const client of clients) client.clear()

  clients.length = 0

  localStorage.removeItem('task-plugins-view-mode')
})

function renderPlugins(
  enabled: boolean,

  view: 'card' | 'table' = 'card',

  source: TaskPluginListItem['source'] = 'override'
) {
  localStorage.setItem('task-plugins-view-mode', view)

  const item: TaskPluginListItem = {
    meta: {
      key: 'example',

      name: 'Example',

      version: '1.0.0',

      apiVersion: 1,

      author: { name: 'Example' },

      models: [],

      fetchMode: 'per_task',
    },

    source,

    enabled,

    active: true,

    source_hash: '',

    remark: '',

    runtime_status: enabled ? 'registered' : 'disabled',

    channel_count: 0,

    in_flight_count: 0,
  }

  if (source === 'override_over_factory') {
    item.factory_meta = { ...item.meta, version: '0.9.0' }
  }

  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: Infinity },

      mutations: { retry: false },
    },
  })

  clients.push(client)

  client.setQueryData(['task-plugins'], [item])

  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: [{ ...item, enabled: !enabled }] },
  })

  render(
    <QueryClientProvider client={client}>
      <PluginsTable onDetails={() => undefined} onUpload={() => undefined} />
    </QueryClientProvider>
  )

  return screen.getAllByRole('switch', { name: 'Enable plugin example' })[0]
}

test('usage dialog offers one cancel button and force operation posts force params', async () => {
  const user = userEvent.setup()

  const post = vi

    .spyOn(api, 'post')

    .mockResolvedValueOnce({
      data: {
        success: false,

        message: 'Plugin in use',

        data: {
          channels: [{ id: 7, name: 'Video channel' }],

          in_flight_count: 0,
        },
      },
    })

    .mockResolvedValueOnce({ data: { success: true, data: null } })

  await user.click(renderPlugins(true))

  const usage = await screen.findByRole('alertdialog', {
    name: 'Plugin is still in use',
  })

  expect(within(usage).getAllByRole('button', { name: 'Cancel' })).toHaveLength(
    1
  )

  await user.click(
    within(usage).getByRole('button', { name: 'Force operation' })
  )

  await waitFor(() =>
    expect(post).toHaveBeenLastCalledWith(
      '/api/plugin/task/example/status',
      { enabled: false },
      expect.objectContaining({ params: { cascade: true, force: true } })
    )
  )
})
