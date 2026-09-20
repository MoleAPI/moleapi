import { Shield, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { IconBadge } from '@/components/ui/icon-badge'
import { Skeleton } from '@/components/ui/skeleton'
import { TitledCard } from '@/components/ui/titled-card'
import { useDialogs } from '@/hooks/use-dialog'

import type { UserProfile } from '../types'
import { ChangePasswordDialog } from './dialogs/change-password-dialog'
import { DeleteAccountDialog } from './dialogs/delete-account-dialog'

interface ProfileSecurityCardProps {
  profile: UserProfile | null
  loading: boolean
  onProfileUpdate: () => void
}

export function ProfileSecurityCard(props: ProfileSecurityCardProps) {
  const { t } = useTranslation()
  const dialogs = useDialogs<'password' | 'delete'>()

  if (props.loading && !props.profile) {
    return (
      <Card data-card-hover='false' className='gap-0 overflow-hidden py-0'>
        <CardHeader className='border-b p-3 !pb-3 sm:p-5 sm:!pb-5'>
          <Skeleton className='h-6 w-32' />
          <Skeleton className='mt-2 h-4 w-48' />
        </CardHeader>
        <CardContent className='space-y-3 p-3 sm:p-5'>
          {['password', 'token', 'delete'].map((key) => (
            <Skeleton key={key} className='h-16 w-full' />
          ))}
        </CardContent>
      </Card>
    )
  }

  if (!props.profile) return null

  const securityActions = [
    {
      icon: Shield,
      title: t(
        props.profile.has_password === false
          ? 'Set Password'
          : 'Change Password'
      ),
      description: t(
        props.profile.has_password === false
          ? 'Add a password after verifying your identity'
          : 'Update your password to keep your account secure'
      ),
      action: () => dialogs.open('password'),
      variant: 'default' as const,
    },
    {
      icon: Trash2,
      title: t('Delete Account'),
      description: t('Permanently delete your account and all data'),
      action: () => dialogs.open('delete'),
      variant: 'destructive' as const,
    },
  ]

  return (
    <>
      <TitledCard
        title={t('Security')}
        description={t('Manage your security settings and account access')}
        icon={<Shield className='h-4 w-4' />}
        iconTone='success'
        disableHoverEffect
      >
        <div className='grid grid-cols-1 gap-2.5 sm:gap-3 md:grid-cols-2'>
          {securityActions.map((item) => (
            <button
              key={item.title}
              type='button'
              aria-label={item.title}
              onClick={item.action}
              className={`flex items-center gap-3 rounded-lg border p-3 text-left md:flex-col md:gap-2 md:p-4 md:text-center ${
                item.variant === 'destructive'
                  ? 'border-destructive/30 text-destructive'
                  : ''
              }`}
            >
              <IconBadge tone='neutral' size='sm'>
                <item.icon />
              </IconBadge>
              <div className='min-w-0 md:contents'>
                <p className='text-sm font-medium'>{item.title}</p>
                <p className='text-muted-foreground line-clamp-1 text-xs md:line-clamp-none'>
                  {item.description}
                </p>
              </div>
            </button>
          ))}
        </div>
      </TitledCard>

      <ChangePasswordDialog
        open={dialogs.isOpen('password')}
        onOpenChange={(open) =>
          open ? dialogs.open('password') : dialogs.close('password')
        }
        username={props.profile.username}
        hasPassword={props.profile.has_password}
        onSuccess={props.onProfileUpdate}
      />

      <DeleteAccountDialog
        open={dialogs.isOpen('delete')}
        onOpenChange={(open) =>
          open ? dialogs.open('delete') : dialogs.close('delete')
        }
        username={props.profile.username}
      />
    </>
  )
}
