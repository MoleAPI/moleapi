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

import type { PaymentMethod, TopupInfo } from '../../types'
import { RechargeFormCard } from '../recharge-form-card'

async function renderRechargeForm(
  payMethods: PaymentMethod[] = [],
  showBillingButton = false,
  topupInfoOverrides: Partial<TopupInfo> = {}
) {
  const i18n = i18next.createInstance()
  await i18n.use(initReactI18next).init({ lng: 'en' })

  return renderToStaticMarkup(
    <I18nextProvider i18n={i18n}>
      <RechargeFormCard
        topupInfo={{
          enable_online_topup: true,
          enable_stripe_topup: false,
          pay_methods: payMethods,
          min_topup: 1,
          stripe_min_topup: 1,
          amount_options: [],
          discount: {},
          bonus: {},
          quota_for_inviter: 0,
          quota_for_invitee: 0,
          quota_for_inviter_on_first_topup: 0,
          ...topupInfoOverrides,
        }}
        creemProducts={topupInfoOverrides.creem_products}
        enableCreemTopup={topupInfoOverrides.enable_creem_topup}
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
        onOpenBilling={showBillingButton ? () => undefined : undefined}
      />
    </I18nextProvider>
  )
}

test('top-up invoice note refers to partner invoicing in some regions', async () => {
  const html = await renderRechargeForm()

  assert.match(
    html,
    /Top-ups are invoiced based on the amount paid\. In some regions, invoices are issued by a partner company\./
  )
  assert.doesNotMatch(html, /Mainland China/)
})

test('WeChat Pay keeps its brand green independently of the active theme', async () => {
  const html = await renderRechargeForm([{ name: 'WeChat Pay', type: 'wxpay' }])

  assert.match(html, /bg-\[#07c160\]!/)
  assert.match(html, /hover:bg-\[#06ad56\]!/)
})

test('Waffo Pancake renders as deep red Global Pay', async () => {
  const html = await renderRechargeForm([
    { name: 'Global Pay', type: 'waffo_pancake' },
  ])

  assert.match(html, /Global Pay/)
  assert.match(html, /bg-\[#b91c1c\]!/)
  assert.match(html, /hover:bg-\[#991b1b\]!/)
})

test('billing entry uses recharge bills copy', async () => {
  const html = await renderRechargeForm([], true)

  assert.match(html, /Recharge Bills/)
  assert.doesNotMatch(html, /Order History/)
})

test('recharge card title uses account recharge copy', async () => {
  const html = await renderRechargeForm()

  assert.match(html, /Account Recharge/)
  assert.doesNotMatch(html, /Add Funds/)
})

test('Creem renders as a single payment button', async () => {
  const html = await renderRechargeForm([], false, {
    enable_online_topup: false,
    enable_creem_topup: true,
    creem_products: [
      {
        name: '$1',
        productId: 'prod_1',
        price: 1,
        quota: 525000,
        currency: 'USD',
      },
    ],
  })

  assert.match(html, /Payment Method/)
  assert.match(html, /Creem/)
  assert.doesNotMatch(html, /Creem Payment/)
  assert.doesNotMatch(html, /525,000/)
})

test('NOWPayments button renders as Crypto Pay', async () => {
  const html = await renderRechargeForm([
    { name: 'NOWPayments', type: 'nowpayments' },
  ])

  assert.match(html, /Crypto Pay/)
  assert.match(html, /bg-\[#f7931a\]!/)
  assert.match(html, /hover:bg-\[#e07f00\]!/)
})
