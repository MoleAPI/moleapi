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
import assert from 'node:assert/strict'
import { test } from 'node:test'

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { renderToStaticMarkup } from 'react-dom/server'
import { I18nextProvider, initReactI18next } from 'react-i18next'

import { BusinessMetrics } from './business-metrics'

test('business metrics show the selected-period order funnel', async () => {
  const i18n = i18next.createInstance()
  await i18n.use(initReactI18next).init({ lng: 'en' })
  const queryClient = new QueryClient()
  queryClient.setQueryData(['dashboard', 'business-metrics', 100, 200], {
    new_users: 12,
    new_purchasing_users: 2,
    new_user_purchasing_users: 1,
    intent_orders: 10,
    intent_amounts: [
      { currency: 'USD', orders: 10, amount: 160, average_amount: 16 },
    ],
    paid_orders: 4,
    paid_amounts: [
      { currency: 'USD', orders: 4, amount: 100, average_amount: 25 },
    ],
    paying_users: 3,
    payment_success_rate: 0.4,
  })

  const html = renderToStaticMarkup(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={queryClient}>
        <BusinessMetrics startTimestamp={100} endTimestamp={200} />
      </QueryClientProvider>
    </I18nextProvider>
  )

  assert.match(html, /New Users/)
  assert.match(html, /Order Intents/)
  assert.match(html, /Paid Orders/)
  assert.match(html, /New Purchasers/)
  assert.match(html, /Repeat Purchasers/)
  assert.match(html, /New User Purchase Rate.*8\.33%/)
  assert.match(html, /Repeat Purchase Rate.*33\.33%/)
  assert.match(html, /Average Order Value/)
  assert.match(html, />12</)
  assert.match(html, />10</)
  assert.match(html, />4</)
  assert.match(html, /40%/)
})
