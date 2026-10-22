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
import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { beforeEach, expect, it, vi } from 'vitest'

import { BillingHistoryDialog } from '@/features/wallet/components/dialogs/billing-history-dialog'
import en from '@/i18n/locales/en.json'
import zh from '@/i18n/locales/zh.json'
import { api } from '@/lib/api'

import { CreemConfirmDialog } from '../components/dialogs/creem-confirm-dialog'
import { PaymentConfirmDialog } from '../components/dialogs/payment-confirm-dialog'
import { RechargeFormCard } from '../components/recharge-form-card'

const i18n = createInstance()

it.each([0, 2000])(
  'shows an exclusive bonus in recharge and confirmation only when the customer has one (%i)',
  async (ratio) => {
    await i18n.changeLanguage('en')
    render(
      <I18nextProvider i18n={i18n}>
        <RechargeFormCard
          topupInfo={{
            enable_online_topup: true,
            enable_stripe_topup: false,
            pay_methods: [],
            min_topup: 1,
            stripe_min_topup: 1,
            amount_options: [100],
            discount: {},
            bonus: { 100: 0.1 },
            exclusive_bonus_ratio: ratio,
            quota_for_inviter: 0,
            quota_for_invitee: 0,
            quota_for_inviter_on_first_topup: 0,
          }}
          presetAmounts={[{ value: 100, bonus: 0.1 }]}
          selectedPreset={100}
          topupAmount={100}
          paymentAmount={100}
          calculating={false}
          paymentLoading={null}
          redemptionCode=''
          redeeming={false}
          onSelectPreset={vi.fn()}
          onTopupAmountChange={vi.fn()}
          onPaymentMethodSelect={vi.fn()}
          onRedemptionCodeChange={vi.fn()}
          onRedeem={vi.fn()}
        />
      </I18nextProvider>
    )
    expect(screen.getByRole('button', { name: /^\$100 / })).toHaveTextContent(
      ratio ? 'Amount received $130' : 'Amount received $110'
    )
    expect(
      screen.queryByText('Exclusive extra bonus', { exact: false }) !== null
    ).toBe(ratio > 0)
    expect(
      screen.queryByText(
        'Higher top-up amounts earn larger bonuses, up to 40% extra.'
      ) !== null
    ).toBe(ratio === 0)
  }
)

it.each(['standard', 'creem'])(
  'shows additive public and exclusive credit in the %s payment confirmation',
  async (kind) => {
    await i18n.changeLanguage('en')
    render(
      <I18nextProvider i18n={i18n}>
        {kind === 'standard' ? (
          <PaymentConfirmDialog
            open
            onOpenChange={vi.fn()}
            onConfirm={vi.fn()}
            topupAmount={100}
            paymentAmount={100}
            paymentMethod={{ name: 'Stripe', type: 'stripe' }}
            calculating={false}
            processing={false}
            bonusRate={0.1}
            exclusiveBonusRatio={2000}
          />
        ) : (
          <CreemConfirmDialog
            open
            onOpenChange={vi.fn()}
            onConfirm={vi.fn()}
            product={{
              name: 'Credit',
              productId: 'product',
              price: 100,
              currency: 'USD',
              quota: 50000000,
            }}
            processing={false}
            publicBonusRate={0.1}
            exclusiveBonusRatio={2000}
          />
        )}
      </I18nextProvider>
    )
    expect(screen.getByText('Public bonus 10%')).toBeVisible()
    expect(screen.getByText('Exclusive extra bonus 20%')).toBeVisible()
    expect(screen.getByText('$130')).toBeVisible()
    expect(
      screen.getByText('Exclusive bonus credit does not earn referral rewards.')
    ).toBeVisible()
  }
)

