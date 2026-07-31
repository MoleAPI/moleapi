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
import { ChevronLeft, ChevronRight } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatQuota } from '@/lib/format'

import { getAffiliateHistory, isApiSuccess } from '../../api'
import { formatTimestamp } from '../../lib/billing'
import {
  formatHistoricalTopUpAmount,
  formatInviteRebateRatio,
} from '../../lib/format'
import type { AffiliateRewardRecord } from '../../types'

interface AffiliateHistoryDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

const PAGE_SIZE = 10

const SKELETON_IDS = Array.from(
  { length: 5 },
  (_, index) => `affiliate-history-skeleton-${index + 1}`
)

export function AffiliateHistoryDialog(props: AffiliateHistoryDialogProps) {
  const { t } = useTranslation()
  const [records, setRecords] = useState<AffiliateRewardRecord[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(false)

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  const fetchHistory = useCallback(async () => {
    if (!props.open) return

    setLoading(true)
    try {
      const response = await getAffiliateHistory(page, PAGE_SIZE)
      if (isApiSuccess(response) && response.data) {
        setRecords(response.data.items ?? [])
        setTotal(response.data.total ?? 0)
        return
      }
      toast.error(response.message || t('Failed to load reward records'))
      setRecords([])
      setTotal(0)
    } catch (error) {
      // eslint-disable-next-line no-console
      console.error('Failed to load reward records:', error)
      toast.error(t('Failed to load reward records'))
      setRecords([])
      setTotal(0)
    } finally {
      setLoading(false)
    }
  }, [page, props.open, t])

  useEffect(() => {
    if (props.open) {
      void fetchHistory()
    }
  }, [fetchHistory, props.open])

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('Referral reward records')}
      description={t("View rewards from invited users' completed top-ups.")}
      contentClassName='flex max-h-[calc(100dvh-2rem)] flex-col max-sm:w-screen max-sm:max-w-none max-sm:rounded-none max-sm:p-4 sm:max-w-4xl'
      contentHeight='auto'
      bodyClassName='space-y-3'
    >
      <div className='max-h-[min(58vh,520px)] overflow-y-auto rounded-md border'>
        {loading && (
          <div className='space-y-2 p-3'>
            {SKELETON_IDS.map((id) => (
              <div key={id} className='flex items-center gap-3'>
                <Skeleton className='h-4 w-40' />
                <Skeleton className='h-4 w-20' />
                <Skeleton className='h-4 w-24' />
                <Skeleton className='h-4 w-20' />
                <Skeleton className='h-4 w-24' />
              </div>
            ))}
          </div>
        )}

        {!loading && records.length === 0 && (
          <div className='text-muted-foreground flex min-h-40 flex-col items-center justify-center py-10 text-center'>
            <p className='text-sm font-medium'>
              {t('No reward records found')}
            </p>
            <p className='mt-1 text-xs'>
              {t("Rewards from invited users' top-ups will appear here.")}
            </p>
          </div>
        )}

        {!loading && records.length > 0 && (
          <Table className='min-w-[760px] text-xs [&_td]:text-xs [&_th]:text-xs'>
            <TableHeader className='bg-muted/40 sticky top-0 z-10'>
              <TableRow>
                <TableHead>{t('Order')}</TableHead>
                <TableHead>{t('Invited user')}</TableHead>
                <TableHead>{t('Amount')}</TableHead>
                <TableHead>{t('Top-up rebate')}</TableHead>
                <TableHead>{t('Reward')}</TableHead>
                <TableHead>{t('Completed')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody className='[&>tr]:h-11'>
              {records.map((record) => (
                <TableRow key={record.id}>
                  <TableCell className='font-mono whitespace-nowrap'>
                    {record.trade_no}
                  </TableCell>
                  <TableCell className='font-mono'>{record.user_id}</TableCell>
                  <TableCell className='font-medium'>
                    {formatHistoricalTopUpAmount(record)}
                  </TableCell>
                  <TableCell>
                    {formatInviteRebateRatio(record.invite_rebate_ratio)}
                  </TableCell>
                  <TableCell className='font-medium'>
                    {formatQuota(record.invite_rebate_quota ?? 0)}
                  </TableCell>
                  <TableCell className='font-mono'>
                    {record.complete_time
                      ? formatTimestamp(record.complete_time)
                      : '-'}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </div>

      {!loading && records.length > 0 && (
        <div className='flex flex-col items-center gap-3 border-t pt-4 sm:flex-row sm:items-center sm:justify-between'>
          <div className='text-muted-foreground text-xs sm:text-sm'>
            {t('Showing')} {(page - 1) * PAGE_SIZE + 1}-
            {Math.min(page * PAGE_SIZE, total)} {t('of')} {total}
          </div>
          <div className='flex items-center gap-2'>
            <Button
              variant='outline'
              size='sm'
              onClick={() => setPage((current) => current - 1)}
              disabled={page <= 1}
              className='h-8 w-8 p-0'
              aria-label={t('Previous page')}
            >
              <ChevronLeft className='h-4 w-4' />
            </Button>
            <div className='text-muted-foreground flex items-center gap-1 text-sm'>
              <span className='font-medium'>{page}</span>
              <span>/</span>
              <span>{totalPages}</span>
            </div>
            <Button
              variant='outline'
              size='sm'
              onClick={() => setPage((current) => current + 1)}
              disabled={page >= totalPages}
              className='h-8 w-8 p-0'
              aria-label={t('Next page')}
            >
              <ChevronRight className='h-4 w-4' />
            </Button>
          </div>
        </div>
      )}
    </Dialog>
  )
}
