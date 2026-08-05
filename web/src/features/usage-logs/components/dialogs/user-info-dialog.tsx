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
import { useQuery } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { StatusBadge } from '@/components/status-badge'
import { Label } from '@/components/ui/label'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Separator } from '@/components/ui/separator'
import {
  formatCompactNumber,
  formatQuota,
  formatTimestampToDate,
} from '@/lib/format'

import { getUserInfo, getUserOAuthBindings } from '../../api'
import type { UserInfo, UserOAuthBinding } from '../../types'

interface UserInfoDialogProps {
  userId: number | null
  open: boolean
  onOpenChange: (open: boolean) => void
}

const BUILTIN_BINDINGS: ReadonlyArray<{
  field: keyof UserInfo
  label: string
}> = [
  { field: 'email', label: 'Email' },
  { field: 'github_id', label: 'GitHub' },
  { field: 'discord_id', label: 'Discord' },
  { field: 'oidc_id', label: 'OIDC' },
  { field: 'wechat_id', label: 'WeChat' },
  { field: 'telegram_id', label: 'Telegram' },
  { field: 'linux_do_id', label: 'LinuxDO' },
  { field: 'stripe_customer', label: 'Stripe Customer' },
]

function InfoItem(props: { label: string; value: string | number }) {
  return (
    <div className='min-w-0 space-y-1'>
      <Label className='text-muted-foreground text-xs'>{props.label}</Label>
      <div
        className='truncate text-sm font-semibold'
        title={String(props.value)}
      >
        {props.value}
      </div>
    </div>
  )
}

function getRoleLabel(role: number | undefined) {
  if (role === 100) return 'Root'
  if (role === 10) return 'Admin'
  return 'User'
}

function getStatusLabel(status: number | undefined) {
  return status === 2 ? 'Disabled' : 'Enabled'
}

