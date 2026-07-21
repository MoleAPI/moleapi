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
import { InvoiceIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { Search, ChevronLeft, ChevronRight, Eye, Download } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { Dialog } from '@/components/dialog'
import { StatusBadge } from '@/components/status-badge'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { toIntlLocale } from '@/i18n/languages'
import { useAuthStore } from '@/stores/auth-store'

import { useBillingHistory } from '../../hooks/use-billing-history'
import {
  getStatusConfig,
  getPaymentMethodName,
  formatTimestamp,
  getTopUpInvoiceUrl,
} from '../../lib/billing'
import {
  formatHistoricalPaymentAmount,
  formatHistoricalTopUpCredit,
} from '../../lib/format'
import type { TopupRecord } from '../../types'

interface BillingHistoryDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  initialUserKeyword?: string
}

const BILLING_HISTORY_SKELETON_IDS = Array.from(
  { length: 5 },
  (_, index) => `billing-history-skeleton-${index + 1}`
)

function BillingDetailRow(props: {
  label: React.ReactNode
  value: React.ReactNode
  mono?: boolean
}) {
  return (
    <div className='grid min-w-0 grid-cols-[7.5rem_minmax(0,1fr)] gap-3 text-sm'>
      <span className='text-muted-foreground text-xs'>{props.label}</span>
      <span className={props.mono ? 'font-mono text-xs break-all' : 'text-sm'}>
        {props.value}
      </span>
    </div>
  )
}

