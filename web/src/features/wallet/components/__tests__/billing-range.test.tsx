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
  fireEvent,
  render,
  screen,
  waitFor,
  cleanup,
} from '@testing-library/react'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { BillingHistoryDialog } from '../dialogs/billing-history-dialog'

afterEach(() => {
  cleanup()
  useAuthStore.getState().auth.setUser(null)
})

test('ordinary users can filter billing by date, paginate with the range, and reset it', async () => {
  useAuthStore
    .getState()
    .auth.setUser({ id: 7, username: 'range-user', role: 1 })
  const calls: string[] = []
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    calls.push(url)
    return {
      data: {
        success: true,
        data: {
          items: [
            {
              id: 1,
              trade_no: 'test-order',
              money: 12,
              amount: 12,
              status: 'success',
              create_time: 100,
              complete_time: 100,
              payment_method: 'alipay',
            },
          ],
          total: 25,
        },
      },
    }
  })
  const i18n = i18next.createInstance()
  await i18n.use(initReactI18next).init({ lng: 'en', resources: {} })
  render(
    <I18nextProvider i18n={i18n}>
      <BillingHistoryDialog open onOpenChange={() => {}} />
    </I18nextProvider>
  )
  const start = await screen.findByLabelText('Start time')
  fireEvent.change(start, { target: { value: '2026-01-01T00:00' } })
  fireEvent.change(screen.getByLabelText('End time'), {
    target: { value: '2026-01-02T23:59' },
  })
  await waitFor(() => {
    const query = new URL(calls.at(-1) ?? '', 'http://localhost').searchParams
    expect(query.get('start_timestamp')).toBe(
      String(new Date('2026-01-01T00:00').getTime() / 1000)
    )
    expect(query.get('end_timestamp')).toBe(
      String(new Date('2026-01-02T23:59').getTime() / 1000)
    )
  })
  expect(screen.queryByLabelText('Search by user')).toBeNull()
  fireEvent.click(screen.getByRole('button', { name: 'Next page' }))
  await waitFor(() => {
    const query = new URL(calls.at(-1) ?? '', 'http://localhost').searchParams
    expect(query.get('p')).toBe('2')
    expect(query.get('start_timestamp')).toBe(
      String(new Date('2026-01-01T00:00').getTime() / 1000)
    )
  })
  fireEvent.click(screen.getByRole('button', { name: 'Reset filters' }))
  await waitFor(() => expect(calls.at(-1)).not.toContain('timestamp'))
})
