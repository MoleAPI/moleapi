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
import {
  PAYMENT_TYPES,
  DEFAULT_PRESET_MULTIPLIERS,
  DEFAULT_PAYMENT_TYPE,
  DEFAULT_MIN_TOPUP,
} from '../constants'
import type {
  CreemProduct,
  PaymentMethod,
  PresetAmount,
  TopupInfo,
} from '../types'

const DEDICATED_PAYMENT_TYPES = new Set<string>([
  PAYMENT_TYPES.STRIPE,
  PAYMENT_TYPES.CREEM,
  PAYMENT_TYPES.WAFFO,
  PAYMENT_TYPES.WAFFO_PANCAKE,
  PAYMENT_TYPES.LANTU,
  PAYMENT_TYPES.NOWPAYMENTS,
])

// ============================================================================
// Payment Processing Functions
// ============================================================================

/**
 * Check if browser is Safari
 */
function isSafariBrowser(): boolean {
  return (
    navigator.userAgent.includes('Safari') &&
    !navigator.userAgent.includes('Chrome')
  )
}

/**
 * Submit payment form (for non-Stripe payments)
 */
export function submitPaymentForm(
  url: string,
  params: Record<string, unknown>
): boolean {
  if (!isSafeHttpPaymentUrl(url)) {
    return false
  }

  const form = document.createElement('form')
  form.action = url
  form.method = 'POST'

  // Don't open in new tab for Safari
  if (!isSafariBrowser()) {
    form.target = '_blank'
  }

  // Add form parameters
  Object.entries(params).forEach(([key, value]) => {
    const input = document.createElement('input')
    input.type = 'hidden'
    input.name = key
    input.value = String(value)
    form.appendChild(input)
  })

  document.body.appendChild(form)
  form.submit()
  document.body.removeChild(form)
  return true
}

/**
 * Check if payment method is Stripe
 */
export function isStripePayment(paymentType: string): boolean {
  return paymentType === PAYMENT_TYPES.STRIPE
}

/**
 * Check if payment method is Waffo
 */
export function isWaffoPayment(paymentType: string): boolean {
  return paymentType === PAYMENT_TYPES.WAFFO
}

/**
 * Check if payment method is Waffo Pancake
 *
 * Pancake is a metered-style payment that goes through a dedicated checkout
 * URL flow rather than the generic epay form submission, so it must be
 * special-cased in payment dispatch logic.
 */
export function isWaffoPancakePayment(paymentType: string): boolean {
  return paymentType === PAYMENT_TYPES.WAFFO_PANCAKE
}

export function isLanTuPayment(paymentType: string): boolean {
  return paymentType === PAYMENT_TYPES.LANTU
}

export function isNowPaymentsPayment(paymentType: string): boolean {
  return paymentType === PAYMENT_TYPES.NOWPAYMENTS
}

export function findNearestCreemProduct(
  products: CreemProduct[] | undefined,
  amount: number
): { product: CreemProduct | null; adjusted: boolean } {
  const validProducts = (products ?? []).filter(
    (product) =>
      product.productId && Number.isFinite(product.price) && product.price > 0
  )
  if (validProducts.length === 0 || !Number.isFinite(amount)) {
    return { product: null, adjusted: false }
  }

  const amountCents = Math.round(amount * 100)
  const product = validProducts.reduce((best, current) => {
    const bestPriceCents = Math.round(best.price * 100)
    const currentPriceCents = Math.round(current.price * 100)
    const bestDistance = Math.abs(bestPriceCents - amountCents)
    const currentDistance = Math.abs(currentPriceCents - amountCents)
    if (currentDistance < bestDistance) return current
    if (
      currentDistance === bestDistance &&
      currentPriceCents < bestPriceCents
    ) {
      return current
    }
    return best
  })

  return {
    product,
    adjusted: Math.round(product.price * 100) !== amountCents,
  }
}

export function isSafeHttpPaymentUrl(value: string): boolean {
  try {
    const url = new URL(value.trim())
    return (
      (url.protocol === 'http:' || url.protocol === 'https:') &&
      !url.username &&
      !url.password
    )
  } catch {
    return false
  }
}

export function getStandardPaymentMethods(
  paymentMethods: PaymentMethod[] = []
): PaymentMethod[] {
  return paymentMethods.filter(
    (method) => method?.type && method.type !== PAYMENT_TYPES.WAFFO
  )
}

export function getEpayPaymentMethods(
  paymentMethods: PaymentMethod[] = []
): PaymentMethod[] {
  return paymentMethods.filter(
    (method) => method?.type && !DEDICATED_PAYMENT_TYPES.has(method.type)
  )
}

