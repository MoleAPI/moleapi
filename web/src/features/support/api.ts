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

import { api } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'

export const ticketSchema = z.object({
  type: z.enum(['Support', 'Billing & Invoice']),
  subject: z.string().trim().min(3).max(200),
  content: z.string().trim().min(10).max(10000),
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
}

export type SupportConversation = {
  id: string
  type: string
  direction: string
  summary: string
  content: string
  createdTime: string
  fromEmailAddress: string
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

export async function getSupportTickets() {
  const response = await api.get<ApiResponse<SupportTicket[]>>(
    '/api/support/tickets'
  )
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

export async function getSupportTicket(id: string) {
  const response = await api.get<
    ApiResponse<{
      ticket: SupportTicket
      conversations: SupportConversation[]
    }>
  >(`/api/support/tickets/${id}`)
  return requireServerSuccess(response.data).data
}

export async function replySupportTicket(id: string, content: string) {
  const response = await api.post<ApiResponse<null>>(
    `/api/support/tickets/${id}/reply`,
    { content }
  )
  return requireServerSuccess(response.data).data
}
