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
  ListOrdered,
  ShoppingCart,
  type LucideIcon,
  UserPlus,
  UsersRound,
} from 'lucide-react'
import type { ComponentProps } from 'react'
import { useTranslation } from 'react-i18next'

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Empty,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { IconBadge } from '@/components/ui/icon-badge'
import { Skeleton } from '@/components/ui/skeleton'
import { getAdminBusinessMetrics } from '@/features/dashboard/api'
import type { AdminBusinessTopUpUser } from '@/features/dashboard/types'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'

interface BusinessMetricsProps {
  startTimestamp: number
  endTimestamp: number
}

interface BusinessMetric {
  key: string
  title: string
  value: string
  detail: string
  icon: LucideIcon
  tone: ComponentProps<typeof IconBadge>['tone']
}

interface MetricCardProps {
  title: string
  icon: LucideIcon
  metrics: BusinessMetric[]
  isLoading: boolean
  isError: boolean
  failedLabel: string
}

function MetricCard(props: MetricCardProps) {
  const SectionIcon = props.icon

  return (
    <Card size='sm'>
      <CardHeader>
        <CardTitle className='flex items-center gap-2'>
          <IconBadge tone='info' size='sm'>
            <SectionIcon />
          </IconBadge>
          {props.title}
        </CardTitle>
      </CardHeader>
      <CardContent className='grid grid-cols-2 gap-2'>
        {props.metrics.map((metric, index) => {
          const MetricIcon = metric.icon
          return (
            <div
              key={metric.key}
              className={cn(
                'bg-muted/40 min-w-0 rounded-lg p-3',
                props.metrics.length % 2 === 1 &&
                  index === props.metrics.length - 1 &&
                  'col-span-2'
              )}
            >
              <div className='text-muted-foreground flex min-w-0 items-center gap-2 text-xs font-medium'>
                <IconBadge tone={metric.tone} size='stat'>
                  <MetricIcon />
                </IconBadge>
                <span className='truncate'>{metric.title}</span>
              </div>
              {props.isLoading ? (
                <div className='mt-2 flex flex-col gap-1.5'>
                  <Skeleton className='h-6 w-20' />
                  <Skeleton className='h-3 w-28' />
                </div>
              ) : (
                <>
                  <div
                    className='mt-2 font-mono text-xl font-semibold break-words tabular-nums'
                    title={metric.value}
                  >
                    {props.isError ? '--' : metric.value}
                  </div>
                  <div className='text-muted-foreground mt-1 text-[11px]'>
                    {props.isError ? props.failedLabel : metric.detail}
                  </div>
                </>
              )}
            </div>
          )
        })}
      </CardContent>
    </Card>
  )
}

