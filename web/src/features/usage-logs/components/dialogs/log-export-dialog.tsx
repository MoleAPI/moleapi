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
import { Download } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'

import { exportUsageLogs, type ExportProgress } from '../../lib/export'
import { useLogsViewScope } from '../usage-logs-provider'

export function LogExportDialog() {
  const { t, i18n } = useTranslation()
  const { isAdminView } = useLogsViewScope()
  const [open, setOpen] = useState(false)
  const [start, setStart] = useState('')
  const [end, setEnd] = useState('')
  const [progress, setProgress] = useState<ExportProgress>({
    count: 0,
    bytes: 0,
  })
  const [busy, setBusy] = useState(false)
  const [finished, setFinished] = useState(false)
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
    endTimestamp >= startTimestamp &&
    endTimestamp <= Date.now() / 1000

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
      <Button variant='outline' size='sm' onClick={() => setOpen(true)}>
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
          'Export all usage logs in the selected time range. Other table filters do not apply.'
        )}
        footer={
          <Button disabled={!valid || busy} onClick={() => void download()}>
            {t('Download CSV')}
          </Button>
        }
      >
        <FieldGroup>
          <p className='text-muted-foreground text-sm'>
            {isAdminView ? t('All users') : t('Only my logs')}
          </p>
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
                size: (progress.bytes / 1024 / 1024).toFixed(2),
              })}
              {' · '}
              {formatNumber(progress.bytes, locale)} B
            </p>
          </div>
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
