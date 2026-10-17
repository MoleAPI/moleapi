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
import { useTranslation } from 'react-i18next'

import { toIntlLocale } from '@/i18n/languages'
import { formatCurrencyFromUSD } from '@/lib/currency'
import { formatNumber } from '@/lib/format'

export function ExclusiveTopupBonus(props: {
  amount: number
  publicBonusRate: number
  exclusiveBonusRatio: number
}) {
  const { t, i18n } = useTranslation()
  if (props.exclusiveBonusRatio <= 0) return null

  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const exclusiveRate = props.exclusiveBonusRatio / 10000
  const rows = [
    { label: t('Public bonus'), rate: props.publicBonusRate },
    { label: t('Exclusive extra bonus'), rate: exclusiveRate },
  ]

  return (
    <div className='bg-success/10 space-y-2 rounded-lg p-3 text-sm'>
      {rows.map((row) => (
        <div
          key={row.label}
          className='flex flex-wrap justify-between gap-x-3 gap-y-1'
        >
          <span>
            {row.label} {formatNumber(row.rate * 100, locale)}%
          </span>
          <span>
            +
            {formatCurrencyFromUSD(props.amount * row.rate, {
              abbreviate: false,
            })}
          </span>
        </div>
      ))}
      <div className='flex flex-wrap justify-between gap-2 border-t pt-2 font-semibold'>
        <span>{t('Amount received')}</span>
        <span>
          {formatCurrencyFromUSD(
            props.amount * (1 + props.publicBonusRate + exclusiveRate),
            { abbreviate: false }
          )}
        </span>
      </div>
      <p className='text-muted-foreground text-xs'>
        {t('Exclusive bonus credit does not earn referral rewards.')}
      </p>
    </div>
  )
}
