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
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { ArrowLeft, Download, Loader2, Pencil } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { handleServerError } from '@/lib/handle-server-error'
import { useSystemConfigStore } from '@/stores/system-config-store'

import {
  downloadTopUpInvoice,
  getTopUpInvoice,
  getTopUpInvoiceDetailsSchema,
  updateTopUpInvoice,
  type TopUpInvoice,
  type TopUpInvoiceDetails,
} from '../lib/invoice'

interface TopUpInvoicePageProps {
  invoiceId: string
}

const invoiceFields = [
  { name: 'name', label: 'Name' },
  { name: 'email', label: 'Email', type: 'email' },
  { name: 'company', label: 'Company name' },
  { name: 'tax_id', label: 'Tax / VAT ID' },
  { name: 'address', label: 'Street address', wide: true },
  { name: 'city', label: 'City' },
  { name: 'state', label: 'State / Province' },
  { name: 'postal_code', label: 'Postal code' },
  { name: 'country', label: 'Country / Region' },
] as const

function InvoiceValue(props: { label: string; value: string; wide?: boolean }) {
  return (
    <dl className={props.wide ? 'sm:col-span-2' : undefined}>
      <dt className='text-muted-foreground text-xs font-semibold tracking-wide uppercase'>
        {props.label}
      </dt>
      <dd className='mt-1 text-sm break-words'>{props.value}</dd>
    </dl>
  )
}

