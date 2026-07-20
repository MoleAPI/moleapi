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
import i18next from 'i18next'
import { useCallback, useEffect, useRef, useState } from 'react'
import { toast } from 'sonner'

import { useIsMobile } from '@/hooks/use-mobile'

import {
  getLanTuPaymentStatus,
  isApiSuccess,
  requestLanTuPayment,
} from '../api'
import { isSafeHttpPaymentUrl } from '../lib'
import type { LanTuCheckout, TopupStatus } from '../types'

export interface LanTuCheckoutSession extends LanTuCheckout {
  expiresAt: number
  status: TopupStatus
}

export function useLanTuPayment(onPaid: () => void | Promise<void>) {
  const isMobile = useIsMobile()
  const checkingRef = useRef(false)
  const [processing, setProcessing] = useState(false)
  const [checking, setChecking] = useState(false)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [session, setSession] = useState<LanTuCheckoutSession | null>(null)

  const checkLanTuPayment = useCallback(
    async (showError = true) => {
      if (!session?.trade_no || checkingRef.current) return false
      checkingRef.current = true
      setChecking(true)
      try {
        const response = await getLanTuPaymentStatus(session.trade_no)
        if (!isApiSuccess(response) || !response.data) {
          if (showError) {
            toast.error(
              response.message || i18next.t('Unable to check payment status')
            )
          }
          return false
        }

        const status = response.data.status
        setSession((current) => (current ? { ...current, status } : current))
        if (status === 'success') {
          toast.success(i18next.t('Payment completed'))
          setDialogOpen(false)
          await onPaid()
          return true
        }
        if (status === 'failed' || status === 'expired') {
          toast.error(i18next.t('Payment was not completed'))
        }
        return false
      } catch {
        if (showError) toast.error(i18next.t('Unable to check payment status'))
        return false
      } finally {
        checkingRef.current = false
        setChecking(false)
      }
    },
    [onPaid, session?.trade_no]
  )

  useEffect(() => {
    if (!dialogOpen || !session || session.status !== 'pending') return
    const interval = window.setInterval(() => {
      void checkLanTuPayment(false)
    }, 5000)
    return () => window.clearInterval(interval)
  }, [checkLanTuPayment, dialogOpen, session])

  const processLanTuPayment = useCallback(
    async (topupAmount: number) => {
      setProcessing(true)
      try {
        const response = await requestLanTuPayment({
          amount: Math.floor(topupAmount),
          client: isMobile ? 'h5' : 'native',
        })
        if (!isApiSuccess(response) || !response.data) {
          toast.error(response.message || i18next.t('Payment request failed'))
          return false
        }

        const checkout = response.data
        if (!checkout.pay_link.trim()) {
          toast.error(i18next.t('Payment request failed'))
          return false
        }
        if (
          (checkout.pay_link_kind === 'url' ||
            checkout.pay_link_kind === 'qr_image') &&
          !isSafeHttpPaymentUrl(checkout.pay_link)
        ) {
          toast.error(i18next.t('Invalid payment redirect URL'))
          return false
        }

        if (isMobile && checkout.pay_link_kind === 'url') {
          window.location.assign(checkout.pay_link)
          return true
        }

        setSession({
          ...checkout,
          expiresAt: Date.now() + 5 * 60 * 1000,
          status: 'pending',
        })
        setDialogOpen(true)
        return true
      } catch {
        toast.error(i18next.t('Payment request failed'))
        return false
      } finally {
        setProcessing(false)
      }
    },
    [isMobile]
  )

  return {
    processing,
    checking,
    dialogOpen,
    setDialogOpen,
    session,
    processLanTuPayment,
    checkLanTuPayment,
  }
}
