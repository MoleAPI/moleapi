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
import type { TFunction } from 'i18next'
import { z } from 'zod'

import { api } from '@/lib/api'
import {
  createServerError,
  requireServerSuccess,
} from '@/lib/server-error-message'

export interface TopUpInvoiceDetails {
  name: string
  email: string
  company: string
  tax_id: string
  address: string
  city: string
  state: string
  postal_code: string
  country: string
}

export interface TopUpInvoice {
  system_name: string
  invoice_no: string
  invoice_details: TopUpInvoiceDetails
  customer_name: string
  customer_email: string
  customer_company: string
  customer_tax_id: string
  customer_address: string
  trade_no: string
  gateway_trade_no: string
  payment_method: string
  payment_provider: string
  top_up_amount: string
  credited_quota: string
  paid_amount: string
  created_at: string
  completed_at: string
  issued_at: string
  can_edit: boolean
}

export function getTopUpInvoiceDetailsSchema(t: TFunction) {
  const text = (limit: number) =>
    z
      .string()
      .trim()
      .refine((value) => [...value].length <= limit, {
        message: t('Maximum {{count}} characters', { count: limit }),
      })

  return z
    .object({
      name: text(120),
      email: z
        .union([
          z.literal(''),
          z.string().trim().email(t('Please enter a valid email address')),
        ])
        .refine((value) => [...value].length <= 254, {
          message: t('Maximum {{count}} characters', { count: 254 }),
        }),
      company: text(160),
      tax_id: text(80),
      address: text(240),
      city: text(100),
      state: text(100),
      postal_code: text(32),
      country: text(100),
    })
    .refine(
      (details) => {
        const address = [
          details.address,
          details.city,
          details.state,
          details.postal_code,
          details.country,
        ]
          .filter(Boolean)
          .join(', ')
        return [...address].length <= 300
      },
      {
        message: t('Billing address must be 300 characters or fewer'),
        path: ['address'],
      }
    )
}

export async function getTopUpInvoice(id: number): Promise<TopUpInvoice> {
  const response = await api.get(`/api/user/topup/${id}/invoice`, {
    params: { format: 'json' },
  })
  const payload = requireServerSuccess(response.data) as {
    data?: TopUpInvoice
  }
  if (!payload.data) throw createServerError(payload, 'Receipt not found')
  return payload.data
}

export async function updateTopUpInvoice(
  id: number,
  details: TopUpInvoiceDetails
): Promise<void> {
  const response = await api.put(`/api/user/topup/${id}/invoice`, details)
  requireServerSuccess(response.data)
}

export async function downloadTopUpInvoice(
  id: number,
  tradeNo: string
): Promise<void> {
  const response = await api.get(`/api/user/topup/${id}/invoice`, {
    params: { download: 1 },
    responseType: 'blob',
  })
  const contentType = response.headers['content-type']
  if (
    typeof contentType !== 'string' ||
    !contentType.includes('application/pdf')
  ) {
    let payload: unknown = response.data
    if (response.data instanceof Blob) {
      try {
        payload = JSON.parse(await response.data.text())
      } catch {
        // Keep the original response as the error source.
      }
    }
    throw createServerError(payload, 'Unable to download receipt.')
  }
  const disposition = response.headers['content-disposition']
  const match =
    typeof disposition === 'string'
      ? disposition.match(/filename="?([^";]+)"?/i)
      : null
  const url = URL.createObjectURL(response.data)
  const link = document.createElement('a')
  link.href = url
  link.download = match?.[1] || `receipt-${tradeNo}.pdf`
  document.body.append(link)
  link.click()
  link.remove()
  URL.revokeObjectURL(url)
}
