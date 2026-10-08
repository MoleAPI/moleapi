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
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from '@tanstack/react-router'
import {
  fireEvent,
  render,
  screen,
  waitFor,
  cleanup,
  within,
} from '@testing-library/react'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { Route as SupportRoute } from '@/routes/_authenticated/support/'
import { useAuthStore } from '@/stores/auth-store'

import { BillingHistoryDialog } from '../dialogs/billing-history-dialog'

afterEach(() => {
  cleanup()
  useAuthStore.getState().auth.setUser(null)
})

function renderBillingDialog(i18n: ReturnType<typeof i18next.createInstance>) {
  const root = createRootRoute({
    component: () => (
      <QueryClientProvider client={new QueryClient()}>
        <I18nextProvider i18n={i18n}>
          <BillingHistoryDialog open onOpenChange={() => {}} />
          <Outlet />
        </I18nextProvider>
      </QueryClientProvider>
    ),
  })
  const index = createRoute({ getParentRoute: () => root, path: '/' })
  const support = createRoute({
    getParentRoute: () => root,
    path: '/support',
    validateSearch: SupportRoute.options.validateSearch,
  })
  const router = createRouter({
    routeTree: root.addChildren([index, support]),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  render(<RouterProvider router={router} />)
}

function latestBillingCall(calls: string[]) {
  for (let index = calls.length - 1; index >= 0; index--) {
    const call = calls.at(index)
    if (call?.startsWith('/api/user/topup/self?')) return call
  }
  return ''
}

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
  renderBillingDialog(i18n)
  const start = await screen.findByLabelText('Start time')
  fireEvent.change(start, { target: { value: '2026-01-01T00:00' } })
  fireEvent.change(screen.getByLabelText('End time'), {
    target: { value: '2026-01-02T23:59' },
  })
  await waitFor(() => {
    const query = new URL(latestBillingCall(calls), 'http://localhost')
      .searchParams
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
    const query = new URL(latestBillingCall(calls), 'http://localhost')
      .searchParams
    expect(query.get('p')).toBe('2')
    expect(query.get('start_timestamp')).toBe(
      String(new Date('2026-01-01T00:00').getTime() / 1000)
    )
  })
  fireEvent.click(screen.getByRole('button', { name: 'Reset filters' }))
  await waitFor(() =>
    expect(latestBillingCall(calls)).not.toContain('timestamp')
  )
})