export function BillingHistoryDialog(props: BillingHistoryDialogProps) {
  const { t, i18n } = useTranslation()
  const {
    records,
    total,
    page,
    pageSize,
    keyword,
    userKeyword,
    startTime,
    endTime,
    loading,
    completing,
    isAdmin,
    handlePageChange,
    handlePageSizeChange,
    handleSearch,
    handleUserSearch,
    handleStartTimeChange,
    handleEndTimeChange,
    resetFilters,
    handleCompleteOrder,
  } = useBillingHistory({ initialUserKeyword: props.initialUserKeyword })
  const currentUserId = useAuthStore((state) => state.auth.user?.id)

  const [confirmTradeNo, setConfirmTradeNo] = useState<string | null>(null)
  const [detailRecord, setDetailRecord] = useState<TopupRecord | null>(null)

  const totalPages = Math.ceil(total / pageSize)
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const detailCredit = detailRecord
    ? formatHistoricalTopUpCredit(detailRecord)
    : null
  const detailStatus = detailRecord
    ? getStatusConfig(detailRecord.status)
    : null
  const detailInvoiceViewUrl = detailRecord
    ? getTopUpInvoiceUrl(detailRecord, currentUserId, isAdmin)
    : null
  const detailInvoiceDownloadUrl = detailRecord
    ? getTopUpInvoiceUrl(detailRecord, currentUserId, isAdmin, true)
    : null

  const handleConfirmComplete = async () => {
    if (confirmTradeNo) {
      const success = await handleCompleteOrder(confirmTradeNo)
      if (success) {
        setConfirmTradeNo(null)
      }
    }
  }

  return (
    <>
      <Dialog
        open={props.open}
        onOpenChange={props.onOpenChange}
        title={t('Recharge Bills')}
        description={t(
          'View your topup transaction records and payment history'
        )}
        contentClassName='flex max-h-[calc(100dvh-2rem)] flex-col max-sm:w-screen max-sm:max-w-none max-sm:rounded-none max-sm:p-4 sm:max-w-6xl'
        contentHeight='auto'
        bodyClassName='space-y-3'
      >
        <div className='min-h-0 space-y-3'>
          {/* Search and Filter Bar */}
          <div className='flex flex-col gap-2 sm:flex-row sm:flex-wrap'>
            <div className='relative flex-1'>
              <Search className='text-muted-foreground absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2' />
              <Input
                placeholder={t('Search by order number...')}
                value={keyword}
                onChange={(e) => handleSearch(e.target.value)}
                className='h-9 pl-10'
              />
            </div>
            {isAdmin && (
              <>
                <Input
                  aria-label={t('Search by user')}
                  placeholder={t('User ID, username, email, or display name')}
                  value={userKeyword}
                  onChange={(event) => handleUserSearch(event.target.value)}
                  className='h-9 sm:max-w-56'
                />
                <Input
                  aria-label={t('Start time')}
                  type='datetime-local'
                  value={startTime}
                  onChange={(event) =>
                    handleStartTimeChange(event.target.value)
                  }
                  className='h-9 sm:max-w-48'
                />
                <Input
                  aria-label={t('End time')}
                  type='datetime-local'
                  value={endTime}
                  onChange={(event) => handleEndTimeChange(event.target.value)}
                  className='h-9 sm:max-w-48'
                />
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  className='h-9'
                  onClick={resetFilters}
                >
                  {t('Reset filters')}
                </Button>
              </>
            )}
            <Select
              items={[
                { value: '10', label: t('10 / page') },
                { value: '20', label: t('20 / page') },
                { value: '50', label: t('50 / page') },
                { value: '100', label: t('100 / page') },
              ]}
              value={pageSize.toString()}
              onValueChange={(value) =>
                value !== null && handlePageSizeChange(Number.parseInt(value))
              }
            >
              <SelectTrigger className='h-9 w-[92px] sm:w-32'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent alignItemWithTrigger={false}>
                <SelectGroup>
                  <SelectItem value='10'>{t('10 / page')}</SelectItem>
                  <SelectItem value='20'>{t('20 / page')}</SelectItem>
                  <SelectItem value='50'>{t('50 / page')}</SelectItem>
                  <SelectItem value='100'>{t('100 / page')}</SelectItem>
                </SelectGroup>
              </SelectContent>
            </Select>
          </div>

          {/* Records List */}
          <div className='max-h-[min(58vh,560px)] overflow-y-auto rounded-md border'>
            {loading && (
              <div className='space-y-2 p-3'>
                {BILLING_HISTORY_SKELETON_IDS.map((id) => (
                  <div key={id} className='flex items-center gap-3'>
                    <Skeleton className='h-4 w-64' />
                    <Skeleton className='h-4 w-24' />
                    <Skeleton className='h-4 w-24' />
                    <Skeleton className='h-4 w-20' />
                    <Skeleton className='h-4 w-20' />
                    <Skeleton className='h-4 w-32' />
                  </div>
                ))}
              </div>
            )}
            {!loading && records.length === 0 && (
              <div className='text-muted-foreground flex min-h-40 flex-col items-center justify-center py-10 text-center'>
                <p className='text-sm font-medium'>
                  {t('No billing records found')}
                </p>
                <p className='mt-1 text-xs'>
                  {keyword || userKeyword || startTime || endTime
                    ? t('Try adjusting your search')
                    : t('Your transaction history will appear here')}
                </p>
              </div>
            )}
            {!loading && records.length > 0 && (
              <Table className='min-w-[980px] text-xs [&_td]:text-xs [&_td_*]:text-xs [&_th]:text-xs'>
                <TableHeader className='bg-muted/40 sticky top-0 z-10'>
                  <TableRow>
                    {isAdmin && <TableHead>{t('User ID')}</TableHead>}
                    <TableHead>{t('Order')}</TableHead>
                    <TableHead>{t('Payment Method')}</TableHead>
                    <TableHead>{t('Amount')}</TableHead>
                    <TableHead>{t('Payment')}</TableHead>
                    <TableHead>{t('Status')}</TableHead>
                    <TableHead>{t('Created At')}</TableHead>
                    <TableHead className='text-right'>{t('Actions')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody className='[&>tr]:h-11'>
                  {records.map((record) => {
                    const statusConfig = getStatusConfig(record.status)
                    const creditDisplay = formatHistoricalTopUpCredit(record)
                    const invoiceViewUrl = getTopUpInvoiceUrl(
                      record,
                      currentUserId,
                      isAdmin
                    )
                    const invoiceDownloadUrl = getTopUpInvoiceUrl(
                      record,
                      currentUserId,
                      isAdmin,
                      true
                    )

                    return (
                      <TableRow
                        key={record.id}
                        className='cursor-pointer'
                        onClick={() => setDetailRecord(record)}
                        title={t('Click to view full details')}
                      >
                        {isAdmin && (
                          <TableCell>
                            <StatusBadge
                              label={String(record.user_id ?? '-')}
                              variant='neutral'
                              size='sm'
                              copyText={
                                record.user_id != null
                                  ? String(record.user_id)
                                  : undefined
                              }
                            />
                          </TableCell>
                        )}
                        <TableCell>
                          <div className='flex min-w-0 items-center gap-1.5'>
                            <code className='font-mono text-xs whitespace-nowrap'>
                              {record.trade_no}
                            </code>
                            <CopyButton
                              value={record.trade_no}
                              size='icon'
                              className='size-6'
                              iconClassName='size-3'
                              tooltip={t('Copy to clipboard')}
                            />
                          </div>
                        </TableCell>
                        <TableCell>
                          {getPaymentMethodName(record.payment_method, t)}
                        </TableCell>
                        <TableCell className='font-medium'>
                          {creditDisplay.value}
                        </TableCell>
                        <TableCell className='font-medium text-red-600'>
                          {formatHistoricalPaymentAmount(
                            record.money,
                            record.payment_currency,
                            locale
                          )}
                        </TableCell>
                        <TableCell>
                          <StatusBadge
                            label={t(statusConfig.label)}
                            variant={statusConfig.variant}
                            showDot
                            copyable={false}
                          />
                        </TableCell>
                        <TableCell className='font-mono'>
                          {formatTimestamp(record.create_time)}
                        </TableCell>
                        <TableCell>
                          <div className='flex items-center justify-end gap-1.5'>
                            <Button
                              size='sm'
                              variant='ghost'
                              className='h-7 px-2'
                              onClick={(event) => {
                                event.stopPropagation()
                                setDetailRecord(record)
                              }}
                            >
                              <Eye className='size-3.5' />
                              {t('Details')}
                            </Button>
                            {invoiceViewUrl && (
                              <Button
                                size='sm'
                                variant='ghost'
                                className='h-7 px-2'
                                render={
                                  <a
                                    href={invoiceViewUrl}
                                    target='_blank'
                                    rel='noreferrer noopener'
                                    onClick={(event) => event.stopPropagation()}
                                  />
                                }
                              >
                                <HugeiconsIcon
                                  icon={InvoiceIcon}
                                  strokeWidth={2}
                                  data-icon='inline-start'
                                />
                                Invoice
                              </Button>
                            )}
                            {invoiceDownloadUrl && (
                              <Button
                                size='sm'
                                variant='ghost'
                                className='h-7 px-2'
                                render={
                                  <a
                                    href={invoiceDownloadUrl}
                                    download
                                    onClick={(event) => event.stopPropagation()}
                                  />
                                }
                              >
                                <Download className='size-3.5' />
                                {t('Download')}
                              </Button>
                            )}
                            {isAdmin && record.status === 'pending' && (
                              <Button
                                size='sm'
                                variant='outline'
                                className='h-7 px-2'
                                onClick={(event) => {
                                  event.stopPropagation()
                                  setConfirmTradeNo(record.trade_no)
                                }}
                                disabled={completing}
                              >
                                {t('Complete Order')}
                              </Button>
                            )}
                          </div>
                        </TableCell>
                      </TableRow>
                    )
                  })}
                </TableBody>
              </Table>
            )}
          </div>

          {/* Pagination */}
          {!loading && records.length > 0 && (
            <div className='flex flex-col items-center gap-3 border-t pt-4 sm:flex-row sm:items-center sm:justify-between'>
              <div className='text-muted-foreground text-xs sm:text-sm'>
                {t('Showing')} {(page - 1) * pageSize + 1}-
                {Math.min(page * pageSize, total)} {t('of')} {total}
              </div>
              <div className='flex items-center gap-2'>
                <Button
                  variant='outline'
                  size='sm'
                  onClick={() => handlePageChange(page - 1)}
                  disabled={page <= 1}
                  className='h-8 w-8 p-0'
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
                  onClick={() => handlePageChange(page + 1)}
                  disabled={page >= totalPages}
                  className='h-8 w-8 p-0'
                >
                  <ChevronRight className='h-4 w-4' />
                </Button>
              </div>
            </div>
          )}
        </div>
      </Dialog>

      <Dialog
        open={!!detailRecord}
        onOpenChange={(open) => !open && setDetailRecord(null)}
        title={t('Top-up Details')}
        description={t(
          'View your topup transaction records and payment history'
        )}
        contentClassName='max-sm:w-[calc(100vw-1.5rem)] sm:max-w-lg'
        contentHeight='auto'
        bodyClassName='space-y-3'
        footer={
          detailRecord ? (
            <div className='flex flex-wrap justify-end gap-2'>
              {detailInvoiceViewUrl && (
                <Button
                  size='sm'
                  variant='outline'
                  render={
                    <a
                      href={detailInvoiceViewUrl}
                      target='_blank'
                      rel='noreferrer noopener'
                    />
                  }
                >
                  <HugeiconsIcon
                    icon={InvoiceIcon}
                    strokeWidth={2}
                    data-icon='inline-start'
                  />
                  {t('View invoice')}
                </Button>
              )}
              {detailInvoiceDownloadUrl && (
                <Button
                  size='sm'
                  variant='outline'
                  render={<a href={detailInvoiceDownloadUrl} download />}
                >
                  <Download className='size-3.5' />
                  {t('Download invoice')}
                </Button>
              )}
              <Button
                size='sm'
                variant='secondary'
                onClick={() => setDetailRecord(null)}
              >
                {t('Close')}
              </Button>
            </div>
          ) : null
        }
      >
        {detailRecord && detailCredit && detailStatus && (
          <div className='space-y-2.5'>
            {isAdmin && (
              <BillingDetailRow
                label={t('User ID')}
                value={detailRecord.user_id ?? '-'}
              />
            )}
            <BillingDetailRow
              label={t('Order')}
              mono
              value={
                <span className='inline-flex min-w-0 items-center gap-1.5'>
                  <span className='break-all'>{detailRecord.trade_no}</span>
                  <CopyButton
                    value={detailRecord.trade_no}
                    size='icon'
                    className='size-6 shrink-0'
                    iconClassName='size-3'
                    tooltip={t('Copy to clipboard')}
                  />
                </span>
              }
            />
            {detailRecord.gateway_trade_no?.trim() && (
              <BillingDetailRow
                label={t('Gateway transaction ID')}
                mono
                value={
                  <span className='inline-flex min-w-0 items-center gap-1.5'>
                    <span className='break-all'>
                      {detailRecord.gateway_trade_no.trim()}
                    </span>
                    <CopyButton
                      value={detailRecord.gateway_trade_no.trim()}
                      size='icon'
                      className='size-6 shrink-0'
                      iconClassName='size-3'
                      tooltip={t('Copy to clipboard')}
                    />
                  </span>
                }
              />
            )}
            <BillingDetailRow
              label={t('Payment Method')}
              value={getPaymentMethodName(detailRecord.payment_method, t)}
            />
            {detailRecord.payment_provider && (
              <BillingDetailRow
                label={t('Payment Channel')}
                value={getPaymentMethodName(detailRecord.payment_provider, t)}
              />
            )}
            <BillingDetailRow
              label={t(
                detailCredit.hasCreditedFact ? 'Credited amount' : 'Amount'
              )}
              value={<span className='font-medium'>{detailCredit.value}</span>}
            />
            <BillingDetailRow
              label={t('Payment')}
              value={
                <span className='font-semibold text-red-600'>
                  {formatHistoricalPaymentAmount(
                    detailRecord.money,
                    detailRecord.payment_currency,
                    locale
                  )}
                </span>
              }
            />
            <BillingDetailRow
              label={t('Status')}
              value={
                <StatusBadge
                  label={t(detailStatus.label)}
                  variant={detailStatus.variant}
                  showDot
                  copyable={false}
                />
              }
            />
            <BillingDetailRow
              label={t('Created At')}
              mono
              value={formatTimestamp(detailRecord.create_time)}
            />
            <BillingDetailRow
              label={t('Completed')}
              mono
              value={
                detailRecord.complete_time
                  ? formatTimestamp(detailRecord.complete_time)
                  : '-'
              }
            />
          </div>
        )}
      </Dialog>

      {/* Confirm Complete Order Dialog */}
      <AlertDialog
        open={!!confirmTradeNo}
        onOpenChange={(open) => !open && setConfirmTradeNo(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('Complete Order')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t(
                'Are you sure you want to manually complete this order? The user will be credited with the corresponding quota.'
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={completing}>
              {t('Cancel')}
            </AlertDialogCancel>
            <AlertDialogAction
              onClick={handleConfirmComplete}
              disabled={completing}
            >
              {completing ? t('Processing...') : t('Confirm')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