export function TopUpInvoicePage(props: TopUpInvoicePageProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [editing, setEditing] = useState(false)
  const [downloading, setDownloading] = useState(false)
  const invoiceId = Number(props.invoiceId)
  const validInvoiceId = Number.isSafeInteger(invoiceId) && invoiceId > 0
  const logo = useSystemConfigStore((state) => state.config.logo)
  const schema = useMemo(() => getTopUpInvoiceDetailsSchema(t), [t])
  const form = useForm<TopUpInvoiceDetails>({
    resolver: zodResolver(schema),
    defaultValues: {
      name: '',
      email: '',
      company: '',
      tax_id: '',
      address: '',
      city: '',
      state: '',
      postal_code: '',
      country: '',
    },
  })
  const invoiceQuery = useQuery({
    queryKey: ['top-up-invoice', invoiceId],
    queryFn: () => getTopUpInvoice(invoiceId),
    enabled: validInvoiceId,
  })
  const updateMutation = useMutation({
    mutationFn: (details: TopUpInvoiceDetails) =>
      updateTopUpInvoice(invoiceId, details),
    onSuccess: (_, details) => {
      queryClient.setQueryData<TopUpInvoice>(
        ['top-up-invoice', invoiceId],
        (invoice) =>
          invoice
            ? {
                ...invoice,
                invoice_details: details,
                customer_name: details.name || '-',
                customer_email: details.email || '-',
                customer_company: details.company || '-',
                customer_tax_id: details.tax_id || '-',
                customer_address:
                  [
                    details.address,
                    details.city,
                    details.state,
                    details.postal_code,
                    details.country,
                  ]
                    .filter(Boolean)
                    .join(', ') || '-',
              }
            : invoice
      )
      setEditing(false)
      toast.success(t('Invoice information updated'))
    },
    onError: (error) =>
      handleServerError(error, t('Unable to save invoice information.')),
  })

  useEffect(() => {
    if (invoiceQuery.data) form.reset(invoiceQuery.data.invoice_details)
  }, [form, invoiceQuery.data])

  const handleDownload = async () => {
    if (!invoiceQuery.data || downloading) return
    setDownloading(true)
    try {
      await downloadTopUpInvoice(invoiceId, invoiceQuery.data.trade_no)
    } catch (error) {
      handleServerError(error, t('Unable to download invoice.'))
    } finally {
      setDownloading(false)
    }
  }

  if (!validInvoiceId || invoiceQuery.isError) {
    return (
      <ErrorState
        className='min-h-screen'
        title={t('Invoice not found')}
        description={t('The invoice could not be opened.')}
        onRetry={validInvoiceId ? () => void invoiceQuery.refetch() : undefined}
      />
    )
  }

  if (!invoiceQuery.data) {
    return (
      <LoadingState
        className='min-h-screen'
        size='lg'
        message={t('Loading invoice...')}
      />
    )
  }

  const invoice = invoiceQuery.data

  return (
    <div className='bg-muted/40 min-h-screen px-4 py-6 sm:px-6 sm:py-8'>
      <main className='border-border bg-background mx-auto max-w-4xl rounded-2xl border p-5 shadow-sm sm:p-8 lg:p-10'>
        <div className='mb-6 flex flex-wrap justify-end gap-2 print:hidden'>
          <Button
            variant='outline'
            render={<Link to='/wallet' search={{ show_history: true }} />}
            nativeButton={false}
          >
            <ArrowLeft data-icon='inline-start' />
            {t('Back')}
          </Button>
          {invoice.can_edit && (
            <Button variant='outline' onClick={() => setEditing(true)}>
              <Pencil data-icon='inline-start' />
              {t('Edit information')}
            </Button>
          )}
          <Button onClick={() => void handleDownload()} disabled={downloading}>
            {downloading ? (
              <Loader2 className='animate-spin' data-icon='inline-start' />
            ) : (
              <Download data-icon='inline-start' />
            )}
            {t('Download PDF')}
          </Button>
        </div>

        <header className='border-border flex flex-col gap-5 border-b pb-6 sm:flex-row sm:items-start sm:justify-between'>
          <div className='flex min-w-0 items-center gap-3.5'>
            <img
              src={logo}
              alt={invoice.system_name}
              className='size-13 rounded-xl object-cover'
            />
            <div className='min-w-0'>
              <p className='text-muted-foreground truncate text-xs font-semibold tracking-wide uppercase'>
                {invoice.system_name}
              </p>
              <h1 className='text-3xl font-semibold tracking-tight'>
                {t('Invoice')}
              </h1>
            </div>
          </div>
          <span className='w-fit rounded-full bg-emerald-50 px-3 py-1.5 text-sm font-semibold text-emerald-700 dark:bg-emerald-950 dark:text-emerald-300'>
            {t('Paid')}
          </span>
        </header>

        <div className='my-7 grid grid-cols-1 gap-x-8 gap-y-5 sm:grid-cols-2'>
          <InvoiceValue label={t('Invoice No.')} value={invoice.invoice_no} />
          <InvoiceValue label={t('Issued At')} value={invoice.issued_at} />
          <InvoiceValue label={t('Customer')} value={invoice.customer_name} />
          <InvoiceValue label={t('Email')} value={invoice.customer_email} />
          <InvoiceValue label={t('Company')} value={invoice.customer_company} />
          <InvoiceValue
            label={t('Tax / VAT ID')}
            value={invoice.customer_tax_id}
          />
          <InvoiceValue
            label={t('Billing address')}
            value={invoice.customer_address}
            wide
          />
          <InvoiceValue label={t('Created At')} value={invoice.created_at} />
          <InvoiceValue
            label={t('Completed At')}
            value={invoice.completed_at}
          />
        </div>

        <div className='border-border overflow-x-auto rounded-xl border'>
          <table className='w-full min-w-[620px] border-collapse text-left'>
            <thead className='bg-muted/60 text-muted-foreground text-xs uppercase'>
              <tr>
                <th className='px-4 py-3 font-semibold'>{t('Item')}</th>
                <th className='px-4 py-3 font-semibold'>{t('Payment')}</th>
                <th className='px-4 py-3 font-semibold'>{t('Amount')}</th>
              </tr>
            </thead>
            <tbody>
              <tr className='[&>td]:border-t [&>td]:px-4 [&>td]:py-4 [&>td]:align-top'>
                <td>
                  <InvoiceValue
                    label={t('Description')}
                    value={`${invoice.system_name} Credits`}
                  />
                  <div className='mt-3'>
                    <InvoiceValue
                      label={t('Order No.')}
                      value={invoice.trade_no}
                    />
                  </div>
                  <div className='mt-3'>
                    <InvoiceValue
                      label={t('Gateway Order No.')}
                      value={invoice.gateway_trade_no}
                    />
                  </div>
                </td>
                <td>
                  <InvoiceValue
                    label={t('Provider')}
                    value={invoice.payment_provider}
                  />
                  <div className='mt-3'>
                    <InvoiceValue
                      label={t('Method')}
                      value={invoice.payment_method}
                    />
                  </div>
                </td>
                <td>
                  <InvoiceValue
                    label={t('Top-up Amount')}
                    value={invoice.top_up_amount}
                  />
                  <div className='mt-3'>
                    <InvoiceValue
                      label={t('Credited Quota')}
                      value={invoice.credited_quota}
                    />
                  </div>
                  <div className='mt-3'>
                    <InvoiceValue
                      label={t('Paid Amount')}
                      value={invoice.paid_amount}
                    />
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <footer className='text-muted-foreground mt-7 text-xs'>
          {t(
            'This invoice was generated from the completed top-up record stored by {{systemName}}.',
            { systemName: invoice.system_name }
          )}
        </footer>
      </main>

      <Dialog
        open={editing}
        onOpenChange={(open) => !updateMutation.isPending && setEditing(open)}
        title={t('Edit invoice information')}
        contentClassName='sm:max-w-xl'
        contentHeight='auto'
        footer={
          <>
            <Button
              type='button'
              variant='outline'
              onClick={() => setEditing(false)}
              disabled={updateMutation.isPending}
            >
              {t('Cancel')}
            </Button>
            <Button
              type='submit'
              form='invoice-details-form'
              disabled={updateMutation.isPending}
            >
              {updateMutation.isPending && <Loader2 className='animate-spin' />}
              {t('Save')}
            </Button>
          </>
        }
      >
        <form
          id='invoice-details-form'
          className='grid grid-cols-1 gap-4 sm:grid-cols-2'
          onSubmit={form.handleSubmit((details) =>
            updateMutation.mutate(details)
          )}
        >
          {invoiceFields.map((field) => {
            const error = form.formState.errors[field.name]
            const errorId = `invoice-${field.name}-error`
            return (
              <label
                key={field.name}
                className={
                  'wide' in field && field.wide
                    ? 'grid gap-1.5 sm:col-span-2'
                    : 'grid gap-1.5'
                }
              >
                <span className='text-sm font-medium'>{t(field.label)}</span>
                <Input
                  type={'type' in field ? field.type : 'text'}
                  aria-invalid={Boolean(error)}
                  aria-describedby={error ? errorId : undefined}
                  {...form.register(field.name)}
                />
                {error && (
                  <span id={errorId} className='text-destructive text-xs'>
                    {error.message}
                  </span>
                )}
              </label>
            )
          })}
        </form>
      </Dialog>
    </div>
  )
}
