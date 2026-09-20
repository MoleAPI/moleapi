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
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { useIsSidebarModuleVisible } from '@/hooks/use-sidebar-config'
import { api } from '@/lib/api'
import { STATUS_QUERY_KEY } from '@/lib/status-query'
import { useAuthStore } from '@/stores/auth-store'

import { SidebarModulesCard } from '../sidebar-modules-card'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

test('core console entries ignore old personal hiding but respect administrator visibility', () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  client.setQueryData(STATUS_QUERY_KEY, {
    SidebarModulesAdmin: JSON.stringify({
      console: { enabled: true, task: false },
    }),
  })
  useAuthStore
    .getState()
    .auth.setUser({
      id: 7,
      username: 'test',
      role: 1,
      sidebar_modules: JSON.stringify({
        console: {
          enabled: false,
          token: false,
          log: false,
          midjourney: false,
          task: false,
        },
      }),
    })
  function Visibility() {
    const keys = useIsSidebarModuleVisible('/keys')
    const logs = useIsSidebarModuleVisible('/usage-logs/common')
    const tasks = useIsSidebarModuleVisible('/usage-logs/task')
    return <output>{JSON.stringify({ keys, logs, tasks })}</output>
  }
  render(
    <QueryClientProvider client={client}>
      <Visibility />
    </QueryClientProvider>
  )
  expect(screen.getByRole('status')).toHaveTextContent(
    '{"keys":true,"logs":true,"tasks":false}'
  )
  client.clear()
})

test('personal settings keep dashboard choice without switches for core console tools', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({ data: { success: true, data: {} } })
  render(<SidebarModulesCard />)
  await waitFor(() => expect(api.get).toHaveBeenCalledWith('/api/user/self'))
  expect(screen.getByRole('switch', { name: 'Dashboard' })).toBeInTheDocument()
  for (const name of [
    'Console Area',
    'Token Management',
    'Usage Logs',
    'Drawing Logs',
    'Task Logs',
  ]) {
    expect(screen.queryByRole('switch', { name })).toBeNull()
  }
})
