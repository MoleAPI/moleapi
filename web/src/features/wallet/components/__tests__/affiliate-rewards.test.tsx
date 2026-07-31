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

import i18next from 'i18next'
import { renderToStaticMarkup } from 'react-dom/server'
import { I18nextProvider, initReactI18next } from 'react-i18next'

import { AffiliateRewardsCard } from '../affiliate-rewards-card'

test('referral card explains each reward and how to transfer it', async () => {
  const i18n = i18next.createInstance()
  await i18n.use(initReactI18next).init({ lng: 'en' })

  const html = renderToStaticMarkup(
    <I18nextProvider i18n={i18n}>
      <AffiliateRewardsCard
        user={null}
        affiliateLink='https://example.com/referral'
        onTransfer={() => undefined}
        onOpenHistory={() => undefined}
        inviterReward={25_000}
        inviteeReward={25_000}
        inviteRebateRatio={100}
      />
    </I18nextProvider>
  )

  assert.match(html, /Referral Rewards/)
  assert.match(html, /Invite friends to earn extra rewards/)
  assert.match(html, /Reward Details/)
  assert.match(html, /Reward records/)
  assert.match(html, /Top-up rebate/)
  assert.match(html, /1%/)
  assert.match(html, /\$0\.05.*friend who signs up/)
  assert.match(html, /referral code.*\$0\.05.*account credit/)
  assert.match(html, /referred user&#x27;s credited top-up/)
  assert.match(html, /moved to your account balance using Transfer/)
})