it.each([0, 2000])(
  'shows saved exclusive bonus facts in order details only when the order has them (%i)',
  async (ratio) => {
    await i18n.changeLanguage('en')
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: {
          total: 1,
          items: [
            {
              id: 1,
              user_id: 1,
              status: 'pending',
              amount: 100,
              money: 100,
              promised_quota: 65000000,
              public_bonus_rate: 0.1,
              exclusive_bonus_ratio: ratio,
              exclusive_bonus_quota: 10000000,
              trade_no: 'snapshot-order',
              payment_method: 'nowpayments',
              create_time: 1,
            },
          ],
        },
      },
    })
    render(
      <QueryClientProvider client={new QueryClient()}>
        <I18nextProvider i18n={i18n}>
          <BillingHistoryDialog open onOpenChange={vi.fn()} />
        </I18nextProvider>
      </QueryClientProvider>
    )
    await userEvent.click(await screen.findByText('snapshot-order'))
    expect(screen.getByRole('dialog', { name: 'Top-up Details' })).toBeVisible()
    expect(screen.queryByText('Exclusive extra bonus') !== null).toBe(ratio > 0)
    if (ratio) {
      expect(screen.getByText('20% · $20')).toBeVisible()
      expect(screen.getByText('$130')).toBeVisible()
    }
  }
)

beforeEach(async () => {
  await i18n.use(initReactI18next).init({
    lng: 'zh',
    fallbackLng: false,
    nsSeparator: false,
    resources: { en, zh },
    interpolation: { escapeValue: false },
  })
})

it('translates billing history statuses and updates them when the language changes', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: {
        total: 3,
        items: ['success', 'pending', 'expired'].map((status, id) => ({
          id,
          status,
          amount: 10,
          money: 10,
          trade_no: `order-${id}`,
          payment_method: 'stripe',
          create_time: 1,
        })),
      },
    },
  })
  render(
    <QueryClientProvider client={new QueryClient()}>
      <I18nextProvider i18n={i18n}>
        <BillingHistoryDialog open onOpenChange={vi.fn()} />
      </I18nextProvider>
    </QueryClientProvider>
  )
  await screen.findByText('order-0')
  for (const label of ['成功', '待确认', '已过期']) {
    expect(screen.getByText(label)).toBeVisible()
  }
  await act(() => i18n.changeLanguage('en'))
  for (const label of ['Success', 'Pending', 'Expired']) {
    expect(screen.getByText(label)).toBeVisible()
  }
})

it.each([
  { priceRatio: 7.3, paid: '584', full: '146' },
  { priceRatio: 0.000123, paid: '0.0098', full: '0.0025' },
])(
  'translates recharge presets and minimum amount without changing precision at price ratio $priceRatio',
  async ({ priceRatio, paid, full }) => {
    render(
      <I18nextProvider i18n={i18n}>
        <RechargeFormCard
          topupInfo={{
            enable_online_topup: true,
            enable_stripe_topup: false,
            pay_methods: [],
            min_topup: 1,
            stripe_min_topup: 1,
            amount_options: [100, 20],
            discount: {},
            bonus: {},
            quota_for_inviter: 0,
            quota_for_invitee: 0,
            quota_for_inviter_on_first_topup: 0,
          }}
          presetAmounts={[{ value: 100, discount: 0.8 }, { value: 20 }]}
          selectedPreset={null}
          topupAmount={1}
          paymentAmount={priceRatio}
          priceRatio={priceRatio}
          calculating={false}
          paymentLoading={null}
          redemptionCode=''
          redeeming={false}
          onSelectPreset={vi.fn()}
          onTopupAmountChange={vi.fn()}
          onPaymentMethodSelect={vi.fn()}
          onRedemptionCodeChange={vi.fn()}
          onRedeem={vi.fn()}
        />
      </I18nextProvider>
    )
    expect(screen.getByRole('button', { name: /^\$100 / })).toHaveTextContent(
      `$100实付金额 ¥${paid}加赠0%实到 $100`
    )
    expect(screen.getByRole('button', { name: /^\$20 / })).toHaveTextContent(
      `$20实付金额 ¥${full}加赠0%实到 $20`
    )
    expect(screen.getByPlaceholderText('最低 1')).toBeVisible()
    await act(() => i18n.changeLanguage('en'))
    expect(screen.getByRole('button', { name: /^\$100 / })).toHaveTextContent(
      `$100Amount paid ¥${paid}Bonus 0%Amount received $100`
    )
    expect(screen.getByRole('button', { name: /^\$20 / })).toHaveTextContent(
      `$20Amount paid ¥${full}Bonus 0%Amount received $20`
    )
    expect(screen.getByPlaceholderText('Minimum 1')).toBeVisible()
  }
)
