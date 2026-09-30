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
import { act, renderHook, waitFor } from '@testing-library/react'
import type { PropsWithChildren } from 'react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import type { SupportTicket } from '@/features/support/api'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'
import { useNotificationStore } from '@/stores/notification-store'

import { useNotifications } from '../use-notifications'

const ticket: SupportTicket = {
  id: '42',
  ticketNumber: '1042',
  subject: 'API request failed',
  description: '',
  status: 'Open',
  category: 'API Integration',
  priority: 'Medium',
  email: 'user@example.com',
  createdTime: '2026-09-30T08:00:00Z',
  modifiedTime: '2026-09-30T09:00:00Z',
  activity: 'agent',
}
let client: QueryClient
let tickets: SupportTicket[]
let enabled: boolean

beforeEach(() => {
  localStorage.clear()
  enabled = true
  tickets = [
    ticket,
    { ...ticket, id: '43', activity: 'customer' },
    { ...ticket, id: '44', activity: 'new' },
    { ...ticket, id: '45', isArchived: true },
  ]
  useAuthStore.getState().auth.setUser({
    id: 1,
    username: 'alice',
    role: 1,
    email: 'user@example.com',
  })
  useNotificationStore.setState({
    lastReadNotice: '',
    readAnnouncementKeys: [],
    readSupportTickets: {},
  })
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/support/config') {
      return { data: { success: true, data: { enabled, community_links: {} } } }
    }
    if (url === '/api/support/tickets') {
      return {
        data: {
          success: true,
          data: { tickets, has_more: false, next_from: 20 },
        },
      }
    }
    if (url === '/api/notice') return { data: { success: true, data: '' } }
    if (url === '/api/status') {
      return { data: { success: true, data: { announcements_enabled: false } } }
    }
    throw new Error(`Unexpected URL: ${url}`)
  })
})

afterEach(() => {
  client.clear()
  vi.restoreAllMocks()
  useAuthStore.getState().auth.setUser(null)
  localStorage.clear()
})

function wrapper({ children }: PropsWithChildren) {
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>
}

test('users see agent replies; reading a revision clears it and a later reply becomes unread', async () => {
  const { result } = renderHook(useNotifications, { wrapper })
  await waitFor(() => expect(result.current.unreadCount).toBe(1))
  expect(result.current.support.tickets.map((item) => item.id)).toEqual(['42'])
  act(() => result.current.openPopover())
  expect(result.current.activeTab).toBe('tickets')
  expect(result.current.unreadCount).toBe(1)
  act(() =>
    useNotificationStore
      .getState()
      .markSupportTicketRead(1, '42', ticket.modifiedTime)
  )
  expect(result.current.unreadCount).toBe(0)
  tickets = [{ ...ticket, modifiedTime: '2026-09-30T10:00:00Z' }]
  await act(async () => {
    await client.invalidateQueries({ queryKey: ['support', 'tickets'] })
  })
  await waitFor(() => expect(result.current.unreadCount).toBe(1))
})

test('administrators see new tickets and customer replies but not their own replies or archived tickets', async () => {
  useAuthStore.getState().auth.setUser({ id: 2, username: 'admin', role: 10 })
  const { result } = renderHook(useNotifications, { wrapper })
  await waitFor(() => expect(result.current.unreadCount).toBe(2))
  expect(result.current.support.tickets.map((item) => item.id)).toEqual([
    '43',
    '44',
  ])
})

test('read state persists per account and cached tickets are hidden after sign out', async () => {
  useNotificationStore
    .getState()
    .markSupportTicketRead(1, '42', ticket.modifiedTime)
  await useNotificationStore.persist.rehydrate()
  const { result } = renderHook(useNotifications, { wrapper })
  await waitFor(() => expect(result.current.support.tickets).toHaveLength(1))
  expect(result.current.unreadCount).toBe(0)
  act(() =>
    useAuthStore.getState().auth.setUser({
      id: 2,
      username: 'bob',
      role: 1,
      email: 'bob@example.com',
    })
  )
  await waitFor(() => expect(result.current.unreadCount).toBe(1))
  act(() => useAuthStore.getState().auth.setUser(null))
  expect(result.current.support.enabled).toBe(false)
  expect(result.current.support.tickets).toEqual([])
})

test.each(['guest', 'no email', 'disabled'])(
  '%s does not request tickets',
  async (condition) => {
    if (condition === 'guest') useAuthStore.getState().auth.setUser(null)
    if (condition === 'no email') {
      useAuthStore
        .getState()
        .auth.setUser({ id: 1, username: 'alice', role: 1 })
    }
    if (condition === 'disabled') enabled = false
    const { result } = renderHook(useNotifications, { wrapper })
    await waitFor(() => expect(result.current.loading).toBe(false))
    expect(result.current.support.enabled).toBe(false)
    expect(api.get).not.toHaveBeenCalledWith(
      '/api/support/tickets',
      expect.anything()
    )
  }
)

test('failed ticket requests can be retried without clearing saved read state', async () => {
  vi.mocked(api.get).mockImplementationOnce(async () => {
    throw new Error('Unavailable')
  })
  // Prime config so the next request belongs to tickets, not the configuration.
  client.setQueryData(['support', 'config'], {
    enabled: true,
    community_links: {},
  })
  const { result } = renderHook(useNotifications, { wrapper })
  await waitFor(() => expect(result.current.support.error).toBe(true))
  act(() => result.current.support.retry())
  await waitFor(() => expect(result.current.support.unreadCount).toBe(1))
  expect(result.current.support.error).toBe(false)
})
