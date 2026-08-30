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

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { renderToStaticMarkup } from 'react-dom/server'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { test } from 'vitest'

import { BusinessMetrics } from '../business-metrics'

test('business metrics combine payments and orders while reusing usage data', async () => {
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
    top_up_intent_orders: 8,
    top_up_intent_amount_usd: 120,
    paid_orders: 4,
    paid_amounts: [
      { currency: 'USD', orders: 4, amount: 100, average_amount: 25 },
    ],
    top_up_paid_orders: 3,
    top_up_paid_amount_usd: 90,
    usd_exchange_rate: 7.3,
    paying_users: 3,
    payment_success_rate: 0.4,
    top_up_ranking: [
      {
        rank: 1,
        user_id: 7,
        username: 'alice',
        currency: 'USD',
        orders: 3,
        amount: 80,
      },
      {
        rank: 2,
        user_id: 8,
        username: 'bob',
        currency: 'USD',
        orders: 1,
        amount: 10,
      },
    ],
  })

  const html = renderToStaticMarkup(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={queryClient}>
        <BusinessMetrics
          startTimestamp={100}
          endTimestamp={200}
          usageData={[
            {
              created_at: 150,
              quota: 500_000,
              token_used: 2_500,
              count: 4,
            },
          ]}
        />
      </QueryClientProvider>
    </I18nextProvider>
  )

  assert.match(html, /Business Overview/)
  assert.match(html, /User Analytics/)
  assert.match(html, /Top-up and Orders/)
  assert.doesNotMatch(html, /Payment Success Rate/)
  assert.match(html, /Usage/)
  assert.match(html, /Total Tokens.*2,500/)
  assert.match(html, /Request Count.*4/)
  assert.match(html, /User Top-up Ranking/)
  assert.match(html, /New User Purchase Rate.*8\.33%/)
  assert.match(html, /Repeat Purchase Rate.*33\.33%/)
  assert.match(html, /Amount paid.*\$100\.00/s)
  assert.match(
    html,
    /Credited amount.*\$90\.00.*Average credited amount.*\$30\.00/s
  )
  assert.match(html, /alice/)
  assert.match(html, /\$80/)
  assert.match(html, /¥584/)
  assert.match(html, /aria-label="User Top-up Ranking"/)
  assert.match(html, /aria-label="Copy: alice"/)
  assert.match(html, /grid-cols-\[minmax\(0,12rem\)_minmax\(0,1fr\)\]/)
  assert.match(html, /\$80\.00<\/span><span[^>]*> · ≈ ¥584\.00<\/span><\/div>/)
  const rankingColors = Array.from(
    html.matchAll(/background-color:([^;"]+)/g),
    (match) => match[1]
  )
  assert.equal(rankingColors.length, 2)
  assert.equal(new Set(rankingColors).size, 2)
})
