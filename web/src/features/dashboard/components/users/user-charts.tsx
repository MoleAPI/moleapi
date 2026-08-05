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
import { CalendarDays, Users, Loader2 } from 'lucide-react'
import { useEffect, useMemo, useState, useRef, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { DateTimePicker } from '@/components/datetime-picker'
import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { IconBadge } from '@/components/ui/icon-badge'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useTheme } from '@/context/theme-provider'
import { getUserQuotaDataByUsers } from '@/features/dashboard/api'
import { TIME_GRANULARITY_OPTIONS } from '@/features/dashboard/constants'
import {
  getDefaultDays,
  saveGranularity,
  processUserChartData,
} from '@/features/dashboard/lib'
import type {
  ProcessedUserChartData,
  UserChartsFilters,
} from '@/features/dashboard/types'
import { getNormalizedDateRange, type TimeGranularity } from '@/lib/time'
import { VCHART_OPTION } from '@/lib/vchart'

import { BusinessMetrics } from './business-metrics'

let themeManagerPromise: Promise<
  (typeof import('@visactor/vchart'))['ThemeManager']
> | null = null

const USER_CHARTS: {
  value: string
  labelKey: string
  specKey: keyof ProcessedUserChartData
}[] = [
  {
    value: 'rank',
    labelKey: 'User Consumption Ranking',
    specKey: 'spec_user_rank',
  },
  {
    value: 'trend',
    labelKey: 'User Consumption Trend',
    specKey: 'spec_user_trend',
  },
]

const TOP_USER_LIMIT_OPTIONS = [5, 10, 20, 50]
const USER_ANALYTICS_RANGE_PRESETS = [1, 7, 30]
const MAX_CUSTOM_RANGE_MS = 366 * 24 * 60 * 60 * 1000

interface UserChartsProps {
  filters: UserChartsFilters
  onFiltersChange: (filters: UserChartsFilters) => void
}

