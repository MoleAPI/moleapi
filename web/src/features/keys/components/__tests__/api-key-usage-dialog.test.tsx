import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
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
import type { Row } from '@tanstack/react-table'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { expect, test, vi } from 'vitest'

import type { ApiKey } from '../../types'
import { ApiKeysDialogs } from '../api-keys-dialogs'
import { ApiKeysProvider } from '../api-keys-provider'
import { DataTableRowActions } from '../data-table-row-actions'

vi.mock('@/hooks/use-status', () => ({
  useStatus: () => ({
    status: { server_address: 'https://api.moleapi.com' },
  }),
}))

vi.mock('@/features/chat/hooks/use-chat-presets', () => ({
  useChatPresets: () => ({ chatPresets: [], serverAddress: '' }),
}))

vi.mock('@/features/keys/api', () => ({
  fetchTokenKey: vi.fn(async () => ({
    success: true,
    data: { key: 'test-secret' },
  })),
  fetchTokenKeysBatch: vi.fn(),
  updateApiKeyStatus: vi.fn(),
}))

const apiKey: ApiKey = {
  id: 7,
  name: 'Primary key',
  key: 'masked',
  status: 1,
  remain_quota: 100,
  used_quota: 0,
  unlimited_quota: false,
  expired_time: -1,
  created_time: 0,
  accessed_time: 0,
  group: 'default',
  auto_groups: null,
  cross_group_retry: false,
  model_limits_enabled: false,
  model_limits: '',
  allow_ips: '',
}

test('opens a usage guide from the labeled API key action', async () => {
  const row = { original: apiKey } as Row<ApiKey>
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })

  render(
    <QueryClientProvider client={queryClient}>
      <ApiKeysProvider>
        <DataTableRowActions row={row} />
        <ApiKeysDialogs />
      </ApiKeysProvider>
    </QueryClientProvider>
  )

  const usageButton = screen.getByRole('button', { name: 'Copy and use' })
  expect(usageButton).toBeVisible()
  fireEvent.click(usageButton)

  await waitFor(() => {
    expect(
      screen.getByRole('heading', { name: 'API key and endpoint' })
    ).toBeVisible()
  })
  expect(screen.getByRole('tab', { name: 'Responses' })).toBeVisible()
  expect(screen.getByRole('tab', { name: 'Chat' })).toBeVisible()
  expect(screen.getByText('sk-test-secret')).toBeVisible()
  expect(screen.getByText('https://api.moleapi.com/v1')).toBeVisible()
  expect(
    screen.getAllByText(/Authorization: Bearer sk-test-secret/).length
  ).toBeGreaterThan(0)
  expect(screen.getByRole('textbox', { name: 'cURL example' })).toBeVisible()
})