test('shows provider-specific receipt and invoice guidance', async () => {
  useAuthStore.getState().auth.setUser({
    id: 7,
    username: 'provider-user',
    role: 1,
    email: 'provider@example.com',
  })
  vi.spyOn(api, 'get').mockImplementation(async (url) => ({
    data: {
      success: true,
      data:
        url === '/api/support/invoice-tickets'
          ? { tickets: { 3: '3003' } }
          : {
              items: [
                {
                  id: 1,
                  user_id: 7,
                  trade_no: 'waffo-order',
                  money: 12,
                  amount: 12,
                  status: 'success',
                  create_time: 100,
                  complete_time: 101,
                  payment_method: 'waffo_pancake',
                  payment_provider: 'waffo_pancake',
                  invoice_url:
                    'https://pancake.waffo.ai/invoice/PAY_test?token=test-token',
                },
                {
                  id: 2,
                  user_id: 7,
                  trade_no: 'crypto-order',
                  money: 12,
                  amount: 12,
                  status: 'success',
                  create_time: 100,
                  complete_time: 101,
                  payment_method: 'nowpayments',
                  payment_provider: 'nowpayments',
                },
                {
                  id: 3,
                  user_id: 7,
                  trade_no: 'invoiced-order',
                  money: 12,
                  amount: 12,
                  status: 'success',
                  create_time: 100,
                  complete_time: 101,
                  payment_method: 'alipay',
                  payment_provider: 'epay',
                },
                {
                  id: 4,
                  user_id: 7,
                  trade_no: 'invoiceable-order',
                  money: 12,
                  amount: 12,
                  status: 'success',
                  create_time: 100,
                  complete_time: 101,
                  payment_method: 'wxpay',
                  payment_provider: 'epay',
                },
              ],
              total: 4,
            },
    },
  }))
  const i18n = i18next.createInstance()
  await i18n.use(initReactI18next).init({ lng: 'en', resources: {} })
  renderBillingDialog(i18n)

  const waffoRow = (await screen.findByText('waffo-order')).closest('tr')
  expect(waffoRow).not.toBeNull()
  expect(
    within(waffoRow as HTMLTableRowElement).queryByText('View receipt')
  ).not.toBeInTheDocument()
  fireEvent.click(waffoRow as HTMLTableRowElement)
  expect(
    screen.getByText(
      'Waffo Pancake issued this invoice. Open it to update the billing details or download it.'
    )
  ).toBeVisible()
  expect(
    screen.getAllByRole('button', { name: 'Download invoice' })[0]
  ).toHaveAttribute(
    'href',
    'https://pancake.waffo.ai/invoice/PAY_test?token=test-token'
  )
  fireEvent.click(screen.getAllByRole('button', { name: 'Close' })[0])

  const cryptoRow = screen.getByText('crypto-order').closest('tr')
  expect(cryptoRow).not.toBeNull()
  const viewReceipt = within(cryptoRow as HTMLTableRowElement).getByText(
    'View receipt'
  )
  expect(viewReceipt).toBeVisible()
  expect(viewReceipt.closest('a')).toHaveAttribute('href', '/invoice/2')
  expect(
    within(cryptoRow as HTMLTableRowElement).queryByText('Download invoice')
  ).not.toBeInTheDocument()
  expect(
    within(cryptoRow as HTMLTableRowElement).queryByText('Request tax invoice')
  ).not.toBeInTheDocument()

  const invoicedRow = screen.getByText('invoiced-order').closest('tr')
  const invoiceableRow = screen.getByText('invoiceable-order').closest('tr')
  await waitFor(() =>
    expect(
      within(invoicedRow as HTMLTableRowElement).getByText(
        'View invoice request'
      )
    ).toBeVisible()
  )
  expect(
    within(invoicedRow as HTMLTableRowElement)
      .getByText('View invoice request')
      .closest('a')
  ).toHaveAttribute('href', '/support?ticket=%223003%22')
  expect(
    within(invoiceableRow as HTMLTableRowElement)
      .getByText('Request tax invoice')
      .closest('a')
  ).toHaveAttribute('href', '/support?invoice_record=4')

  fireEvent.click(cryptoRow as HTMLTableRowElement)
  expect(
    screen.getByText(
      'Cryptocurrency payments are not eligible for tax invoices.'
    )
  ).toBeVisible()
})

test('users without an email can request an invoice without ticket lookup retries', async () => {
  useAuthStore
    .getState()
    .auth.setUser({ id: 7, username: 'no-email-user', role: 1 })
  const get = vi.spyOn(api, 'get').mockImplementation(async (url) => ({
    data: {
      success: true,
      data: url.startsWith('/api/user/topup/self?')
        ? {
            items: [
              {
                id: 8,
                user_id: 7,
                trade_no: 'invoiceable-order',
                money: 12,
                amount: 12,
                status: 'success',
                create_time: 100,
                complete_time: 101,
                payment_method: 'alipay',
                payment_provider: 'epay',
              },
            ],
            total: 1,
          }
        : {},
    },
  }))
  const i18n = i18next.createInstance()
  await i18n.use(initReactI18next).init({ lng: 'en', resources: {} })
  renderBillingDialog(i18n)

  const row = (await screen.findByText('invoiceable-order')).closest('tr')
  expect(row).not.toBeNull()
  expect(
    within(row as HTMLTableRowElement)
      .getByText('Request tax invoice')
      .closest('a')
  ).toHaveAttribute('href', '/support?invoice_record=8')
  expect(
    get.mock.calls.filter(([url]) => url === '/api/support/invoice-tickets')
  ).toHaveLength(0)
})