export function UserInfoDialog(props: UserInfoDialogProps) {
  const { t } = useTranslation()
  const query = useQuery({
    queryKey: ['usage-log-user-info', props.userId],
    enabled: props.open && props.userId != null,
    queryFn: async () => {
      const id = props.userId as number
      const [userResponse, bindingsResponse] = await Promise.all([
        getUserInfo(id),
        getUserOAuthBindings(id).catch(() => ({
          success: false,
          data: [] as UserOAuthBinding[],
        })),
      ])
      if (!userResponse.success || !userResponse.data) {
        throw new Error(
          userResponse.message || 'Failed to fetch user information'
        )
      }
      return {
        user: userResponse.data,
        customBindings: bindingsResponse.success
          ? (bindingsResponse.data ?? [])
          : [],
      }
    },
    staleTime: 30_000,
  })

  const user = query.data?.user
  const bindings = user
    ? [
        ...BUILTIN_BINDINGS.flatMap((binding) => {
          const value = user[binding.field]
          return typeof value === 'string' && value
            ? [{ label: binding.label, value }]
            : []
        }),
        ...(query.data?.customBindings ?? []).map((binding) => ({
          label: binding.provider_name || String(binding.provider_id),
          value: binding.provider_user_id || binding.external_id || '-',
        })),
      ]
    : []

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('User Information')}
      description={t(
        'View account, activity, binding, balance, and invitation details for this user.'
      )}
      contentClassName='sm:max-w-2xl'
      contentHeight='min(75vh, 680px)'
      bodyClassName='min-h-0'
    >
      {query.isLoading && (
        <div className='flex items-center justify-center py-12'>
          <Loader2 className='text-muted-foreground size-6 animate-spin' />
        </div>
      )}
      {!query.isLoading && (query.isError || !user) && (
        <div className='text-muted-foreground py-10 text-center text-sm'>
          {t('Failed to fetch user information')}
        </div>
      )}
      {!query.isLoading && !query.isError && user && (
        <ScrollArea className='h-full pr-3'>
          <div className='space-y-4 pb-1'>
            <section className='space-y-3'>
              <h3 className='text-sm font-semibold'>{t('Account')}</h3>
              <div className='grid grid-cols-2 gap-x-4 gap-y-3 sm:grid-cols-3'>
                <InfoItem label='ID' value={user.id} />
                <InfoItem label={t('Username')} value={user.username} />
                {user.display_name && (
                  <InfoItem
                    label={t('Display Name')}
                    value={user.display_name}
                  />
                )}
                <InfoItem
                  label={t('Role')}
                  value={t(getRoleLabel(user.role))}
                />
                <InfoItem
                  label={t('Status')}
                  value={t(getStatusLabel(user.status))}
                />
                {user.group && (
                  <InfoItem label={t('User Group')} value={user.group} />
                )}
              </div>
            </section>

            <Separator />

            <section className='space-y-3'>
              <h3 className='text-sm font-semibold'>
                {t('Usage and Activity')}
              </h3>
              <div className='grid grid-cols-2 gap-x-4 gap-y-3 sm:grid-cols-3'>
                <InfoItem
                  label={t('Balance')}
                  value={formatQuota(user.quota)}
                />
                <InfoItem
                  label={t('Used Quota')}
                  value={formatQuota(user.used_quota)}
                />
                <InfoItem
                  label={t('Request Count')}
                  value={formatCompactNumber(user.request_count)}
                />
                {user.created_at ? (
                  <InfoItem
                    label={t('Created At')}
                    value={formatTimestampToDate(user.created_at, 'seconds')}
                  />
                ) : null}
                {user.last_login_at ? (
                  <InfoItem
                    label={t('Last Login')}
                    value={formatTimestampToDate(user.last_login_at, 'seconds')}
                  />
                ) : null}
              </div>
            </section>

            <Separator />

            <section className='space-y-3'>
              <h3 className='text-sm font-semibold'>{t('Account Bindings')}</h3>
              {bindings.length > 0 ? (
                <div className='flex flex-wrap gap-2'>
                  {bindings.map((binding) => (
                    <StatusBadge
                      key={`${binding.label}:${binding.value}`}
                      label={`${t(binding.label)}: ${binding.value}`}
                      copyText={binding.value}
                      variant='neutral'
                      className='max-w-full'
                    />
                  ))}
                </div>
              ) : (
                <p className='text-muted-foreground text-sm'>
                  {t('This user has no bindings')}
                </p>
              )}
            </section>

            {(user.aff_code || user.aff_count != null || user.inviter_id) && (
              <>
                <Separator />
                <section className='space-y-3'>
                  <h3 className='text-sm font-semibold'>
                    {t('Invitation Details')}
                  </h3>
                  <div className='grid grid-cols-2 gap-x-4 gap-y-3 sm:grid-cols-3'>
                    {user.aff_code && (
                      <InfoItem
                        label={t('Invitation Code')}
                        value={user.aff_code}
                      />
                    )}
                    {user.aff_count != null && (
                      <InfoItem
                        label={t('Invited Users')}
                        value={formatCompactNumber(user.aff_count)}
                      />
                    )}
                    {user.aff_quota != null && (
                      <InfoItem
                        label={t('Invitation Quota')}
                        value={formatQuota(user.aff_quota)}
                      />
                    )}
                    {user.inviter_id ? (
                      <InfoItem
                        label={t('Inviter ID')}
                        value={user.inviter_id}
                      />
                    ) : null}
                  </div>
                </section>
              </>
            )}

            {user.remark && (
              <>
                <Separator />
                <section className='space-y-1'>
                  <h3 className='text-sm font-semibold'>{t('Remark')}</h3>
                  <p className='text-muted-foreground text-sm leading-relaxed break-words'>
                    {user.remark}
                  </p>
                </section>
              </>
            )}
          </div>
        </ScrollArea>
      )}
    </Dialog>
  )
}
