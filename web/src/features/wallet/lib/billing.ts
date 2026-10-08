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
import type { StatusBadgeProps } from '@/components/status-badge'
import { api } from '@/lib/api'
import { formatTimestampToDate } from '@/lib/format'

import type { TopupRecord, TopupStatus } from '../types'

// ============================================================================
// Billing Utility Functions
// ============================================================================

interface StatusConfig {
  variant: StatusBadgeProps['variant']
  label: string
}

/**
 * Status badge configuration
 */
export const STATUS_CONFIG: Record<TopupStatus, StatusConfig> = {
  success: {
    variant: 'success',
    label: 'Success',
  },
  pending: {
    variant: 'warning',
    label: 'Pending',
  },
  failed: {
    variant: 'danger',
    label: 'Failed',
  },
  expired: {
    variant: 'danger',
    label: 'Expired',
  },
}

/**
 * Get status badge configuration
 */
export function getStatusConfig(status: TopupStatus): StatusConfig {
  return STATUS_CONFIG[status] || STATUS_CONFIG.pending
}

/**
 * Payment method display names
 */
export const PAYMENT_METHOD_NAMES: Record<string, string> = {
  stripe: 'Stripe',
  alipay: 'Alipay',
  wxpay: 'WeChat Pay',
  waffo: 'Waffo',
  waffo_pancake: 'Waffo Pancake',
  lantu: 'WeChat Pay',
  nowpayments: 'Crypto Pay',
}

/**
 * Get payment method display name
 */
export function getPaymentMethodName(
  method: string,
  t?: (key: string) => string
): string {
  const name = PAYMENT_METHOD_NAMES[method] || method
  return t ? t(name) : name
}

/**
 * Format timestamp to readable date string
 */
export function formatTimestamp(timestamp: number): string {
  return formatTimestampToDate(timestamp)
}

/**
 * Owners and admins can view a completed top-up invoice.
 */
export function getTopUpInvoiceUrl(
  record: Pick<
    TopupRecord,
    'id' | 'status' | 'user_id' | 'payment_method' | 'payment_provider'
  >,
  currentUserId?: number,
  isAdmin = false,
  download = false
): string | null {
  if (
    (!isAdmin && record.user_id !== currentUserId) ||
    record.status !== 'success' ||
    record.payment_method === 'waffo_pancake' ||
    record.payment_provider === 'waffo_pancake' ||
    !Number.isSafeInteger(record.id) ||
    record.id <= 0
  ) {
    return null
  }
  return `/api/user/topup/${record.id}/invoice${download ? '?download=1' : ''}`
}

export function getTopUpInvoiceDownloadUrl(
  record: Pick<
    TopupRecord,
    'id' | 'status' | 'user_id' | 'payment_method' | 'payment_provider'
  >,
  currentUserId?: number,
  isAdmin = false
): string | null {
  return getTopUpInvoiceUrl(record, currentUserId, isAdmin, true)
}

export function canRequestTopUpInvoice(
  record: Pick<
    TopupRecord,
    'id' | 'status' | 'payment_method' | 'payment_provider' | 'money'
  >
): boolean {
  const provider = record.payment_provider?.toLowerCase()
  return (
    Number.isSafeInteger(record.id) &&
    record.id > 0 &&
    record.status === 'success' &&
    (provider === 'epay' ||
      provider === 'lantu' ||
      ['alipay', 'wxpay', 'lantu'].includes(record.payment_method)) &&
    Number.isFinite(record.money) &&
    record.money > 0
  )
}

export function getWaffoPancakeInvoiceUrl(
  record: Pick<
    TopupRecord,
    'status' | 'payment_method' | 'payment_provider' | 'invoice_url'
  >
): string | null {
  const value = record.invoice_url?.trim()
  if (
    !value ||
    record.status !== 'success' ||
    (record.payment_method !== 'waffo_pancake' &&
      record.payment_provider !== 'waffo_pancake')
  ) {
    return null
  }
  try {
    const url = new URL(value)
    return url.protocol === 'https:' &&
      url.host === 'pancake.waffo.ai' &&
      !url.username &&
      !url.password
      ? value
      : null
  } catch {
    return null
  }
}

export function getInvoiceFilename(
  contentDisposition: string | undefined,
  fallback: string
): string {
  const match = contentDisposition?.match(/filename="?([^";]+)"?/i)
  return match?.[1] || fallback
}

export async function fetchTopUpInvoiceFile(
  record: Pick<
    TopupRecord,
    | 'id'
    | 'status'
    | 'user_id'
    | 'trade_no'
    | 'payment_method'
    | 'payment_provider'
  >,
  currentUserId: number | undefined,
  isAdmin: boolean,
  download = false
): Promise<{ filename: string; url: string } | null> {
  const path = getTopUpInvoiceUrl(
    record,
    isAdmin ? undefined : currentUserId,
    isAdmin,
    download
  )
  if (!path) return null

  const response = await api.get(path, { responseType: 'blob' })
  const contentTypeHeader = response.headers['content-type']
  const contentDispositionHeader = response.headers['content-disposition']
  const contentType =
    typeof contentTypeHeader === 'string'
      ? contentTypeHeader
      : 'text/html; charset=utf-8'
  const contentDisposition =
    typeof contentDispositionHeader === 'string'
      ? contentDispositionHeader
      : undefined
  const blob =
    response.data instanceof Blob
      ? response.data
      : new Blob([response.data], { type: contentType })
  return {
    filename: getInvoiceFilename(
      contentDisposition,
      `receipt-${record.trade_no}.pdf`
    ),
    url: URL.createObjectURL(blob),
  }
}