function TopUpRankingChart(props: {
  rows: AdminBusinessTopUpUser[]
  locale?: string
  usdExchangeRate: number
}) {
  const { t } = useTranslation()
  const maxAmount = Math.max(...props.rows.map((item) => item.amount), 0)
  const formatCurrency = (amount: number, currency: 'USD' | 'CNY') =>
    new Intl.NumberFormat(props.locale, {
      style: 'currency',
      currency,
      currencyDisplay: 'narrowSymbol',
      maximumFractionDigits: 2,
    }).format(amount)

  return (
    <ol
      className='space-y-3 px-4 pb-4 sm:px-5'
      aria-label={t('User Top-up Ranking')}
    >
      {props.rows.map((item) => {
        const width = maxAmount > 0 ? (item.amount / maxAmount) * 100 : 0
        return (
          <li key={item.user_id} className='space-y-1.5'>
            <div className='flex items-start justify-between gap-3 text-sm select-text'>
              <div className='min-w-0 font-medium break-all'>
                <span className='text-muted-foreground mr-2 font-mono'>
                  #{item.rank}
                </span>
                {item.username}
              </div>
              <div className='shrink-0 text-right font-mono text-xs tabular-nums'>
                <div className='font-semibold'>
                  {formatCurrency(item.amount, 'USD')}
                </div>
                <div className='text-muted-foreground'>
                  ≈ {formatCurrency(item.amount * props.usdExchangeRate, 'CNY')}
                </div>
              </div>
            </div>
            <div className='bg-muted h-2.5 overflow-hidden rounded-full'>
              <div
                className='bg-chart-3 h-full rounded-full'
                style={{ width: `${width}%` }}
                aria-hidden='true'
              />
            </div>
            <div className='text-muted-foreground text-right text-[11px] select-text'>
              {t('Paid Orders')} · {formatNumber(item.orders, props.locale)}
            </div>
          </li>
        )
      })}
    </ol>
  )
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
  const configuredUsdExchangeRate = data?.usd_exchange_rate ?? 1
  const usdExchangeRate =
    Number.isFinite(configuredUsdExchangeRate) && configuredUsdExchangeRate > 0
      ? configuredUsdExchangeRate
      : 1

  const formatCurrency = (amount: number, currency: 'USD' | 'CNY') =>
    new Intl.NumberFormat(locale, {
      style: 'currency',
      currency,
      currencyDisplay: 'narrowSymbol',
      maximumFractionDigits: 2,
    }).format(amount)
  const formatUSD = (amount: number) => formatCurrency(amount, 'USD')
  const formatRMB = (amountUSD: number) =>
    formatCurrency(amountUSD * usdExchangeRate, 'CNY')
  const payingUsers = data?.paying_users ?? 0
  const newPurchasingUsers = data?.new_purchasing_users ?? 0
  const repeatPurchasingUsers = Math.max(payingUsers - newPurchasingUsers, 0)
  const repeatPurchaseRate = payingUsers
    ? repeatPurchasingUsers / payingUsers
    : 0
  const newUserPurchaseRate = data?.new_users
    ? (data.new_user_purchasing_users ?? 0) / data.new_users
    : 0
  const topUpPaidAmountUSD = data?.top_up_paid_amount_usd ?? 0
  const topUpPaidOrders = data?.top_up_paid_orders ?? 0
  const averageTopUpAmountUSD = topUpPaidOrders
    ? topUpPaidAmountUSD / topUpPaidOrders
    : 0
  const userMetrics: BusinessMetric[] = [
    {
      key: 'new-users',
      title: t('New Users'),
      value: formatNumber(data?.new_users, locale),
      detail: t('Registered in this period'),
      icon: UserPlus,
      tone: 'chart-1',
    },
    {
      key: 'paying-users',
      title: t('Paying Users'),
      value: formatNumber(payingUsers, locale),
      detail: t('Unique users who paid'),
      icon: UsersRound,
      tone: 'chart-2',
    },
    {
      key: 'new-purchasers',
      title: t('New Purchasers'),
      value: formatNumber(newPurchasingUsers, locale),
      detail: `${t('New User Purchase Rate')} · ${formatNumber(newUserPurchaseRate * 100, locale)}%`,
      icon: UserPlus,
      tone: 'chart-1',
    },
    {
      key: 'repeat-purchasers',
      title: t('Repeat Purchasers'),
      value: formatNumber(repeatPurchasingUsers, locale),
      detail: `${t('Repeat Purchase Rate')} · ${formatNumber(repeatPurchaseRate * 100, locale)}%`,
      icon: UsersRound,
      tone: 'chart-2',
    },
  ]
  const revenueMetrics: BusinessMetric[] = [
    {
      key: 'revenue',
      title: t('Credited amount'),
      value: formatUSD(topUpPaidAmountUSD),
      detail: `≈ ${formatRMB(topUpPaidAmountUSD)} · ${t('Paid Orders')} · ${formatNumber(topUpPaidOrders, locale)}`,
      icon: CircleDollarSign,
      tone: 'success',
    },
    {
      key: 'average-order',
      title: t('Average credited amount'),
      value: formatUSD(averageTopUpAmountUSD),
      detail: `≈ ${formatRMB(averageTopUpAmountUSD)}`,
      icon: CircleDollarSign,
      tone: 'warning',
    },
  ]
  const orderMetrics: BusinessMetric[] = [
    {
      key: 'intent-orders',
      title: t('Order Intents'),
      value: formatNumber(data?.intent_orders, locale),
      detail: `${t('Intent Amount')} · ${formatUSD(data?.top_up_intent_amount_usd ?? 0)} · ≈ ${formatRMB(data?.top_up_intent_amount_usd ?? 0)}`,
      icon: ShoppingCart,
      tone: 'chart-3',
    },
    {
      key: 'paid-orders',
      title: t('Paid Orders'),
      value: formatNumber(data?.paid_orders, locale),
      detail: t('Completed'),
      icon: BadgeCheck,
      tone: 'success',
    },
    {
      key: 'success-rate',
      title: t('Payment Success Rate'),
      value: `${formatNumber((data?.payment_success_rate ?? 0) * 100, locale)}%`,
      detail: `${t('Paid Orders')} · ${formatNumber(data?.paid_orders, locale)} / ${formatNumber(data?.intent_orders, locale)}`,
      icon: BadgeCheck,
      tone: 'warning',
    },
  ]
  const failedLabel = t('Failed to load')
  let rankingContent = (
    <div className='flex flex-col gap-2 px-3'>
      {Array.from({ length: 5 }, (_, index) => (
        <Skeleton key={index} className='h-10 w-full' />
      ))}
    </div>
  )
  if (!isLoading && (isError || !data?.top_up_ranking?.length)) {
    rankingContent = (
      <Empty className='border-0 py-8'>
        <EmptyHeader>
          <EmptyMedia variant='icon'>
            <ListOrdered />
          </EmptyMedia>
          <EmptyTitle>
            {isError ? t('Failed to load') : t('No data')}
          </EmptyTitle>
        </EmptyHeader>
      </Empty>
    )
  } else if (!isLoading && data?.top_up_ranking?.length) {
    rankingContent = (
      <TopUpRankingChart
        rows={data.top_up_ranking}
        locale={locale}
        usdExchangeRate={usdExchangeRate}
      />
    )
  }

  return (
    <section className='flex flex-col gap-3'>
      <div>
        <h2 className='text-sm font-semibold'>{t('Business Overview')}</h2>
        <p className='text-muted-foreground mt-0.5 text-xs'>
          {t('Registration and payment results for the selected period.')}
        </p>
      </div>

      <div className='grid gap-3 lg:grid-cols-3'>
        <MetricCard
          title={t('User Analytics')}
          icon={UsersRound}
          metrics={userMetrics}
          isLoading={isLoading}
          isError={isError}
          failedLabel={failedLabel}
        />
        <MetricCard
          title={t('Top-up')}
          icon={CircleDollarSign}
          metrics={revenueMetrics}
          isLoading={isLoading}
          isError={isError}
          failedLabel={failedLabel}
        />
        <MetricCard
          title={t('Order Statistics')}
          icon={ShoppingCart}
          metrics={orderMetrics}
          isLoading={isLoading}
          isError={isError}
          failedLabel={failedLabel}
        />
      </div>

      <Card size='sm'>
        <CardHeader>
          <CardTitle className='flex items-center gap-2'>
            <IconBadge tone='chart-3' size='sm'>
              <ListOrdered />
            </IconBadge>
            {t('User Top-up Ranking')}
          </CardTitle>
          <CardDescription>
            {t(
              'Top 10 users by credited USD value. RMB values use the configured exchange rate.'
            )}
          </CardDescription>
        </CardHeader>
        <CardContent className='px-0'>{rankingContent}</CardContent>
      </Card>
    </section>
  )
}
