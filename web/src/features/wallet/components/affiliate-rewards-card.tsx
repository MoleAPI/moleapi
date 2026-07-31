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
import { Share2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { TitledCard } from '@/components/ui/titled-card'
import { formatQuota } from '@/lib/format'

import type { UserWalletData } from '../types'

interface AffiliateRewardsCardProps {
  user: UserWalletData | null
  affiliateLink: string
  onTransfer: () => void
  inviterReward?: number
  inviteeReward?: number
  complianceConfirmed?: boolean
  loading?: boolean
}

export function AffiliateRewardsCard({
  user,
  affiliateLink,
  onTransfer,
  inviterReward = 0,
  inviteeReward = 0,
  complianceConfirmed = true,
  loading,
}: AffiliateRewardsCardProps) {
  const { t } = useTranslation()
  if (loading) {
    return (
      <TitledCard
        title={<Skeleton className='h-6 w-32' />}
        description={<Skeleton className='h-4 w-56' />}
        icon={<Share2 className='size-4' />}
        iconTone='chart-3'
        disableHoverEffect
        contentClassName='flex flex-col gap-4'
      >
        <Skeleton className='h-20 rounded-lg' />
        <Skeleton className='h-32 rounded-lg' />
        <Skeleton className='h-10 rounded-lg' />
      </TitledCard>
    )
  }

  const hasRewards = (user?.aff_quota ?? 0) > 0

  return (
    <TitledCard
      title={t('Referral Rewards')}
      description={t('Invite friends to earn extra rewards.')}
      icon={<Share2 className='size-4' />}
      iconTone='chart-3'
      disableHoverEffect
      contentClassName='flex flex-col gap-5'
    >
      <div className='grid grid-cols-3 gap-2 text-center'>
        {[
          [t('Pending'), formatQuota(user?.aff_quota ?? 0)],
          [t('Total Earned'), formatQuota(user?.aff_history_quota ?? 0)],
          [t('Invites'), String(user?.aff_count ?? 0)],
        ].map(([label, value]) => (
          <div key={label} className='bg-muted/30 rounded-lg border p-2.5'>
            <div className='text-muted-foreground truncate text-[10px] font-medium tracking-wider uppercase'>
              {label}
            </div>
            <div className='mt-1 truncate text-sm font-semibold tabular-nums'>
              {value}
            </div>
          </div>
        ))}
      </div>

      <div className='flex flex-col gap-2 rounded-lg border p-3'>
        <h3 className='text-sm font-semibold'>{t('Reward Details')}</h3>
        <ul className='text-muted-foreground flex list-disc flex-col gap-1.5 ps-5 text-sm'>
          <li>
            {t(
              'Earn {{reward}} in referral credit for each friend who signs up.',
              { reward: formatQuota(inviterReward) }
            )}
          </li>
          <li>
            {t(
              'Friends who sign up with your referral code receive {{reward}} in account credit.',
              { reward: formatQuota(inviteeReward) }
            )}
          </li>
          <li>
            {t(
              'Referral rewards are first added to referral credit and can be moved to your account balance using Transfer.'
            )}
          </li>
        </ul>
      </div>

      <div className='flex flex-col gap-2'>
        <div className='flex items-center gap-2'>
          <Input
            value={affiliateLink}
            readOnly
            aria-label={t('Copy referral link')}
            className='border-muted bg-background/70 h-9 min-w-0 flex-1 font-mono text-xs'
          />
          <CopyButton
            value={affiliateLink}
            variant='outline'
            className='bg-background size-9 shrink-0'
            iconClassName='size-4'
            tooltip={t('Copy referral link')}
            aria-label={t('Copy referral link')}
          />
        </div>
        {hasRewards ? (
          <Button
            onClick={onTransfer}
            disabled={!complianceConfirmed}
            className='h-9 w-full'
            size='sm'
          >
            {t('Transfer to Balance')}
          </Button>
        ) : null}
        {!complianceConfirmed ? (
          <p className='text-muted-foreground text-xs'>
            {t(
              'Referral reward transfer is disabled until the administrator confirms compliance terms.'
            )}
          </p>
        ) : null}
      </div>
    </TitledCard>
  )
}
