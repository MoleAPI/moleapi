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

import { RechargeFormCard } from '../recharge-form-card'

test('top-up invoice note refers to partner invoicing in some regions', async () => {
  const i18n = i18next.createInstance()
  await i18n.use(initReactI18next).init({ lng: 'en' })

  const html = renderToStaticMarkup(
    <I18nextProvider i18n={i18n}>
      <RechargeFormCard
        topupInfo={{
          enable_online_topup: true,
          enable_stripe_topup: false,
          pay_methods: [],
          min_topup: 1,
          stripe_min_topup: 1,
          amount_options: [],
          discount: {},
          bonus: {},
          quota_for_inviter: 0,
          quota_for_invitee: 0,
          quota_for_inviter_on_first_topup: 0,
        }}
        presetAmounts={[]}
        selectedPreset={null}
        onSelectPreset={() => undefined}
        topupAmount={1}
        onTopupAmountChange={() => undefined}
        paymentAmount={1}
        calculating={false}
        onPaymentMethodSelect={() => undefined}
        paymentLoading={null}
        redemptionCode=''
        onRedemptionCodeChange={() => undefined}
        onRedeem={() => undefined}
        redeeming={false}
      />
    </I18nextProvider>
  )

  assert.match(
    html,
    /Top-ups are invoiced based on the amount paid\. In some regions, invoices are issued by a partner company\./
  )
  assert.doesNotMatch(html, /Mainland China/)
})
