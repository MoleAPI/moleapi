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
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { expect, test } from 'vitest'

import en from '@/i18n/locales/en.json'
import zh from '@/i18n/locales/zh.json'

import { Offers } from '../components/sections/offers'

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
      <RouterProvider router={router} />
    </I18nextProvider>
  )
  await screen.findByRole('table')
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

test('homepage omits group calculations while retaining the provider comparison', async () => {
  await renderOffers()
  expect(
    screen.queryByRole('heading', { name: 'See exactly how much you save' })
  ).not.toBeInTheDocument()
  expect(screen.queryByRole('spinbutton')).not.toBeInTheDocument()
  expect(screen.queryByRole('combobox')).not.toBeInTheDocument()
  expect(
    screen.getByRole('link', { name: 'Official Anthropic API' })
  ).toBeVisible()
  expect(
    screen.getByRole('link', { name: 'OpenRouter + 5.5% fee' })
  ).toBeVisible()
})
