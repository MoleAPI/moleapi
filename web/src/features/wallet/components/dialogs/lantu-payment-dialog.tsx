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
import { Loader2, RefreshCw } from 'lucide-react'
import { QRCodeSVG } from 'qrcode.react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'

import type { LanTuCheckoutSession } from '../../hooks/use-lantu-payment'

interface LanTuPaymentDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  session: LanTuCheckoutSession | null
  checking: boolean
  onCheck: () => void
}

function formatCountdown(milliseconds: number): string {
  const totalSeconds = Math.max(0, Math.ceil(milliseconds / 1000))
  const minutes = Math.floor(totalSeconds / 60)
  const seconds = totalSeconds % 60
  return `${minutes}:${seconds.toString().padStart(2, '0')}`
}

export function LanTuPaymentDialog({
  open,
  onOpenChange,
  session,
  checking,
  onCheck,
}: LanTuPaymentDialogProps) {
  const { t } = useTranslation()
  const [now, setNow] = useState(Date.now())

  useEffect(() => {
    if (!open || !session) return
    setNow(Date.now())
    const interval = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(interval)
  }, [open, session])

  const expired = !!session && now >= session.expiresAt

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='max-sm:w-[calc(100vw-1.5rem)] sm:max-w-sm'>
        <DialogHeader>
          <DialogTitle>{t('WeChat Pay')}</DialogTitle>
          <DialogDescription>
            {t('Scan the QR code with WeChat to complete payment')}
          </DialogDescription>
        </DialogHeader>

        {session && (
          <div className='space-y-4'>
            <div className='bg-muted/30 mx-auto flex size-56 items-center justify-center rounded-xl border p-3'>
              {session.pay_link_kind === 'qr_image' ? (
                <img
                  src={session.pay_link}
                  alt={t('WeChat payment QR code')}
                  className='size-full object-contain'
                  referrerPolicy='no-referrer'
                />
              ) : (
                <QRCodeSVG
                  value={session.pay_link}
                  size={196}
                  title={t('WeChat payment QR code')}
                />
              )}
            </div>

            <div className='grid grid-cols-2 gap-3 rounded-lg border p-3 text-sm'>
              <span className='text-muted-foreground'>{t('You Pay')}</span>
              <span className='text-right font-semibold'>
                ¥{session.pay_money}
              </span>
              <span className='text-muted-foreground'>
                {t('Time remaining')}
              </span>
              <span className='text-right font-mono font-medium'>
                {expired
                  ? t('Expired')
                  : formatCountdown(session.expiresAt - now)}
              </span>
            </div>

            <p className='text-muted-foreground text-center text-xs'>
              {t(
                'The balance updates automatically after payment is confirmed'
              )}
            </p>
          </div>
        )}

        <DialogFooter className='grid grid-cols-2 gap-2 sm:grid-cols-2'>
          <Button variant='outline' onClick={() => onOpenChange(false)}>
            {t('Close')}
          </Button>
          <Button onClick={onCheck} disabled={checking || expired}>
            {checking ? (
              <Loader2 className='mr-2 h-4 w-4 animate-spin' />
            ) : (
              <RefreshCw className='mr-2 h-4 w-4' />
            )}
            {t('I have paid')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
