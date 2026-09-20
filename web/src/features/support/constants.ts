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
export const TICKET_TYPES = [
  {
    value: 'API Integration',
    template:
      'Please include the API endpoint, request example, response, and request ID.',
  },
  {
    value: 'Authentication Issue',
    template:
      'Please describe the authentication method, error message, and when the issue started.',
  },
  {
    value: 'Billing & Credits',
    template:
      'Please include the related order number, amount, payment method, and expected result.',
  },
  {
    value: 'Model Availability',
    template:
      'Please include the model name, region, endpoint, and the availability issue you observed.',
  },
  {
    value: 'Model Rate Limit',
    template:
      'Please include the model name, request rate, error response, and approximate occurrence time.',
  },
  {
    value: 'Partnership Inquiry',
    template:
      'Please introduce your organization, cooperation proposal, expected scale, and contact method.',
  },
  {
    value: 'Feature Request',
    template:
      'Please describe the use case, current workaround, desired behavior, and expected benefit.',
  },
  {
    value: 'Invoice Request',
    template:
      'Please select the related billing records and add any special invoice requirements.',
  },
  {
    value: 'Other',
    template:
      'Please describe what happened, what you expected, and any steps needed to reproduce it.',
  },
] as const

export type TicketType = (typeof TICKET_TYPES)[number]['value']

export const INVOICE_TYPE: TicketType = 'Invoice Request'

export const BILLING_TYPES: readonly TicketType[] = [
  'Billing & Credits',
  INVOICE_TYPE,
]

const ticketTypeValues = new Set<string>(TICKET_TYPES.map((item) => item.value))
const ticketMarker = /\s*<ticket>([\s\S]*?)<\/ticket>\s*$/i

export function parseAssistantReply(content: string) {
  const marker = content.match(ticketMarker)
  const answer = content.replace(ticketMarker, '').trim()
  if (!marker) return { answer, type: 'Other' as TicketType, subject: '' }

  try {
    const suggestion = JSON.parse(marker[1]) as {
      type?: string
      subject?: string
    }
    return {
      answer,
      type: ticketTypeValues.has(suggestion.type ?? '')
        ? (suggestion.type as TicketType)
        : ('Other' as TicketType),
      subject:
        typeof suggestion.subject === 'string'
          ? suggestion.subject.trim().slice(0, 200)
          : '',
    }
  } catch {
    return { answer, type: 'Other' as TicketType, subject: '' }
  }
}

export const SUPPORT_ATTACHMENT_LIMIT = 3
export const SUPPORT_ATTACHMENT_MAX_BYTES = 5 * 1024 * 1024
export const SUPPORT_ATTACHMENT_ACCEPT =
  'image/png,image/jpeg,image/gif,image/webp,application/pdf,text/plain,.log'

export function mergeSupportFiles(current: File[], incoming: File[]) {
  const files = [...current]
  for (const file of incoming) {
    if (file.size > SUPPORT_ATTACHMENT_MAX_BYTES) {
      return { files, oversized: file }
    }
    const duplicate = files.some(
      (item) => item.name === file.name && item.size === file.size
    )
    if (!duplicate) files.push(file)
  }
  return { files: files.slice(0, SUPPORT_ATTACHMENT_LIMIT), oversized: null }
}
