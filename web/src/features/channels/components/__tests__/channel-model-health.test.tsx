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
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { ChannelModelHealthTable } from '../channel-model-health'

function Providers(props: { children: ReactNode }) {
  return (
    <QueryClientProvider client={new QueryClient()}>
      {props.children}
    </QueryClientProvider>
  )
}

afterEach(() => vi.restoreAllMocks())

describe('channel model health table', () => {
  it('shows model status, expands daily history, and deletes from the form', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: {
          history_available: true,
          models: [
            {
              model: 'model-a',
              status: 'disabled',
              today: {
                date: '2026-10-03',
                requests: 4,
                failures: 3,
                failure_rate: 75,
                last_failure_at: 1790985600,
              },
              days: [
                {
                  date: '2026-10-03',
                  requests: 4,
                  failures: 3,
                  failure_rate: 75,
                },
              ],
            },
          ],
        },
      },
    })
    const onDelete = vi.fn()
    const user = userEvent.setup()

    render(
      <ChannelModelHealthTable
        channelId={7}
        models={['model-a']}
        onDelete={onDelete}
      />,
      { wrapper: Providers }
    )

    const modelButton = await screen.findByRole('button', { name: 'model-a' })
    expect(screen.getByText('Disabled')).toBeVisible()
    expect(screen.getByText('3 / 4')).toBeVisible()
    expect(modelButton).toHaveAttribute('aria-expanded', 'false')

    await user.click(modelButton)
    expect(modelButton).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByText('2026-10-03')).toBeVisible()

    await user.click(screen.getByRole('button', { name: 'Delete' }))
    expect(onDelete).toHaveBeenCalledWith('model-a')
  })
})
