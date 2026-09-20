/* Copyright (C) 2023-2026 QuantumNous
 * SPDX-License-Identifier: AGPL-3.0-or-later
 * For commercial licensing, please contact support@quantumnous.com
 */
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { getPricing } from '@/features/pricing/api'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'
import { requireServerSuccess } from '@/lib/server-error-message'

export function SavingsComparison(props: {
  examples: { amount: number; bonus: number; credit: string }[]
}) {
  const { t, i18n } = useTranslation()
  const [usage, setUsage] = useState('100')
  const [topup, setTopup] = useState(280)
  const example = props.examples.find((item) => item.amount === topup)
  const bonus = (example?.bonus ?? 0) / 100
  const amount = Number(usage)
  const valid =
    usage.trim() !== '' &&
    Number.isFinite(amount) &&
    amount >= 0 &&
    amount <= 1_000_000
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  // This editorial comparison must stay in USD, independently of wallet display currency.
  const usd = new Intl.NumberFormat(locale, {
    style: 'currency',
    currency: 'USD',
  })
  const pricing = useQuery({
    queryKey: ['pricing'],
    queryFn: async () => requireServerSuccess(await getPricing()),
    staleTime: 5 * 60 * 1000,
  })
  const standard = pricing.data?.group_ratio?.default
  const groups = Object.entries(pricing.data?.group_ratio ?? {}).filter(
    ([group, ratio]) =>
      group !== 'auto' &&
      Number.isFinite(ratio) &&
      ratio >= 0 &&
      group in (pricing.data?.usable_group ?? {})
  )
  const available =
    standard !== undefined &&
    Number.isFinite(standard) &&
    standard > 0 &&
    groups.length > 0

  return (
    <div className='mt-14 border-t pt-10' aria-labelledby='savings-title'>
      <h3 id='savings-title' className='text-2xl font-semibold tracking-tight'>
        {t('See exactly how much you save')}
      </h3>
      <p className='text-muted-foreground mt-3 max-w-3xl text-sm leading-7'>
        {t(
          'Compare the same usage across groups: balance deducted versus the cash cost before top-up bonuses.'
        )}
      </p>
      <div className='mt-6 grid max-w-3xl gap-4 sm:grid-cols-2'>
        <Field>
          <FieldLabel htmlFor='savings-usage'>
            {t('Usage at standard-group prices (USD)')}
          </FieldLabel>
          <Input
            id='savings-usage'
            type='number'
            min='0'
            max='1000000'
            step='0.01'
            value={usage}
            aria-invalid={!valid}
            onChange={(event) => setUsage(event.target.value)}
          />
        </Field>
        <Field>
          <FieldLabel htmlFor='savings-topup'>{t('Top-up example')}</FieldLabel>
          <NativeSelect
            id='savings-topup'
            className='w-full'
            value={topup}
            onChange={(event) => setTopup(Number(event.target.value))}
          >
            {props.examples.map((item) => (
              <NativeSelectOption key={item.amount} value={item.amount}>
                {t(
                  'Pay {{amount}} USD → receive {{credit}} USD (+{{bonus}}%)',
                  {
                    amount: formatNumber(item.amount, locale),
                    credit: item.credit,
                    bonus: item.bonus,
                  }
                )}
              </NativeSelectOption>
            ))}
          </NativeSelect>
        </Field>
      </div>
      {!valid && (
        <p role='alert' className='text-destructive mt-3 text-sm'>
          {t('Enter an amount from 0 to 1,000,000 USD.')}
        </p>
      )}
      {pricing.isPending && (
        <p role='status' className='mt-6 text-sm'>
          {t('Loading...')}
        </p>
      )}
      {!pricing.isPending && (pricing.isError || !available) && (
        <p role='status' className='mt-6 text-sm'>
          {t(
            'Group comparison is unavailable. Check current model pricing below.'
          )}
        </p>
      )}
      {available && !pricing.isError && valid && (
        <div className='mt-6 overflow-x-auto'>
          <table className='w-full min-w-[480px] text-left text-sm tabular-nums'>
            <caption className='sr-only'>
              {t('Group cost comparison (USD)')}
            </caption>
            <thead className='text-muted-foreground border-b'>
              <tr>
                <th scope='col' className='py-3 pr-4 font-medium'>
                  {t('Group')}
                </th>
                <th scope='col' className='px-3 py-3 text-right font-medium'>
                  {t('Balance deducted (USD)')}
                </th>
                <th scope='col' className='px-3 py-3 text-right font-medium'>
                  {t('Effective cash cost (USD)')}
                </th>
                <th scope='col' className='py-3 pl-3 text-right font-medium'>
                  {t('Savings (USD)')}
                </th>
              </tr>
            </thead>
            <tbody>
              {groups.map(([group, ratio]) => {
                const charged = (amount * ratio) / (standard ?? 1)
                const cost = charged / (1 + bonus)
                return (
                  <tr key={group} className='border-b'>
                    <th scope='row' className='py-4 pr-4 font-medium'>
                      <Link
                        to='/pricing'
                        search={{ group }}
                        className='underline underline-offset-4'
                      >
                        {group}
                      </Link>
                    </th>
                    <td className='px-3 py-4 text-right'>
                      {usd.format(charged)}
                    </td>
                    <td className='px-3 py-4 text-right font-semibold'>
                      {usd.format(cost)}
                    </td>
                    <td className='py-4 pl-3 text-right'>
                      {usd.format(amount - cost)}
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}
      <p className='text-muted-foreground mt-4 max-w-4xl text-xs leading-6'>
        {t(
          'Balance deducted = standard-group usage × relative group rate. Cash cost = balance deducted ÷ (1 + bonus). Savings are relative to standard-group usage without a bonus; negative savings mean higher cost.'
        )}
      </p>
      <p className='text-muted-foreground mt-2 max-w-4xl text-xs leading-6'>
        {t(
          'Illustration, not your account history. Assumes all purchased credits are used at the selected bonus rate. Group rates are current; model availability and pricing vary. Top-up examples exclude fees, tax and currency conversion; checkout terms apply.'
        )}
      </p>
    </div>
  )
}
