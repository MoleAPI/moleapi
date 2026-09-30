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
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { beforeAll, expect, test, vi } from 'vitest'

import { NotificationPopover } from '@/components/notification-popover'
import type { SupportTicket } from '@/features/support/api'

beforeAll(() => {
  Element.prototype.getAnimations ??= () => []
})

const ticket: SupportTicket & { unread: boolean } = {
  id: '42',
  ticketNumber: '1042',
  subject: 'Long ticket title '.repeat(20).trim(),
  description: '',
  status: 'Open',
  category: 'API Integration',
  priority: 'Medium',
  email: 'user@example.com',
  createdTime: '2026-09-30T08:00:00Z',
  modifiedTime: '2026-09-30T09:00:00Z',
  unread: true,
}

function setup(state: 'ready' | 'loading' | 'empty' | 'error' = 'ready') {
  const retry = vi.fn()
  function Notifications() {
    const [open, setOpen] = useState(false)
    return (
      <>
        <NotificationPopover
          open={open}
          onOpenChange={setOpen}
          activeTab='tickets'
          onTabChange={vi.fn()}
          unreadCount={1}
          notice=''
          announcements={[]}
          loading={false}
          support={{
            enabled: true,
            tickets: state === 'empty' ? [] : [ticket],
            unreadCount: 1,
            loading: state === 'loading',
            error: state === 'error',
            retry,
          }}
        />
        <Outlet />
      </>
    )
  }
  const root = createRootRoute({ component: Notifications })
  const support = createRoute({
    getParentRoute: () => root,
    path: '/support',
    component: () => <p>Selected ticket</p>,
  })
  const index = createRoute({ getParentRoute: () => root, path: '/' })
  const router = createRouter({
    routeTree: root.addChildren([index, support]),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  render(<RouterProvider router={router} />)
  return { router, retry, user: userEvent.setup() }
}

test('ticket notification opens the selected ticket and closes the popover', async () => {
  const { router, user } = setup()
  await user.click(await screen.findByRole('button', { name: 'Notifications' }))
  expect(screen.getByRole('tab', { name: /Tickets/ })).toHaveAttribute(
    'aria-selected',
    'true'
  )
  const link = screen.getByRole('link', { name: /1042/ })
  expect(link).toHaveAttribute('href', '/support?ticket=%2242%22')
  expect(screen.getByText(ticket.subject)).toHaveClass(
    '[overflow-wrap:anywhere]'
  )
  await user.click(link)
  await waitFor(() =>
    expect(router.state.location.href).toBe('/support?ticket=%2242%22')
  )
  await waitFor(() =>
    expect(
      screen.queryByRole('tab', { name: /Tickets/ })
    ).not.toBeInTheDocument()
  )
})

test.each([
  ['loading', 'Loading...'],
  ['empty', 'No new ticket updates'],
  ['error', 'Unable to load ticket updates.'],
] as const)(
  '%s has an explicit visible state and keeps navigation available',
  async (state, message) => {
    const { user, retry } = setup(state)
    await user.click(
      await screen.findByRole('button', { name: 'Notifications' })
    )
    expect(screen.getByText(message)).toBeVisible()
    expect(screen.getByRole('link', { name: 'Support center' })).toBeVisible()
    if (state === 'error') {
      await user.click(screen.getByRole('button', { name: 'Retry' }))
      expect(retry).toHaveBeenCalledOnce()
    }
    await user.keyboard('{Escape}')
    await waitFor(() =>
      expect(screen.queryByText(message)).not.toBeInTheDocument()
    )
  }
)
