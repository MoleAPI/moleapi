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
import { formatCurrencyFromUSD } from '@/lib/currency'
import { formatNumber, formatQuota } from '@/lib/format'

import { DEFAULT_DISCOUNT_RATE } from '../constants'
import type { TopupRecord } from '../types'

type IntlWithSupportedValues = typeof Intl & {
  supportedValuesOf?: (key: 'currency') => string[]
}

const ISO_CURRENCY_CODES = new Set(
  (Intl as IntlWithSupportedValues).supportedValuesOf?.('currency') ?? []
)

// ============================================================================
// Wallet-specific Formatting Functions
// ============================================================================

/**
 * Format Creem price with currency symbol (USD/EUR)
 */
export function formatCreemPrice(
  price: number,
  currency: 'USD' | 'EUR'
): string {
  const symbol = currency === 'EUR' ? '€' : '$'
  return `${symbol}${price.toFixed(2)}`
}

/**
 * Format large quota numbers with K/M suffix
 */
export function formatQuotaShort(quota: number): string {
  if (quota >= 1000000) {
    return `${(quota / 1000000).toFixed(1)}M`
  }
  if (quota >= 1000) {
    return `${(quota / 1000).toFixed(1)}K`
  }
  return quota.toString()
}

/**
 * Format currency amount that is already in local currency.
 * This is used for payment amounts that have been calculated via priceRatio.
 */
export function formatCurrency(amount: number | string): string {
  const numeric =
    typeof amount === 'number' ? amount : Number.parseFloat(String(amount))
  if (!Number.isFinite(numeric)) return '-'

  return new Intl.NumberFormat(undefined, {
    minimumFractionDigits: 0,
    maximumFractionDigits: Math.abs(numeric) >= 1 ? 2 : 4,
  }).format(numeric)
}

/** Format the amount charged by its recorded ISO currency. */
export function formatHistoricalPaymentAmount(
  amount: number | null | undefined,
  currency?: string | null,
  locales?: Intl.LocalesArgument
): string {
  if (amount == null || !Number.isFinite(amount)) return '-'

  const normalizedCurrency = currency?.trim().toUpperCase()
  if (!normalizedCurrency || !ISO_CURRENCY_CODES.has(normalizedCurrency)) {
    return formatNumber(amount, locales)
  }

  try {
    return new Intl.NumberFormat(locales, {
      style: 'currency',
      currency: normalizedCurrency,
    }).format(amount)
  } catch {
    return formatNumber(amount, locales)
  }
}

export function formatHistoricalTopUpAmount(
  record: Pick<TopupRecord, 'amount' | 'payment_method' | 'payment_provider'>
): string {
  const provider = record.payment_provider?.trim().toLowerCase()
  const method = record.payment_method.trim().toLowerCase()
  if (provider === 'creem' || method === 'creem') {
    return formatQuota(record.amount)
  }

  return formatCurrencyFromUSD(record.amount, {
    digitsLarge: 2,
    digitsSmall: 2,
    abbreviate: false,
  })
}

export function formatHistoricalCreditedAmount(
  record: Pick<TopupRecord, 'credited_quota'>
): string | null {
  const hasCreditedFact =
    typeof record.credited_quota === 'number' &&
    Number.isFinite(record.credited_quota) &&
    record.credited_quota > 0
  return hasCreditedFact ? formatQuota(record.credited_quota as number) : null
}

export function formatInviteRebateRatio(
  ratio: number | null | undefined
): string {
  const normalized =
    typeof ratio === 'number' && Number.isFinite(ratio) ? Math.max(0, ratio) : 0
  return `${new Intl.NumberFormat(undefined, {
    maximumFractionDigits: 2,
  }).format(normalized / 100)}%`
}

/**
 * Get discount label for display (e.g., "20% OFF")
 */
export function getDiscountLabel(discount: number): string {
  if (discount >= DEFAULT_DISCOUNT_RATE) {
    return ''
  }
  const off = Math.round((1 - discount) * 100)
  return `${off}% OFF`
}

/**
 * Calculate pricing details for a preset amount
 */
export function calculatePresetPricing(
  presetValue: number,
  priceRatio: number,
  discount: number,
  usdExchangeRate: number = 1,
  bonus: number = 0
) {
  const originalPrice = presetValue * priceRatio
  const actualPrice = originalPrice * discount
  const savedAmount = originalPrice - actualPrice
  const hasDiscount = discount < 1.0
  const displayValue = presetValue * usdExchangeRate
  const receivedAmount = presetValue * (1 + bonus)

  return {
    displayValue,
    originalPrice,
    actualPrice,
    savedAmount,
    hasDiscount,
    receivedAmount,
  }
}
