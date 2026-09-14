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
import { Shield } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Skeleton } from '@/components/ui/skeleton'
import { TitledCard } from '@/components/ui/titled-card'
import { AccessTokenCard } from '@/features/security/components/access-token-card'
import { AccountActionCard } from '@/features/security/components/account-action-card'

import type { UserProfile } from '../types'

interface ProfileSecurityCardProps {
  profile: UserProfile | null
  loading: boolean
  onProfileUpdate: () => void
}

export function ProfileSecurityCard(props: ProfileSecurityCardProps) {
  const { t } = useTranslation()

  if (!props.loading && !props.profile) return null

  return (
    <>
      <TitledCard
        title={t('Security')}
        description={t('Manage your security settings and account access')}
        icon={<Shield className='size-4' />}
        iconTone='success'
        disableHoverEffect
      >
        <div className='space-y-3'>
          {props.profile ? (
            <>
              <AccountActionCard
                action='password'
                username={props.profile.username}
                hasPassword={props.profile.has_password}
                onUpdate={props.onProfileUpdate}
              />
              <AccountActionCard
                action='delete'
                username={props.profile.username}
              />
            </>
          ) : (
            <Skeleton className='h-32 w-full' />
          )}
        </div>
      </TitledCard>
      {props.profile && <AccessTokenCard />}
    </>
  )
}
