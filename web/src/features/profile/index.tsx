import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Main } from '@/components/layout/components/main'
import {
  CardStaggerContainer,
  CardStaggerItem,
} from '@/components/page-transition'
import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from '@/components/ui/empty'
import { LoginSessionsCard } from '@/features/security/components/login-sessions-card'
import { PasskeyCard } from '@/features/security/components/passkey-card'
import { TwoFACard } from '@/features/security/components/two-fa-card'
import { useStatus } from '@/hooks/use-status'
import { useAuthStore } from '@/stores/auth-store'

import { AccountBindingsCard } from './components/account-bindings-card'
import { CheckinCalendarCard } from './components/checkin-calendar-card'
import { LanguagePreferencesCard } from './components/language-preferences-card'
import { ProfileHeader } from './components/profile-header'
import { ProfileSecurityCard } from './components/profile-security-card'
import { ProfileSettingsCard } from './components/profile-settings-card'
import { SidebarModulesCard } from './components/sidebar-modules-card'
import { useProfile } from './hooks/use-profile'
import {
  profileSecuritySectionOrder,
  type ProfileSecuritySection,
} from './lib/layout'

export function Profile() {
  const { t } = useTranslation()
  const { profile, loading, refreshProfile, fetchProfile } = useProfile()
  const { status } = useStatus()
  const permissions = useAuthStore((s) => s.auth.user?.permissions)

  const checkinEnabled = status?.checkin_enabled === true
  const turnstileEnabled = !!(
    status?.turnstile_check && status?.turnstile_site_key
  )
  const turnstileSiteKey = status?.turnstile_site_key || ''
  const canConfigureSidebar = permissions?.sidebar_settings !== false
  const securitySections: Record<ProfileSecuritySection, ReactNode> = {
    passkey: <PasskeyCard loading={loading} />,
    'two-factor': <TwoFACard loading={loading} />,
  }

  if (!loading && !profile) {
    return (
      <Main>
        <Empty>
          <EmptyHeader>
            <EmptyTitle>{t('Failed to load profile')}</EmptyTitle>
            <EmptyDescription>
              {t('Refresh the list and try again.')}
            </EmptyDescription>
          </EmptyHeader>
          <Button
            type='button'
            variant='outline'
            onClick={() => void fetchProfile()}
          >
            {t('Retry')}
          </Button>
        </Empty>
      </Main>
    )
  }

  return (
    <Main>
      <div className='min-h-0 flex-1 overflow-auto px-3 py-3 sm:px-4 sm:py-6'>
        <CardStaggerContainer className='mx-auto flex w-full max-w-7xl flex-col gap-4 sm:gap-6'>
          <CardStaggerItem>
            <ProfileHeader profile={profile} loading={loading} />
          </CardStaggerItem>

          <CardStaggerItem>
            <div className='grid gap-4 sm:gap-5 xl:grid-cols-[minmax(0,1fr)_minmax(360px,0.46fr)] xl:items-start'>
              <div
                data-profile-column='left'
                className='flex min-w-0 flex-col gap-4 sm:gap-6'
              >
                <ProfileSettingsCard
                  profile={profile}
                  loading={loading}
                  onProfileUpdate={refreshProfile}
                />
                <ProfileSecurityCard
                  profile={profile}
                  loading={loading}
                  onProfileUpdate={refreshProfile}
                />
                <LoginSessionsCard />
              </div>

              <div
                data-profile-column='right'
                className='flex min-w-0 flex-col gap-4 sm:gap-6'
              >
                <AccountBindingsCard
                  profile={profile}
                  loading={loading}
                  onProfileUpdate={refreshProfile}
                />
                <LanguagePreferencesCard
                  profile={profile}
                  onProfileUpdate={refreshProfile}
                />
                {checkinEnabled && (
                  <CheckinCalendarCard
                    checkinEnabled={checkinEnabled}
                    turnstileEnabled={turnstileEnabled}
                    turnstileSiteKey={turnstileSiteKey}
                  />
                )}
                {canConfigureSidebar && <SidebarModulesCard />}
                {profileSecuritySectionOrder.map((section) => (
                  <div key={section} data-profile-section={section}>
                    {securitySections[section]}
                  </div>
                ))}
              </div>
            </div>
          </CardStaggerItem>
        </CardStaggerContainer>
      </div>
    </Main>
  )
}
