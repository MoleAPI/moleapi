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
import { act, renderHook } from '@testing-library/react'
import type { PropsWithChildren } from 'react'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import {
  useSupportWorkingHours,
  useTicketUpdates,
} from '../hooks/use-ticket-updates'

afterEach(() => {
  vi.useRealTimers()
  vi.restoreAllMocks()
})

test.each([
  ['2026-10-02T00:59:00Z', false],
  ['2026-10-02T01:00:00Z', true],
  ['2026-10-02T09:59:00Z', true],
  ['2026-10-02T10:00:00Z', false],
  ['2026-10-03T01:00:00Z', false],
  ['2026-10-04T01:00:00Z', false],
])('automatic checks at %s respect Taipei working hours', (time, expected) => {
  vi.useFakeTimers()
  vi.setSystemTime(new Date(time))
  const { result, unmount } = renderHook(useSupportWorkingHours)
  expect(result.current).toBe(expected)
  unmount()
})

test('checks start at opening time and pause while the tab is hidden', async () => {
  vi.useFakeTimers()
  vi.setSystemTime(new Date('2026-10-02T00:59:00Z'))
  const visibility = vi.spyOn(document, 'visibilityState', 'get')
  const { result, unmount } = renderHook(useSupportWorkingHours)
  await act(() => vi.advanceTimersByTimeAsync(60_000))
  expect(result.current).toBe(true)
  visibility.mockReturnValue('hidden')
  act(() => document.dispatchEvent(new Event('visibilitychange')))
  expect(result.current).toBe(false)
  unmount()
})

test.each(['modifiedTime', 'commentCount', 'threadCount'] as const)(
  'a new ticket checks metadata; a changed %s refreshes once and checks stop after staff reply',
  async (field) => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-10-02T01:00:00Z'))
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    const wrapper = ({ children }: PropsWithChildren) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    )
    const ticket = {
      id: '42',
      ticketNumber: '42',
      subject: 'Help',
      description: '',
      status: 'Open',
      category: '',
      priority: '',
      email: 'a@example.com',
      createdTime: '2026-10-02T01:00:00Z',
      modifiedTime: '2026-10-02T01:00:00Z',
      commentCount: '0',
      threadCount: '1',
    }
    const metadata = {
      modifiedTime: ticket.modifiedTime,
      commentCount: '0',
      threadCount: '1',
    }
    const get = vi.spyOn(api, 'get').mockImplementation(async () => ({
      data: { success: true, data: { ...metadata } },
    }))
    const refresh = vi.fn().mockResolvedValue(undefined)
    const { rerender, unmount } = renderHook(
      ({ replied }) => useTicketUpdates(ticket, false, replied, 1, refresh),
      { wrapper, initialProps: { replied: false } }
    )
    await act(() => vi.advanceTimersByTimeAsync(1))
    expect(get).not.toHaveBeenCalled()
    await act(() => vi.advanceTimersByTimeAsync(5 * 60_000))
    expect(get).toHaveBeenCalledTimes(1)
    expect(get).toHaveBeenLastCalledWith('/api/support/tickets/42', {
      params: { view: 'updates' },
    })
    expect(refresh).not.toHaveBeenCalled()
    metadata[field] = field === 'modifiedTime' ? '2026-10-02T01:09:00Z' : '2'
    await act(() => vi.advanceTimersByTimeAsync(5 * 60_000))
    expect(refresh).toHaveBeenCalledTimes(1)
    rerender({ replied: true })
    await act(() => vi.advanceTimersByTimeAsync(30 * 60_000))
    expect(get).toHaveBeenCalledTimes(2)
    unmount()
    client.clear()
  }
)

test.each([
  {
    name: 'yesterday',
    createdTime: '2026-10-01T01:00:00Z',
    status: 'Open',
    isArchived: false,
  },
  {
    name: 'closed',
    createdTime: '2026-10-02T01:00:00Z',
    status: 'Closed',
    isArchived: false,
  },
  {
    name: 'archived',
    createdTime: '2026-10-02T01:00:00Z',
    status: 'Open',
    isArchived: true,
  },
])('$name tickets do not poll', async ({ createdTime, status, isArchived }) => {
  vi.useFakeTimers()
  vi.setSystemTime(new Date('2026-10-02T01:00:00Z'))
  const client = new QueryClient()
  const wrapper = ({ children }: PropsWithChildren) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  )
  const get = vi.spyOn(api, 'get')
  const ticket = {
    id: '42',
    ticketNumber: '42',
    subject: 'Help',
    description: '',
    category: '',
    priority: '',
    email: 'a@example.com',
    modifiedTime: createdTime,
    createdTime,
    status,
    isArchived,
  }
  const { unmount } = renderHook(
    () => useTicketUpdates(ticket, false, false, 1, vi.fn()),
    { wrapper }
  )
  await act(() => vi.advanceTimersByTimeAsync(30 * 60_000))
  expect(get).not.toHaveBeenCalled()
  unmount()
  client.clear()
})