export function UserCharts(props: UserChartsProps) {
  const { t } = useTranslation()
  const { resolvedTheme } = useTheme()
  const [themeReady, setThemeReady] = useState(false)
  const [customRangeOpen, setCustomRangeOpen] = useState(false)
  const [customStart, setCustomStart] = useState<Date>()
  const [customEnd, setCustomEnd] = useState<Date>()
  const themeManagerRef = useRef<
    (typeof import('@visactor/vchart'))['ThemeManager'] | null
  >(null)

  // The selection is owned by the dashboard parent so it persists across
  // sub-section switches.
  const timeGranularity = props.filters.timeGranularity
  const selectedRange = props.filters.selectedRange
  const topUserLimit = props.filters.topUserLimit
  const onFiltersChange = props.onFiltersChange

  const selectedDates = useMemo(() => {
    if (
      selectedRange == null &&
      props.filters.customStart &&
      props.filters.customEnd
    ) {
      return {
        start: props.filters.customStart,
        end: props.filters.customEnd,
      }
    }
    return getNormalizedDateRange(Math.max((selectedRange ?? 7) - 1, 0))
  }, [props.filters.customEnd, props.filters.customStart, selectedRange])

  const timeRange = useMemo(() => {
    return {
      start_timestamp: Math.floor(selectedDates.start.getTime() / 1000),
      end_timestamp: Math.floor(selectedDates.end.getTime() / 1000),
    }
  }, [selectedDates])

  const handleRangeChange = useCallback(
    (days: number) => {
      onFiltersChange({
        ...props.filters,
        selectedRange: days,
        customStart: undefined,
        customEnd: undefined,
      })
    },
    [onFiltersChange, props.filters]
  )

  const handleGranularityChange = useCallback(
    (g: TimeGranularity) => {
      saveGranularity(g)
      const days = getDefaultDays(g)
      onFiltersChange({
        ...props.filters,
        timeGranularity: g,
        selectedRange: days,
        customStart: undefined,
        customEnd: undefined,
      })
    },
    [onFiltersChange, props.filters]
  )

  const handleTopUserLimitChange = useCallback(
    (limit: number) => {
      onFiltersChange({ ...props.filters, topUserLimit: limit })
    },
    [onFiltersChange, props.filters]
  )

  const handleCustomRangeOpenChange = (open: boolean) => {
    if (open) {
      setCustomStart(selectedDates.start)
      setCustomEnd(selectedDates.end)
    }
    setCustomRangeOpen(open)
  }

  const handleCustomRangeApply = () => {
    if (!customStart || !customEnd || customEnd < customStart) {
      toast.error(t('Please select a valid time range'))
      return
    }
    if (customEnd.getTime() - customStart.getTime() > MAX_CUSTOM_RANGE_MS) {
      toast.error(t('The time range cannot exceed 366 days'))
      return
    }
    onFiltersChange({
      ...props.filters,
      selectedRange: null,
      customStart,
      customEnd,
    })
    setCustomRangeOpen(false)
  }

  useEffect(() => {
    const updateTheme = async () => {
      setThemeReady(false)
      if (!themeManagerPromise) {
        themeManagerPromise = import('@visactor/vchart').then(
          (m) => m.ThemeManager
        )
      }
      const ThemeManager = await themeManagerPromise
      themeManagerRef.current = ThemeManager
      ThemeManager.setCurrentTheme(resolvedTheme === 'dark' ? 'dark' : 'light')
      setThemeReady(true)
    }
    updateTheme()
  }, [resolvedTheme])

  const { data: userData, isLoading } = useQuery({
    queryKey: ['dashboard', 'user-quota', timeRange],
    queryFn: () => getUserQuotaDataByUsers(timeRange),
    select: (res) => (res.success ? res.data : []),
    staleTime: 60_000,
  })

  const chartData = useMemo(
    () =>
      processUserChartData(
        isLoading ? [] : (userData ?? []),
        timeGranularity,
        t,
        topUserLimit
      ),
    [userData, isLoading, timeGranularity, t, topUserLimit]
  )

  return (
    <div className='space-y-3'>
      <div className='flex items-center gap-1.5 overflow-x-auto pb-1 sm:gap-2'>
        <Tabs
          value={selectedRange == null ? 'custom' : String(selectedRange)}
          onValueChange={(value) => handleRangeChange(Number(value))}
          className='shrink-0'
        >
          <TabsList>
            {USER_ANALYTICS_RANGE_PRESETS.map((days) => (
              <TabsTrigger
                key={days}
                value={String(days)}
                className='px-2.5 text-xs'
              >
                {days === 1 ? t('Today') : t('{{count}} Days', { count: days })}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>

        <Dialog
          open={customRangeOpen}
          onOpenChange={handleCustomRangeOpenChange}
          trigger={
            <Button
              variant={selectedRange == null ? 'default' : 'outline'}
              size='sm'
              className='shrink-0'
            >
              <CalendarDays className='size-4' />
              {t('Custom')}
            </Button>
          }
          title={t('Custom Time Range')}
          description={t('Choose a range of up to 366 days.')}
          contentClassName='sm:max-w-md'
          contentHeight='auto'
          footer={
            <Button onClick={handleCustomRangeApply}>{t('Apply')}</Button>
          }
        >
          <div className='grid gap-4 py-2'>
            <div className='grid gap-2'>
              <Label>{t('Start Time')}</Label>
              <DateTimePicker value={customStart} onChange={setCustomStart} />
            </div>
            <div className='grid gap-2'>
              <Label>{t('End Time')}</Label>
              <DateTimePicker value={customEnd} onChange={setCustomEnd} />
            </div>
          </div>
        </Dialog>

        <Tabs
          value={timeGranularity}
          onValueChange={(value) =>
            handleGranularityChange(value as TimeGranularity)
          }
          className='shrink-0'
        >
          <TabsList>
            {TIME_GRANULARITY_OPTIONS.map((opt) => (
              <TabsTrigger
                key={opt.value}
                value={opt.value}
                className='px-2.5 text-xs'
              >
                {t(opt.label)}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>

        <Tabs
          value={String(topUserLimit)}
          onValueChange={(value) => handleTopUserLimitChange(Number(value))}
          className='shrink-0'
        >
          <TabsList>
            <span className='text-muted-foreground px-2 text-xs font-medium whitespace-nowrap'>
              {t('Top Users')}
            </span>
            {TOP_USER_LIMIT_OPTIONS.map((limit) => (
              <TabsTrigger
                key={limit}
                value={String(limit)}
                className='px-2.5 text-xs'
              >
                {t('Top {{count}}', { count: limit })}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>

        {isLoading && (
          <Loader2 className='text-muted-foreground size-4 animate-spin' />
        )}
      </div>

      <BusinessMetrics
        startTimestamp={timeRange.start_timestamp}
        endTimestamp={timeRange.end_timestamp}
      />

      <div className='grid gap-3'>
        {USER_CHARTS.map((chart) => {
          const spec = chartData[chart.specKey]

          return (
            <div
              key={chart.value}
              className='overflow-hidden rounded-lg border'
            >
              <div className='flex w-full items-center gap-2 border-b px-3 py-2 sm:px-5 sm:py-3'>
                <IconBadge tone='info' size='sm'>
                  <Users />
                </IconBadge>
                <div className='text-sm font-semibold'>{t(chart.labelKey)}</div>
              </div>

              <div className='h-[300px] p-1.5 sm:h-96 sm:p-2'>
                {isLoading ? (
                  <Skeleton className='h-full w-full' />
                ) : (
                  themeReady &&
                  spec && (
                    <VChart
                      key={`user-${chart.value}-${topUserLimit}-${resolvedTheme}`}
                      spec={{
                        ...spec,
                        theme: resolvedTheme === 'dark' ? 'dark' : 'light',
                        background: 'transparent',
                      }}
                      option={VCHART_OPTION}
                    />
                  )
                )}
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}
