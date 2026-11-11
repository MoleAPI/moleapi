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
import DOMPurify from 'dompurify'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Field, FieldError, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'

import type { SupportTicket } from '../api'

const schema = z.object({
  title: z.string().trim().min(1).max(200),
  taxId: z.string().trim().min(1).max(100),
  total: z.string().trim().min(1).max(100),
  number: z.string().regex(/^\d{8,30}$/),
  date: z.iso.date(),
  email: z.email().max(254),
})

export function InvoiceReplyTemplate(props: {
  ticket: SupportTicket
  open: boolean
  onOpenChange: (open: boolean) => void
  onApply: (content: string, email: string) => void
}) {
  const { t } = useTranslation()
  // ponytail: reuse the existing invoice request labels; leave missing fields editable.
  const text =
    new DOMParser().parseFromString(
      DOMPurify.sanitize(
        props.ticket.description.replaceAll(/<br\s*\/?>|<\/p>|<\/div>/gi, '\n'),
        { ALLOWED_TAGS: [], ALLOWED_ATTR: [] }
      ),
      'text/html'
    ).body.textContent ?? ''
  const lines = text.split('\n')
  const read = (label: string) => {
    const value = lines
      .find((line) => line.trim().startsWith(`${label}:`))
      ?.trim()
      .slice(label.length + 1)
      .trim()
    return value === '-' ? '' : (value ?? '')
  }
  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: {
      title: read('Invoice title'),
      taxId: read('Tax ID'),
      total:
        lines.filter((line) => line.trim().startsWith('Invoice total:'))
          .length === 1
          ? read('Invoice total')
          : '',
      number: '',
      date: '',
      email: read('Delivery email') || props.ticket.email,
    },
  })
  const fields = [
    { name: 'title', label: t('Invoice title'), type: 'text', max: 200 },
    { name: 'taxId', label: t('Tax ID'), type: 'text', max: 100 },
    { name: 'total', label: t('Invoice total'), type: 'text', max: 100 },
    { name: 'number', label: t('Invoice number'), type: 'text', max: 30 },
    { name: 'date', label: t('Invoice date'), type: 'date', max: undefined },
    { name: 'email', label: t('Delivery email'), type: 'email', max: 254 },
  ] as const

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('Invoice reply template')}
      description={t(
        'Fill in the invoice details, then review the reply and attach the PDF before sending.'
      )}
      footer={
        <Button type='submit' form='invoice-reply-template'>
          {t('Insert template')}
        </Button>
      }
    >
      <form
        id='invoice-reply-template'
        className='grid gap-4 sm:grid-cols-2'
        onSubmit={form.handleSubmit((values) => {
          props.onApply(
            [
              t('Hello,'),
              '',
              t(
                'Your VAT invoice has been issued. The PDF is attached to this reply.'
              ),
              '',
              `${t('Invoice title')}: ${values.title}`,
              `${t('Tax ID')}: ${values.taxId}`,
              `${t('Invoice total')}: ${values.total}`,
              `${t('Invoice number')}: ${values.number}`,
              `${t('Invoice date')}: ${values.date}`,
              '',
              t('Please reply to this email if you have any questions.'),
              t('Thank you for your support.'),
              'MoleAPI',
            ].join('\n'),
            values.email
          )
          props.onOpenChange(false)
        })}
      >
        {fields.map((field) => (
          <Field
            key={field.name}
            data-invalid={Boolean(form.formState.errors[field.name])}
          >
            <FieldLabel htmlFor={`invoice-reply-${field.name}`}>
              {field.label}
            </FieldLabel>
            <Input
              id={`invoice-reply-${field.name}`}
              type={field.type}
              maxLength={field.max}
              required
              aria-invalid={Boolean(form.formState.errors[field.name])}
              {...form.register(field.name)}
            />
            {form.formState.errors[field.name] && (
              <FieldError>{t('Check this field and try again.')}</FieldError>
            )}
          </Field>
        ))}
      </form>
    </Dialog>
  )
}
