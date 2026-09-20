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
import { Download } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Progress } from '@/components/ui/progress'
import { useDebounce } from '@/hooks/use-debounce'
import { toIntlLocale } from '@/i18n/languages'
import dayjs from '@/lib/dayjs'
import { formatNumber } from '@/lib/format'
import { requireServerSuccess } from '@/lib/server-error-message'
import { useAuthStore } from '@/stores/auth-store'

import { getAllLogs, getUserLogs } from '../../api'
import { LOG_TYPE_FILTERS } from '../../constants'
import type { UsageLog } from '../../data/schema'
import {
  estimateLogExportBytes,
  exportUsageLogs,
  type ExportProgress,
} from '../../lib/export'
import type { GetLogsParams } from '../../types'
import { useLogsViewScope } from '../usage-logs-provider'

export function LogExportDialog({
  startTime,
  endTime,
  filters = {},
}: {
  startTime?: Date
  endTime?: Date
  filters?: GetLogsParams
}) {
  const { t, i18n } = useTranslation()
  const { isAdminView } = useLogsViewScope()
  const userId = useAuthStore((state) => state.auth.user?.id)
  const [open, setOpen] = useState(false)
  const [start, setStart] = useState('')
  const [end, setEnd] = useState('')
  const [progress, setProgress] = useState<ExportProgress>({
    count: 0,
    bytes: 0,
  })
  const [busy, setBusy] = useState(false)
  const [finished, setFinished] = useState(false)
  const completedProgress = finished ? 100 : 0
  const [error, setError] = useState('')
  const controller = useRef<AbortController | null>(null)
  useEffect(() => () => controller.current?.abort(), [])
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const startTimestamp = Math.floor(new Date(start).getTime() / 1000)
  const endTimestamp = Math.floor(new Date(end).getTime() / 1000)
  const valid =
    Number.isFinite(startTimestamp) &&
    startTimestamp > 0 &&
    Number.isFinite(endTimestamp) &&
    endTimestamp >= startTimestamp
  const estimateStart = useDebounce(start)
  const estimateEnd = useDebounce(end)
  const estimateParams = {
    ...filters,
    username: isAdminView ? filters.username : undefined,
    channel: isAdminView ? filters.channel : undefined,
    start_timestamp: Math.floor(new Date(estimateStart).getTime() / 1000),
    end_timestamp: Math.floor(new Date(estimateEnd).getTime() / 1000),
    p: 1,
    page_size: 20,
  }
  const estimate = useQuery({
    queryKey: ['log-export-estimate', userId, isAdminView, estimateParams],
    enabled:
      open && valid && start === estimateStart && end === estimateEnd && !busy,
    staleTime: 30_000,
    retry: false,
    refetchOnWindowFocus: false,
    queryFn: async () => {
      const response = requireServerSuccess(
        await (isAdminView
          ? getAllLogs(estimateParams)
          : getUserLogs(estimateParams))
      )
      if (!response.data) throw new Error('Missing log estimate')
      const bytes = estimateLogExportBytes(
        response.data.items as UsageLog[],
        response.data.total,
        isAdminView
      )
      if (bytes === undefined) throw new Error('Missing log sample')
      return { count: response.data.total, bytes }
    },
  })
  const estimating =
    estimate.isPending ||
    estimate.isFetching ||
    start !== estimateStart ||
    end !== estimateEnd

  async function download() {
    if (!valid || controller.current) return
    const abort = new AbortController()
    controller.current = abort
    setBusy(true)
    setFinished(false)
    setError('')
    setProgress({ count: 0, bytes: 0 })
    try {
      const file = await exportUsageLogs(
        {
          start_timestamp: startTimestamp,
          end_timestamp: endTimestamp,
          all_users: isAdminView,
          type: filters.type,
          model_name: filters.model_name,
          group: filters.group,
          token_name: filters.token_name,
          request_id: filters.request_id,
          upstream_request_id: filters.upstream_request_id,
          ...(isAdminView && {
            username: filters.username,
            channel: filters.channel,
          }),
        },
        abort.signal,
        setProgress
      )
      if (abort.signal.aborted) return
      const url = URL.createObjectURL(file)
      const link = document.createElement('a')
      link.href = url
      link.download = `usage-logs-${startTimestamp}-${endTimestamp}.csv`
      link.click()
      window.setTimeout(() => URL.revokeObjectURL(url), 60_000)
      setFinished(true)
    } catch (caught) {
      if (!abort.signal.aborted) {
        const response = (caught as { response?: { status?: number } }).response
        setError(
          response?.status === 429
            ? 'Daily export limit reached (3 per UTC day)'
            : 'Export interrupted; select a shorter range and retry'
        )
      }
    } finally {
      controller.current = null
      setBusy(false)
    }
  }

  return (
    <>
      <Button
        variant='outline'
        size='sm'
        onClick={() => {
          setStart(
            startTime ? dayjs(startTime).format('YYYY-MM-DDTHH:mm:ss') : ''
          )
          setEnd(endTime ? dayjs(endTime).format('YYYY-MM-DDTHH:mm:ss') : '')
          setProgress({ count: 0, bytes: 0 })
          setFinished(false)
          setError('')
          setOpen(true)
        }}
      >
        <Download />
        {t('Export logs')}
      </Button>
      <Dialog
        open={open}
        onOpenChange={(next) => {
          if (!busy) setOpen(next)
        }}
        title={t('Export logs')}
        description={t(
          'Export usage logs matching the applied filters and time range, across all pages.'
        )}
        footer={
          <Button disabled={!valid || busy} onClick={() => void download()}>
            {t('Download CSV')}
          </Button>
        }
      >
        <FieldGroup>
          <p className='text-muted-foreground text-sm'>
            {t(
              'The current log list filters are applied. To preview a different selection, update the filters and search the list before exporting.'
            )}
          </p>
          <p className='text-muted-foreground text-sm'>
            {isAdminView ? t('All users') : t('Only my logs')}
          </p>
          <dl className='grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-2 text-sm'>
            <dt className='text-muted-foreground'>{t('Type')}</dt>
            <dd>
              {t(
                LOG_TYPE_FILTERS.find(
                  (type) => Number(type.value) === (filters.type ?? 0)
                )?.label ?? 'All Types'
              )}
            </dd>
            {[
              [t('Model'), filters.model_name],
              [t('Group'), filters.group],
              [t('Token'), filters.token_name],
              [
                t('Request ID'),
                filters.request_id || filters.upstream_request_id,
              ],
              [t('User'), isAdminView ? filters.username : undefined],
              [t('Channel'), isAdminView ? filters.channel : undefined],
            ]
              .filter(([, value]) => value)
              .map(([label, value]) => (
                <div key={label} className='contents'>
                  <dt className='text-muted-foreground'>{label}</dt>
                  <dd className='break-all'>{value}</dd>
                </div>
              ))}
          </dl>
          <Field>
            <FieldLabel htmlFor='log-export-start'>
              {t('Start time')}
            </FieldLabel>
            <Input
              id='log-export-start'
              type='datetime-local'
              step='1'
              value={start}
              disabled={busy}
              onChange={(event) => setStart(event.target.value)}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor='log-export-end'>{t('End time')}</FieldLabel>
            <Input
              id='log-export-end'
              type='datetime-local'
              step='1'
              value={end}
              disabled={busy}
              onChange={(event) => setEnd(event.target.value)}
            />
          </Field>
          {start && end && !valid && (
            <p role='alert' className='text-destructive text-sm'>
              {t('Invalid date range')}
            </p>
          )}
          {valid && (
            <div aria-live='polite' className='rounded-lg border p-3 text-sm'>
              {estimating && <p>{t('Calculating estimate...')}</p>}
              {!estimating && estimate.data && (
                <p>
                  {t(
                    'Estimated: {{count}} records · about {{size}} MiB ({{bytes}} B)',
                    {
                      count: formatNumber(estimate.data.count, locale),
                      size: formatNumber(
                        estimate.data.bytes / 1024 / 1024,
                        locale
                      ),
                      bytes: formatNumber(estimate.data.bytes, locale),
                    }
                  )}
                </p>
              )}
              {!estimating && !estimate.data && (
                <p>{t('Estimate unavailable. You can still export.')}</p>
              )}
              <p className='text-muted-foreground mt-1'>
                {t(
                  'Size is estimated from a sample. Actual download size may vary; estimates do not use your daily export allowance.'
                )}
              </p>
            </div>
          )}
          <p className='text-muted-foreground text-sm'>
            {t(
              'Up to 3 export attempts per UTC day, including cancelled or failed attempts. Limit: 50 MiB per file.'
            )}
          </p>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Large queries may take several minutes. Keep this window open while logs are fetched.'
            )}
          </p>
          <div role='status' aria-live='polite' className='text-sm'>
            {busy && <p>{t('Fetching logs...')}</p>}
            {finished && <p>{t('Export complete')}</p>}
            <p>
              {t('Fetched {{count}} records · {{size}} MiB', {
                count: formatNumber(progress.count, locale),
                size: formatNumber(progress.bytes / 1024 / 1024, locale),
              })}
              {' · '}
              {formatNumber(progress.bytes, locale)} B
            </p>
          </div>
          <Progress
            value={busy ? null : completedProgress}
            aria-label={busy ? t('Fetching logs...') : t('Export logs')}
            className={
              busy
                ? '[&_[data-slot=progress-indicator]]:w-2/5 [&_[data-slot=progress-indicator]]:motion-safe:animate-pulse'
                : undefined
            }
          />
          {error && (
            <p role='alert' className='text-destructive text-sm'>
              {t(error)}
            </p>
          )}
          {busy && (
            <Button
              variant='outline'
              onClick={() => controller.current?.abort()}
            >
              {t('Cancel')}
            </Button>
          )}
        </FieldGroup>
      </Dialog>
    </>
  )
}
