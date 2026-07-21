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
import { VChart } from '@visactor/react-vchart'
import { Activity, AlertCircle, ChartNoAxesCombined } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { IconBadge } from '@/components/ui/icon-badge'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { getChannelSuccessMetrics } from '@/features/dashboard/api'
import { getDashboardChartColors } from '@/features/dashboard/lib/charts'
import type { ChannelSuccessSummary } from '@/features/dashboard/types'
import {
  formatUptimePct,
  getSuccessRateColor,
  getSuccessRateTextClass,
} from '@/features/performance-metrics/lib/format'
import { formatChartTime } from '@/lib/time'
import { useChartTheme } from '@/lib/use-chart-theme'
import { cn } from '@/lib/utils'
import { VCHART_OPTION } from '@/lib/vchart'

const TIME_RANGES = [
  { hours: 6, labelKey: 'Last 6 hours' },
  { hours: 24, labelKey: 'Last 24 hours' },
  { hours: 168, labelKey: 'Last 7 days' },
] as const
const MAX_CHART_CHANNELS = 8

type TimeRangeHours = (typeof TIME_RANGES)[number]['hours']

export function ChannelSuccessCharts() {
  const { t } = useTranslation()
  const { resolvedTheme, themeReady } = useChartTheme()
  const [hours, setHours] = useState<TimeRangeHours>(24)
  const metricsQuery = useQuery({
    queryKey: ['channel-success-metrics', hours],
    queryFn: () => getChannelSuccessMetrics(hours),
    staleTime: 60 * 1000,
    retry: false,
  })
  const channels = useMemo(
    () => metricsQuery.data?.data.channels ?? [],
    [metricsQuery.data]
  )
  const chartSpec = useMemo(
    () =>
      buildChannelChartSpec(
        channels.slice(0, MAX_CHART_CHANNELS),
        resolvedTheme
      ),
    [channels, resolvedTheme]
  )

  return (
    <section className='overflow-hidden rounded-lg border'>
      <header className='flex flex-col gap-3 border-b px-4 py-3 sm:px-5 lg:flex-row lg:items-center lg:justify-between'>
        <div className='flex min-w-0 items-start gap-2.5'>
          <IconBadge tone='success' size='sm'>
            <Activity />
          </IconBadge>
          <div className='min-w-0'>
            <h2 className='text-sm font-semibold'>
              {t('Channel Success Rate')}
            </h2>
            <p className='text-muted-foreground mt-0.5 text-xs'>
              {t('Calculated from upstream attempts, including retries.')}
            </p>
          </div>
        </div>
        <Tabs
          value={String(hours)}
          onValueChange={(value) => setHours(Number(value) as TimeRangeHours)}
        >
          <TabsList className='h-8'>
            {TIME_RANGES.map((range) => (
              <TabsTrigger
                key={range.hours}
                value={String(range.hours)}
                className='px-3 text-xs'
              >
                {t(range.labelKey)}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
      </header>

      {metricsQuery.isError && (
        <div className='p-4 sm:p-5'>
          <Alert variant='destructive'>
            <AlertCircle />
            <AlertTitle>{t('Failed to load')}</AlertTitle>
            <AlertDescription>{t('Please try again later.')}</AlertDescription>
          </Alert>
        </div>
      )}
      {!metricsQuery.isError && metricsQuery.isLoading && (
        <ChannelSuccessLoading />
      )}
      {!metricsQuery.isError &&
        !metricsQuery.isLoading &&
        channels.length === 0 && (
          <Empty className='min-h-80 rounded-none border-0'>
            <EmptyHeader>
              <EmptyMedia variant='icon'>
                <Activity />
              </EmptyMedia>
              <EmptyTitle>
                {t('No channel performance data available')}
              </EmptyTitle>
              <EmptyDescription>
                {t(
                  'Channel performance data appears after new calls are recorded.'
                )}
              </EmptyDescription>
            </EmptyHeader>
          </Empty>
        )}
      {!metricsQuery.isError &&
        !metricsQuery.isLoading &&
        channels.length > 0 && (
          <div className='grid xl:grid-cols-[minmax(0,1.7fr)_minmax(20rem,1fr)]'>
            <div className='min-w-0 border-b p-3 sm:p-4 xl:border-r xl:border-b-0'>
              <div className='mb-2 flex items-center gap-2 px-1'>
                <ChartNoAxesCombined className='text-muted-foreground size-4' />
                <h3 className='text-xs font-semibold'>
                  {t('Success rate trend')}
                </h3>
              </div>
              <div className='h-[360px]'>
                {themeReady && (
                  <VChart
                    key={`${hours}-${resolvedTheme}`}
                    spec={chartSpec}
                    option={VCHART_OPTION}
                  />
                )}
              </div>
            </div>
            <div className='min-w-0 p-3 sm:p-4'>
              <h3 className='mb-2 px-1 text-xs font-semibold'>
                {t('Channel ranking')}
              </h3>
              <div className='max-h-[390px] space-y-1 overflow-y-auto pr-1'>
                {channels.map((channel) => (
                  <ChannelRankItem key={channel.channel_id} channel={channel} />
                ))}
              </div>
            </div>
          </div>
        )}
    </section>
  )
}

function ChannelRankItem({ channel }: { channel: ChannelSuccessSummary }) {
  const rate = Math.max(0, Math.min(100, channel.success_rate))

  return (
    <div className='hover:bg-muted/45 rounded-md px-2.5 py-2 transition-colors'>
      <div className='flex items-center justify-between gap-3'>
        <div className='min-w-0'>
          <div className='truncate text-xs font-medium'>
            {channel.channel_name}
          </div>
          <div className='text-muted-foreground mt-0.5 font-mono text-[10px] tabular-nums'>
            #{channel.channel_id} · {channel.success_count.toLocaleString()} /{' '}
            {channel.request_count.toLocaleString()}
          </div>
        </div>
        <span
          className={cn(
            'shrink-0 font-mono text-xs font-semibold tabular-nums',
            getSuccessRateTextClass(channel.success_rate)
          )}
        >
          {formatUptimePct(channel.success_rate)}
        </span>
      </div>
      <div
        className='bg-muted mt-2 h-1 overflow-hidden rounded-full'
        role='meter'
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={rate}
      >
        <div
          className='h-full rounded-full transition-[width]'
          style={{
            width: `${rate}%`,
            backgroundColor: getSuccessRateColor(channel.success_rate),
          }}
        />
      </div>
    </div>
  )
}

function ChannelSuccessLoading() {
  return (
    <div className='grid xl:grid-cols-[minmax(0,1.7fr)_minmax(20rem,1fr)]'>
      <div className='border-b p-4 xl:border-r xl:border-b-0'>
        <Skeleton className='h-[360px] w-full' />
      </div>
      <div className='space-y-3 p-4'>
        {Array.from({ length: 6 }, (_, index) => (
          <Skeleton key={index} className='h-12 w-full' />
        ))}
      </div>
    </div>
  )
}

function buildChannelChartSpec(
  channels: ChannelSuccessSummary[],
  resolvedTheme: string
) {
  const timestamps = [
    ...new Set(
      channels.flatMap((channel) => channel.series.map((point) => point.ts))
    ),
  ].sort((a, b) => a - b)
  const values = channels.flatMap((channel) => {
    const points = new Map(channel.series.map((point) => [point.ts, point]))
    return timestamps.map((timestamp) => {
      const point = points.get(timestamp)
      return {
        time: formatChartTime(timestamp, 'hour'),
        rate: point?.success_rate ?? null,
        requests: point?.request_count ?? 0,
        successes: point?.success_count ?? 0,
        channel: `${channel.channel_name} #${channel.channel_id}`,
      }
    })
  })

  return {
    type: 'line',
    theme: resolvedTheme === 'dark' ? 'dark' : 'light',
    background: 'transparent',
    data: { values },
    xField: 'time',
    yField: 'rate',
    seriesField: 'channel',
    color: getDashboardChartColors(channels.length),
    invalidType: 'break',
    point: { visible: false },
    line: { style: { lineWidth: 2 } },
    axes: [
      {
        orient: 'left',
        min: 0,
        max: 100,
        label: { formatMethod: (value: number) => `${value}%` },
      },
      { orient: 'bottom', label: { autoRotate: true, autoHide: true } },
    ],
    legends: {
      visible: true,
      orient: 'bottom',
      position: 'middle',
      maxRow: 2,
    },
    tooltip: {
      dimension: {
        title: { value: (datum: { time: string }) => datum.time },
      },
    },
    padding: { top: 12, right: 16, bottom: 8, left: 8 },
  }
}
