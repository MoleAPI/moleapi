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
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { beforeEach, expect, test, vi } from 'vitest'

import { getPricing } from '@/features/pricing/api'
import en from '@/i18n/locales/en.json'
import zh from '@/i18n/locales/zh.json'

import { Offers } from '../components/sections/offers'

vi.mock('@/features/pricing/api', () => ({ getPricing: vi.fn() }))
beforeEach(() => {
  vi.mocked(getPricing).mockResolvedValue({
    success: true,
    data: [],
    vendors: [],
    group_ratio: { default: 2, temp: 0.2, premium: 4, free: 0, invalid: -1 },
    usable_group: Object.fromEntries(
      ['default', 'temp', 'premium', 'free', 'invalid'].map((group) => [
        group,
        { desc: group, ratio: 1 },
      ])
    ),
    supported_endpoint: {},
    auto_groups: [],
  })
})

async function renderOffers(isAuthenticated = false, language = 'en') {
  const i18n = i18next.createInstance()
  await i18n
    .use(initReactI18next)
    .init({ lng: language, resources: { en, zh } })
  const root = createRootRoute()
  const home = createRoute({
    getParentRoute: () => root,
    path: '/',
    component: () => <Offers isAuthenticated={isAuthenticated} />,
  })
  const destination = createRoute({
    getParentRoute: () => root,
    path: '/pricing',
    component: () => <h1>Model pricing</h1>,
  })
  const router = createRouter({
    routeTree: root.addChildren([home, destination]),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider
        client={
          new QueryClient({ defaultOptions: { queries: { retry: false } } })
        }
      >
        <RouterProvider router={router} />
      </QueryClientProvider>
    </I18nextProvider>
  )
  await screen.findByRole('link', {
    name: language === 'zh' ? '查看 1 折模型' : 'Explore temp-group models',
  })
  return router
}

test('top-up examples label both amounts in USD and show the bonus in the received balance', async () => {
  await renderOffers()
  const table = screen.getByRole('table', { name: 'Top-up examples in USD' })
  expect(
    within(table)
      .getAllByRole('columnheader')
      .map((cell) => cell.textContent)
  ).toEqual(['Top-up (USD)', 'Bonus', 'Receive (USD)'])
  expect(
    within(table)
      .getAllByRole('row')
      .slice(1)
      .map((row) =>
        within(row)
          .getAllByRole('cell')
          .map((cell) => cell.textContent)
      )
  ).toEqual([
    ['1', '+5%', '1.05'],
    ['15', '+18%', '17.70'],
    ['70', '+26%', '88.20'],
    ['280', '+40%', '392.00'],
  ])
  expect(
    screen.getByRole('link', { name: 'Create an account to top up' })
  ).toHaveAttribute('href', '/sign-up')
})

test('the low-price offer opens the eligible temp group rather than unfiltered pricing', async () => {
  const router = await renderOffers()
  const user = userEvent.setup()
  expect(screen.getByText(/10% of MoleAPI standard-group prices/)).toBeVisible()
  await user.click(
    screen.getByRole('link', { name: 'Explore temp-group models' })
  )
  expect(
    await screen.findByRole('heading', { name: 'Model pricing' })
  ).toBeVisible()
  expect(router.state.location.search).toEqual({ group: 'temp' })
})

test('the comparison discloses the required bonus and distinguishes effective cost from balance deduction', async () => {
  await renderOffers(true)
  expect(screen.getByText(/Top up 280 USD, get 40% extra/)).toBeVisible()
  expect(screen.getByText(/balance charged: 18 USD/)).toBeVisible()
  expect(screen.getByText('≈ 12.86 USD')).toBeVisible()
  expect(
    screen.getByRole('link', { name: 'Check current model pricing' })
  ).toHaveAttribute('href', '/pricing?search=claude-sonnet-4-6&group=default')
  expect(
    screen.getByRole('link', { name: 'View all top-up offers' })
  ).toHaveAttribute('href', '/wallet')
})

test('Chinese copy presents one USD minimum and translates ten percent pricing as one tenth of the price', async () => {
  await renderOffers(false, 'zh')
  expect(screen.getByRole('heading', { name: '低至 1 折' })).toBeVisible()
  expect(
    screen.getByRole('heading', { name: '1 USD 起充，最高加赠 40%' })
  ).toBeVisible()
  expect(screen.getByRole('link', { name: '查看 1 折模型' })).toHaveAttribute(
    'href',
    '/pricing?group=temp'
  )
})

test('group comparison applies relative rates and the selected bonus once, including zero and higher-cost groups', async () => {
  await renderOffers()
  const table = await screen.findByRole('table', {
    name: 'Group cost comparison (USD)',
  })
  expect(
    within(table).getByRole('row', { name: 'default $100.00 $71.43 $28.57' })
  ).toBeVisible()
  expect(
    within(table).getByRole('row', { name: 'temp $10.00 $7.14 $92.86' })
  ).toBeVisible()
  expect(
    within(table).getByRole('row', { name: 'premium $200.00 $142.86 -$42.86' })
  ).toBeVisible()
  expect(
    within(table).getByRole('row', { name: 'free $0.00 $0.00 $100.00' })
  ).toBeVisible()
  expect(
    within(table).queryByRole('link', { name: 'invalid' })
  ).not.toBeInTheDocument()
  const user = userEvent.setup()
  await user.selectOptions(screen.getByLabelText('Top-up example'), '1')
  expect(
    within(table).getByRole('row', { name: 'default $100.00 $95.24 $4.76' })
  ).toBeVisible()
  const input = screen.getByLabelText('Usage at standard-group prices (USD)')
  await user.clear(input)
  expect(input).toHaveAttribute('aria-invalid', 'true')
  expect(
    screen.queryByRole('table', { name: 'Group cost comparison (USD)' })
  ).not.toBeInTheDocument()
  await user.type(input, '0')
  expect(
    screen.getByRole('row', { name: 'default $0.00 $0.00 $0.00' })
  ).toBeVisible()
})

test('unavailable group rates show a fallback instead of invented savings', async () => {
  vi.mocked(getPricing).mockRejectedValue(new Error('offline'))
  await renderOffers()
  expect(
    await screen.findByText(
      'Group comparison is unavailable. Check current model pricing below.'
    )
  ).toBeVisible()
  expect(
    screen.queryByRole('table', { name: 'Group cost comparison (USD)' })
  ).not.toBeInTheDocument()
})
