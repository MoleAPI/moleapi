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
import type { Row } from '@tanstack/react-table'
import { render, renderHook, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import type { Channel } from '../../types'

const setOpen = vi.fn()
const setCurrentRow = vi.fn()
const channel = {
  id: 7,
  name: 'Primary channel',
  type: 1,
  status: 1,
  models: 'gpt-4o,gpt-4o-mini',
  group: 'default',
  priority: 0,
  weight: 0,
  used_quota: 0,
  balance: 0,
  response_time: 0,
  test_time: 0,
} as Channel

vi.mock('../channels-provider', () => ({
  useChannels: () => ({
    currentRow: channel,
    sensitiveVisible: true,
    setOpen,
    setCurrentRow,
    upstream: {
      openModal: vi.fn(),
      detectChannelUpdates: vi.fn(),
    },
  }),
}))
vi.mock('@/components/provider-badge', () => ({ ProviderBadge: () => null }))
vi.mock('@/components/group-badge', () => ({ GroupBadge: () => null }))
vi.mock('@/lib/admin-permissions', () => ({
  ADMIN_PERMISSION_ACTIONS: { SENSITIVE_WRITE: 'sensitive_write' },
  ADMIN_PERMISSION_RESOURCES: { CHANNEL: 'channel' },
  hasPermission: () => true,
}))

const { useChannelsColumns } = await import('../channels-columns')
const { DataTableRowActions } = await import('../data-table-row-actions')
const { ChannelTestDialog } = await import('../dialogs/channel-test-dialog')

function TestProviders(props: { children: ReactNode }) {
  return (
    <QueryClientProvider client={new QueryClient()}>
      {props.children}
    </QueryClientProvider>
  )
}

function columnId(column: ReturnType<typeof useChannelsColumns>[number]) {
  if (column.id) return column.id
  return 'accessorKey' in column ? String(column.accessorKey) : ''
}

beforeEach(() => {
  setOpen.mockReset()
  setCurrentRow.mockReset()
  vi.spyOn(api, 'get').mockResolvedValue({ data: { success: true } })
  vi.spyOn(api, 'post').mockResolvedValue({
    data: {
      success: true,
      probe: { mode: 'custom', outcome: 'pass' },
    },
  })
})

describe('channel table restored controls', () => {
  test('opens the connection test dialog from the row button and keeps actions right aligned', async () => {
    const user = userEvent.setup()
    render(
      <DataTableRowActions row={{ original: channel } as Row<Channel>} />,
      { wrapper: TestProviders }
    )

    const testButton = screen.getByRole('button', { name: 'Test Connection' })
    expect(testButton.closest('.justify-end')).not.toBeNull()
    await user.click(testButton)

    expect(setCurrentRow).toHaveBeenCalledWith(channel)
    expect(setOpen).toHaveBeenCalledWith('test-channel')
  })

  test('offers intelligence and custom connection checks in the dialog', async () => {
    const user = userEvent.setup()
    render(<ChannelTestDialog open onOpenChange={vi.fn()} />, {
      wrapper: TestProviders,
    })

    await user.click(screen.getByRole('combobox', { name: 'Probe type' }))
    expect(
      screen.getByRole('option', { name: 'Intelligence check' })
    ).toBeVisible()
    await user.click(
      screen.getByRole('option', { name: 'Custom prompt check' })
    )

    expect(screen.getByLabelText('Custom prompt')).toBeVisible()
    expect(screen.getByLabelText('Expected answer')).toBeVisible()
    await user.type(screen.getByLabelText('Custom prompt'), 'Reply with mole')
    await user.type(screen.getByLabelText('Expected answer'), 'mole')
    await user.click(
      screen.getAllByRole('button', { name: 'Test Connection' })[0]
    )

    await waitFor(() =>
      expect(api.post).toHaveBeenCalledWith(
        '/api/channel/test/7',
        expect.objectContaining({
          model: 'gpt-4o',
          test_type: 'custom',
          prompt: 'Reply with mole',
          expected_answer: 'mole',
        }),
        expect.any(Object)
      )
    )
  })

  test('places the compact 24 hour column after used quota and narrows type', () => {
    const { result } = renderHook(
      () => useChannelsColumns({ enableSelection: false, usage24h: {} }),
      { wrapper: TestProviders }
    )
    const ids = result.current.map(columnId)
    const usageColumn = result.current[ids.indexOf('usage_24h')]
    const typeColumn = result.current[ids.indexOf('type')]

    expect(ids.indexOf('usage_24h')).toBe(ids.indexOf('used_quota') + 1)
    expect(usageColumn.header).toBe('24 Hours')
    expect(typeColumn.size).toBe(160)
  })
})
