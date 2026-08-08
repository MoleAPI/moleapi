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
import { useQuery } from '@tanstack/react-query'
import {
  BadgeCheck,
  CircleDollarSign,
  ShoppingCart,
  UserPlus,
  UsersRound,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { IconBadge } from '@/components/ui/icon-badge'
import { Skeleton } from '@/components/ui/skeleton'
import { getAdminBusinessMetrics } from '@/features/dashboard/api'
import type { AdminBusinessOrderAmount } from '@/features/dashboard/types'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'

interface BusinessMetricsProps {
  startTimestamp: number
  endTimestamp: number
}

export function BusinessMetrics(props: BusinessMetricsProps) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const { data, isLoading, isError } = useQuery({
    queryKey: [
      'dashboard',
      'business-metrics',
      props.startTimestamp,
      props.endTimestamp,
    ],
    queryFn: async () => {
      const response = await getAdminBusinessMetrics({
        start_timestamp: props.startTimestamp,
        end_timestamp: props.endTimestamp,
      })
      if (!response.success || !response.data) {
        throw new Error(response.message || 'Failed to load business metrics')
      }
      return response.data
    },
    staleTime: 60_000,
  })

  const formatAmounts = (
    amounts: AdminBusinessOrderAmount[] | undefined,
    average = false
  ) => {
    if (!amounts?.length) return formatNumber(0, locale)
    return amounts
      .map((item) => {
        const value = average ? item.average_amount : item.amount
        if (!item.currency) return formatNumber(value, locale)
        try {
          return new Intl.NumberFormat(locale, {
            style: 'currency',
            currency: item.currency,
            maximumFractionDigits: 2,
          }).format(value)
        } catch {
          return `${formatNumber(value, locale)} ${item.currency}`
        }
      })
      .join(' · ')
  }
  const payingUsers = data?.paying_users ?? 0
  const newPurchasingUsers = data?.new_purchasing_users ?? 0
  const repeatPurchasingUsers = Math.max(payingUsers - newPurchasingUsers, 0)
  const repeatPurchaseRate = payingUsers
    ? repeatPurchasingUsers / payingUsers
    : 0
  const newUserPurchaseRate = data?.new_users
    ? (data.new_user_purchasing_users ?? 0) / data.new_users
    : 0
  const metrics = [
    {
      key: 'new-users',
      title: t('New Users'),
      value: formatNumber(data?.new_users, locale),
      detail: t('Registered in this period'),
      icon: UserPlus,
      tone: 'chart-1' as const,
    },
    {
      key: 'paying-users',
      title: t('Paying Users'),
      value: formatNumber(payingUsers, locale),
      detail: t('Unique users who paid'),
      icon: UsersRound,
      tone: 'chart-2' as const,
    },
    {
      key: 'new-purchasers',
      title: t('New Purchasers'),
      value: formatNumber(newPurchasingUsers, locale),
      detail: `${t('New User Purchase Rate')} · ${formatNumber(newUserPurchaseRate * 100, locale)}%`,
      icon: UserPlus,
      tone: 'chart-1' as const,
    },
    {
      key: 'repeat-purchasers',
      title: t('Repeat Purchasers'),
      value: formatNumber(repeatPurchasingUsers, locale),
      detail: `${t('Repeat Purchase Rate')} · ${formatNumber(repeatPurchaseRate * 100, locale)}%`,
      icon: UsersRound,
      tone: 'chart-2' as const,
    },
    {
      key: 'intent-orders',
      title: t('Order Intents'),
      value: formatNumber(data?.intent_orders, locale),
      detail: `${t('Intent Amount')} · ${formatAmounts(data?.intent_amounts)}`,
      icon: ShoppingCart,
      tone: 'chart-3' as const,
    },
    {
      key: 'paid-orders',
      title: t('Paid Orders'),
      value: formatNumber(data?.paid_orders, locale),
      detail: `${t('Paid Amount')} · ${formatAmounts(data?.paid_amounts)}`,
      icon: BadgeCheck,
      tone: 'success' as const,
    },
    {
      key: 'average-order',
      title: t('Average Order Value'),
      value: formatAmounts(data?.paid_amounts, true),
      detail: `${t('Payment Success Rate')} · ${formatNumber((data?.payment_success_rate ?? 0) * 100, locale)}%`,
      icon: CircleDollarSign,
      tone: 'warning' as const,
    },
  ]

  return (
    <section className='overflow-hidden rounded-lg border'>
      <div className='border-b px-3 py-2 sm:px-5 sm:py-3'>
        <h2 className='text-sm font-semibold'>{t('Growth and Revenue')}</h2>
        <p className='text-muted-foreground mt-0.5 text-xs'>
          {t('Registration and payment results for the selected period.')}
        </p>
      </div>
      <div className='divide-border/60 grid grid-cols-2 divide-x sm:grid-cols-3 xl:grid-cols-5'>
        {metrics.map((metric, index) => {
          const Icon = metric.icon
          return (
            <div
              key={metric.key}
              className={cn(
                'min-w-0 px-3 py-3 sm:px-4 sm:py-4',
                index === metrics.length - 1 && 'col-span-2 sm:col-span-1'
              )}
            >
              <div className='text-muted-foreground flex min-w-0 items-center gap-2 text-xs font-medium'>
                <IconBadge tone={metric.tone} size='stat'>
                  <Icon />
                </IconBadge>
                <span className='truncate'>{metric.title}</span>
              </div>
              {isLoading ? (
                <div className='mt-2 space-y-1.5'>
                  <Skeleton className='h-7 w-20' />
                  <Skeleton className='h-3 w-28' />
                </div>
              ) : (
                <>
                  <div
                    className='mt-2 truncate font-mono text-xl font-semibold tabular-nums sm:text-2xl'
                    title={metric.value}
                  >
                    {isError ? '--' : metric.value}
                  </div>
                  <div
                    className='text-muted-foreground mt-1 truncate text-[11px]'
                    title={metric.detail}
                  >
                    {isError ? t('Failed to load') : metric.detail}
                  </div>
                </>
              )}
            </div>
          )
        })}
      </div>
    </section>
  )
}
