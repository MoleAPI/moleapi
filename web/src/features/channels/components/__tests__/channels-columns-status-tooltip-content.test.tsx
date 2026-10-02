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
import {
  flexRender,
  getCoreRowModel,
  useReactTable,
} from '@tanstack/react-table'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test, vi } from 'vitest'

import { retestChannelModel } from '../../api'
import { formatRelativeTime } from '../../lib'
import { channelSchema, type Channel } from '../../types'
import { useChannelsColumns } from '../channels-columns'
import { ChannelsProvider } from '../channels-provider'

vi.mock('../../api', async () => {
  const actual = await vi.importActual<typeof import('../../api')>('../../api')
  return { ...actual, retestChannelModel: vi.fn() }
})

function ExampleAutoDisabledStatusCell(props: { channel: Channel }) {
  const table = useReactTable({
    data: [props.channel],
    columns: useChannelsColumns({ enableSelection: false }),
    getCoreRowModel: getCoreRowModel(),
  })
  const cell = table
    .getRowModel()
    .rows[0]?.getAllCells()
    .find((item) => item.column.id === 'status')

  return cell ? flexRender(cell.column.columnDef.cell, cell.getContext()) : null
}

test('keeps the long string inside the status tooltip is wrapped when show', async () => {
  const reason = '114514'.repeat(40)
  const channelItem = channelSchema.parse({
    id: 1,
    type: 1,
    key: 'test-key',
    name: 'Test-Channel-Item',
    status: 3,
    created_time: 1,
    test_time: 0,
    response_time: 0,
    balance_updated_time: 0,
    other_info: JSON.stringify({ status_reason: reason }),
  })
  const queryClient = new QueryClient()
  const user = userEvent.setup()

  render(
    <QueryClientProvider client={queryClient}>
      <ChannelsProvider>
        <ExampleAutoDisabledStatusCell channel={channelItem} />
      </ChannelsProvider>
    </QueryClientProvider>
  )

  await user.hover(screen.getByText('Auto Disabled'))

  expect(await screen.findByText(reason, { exact: false })).toHaveClass(
    'wrap-anywhere'
  )
})

test('shows auto-paused model details and offers an immediate retest', async () => {
  vi.mocked(retestChannelModel).mockResolvedValue({ success: true })
  const pausedAt = Math.floor(Date.now() / 1000) - 60 * 60
  const channelItem = channelSchema.parse({
    id: 2,
    type: 1,
    key: 'test-key',
    name: 'Model health channel',
    status: 1,
    models: 'paused-model,healthy-model',
    created_time: 1,
    test_time: 0,
    response_time: 0,
    balance_updated_time: 0,
    other_info: JSON.stringify({
      channel_probe: {
        models: {
          'paused-model': {
            auto_paused: true,
            pause_reason:
              'Production failures confirmed by target and control probes',
            paused_at: pausedAt,
            next_probe_at: 1_700_001_800,
            last_probe_result: 'failed_target_passed_control',
          },
          'removed-model': { auto_paused: true },
        },
      },
    }),
  })
  const queryClient = new QueryClient()
  const user = userEvent.setup()

  render(
    <QueryClientProvider client={queryClient}>
      <ChannelsProvider>
        <ExampleAutoDisabledStatusCell channel={channelItem} />
      </ChannelsProvider>
    </QueryClientProvider>
  )

  await user.click(screen.getByRole('button', { name: 'Auto-paused models' }))
  expect(screen.getByText('paused-model')).toBeInTheDocument()
  expect(screen.queryByText('removed-model')).not.toBeInTheDocument()
  expect(screen.getByText('Paused at:', { exact: false })).toHaveTextContent(
    formatRelativeTime(pausedAt, 'en')
  )

  await user.click(
    screen.getByRole('button', { name: 'Retest and restore now' })
  )
  await waitFor(() => {
    expect(retestChannelModel).toHaveBeenCalledWith(2, 'paused-model')
  })
})
