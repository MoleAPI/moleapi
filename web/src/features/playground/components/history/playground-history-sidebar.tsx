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
import { Delete02Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { MessageSquareTextIcon, PlusIcon } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Button } from '@/components/ui/button'
import { Progress, ProgressLabel } from '@/components/ui/progress'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'

import type { ConversationStorageUsage } from '../../lib'
import type { PlaygroundConversationSession } from '../../types'

type PlaygroundHistorySidebarProps = {
  activeSessionId: string
  disabled?: boolean
  onDeleteConversation: (sessionId: string) => void
  onNewConversation: () => void
  onSelectConversation: (sessionId: string) => void
  sessions: PlaygroundConversationSession[]
  storageUsage: ConversationStorageUsage
}

function formatUpdatedAt(updatedAt: number): string {
  return new Intl.DateTimeFormat(undefined, {
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    month: 'short',
  }).format(updatedAt)
}

export function PlaygroundHistorySidebar(props: PlaygroundHistorySidebarProps) {
  const { t, i18n } = useTranslation()
  const [pendingDeleteSessionId, setPendingDeleteSessionId] = useState<
    string | null
  >(null)
  const sessions = [...props.sessions].sort((a, b) => b.updatedAt - a.updatedAt)
  const pendingDeleteSession = props.sessions.find(
    (session) => session.id === pendingDeleteSessionId
  )
  const usedPercent = Math.min(
    (props.storageUsage.usedBytes / props.storageUsage.capacityBytes) * 100,
    100
  )
  const isStorageLow =
    props.storageUsage.remainingBytes <= props.storageUsage.capacityBytes * 0.2
  const remainingStorage = `${formatNumber(
    props.storageUsage.remainingBytes / (1024 * 1024),
    toIntlLocale(i18n.resolvedLanguage || i18n.language)
  )} MB`

  const confirmDelete = () => {
    if (!pendingDeleteSessionId) return

    props.onDeleteConversation(pendingDeleteSessionId)
    setPendingDeleteSessionId(null)
  }

  return (
    <aside className='bg-background/70 hidden w-64 shrink-0 border-l lg:flex lg:flex-col'>
      <div className='border-b p-3'>
        <div className='mb-2 flex items-center justify-between gap-2'>
          <div className='min-w-0'>
            <h2 className='truncate text-sm font-semibold'>
              {t('Playground history')}
            </h2>
            <div className='text-muted-foreground space-y-0.5 text-xs'>
              <p className='truncate'>{t('Only available in this browser')}</p>
            </div>
          </div>
          <Button
            aria-label={t('New chat')}
            disabled={props.disabled}
            onClick={props.onNewConversation}
            size='icon'
            variant='outline'
          >
            <PlusIcon className='size-4' />
          </Button>
        </div>
        <div
          className={cn(
            'space-y-1.5',
            isStorageLow &&
              'rounded-md bg-red-950/10 p-2 text-red-900 dark:bg-red-950/40 dark:text-red-200'
          )}
        >
          <Progress
            className={cn(
              'gap-1.5 [&_[data-slot=progress-track]]:h-1.5',
              isStorageLow &&
                '[&_[data-slot=progress-indicator]]:bg-red-900 dark:[&_[data-slot=progress-indicator]]:bg-red-700'
            )}
            value={usedPercent}
          >
            <ProgressLabel className='text-[11px]'>
              {t('Local storage remaining: {{size}}', {
                size: remainingStorage,
              })}
            </ProgressLabel>
          </Progress>
          {isStorageLow && (
            <p className='text-[11px] leading-snug'>
              {t(
                'Storage is running low. Oldest records will be deleted automatically.'
              )}
            </p>
          )}
        </div>
      </div>

      <div className='min-h-0 flex-1 overflow-y-auto p-2'>
        {sessions.map((session) => {
          const isActive = session.id === props.activeSessionId
          const title = session.title || t('New chat')

          return (
            <div className='group relative mb-1' key={session.id}>
              <button
                aria-current={isActive ? 'page' : undefined}
                className={cn(
                  'hover:bg-muted/70 focus-visible:ring-ring flex w-full min-w-0 items-start gap-2 rounded-md px-2 py-2 pr-9 text-left text-sm outline-none focus-visible:ring-2',
                  isActive && 'bg-muted text-foreground',
                  props.disabled && 'cursor-not-allowed opacity-60'
                )}
                disabled={props.disabled}
                onClick={() => props.onSelectConversation(session.id)}
                type='button'
              >
                <MessageSquareTextIcon className='text-muted-foreground mt-0.5 size-4 shrink-0' />
                <span className='min-w-0 flex-1'>
                  <span className='block truncate font-medium'>{title}</span>
                  <span className='text-muted-foreground block truncate text-xs'>
                    {formatUpdatedAt(session.updatedAt)}
                  </span>
                </span>
              </button>
              <Button
                aria-label={`${t('Delete')}: ${title}`}
                className='absolute top-1/2 right-1 -translate-y-1/2 opacity-0 transition-opacity group-focus-within:opacity-100 group-hover:opacity-100'
                disabled={props.disabled}
                onClick={() => setPendingDeleteSessionId(session.id)}
                size='icon-sm'
                variant='destructive'
              >
                <HugeiconsIcon icon={Delete02Icon} />
              </Button>
            </div>
          )
        })}
      </div>

      <ConfirmDialog
        desc={t(
          'Delete "{{title}}" from local history? This action cannot be undone.',
          { title: pendingDeleteSession?.title || t('New chat') }
        )}
        destructive
        handleConfirm={confirmDelete}
        onOpenChange={(open) => {
          if (!open) setPendingDeleteSessionId(null)
        }}
        open={Boolean(pendingDeleteSession)}
        confirmText={t('Delete')}
        title={t('Delete conversation?')}
      />
    </aside>
  )
}
