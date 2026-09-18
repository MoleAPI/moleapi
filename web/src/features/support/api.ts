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
import { z } from 'zod'

import type { TopupRecord } from '@/features/wallet/types'
import { api } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'

import { INVOICE_TYPE, TICKET_TYPES } from './constants'

const ticketTypeValues = TICKET_TYPES.map((item) => item.value) as [
  (typeof TICKET_TYPES)[number]['value'],
  ...(typeof TICKET_TYPES)[number]['value'][],
]

export const ticketSchema = z
  .object({
    type: z.enum(ticketTypeValues),
    subject: z.string().trim().min(3).max(200),
    content: z.string().trim().min(10).max(10000),
    invoiceTitle: z.string().trim().max(200).optional(),
    taxId: z.string().trim().max(100).optional(),
    invoiceEmail: z
      .string()
      .trim()
      .email()
      .max(200)
      .optional()
      .or(z.literal('')),
    invoiceAddressPhone: z.string().trim().max(500).optional(),
    bankAccount: z.string().trim().max(500).optional(),
  })
  .superRefine((values, context) => {
    if (values.type === INVOICE_TYPE && !values.invoiceTitle) {
      context.addIssue({
        code: 'custom',
        path: ['invoiceTitle'],
        message: 'Invoice title is required.',
      })
    }
  })

export type SupportTicket = {
  id: string
  ticketNumber: string
  subject: string
  description: string
  status: string
  category: string
  priority: string
  email: string
  createdTime: string
  modifiedTime: string
  statusType?: string
  activity?: 'new' | 'customer' | 'agent' | 'unknown'
  user?: { id: number; username: string }
}

export type SupportConversation = {
  id: string
  type: string
  direction: string
  summary: string
  content: string
  createdTime: string
  fromEmailAddress: string
  commentedTime?: string
  visibility?: string
  isPublic?: boolean
  contentType?: string
  author?: { name: string; type: string }
  commenter?: { name: string; type: string }
}

type ApiResponse<T> = { success: boolean; message?: string; data: T }

export async function getSupportConfig() {
  const response = await api.get<
    ApiResponse<{
      enabled: boolean
      community_links: Record<string, string>
    }>
  >('/api/support/config')
  return requireServerSuccess(response.data).data
}

export async function getSupportTickets(from = 0) {
  const response = await api.get<
    ApiResponse<{
      tickets: SupportTicket[]
      next_from: number
      has_more: boolean
    }>
  >('/api/support/tickets', { params: { from } })
  return requireServerSuccess(response.data).data
}

export async function createSupportTicket(input: {
  subject: string
  content: string
  type: string
}) {
  const response = await api.post<ApiResponse<SupportTicket>>(
    '/api/support/tickets',
    input
  )
  return requireServerSuccess(response.data).data
}

export async function uploadSupportAttachments(id: string, files: File[]) {
  const form = new FormData()
  files.forEach((file) => form.append('files', file))
  const response = await api.post<ApiResponse<SupportAttachment[]>>(
    `/api/support/tickets/${id}/attachments`,
    form
  )
  return requireServerSuccess(response.data).data
}

export type SupportAttachment = {
  id: string
  name: string
  size: string
  href: string
}

function billingRecordLine(record: TopupRecord) {
  const currency = record.payment_currency || 'USD'
  return `- ${record.trade_no} | ${currency} ${record.money} | ${record.payment_method} | ${record.status}`
}

export function buildTicketDescription(
  values: z.infer<typeof ticketSchema>,
  billingRecords: TopupRecord[]
) {
  const sections = [values.content.trim()]
  if (values.type === INVOICE_TYPE) {
    sections.push(
      [
        'Invoice information',
        `Invoice title: ${values.invoiceTitle || '-'}`,
        `Tax ID: ${values.taxId || '-'}`,
        `Delivery email: ${values.invoiceEmail || '-'}`,
        `Address and phone: ${values.invoiceAddressPhone || '-'}`,
        `Bank and account: ${values.bankAccount || '-'}`,
      ].join('\n')
    )
  }
  if (billingRecords.length > 0) {
    sections.push(
      [
        'Related billing records',
        ...billingRecords.map(billingRecordLine),
      ].join('\n')
    )
  }
  return sections.join('\n\n')
}

export async function getSupportTicket(id: string) {
  const response = await api.get<
    ApiResponse<{
      ticket: SupportTicket
      conversations: SupportConversation[]
      attachments: SupportAttachment[]
    }>
  >(`/api/support/tickets/${id}`)
  return requireServerSuccess(response.data).data
}

export async function downloadSupportAttachment(
  ticketId: string,
  attachment: SupportAttachment
) {
  const response = await api.get(
    `/api/support/tickets/${ticketId}/attachments/${attachment.id}`,
    { responseType: 'blob' }
  )
  const contentType = response.headers['content-type']
  const blob =
    response.data instanceof Blob
      ? response.data
      : new Blob([response.data], {
          type: typeof contentType === 'string' ? contentType : undefined,
        })
  return { filename: attachment.name, url: URL.createObjectURL(blob) }
}

export async function replySupportTicket(id: string, content: string) {
  const response = await api.post<ApiResponse<null>>(
    `/api/support/tickets/${id}/reply`,
    { content }
  )
  return requireServerSuccess(response.data).data
}

export async function updateSupportTicketStatus(id: string, status: string) {
  const response = await api.patch<ApiResponse<null>>(
    `/api/support/tickets/${id}/status`,
    { status }
  )
  return requireServerSuccess(response.data).data
}
