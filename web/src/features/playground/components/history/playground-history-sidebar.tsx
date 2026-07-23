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
import { MessageSquareTextIcon, PlusIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

import type { PlaygroundConversationSession } from '../../types'

type PlaygroundHistorySidebarProps = {
  activeSessionId: string
  disabled?: boolean
  onNewConversation: () => void
  onSelectConversation: (sessionId: string) => void
  sessions: PlaygroundConversationSession[]
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
  const { t } = useTranslation()
  const sessions = [...props.sessions].sort((a, b) => b.updatedAt - a.updatedAt)

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
              <p className='truncate'>
                {t('Local history has a storage limit')}
              </p>
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
      </div>

      <div className='min-h-0 flex-1 overflow-y-auto p-2'>
        {sessions.map((session) => {
          const isActive = session.id === props.activeSessionId
          const title = session.title || t('New chat')

          return (
            <button
              aria-current={isActive ? 'page' : undefined}
              className={cn(
                'hover:bg-muted/70 focus-visible:ring-ring mb-1 flex w-full min-w-0 items-start gap-2 rounded-md px-2 py-2 text-left text-sm outline-none focus-visible:ring-2',
                isActive && 'bg-muted text-foreground',
                props.disabled && 'cursor-not-allowed opacity-60'
              )}
              disabled={props.disabled}
              key={session.id}
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
          )
        })}
      </div>
    </aside>
  )
}
