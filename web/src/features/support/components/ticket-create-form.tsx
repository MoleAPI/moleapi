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
import { SparklesIcon } from 'lucide-react'
import { useState } from 'react'
import { Controller, useForm, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
  FieldLegend,
  FieldSet,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Spinner } from '@/components/ui/spinner'
import { Textarea } from '@/components/ui/textarea'
import { sendChatCompletion } from '@/features/playground/api'
import {
  buildChatCompletionPayload,
  getInitialParameterEnabled,
  getInitialPlaygroundConfig,
} from '@/features/playground/lib'
import { getUserBillingHistory } from '@/features/wallet/api'
import { handleServerError } from '@/lib/handle-server-error'

import {
  buildTicketDescription,
  createSupportTicket,
  ticketSchema,
  uploadSupportAttachments,
} from '../api'
import {
  BILLING_TYPES,
  INVOICE_TYPE,
  TICKET_TYPES,
  type TicketType,
  mergeSupportFiles,
} from '../constants'
import { AttachmentPicker } from './attachment-picker'

type TicketForm = import('zod').infer<typeof ticketSchema>

export function TicketCreateForm(props: {
  accountEmail?: string
  onCreated: (id: string) => void
  initialValues?: {
    type: TicketType
    subject: string
    content: string
  }
}) {
  const { t, i18n } = useTranslation()
  const queryClient = useQueryClient()
  const [files, setFiles] = useState<File[]>([])
  const [selectedBillingIds, setSelectedBillingIds] = useState<number[]>([])
  const form = useForm<TicketForm>({
    resolver: zodResolver(ticketSchema),
    defaultValues: {
      type: props.initialValues?.type ?? 'API Integration',
      subject: props.initialValues?.subject ?? '',
      content: props.initialValues?.content ?? '',
      invoiceTitle: '',
      taxId: '',
      invoiceEmail: props.accountEmail ?? '',
      invoiceAddressPhone: '',
      bankAccount: '',
    },
  })
  const type = useWatch({ control: form.control, name: 'type' })
  const content = useWatch({ control: form.control, name: 'content' })
  const selectedType =
    TICKET_TYPES.find((item) => item.value === type) ?? TICKET_TYPES[0]
  const billingType = BILLING_TYPES.includes(type)
  const billing = useQuery({
    queryKey: ['support', 'billing-records'],
    queryFn: async () => {
      const response = await getUserBillingHistory(1, 10)
      if (!response.success) {
        throw new Error(response.message || 'Failed to load billing records')
      }
      return (response.data?.items ?? []).filter(
        (record) => record.status === 'success'
      )
    },
    enabled: billingType && Boolean(props.accountEmail),
  })

  const createTicket = useMutation({
    mutationFn: async (values: TicketForm) => {
      const relatedRecords = (billing.data ?? []).filter((record) =>
        selectedBillingIds.includes(record.id)
      )
      const ticket = await createSupportTicket({
        type: values.type,
        subject: values.subject,
        content: buildTicketDescription(values, relatedRecords),
      })
      let attachmentsFailed = false
      if (files.length > 0) {
        try {
          await uploadSupportAttachments(ticket.id, files)
        } catch {
          attachmentsFailed = true
        }
      }
      return { ticket, attachmentsFailed }
    },
    onSuccess: async (result) => {
      form.reset()
      setFiles([])
      setSelectedBillingIds([])
      await queryClient.invalidateQueries({ queryKey: ['support', 'tickets'] })
      props.onCreated(result.ticket.id)
      if (result.attachmentsFailed) {
        toast.warning(
          t(
            'The ticket was created, but some attachments could not be uploaded.'
          )
        )
      } else {
        toast.success(t('Ticket created'))
      }
    },
    onError: (error) => handleServerError(error),
  })
  const polishDescription = useMutation({
    mutationFn: async (description: string) => {
      const config = getInitialPlaygroundConfig()
      const payload = buildChatCompletionPayload(
        [
          {
            key: crypto.randomUUID(),
            from: 'user',
            versions: [
              {
                id: crypto.randomUUID(),
                content: `${t('Ticket type')}: ${t(type)}\n\n${description}`,
              },
            ],
          },
        ],
        { ...config, stream: false },
        getInitialParameterEnabled()
      )
      payload.messages.unshift({
        role: 'system',
        content:
          'Rewrite this support-ticket description so it is clear, concise, and useful for troubleshooting. Preserve every fact, identifier, error message, and redaction. Do not invent information. Reply in the same language as the user and output only the revised description.',
      })
      const response = await sendChatCompletion(payload)
      const polished = response.choices?.[0]?.message?.content?.trim()
      if (!polished) throw new Error('AI returned an empty response.')
      return polished
    },
    onSuccess: (polished) => {
      form.setValue('content', polished, {
        shouldDirty: true,
        shouldValidate: true,
      })
      toast.success(t('Description polished'))
    },
    onError: (error) => handleServerError(error),
  })

  if (!props.accountEmail) {
    return (
      <Alert>
        <AlertTitle>{t('Bind an email address to use support')}</AlertTitle>
        <AlertDescription>
          {t(
            'Your account email is used automatically for ticket notifications and ownership verification.'
          )}
        </AlertDescription>
        <Button className='mt-3' size='sm' render={<Link to='/profile' />}>
          {t('Go to profile')}
        </Button>
      </Alert>
    )
  }

  return (
    <form
      className='mx-auto w-full max-w-3xl'
      onSubmit={form.handleSubmit((values) => createTicket.mutate(values))}
    >
      <FieldGroup>
        <Field>
          <FieldLabel htmlFor='ticket-email'>{t('Contact email')}</FieldLabel>
          <Input id='ticket-email' value={props.accountEmail} disabled />
          <FieldDescription>
            {t('Filled automatically from your account profile.')}
          </FieldDescription>
        </Field>

        <Field data-invalid={Boolean(form.formState.errors.type)}>
          <FieldLabel htmlFor='ticket-type'>{t('Ticket type')}</FieldLabel>
          <Controller
            control={form.control}
            name='type'
            render={({ field }) => (
              <Select
                items={TICKET_TYPES.map((item) => ({
                  value: item.value,
                  label: t(item.value),
                }))}
                value={field.value}
                onValueChange={(value) => {
                  field.onChange(value)
                  setSelectedBillingIds([])
                }}
              >
                <SelectTrigger id='ticket-type' className='w-full'>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    {TICKET_TYPES.map((item) => (
                      <SelectItem key={item.value} value={item.value}>
                        {t(item.value)}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
            )}
          />
          <FieldDescription>{t(selectedType.template)}</FieldDescription>
        </Field>

        <Field data-invalid={Boolean(form.formState.errors.subject)}>
          <FieldLabel htmlFor='ticket-subject'>{t('Subject')}</FieldLabel>
          <Input
            id='ticket-subject'
            aria-invalid={Boolean(form.formState.errors.subject)}
            {...form.register('subject')}
          />
          <FieldError>
            {form.formState.errors.subject
              ? t('Enter a subject between 3 and 200 characters.')
              : null}
          </FieldError>
        </Field>

        <Field data-invalid={Boolean(form.formState.errors.content)}>
          <div className='flex items-center justify-between gap-3'>
            <FieldLabel htmlFor='ticket-content'>
              {t('Detailed description')}
            </FieldLabel>
            <Button
              type='button'
              size='sm'
              variant='outline'
              disabled={
                content.trim().length < 10 || polishDescription.isPending
              }
              onClick={() => polishDescription.mutate(content.trim())}
              title={t('Uses your selected Playground model')}
            >
              {polishDescription.isPending ? (
                <Spinner data-icon='inline-start' />
              ) : (
                <SparklesIcon data-icon='inline-start' />
              )}
              {polishDescription.isPending ? t('Polishing...') : t('AI polish')}
            </Button>
          </div>
          <Textarea
            id='ticket-content'
            rows={8}
            aria-invalid={Boolean(form.formState.errors.content)}
            placeholder={t(selectedType.template)}
            {...form.register('content')}
            onPaste={(event) => {
              const pastedFiles = [...event.clipboardData.files]
              if (pastedFiles.length === 0) return
              const result = mergeSupportFiles(files, pastedFiles)
              setFiles(result.files)
              if (result.oversized) {
                toast.error(
                  t('{{file}} exceeds the 5 MB attachment limit.', {
                    file: result.oversized.name,
                  })
                )
              }
            }}
          />
          <FieldError>
            {form.formState.errors.content
              ? t('Enter at least 10 characters.')
              : null}
          </FieldError>
        </Field>

        {type === INVOICE_TYPE && (
          <FieldSet>
            <FieldLegend>{t('Invoice information')}</FieldLegend>
            <FieldGroup>
              <Field data-invalid={Boolean(form.formState.errors.invoiceTitle)}>
                <FieldLabel htmlFor='invoice-title'>
                  {t('Invoice title')}
                </FieldLabel>
                <Input
                  id='invoice-title'
                  aria-invalid={Boolean(form.formState.errors.invoiceTitle)}
                  {...form.register('invoiceTitle')}
                />
                <FieldError>
                  {form.formState.errors.invoiceTitle
                    ? t('Invoice title is required.')
                    : null}
                </FieldError>
              </Field>
              <div className='grid gap-5 sm:grid-cols-2'>
                <Field>
                  <FieldLabel htmlFor='invoice-tax-id'>
                    {t('Tax ID')}
                  </FieldLabel>
                  <Input id='invoice-tax-id' {...form.register('taxId')} />
                </Field>
                <Field>
                  <FieldLabel htmlFor='invoice-email'>
                    {t('Invoice delivery email')}
                  </FieldLabel>
                  <Input
                    id='invoice-email'
                    type='email'
                    {...form.register('invoiceEmail')}
                  />
                </Field>
              </div>
              <Field>
                <FieldLabel htmlFor='invoice-address-phone'>
                  {t('Registered address and phone')}
                </FieldLabel>
                <Input
                  id='invoice-address-phone'
                  {...form.register('invoiceAddressPhone')}
                />
              </Field>
              <Field>
                <FieldLabel htmlFor='invoice-bank-account'>
                  {t('Bank and account')}
                </FieldLabel>
                <Input
                  id='invoice-bank-account'
                  {...form.register('bankAccount')}
                />
              </Field>
            </FieldGroup>
          </FieldSet>
        )}

        {billingType && (
          <FieldSet>
            <FieldLegend>{t('Related billing records')}</FieldLegend>
            <FieldDescription>
              {t('Select completed orders to include them automatically.')}
            </FieldDescription>
            {billing.isLoading ? (
              <Spinner />
            ) : (
              <FieldGroup className='gap-3'>
                {(billing.data ?? []).map((record) => {
                  const checked = selectedBillingIds.includes(record.id)
                  return (
                    <Field key={record.id} orientation='horizontal'>
                      <Checkbox
                        id={`billing-record-${record.id}`}
                        checked={checked}
                        onCheckedChange={(nextChecked) =>
                          setSelectedBillingIds((current) =>
                            nextChecked
                              ? [...current, record.id]
                              : current.filter((id) => id !== record.id)
                          )
                        }
                      />
                      <FieldLabel
                        htmlFor={`billing-record-${record.id}`}
                        className='font-normal'
                      >
                        {record.trade_no} ·{' '}
                        {new Intl.NumberFormat(i18n.language, {
                          style: 'currency',
                          currency: record.payment_currency || 'USD',
                        }).format(record.money)}
                      </FieldLabel>
                    </Field>
                  )
                })}
                {!billing.isLoading && !billing.data?.length && (
                  <FieldDescription>
                    {t('No completed billing records found.')}
                  </FieldDescription>
                )}
              </FieldGroup>
            )}
          </FieldSet>
        )}

        <Field>
          <FieldLabel>{t('Attachments')}</FieldLabel>
          <AttachmentPicker
            files={files}
            onFilesChange={setFiles}
            onRejected={(fileName) =>
              toast.error(
                t('{{file}} exceeds the 5 MB attachment limit.', {
                  file: fileName,
                })
              )
            }
          />
          <FieldDescription>
            {t(
              'Up to 3 files, 5 MB each. Images, PDF, text, and log files are supported.'
            )}
          </FieldDescription>
        </Field>

        <div className='flex justify-end'>
          <Button type='submit' disabled={createTicket.isPending}>
            {createTicket.isPending && <Spinner data-icon='inline-start' />}
            {createTicket.isPending ? t('Submitting...') : t('Submit ticket')}
          </Button>
        </div>
      </FieldGroup>
    </form>
  )
}