export interface PaymentProcessors {
  regular: (topupAmount: number, paymentType: string) => Promise<boolean>
  waffo: (topupAmount: number, payMethodIndex: number) => Promise<boolean>
  waffoPancake: (topupAmount: number) => Promise<boolean>
  lantu: (topupAmount: number) => Promise<boolean>
  nowPayments: (topupAmount: number) => Promise<boolean>
}

export async function dispatchSelectedPayment(
  paymentMethod: PaymentMethod,
  topupAmount: number,
  waffoMethodIndex: number | null,
  processors: PaymentProcessors
): Promise<boolean> {
  if (isWaffoPayment(paymentMethod.type)) {
    if (waffoMethodIndex === null) {
      return false
    }
    return processors.waffo(topupAmount, waffoMethodIndex)
  }

  if (isWaffoPancakePayment(paymentMethod.type)) {
    return processors.waffoPancake(topupAmount)
  }

  if (isLanTuPayment(paymentMethod.type)) {
    return processors.lantu(topupAmount)
  }

  if (isNowPaymentsPayment(paymentMethod.type)) {
    return processors.nowPayments(topupAmount)
  }

  return processors.regular(topupAmount, paymentMethod.type)
}

/**
 * Get default payment type from topup info
 */
export function getDefaultPaymentType(topupInfo: TopupInfo | null): string {
  if (!topupInfo) {
    return DEFAULT_PAYMENT_TYPE
  }

  // Return first available payment method or default
  if (topupInfo.pay_methods?.length > 0) {
    return topupInfo.pay_methods[0].type
  }

  if (topupInfo.enable_stripe_topup) {
    return PAYMENT_TYPES.STRIPE
  }

  if (topupInfo.enable_waffo_topup) {
    return PAYMENT_TYPES.WAFFO
  }

  if (topupInfo.enable_waffo_pancake_topup) {
    return PAYMENT_TYPES.WAFFO_PANCAKE
  }

  if (topupInfo.enable_nowpayments_topup) {
    return PAYMENT_TYPES.NOWPAYMENTS
  }

  return DEFAULT_PAYMENT_TYPE
}

/**
 * Get minimum topup amount from topup info
 */
export function getMinTopupAmount(topupInfo: TopupInfo | null): number {
  if (!topupInfo) {
    return DEFAULT_MIN_TOPUP
  }

  if (topupInfo.enable_online_topup) {
    return topupInfo.min_topup
  }

  if (topupInfo.enable_stripe_topup) {
    return topupInfo.stripe_min_topup
  }

  if (topupInfo.enable_waffo_topup) {
    return topupInfo.waffo_min_topup || DEFAULT_MIN_TOPUP
  }

  if (topupInfo.enable_waffo_pancake_topup) {
    return topupInfo.waffo_pancake_min_topup || DEFAULT_MIN_TOPUP
  }

  if (topupInfo.enable_lantu_topup) {
    return topupInfo.lantu_min_topup || DEFAULT_MIN_TOPUP
  }

  if (topupInfo.enable_nowpayments_topup) {
    return topupInfo.nowpayments_min_topup || DEFAULT_MIN_TOPUP
  }

  return DEFAULT_MIN_TOPUP
}

/**
 * Generate preset amounts based on minimum topup
 */
export function generatePresetAmounts(minAmount: number): PresetAmount[] {
  return DEFAULT_PRESET_MULTIPLIERS.map((multiplier) => ({
    value: minAmount * multiplier,
  }))
}

function getTieredRate(
  rates: Record<number, number>,
  amount: number,
  fallback: number
): number {
  let matchedTier = -1
  let matchedRate = fallback

  for (const [rawTier, rawRate] of Object.entries(rates)) {
    const tier = Number(rawTier)
    const rate = Number(rawRate)
    if (
      Number.isFinite(tier) &&
      Number.isFinite(rate) &&
      tier >= 0 &&
      tier <= amount &&
      tier > matchedTier
    ) {
      matchedTier = tier
      matchedRate = rate
    }
  }

  return matchedRate
}

export function getTopupDiscountRate(
  discounts: Record<number, number>,
  amount: number
): number {
  const rate = getTieredRate(discounts, amount, 1)
  return rate > 0.5 && rate <= 1 ? rate : 1
}

export function getTopupBonusRate(
  bonuses: Record<number, number>,
  amount: number
): number {
  const rate = getTieredRate(bonuses, amount, 0)
  return rate >= 0 && rate <= 1 ? rate : 0
}

/**
 * Merge custom preset amounts with the effective discount and bonus tiers.
 */
export function mergePresetAmounts(
  amountOptions: number[],
  discounts: Record<number, number>,
  bonuses: Record<number, number> = {}
): PresetAmount[] {
  if (!amountOptions || amountOptions.length === 0) {
    return []
  }

  return amountOptions.map((amount) => ({
    value: amount,
    discount: getTopupDiscountRate(discounts, amount),
    bonus: getTopupBonusRate(bonuses, amount),
  }))
}
