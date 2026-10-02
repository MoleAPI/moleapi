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
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Loader2, RotateCcw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverHeader,
  PopoverTitle,
  PopoverTrigger,
} from '@/components/ui/popover'
import { formatTimestampToDate } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { createServerError } from '@/lib/server-error-message'

import { retestChannelModel } from '../api'
import { channelsQueryKeys } from '../lib'
import type { Channel } from '../types'

type AutoPausedModel = {
  model: string
  pauseReason: string
  pausedAt: number
  nextProbeAt: number
  lastProbeResult: string
}

function getAutoPausedModels(channel: Channel): AutoPausedModel[] {
  try {
    const other = JSON.parse(channel.other_info) as {
      channel_probe?: { models?: Record<string, Record<string, unknown>> }
    }
    const states = other.channel_probe?.models ?? {}
    const declaredModels = new Set(
      channel.models
        .split(',')
        .map((model) => model.trim())
        .filter(Boolean)
    )
    return Object.entries(states)
      .filter(
        ([model, state]) =>
          declaredModels.has(model) && state.auto_paused === true
      )
      .map(([model, state]) => ({
        model,
        pauseReason:
          typeof state.pause_reason === 'string' ? state.pause_reason : '',
        pausedAt: typeof state.paused_at === 'number' ? state.paused_at : 0,
        nextProbeAt:
          typeof state.next_probe_at === 'number' ? state.next_probe_at : 0,
        lastProbeResult:
          typeof state.last_probe_result === 'string'
            ? state.last_probe_result
            : '',
      }))
      .sort((a, b) => a.model.localeCompare(b.model))
  } catch {
    return []
  }
}

export function AutoPausedModels(props: { channel: Channel }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const pausedModels = getAutoPausedModels(props.channel)
  const mutation = useMutation({
    mutationFn: async (model: string) => {
      const response = await retestChannelModel(props.channel.id, model)
      if (!response.success) {
        throw createServerError(response, t('Model is still unavailable'))
      }
      return model
    },
    onSuccess: () => {
      toast.success(t('Model restored'))
      queryClient.invalidateQueries({ queryKey: channelsQueryKeys.lists() })
    },
    onError: (error) => {
      handleServerError(error, t('Failed to retest model'))
    },
  })

  if (pausedModels.length === 0) {
    return null
  }

  return (
    <Popover>
      <PopoverTrigger
        render={
          <Button
            variant='outline'
            size='xs'
            className='h-6 px-1.5 text-[11px]'
            aria-label={t('Auto-paused models')}
          />
        }
      >
        {t('Auto-paused models')}: {pausedModels.length}
      </PopoverTrigger>
      <PopoverContent align='start' className='w-96 gap-3 p-3'>
        <PopoverHeader>
          <PopoverTitle>{t('Auto-paused models')}</PopoverTitle>
        </PopoverHeader>
        <div className='max-h-80 space-y-3 overflow-y-auto'>
          {pausedModels.map((item) => {
            const pending =
              mutation.isPending && mutation.variables === item.model
            let result = t('Inconclusive')
            if (item.lastProbeResult.includes('pass')) result = t('Passed')
            if (item.lastProbeResult.includes('fail')) result = t('Failed')
            return (
              <div
                key={item.model}
                className='border-border space-y-1.5 rounded-md border p-2.5 text-xs'
              >
                <div className='font-mono font-medium wrap-anywhere'>
                  {item.model}
                </div>
                <div className='text-muted-foreground space-y-1'>
                  <div>
                    {t('Pause reason')}: {t(item.pauseReason)}
                  </div>
                  <div>
                    {t('Paused at')}:{' '}
                    {item.pausedAt ? formatTimestampToDate(item.pausedAt) : '-'}
                  </div>
                  <div>
                    {t('Next retest')}:{' '}
                    {item.nextProbeAt
                      ? formatTimestampToDate(item.nextProbeAt)
                      : '-'}
                  </div>
                  <div>
                    {t('Last retest')}: {result}
                  </div>
                </div>
                <Button
                  variant='secondary'
                  size='sm'
                  className='mt-1 w-full'
                  disabled={mutation.isPending}
                  onClick={() => mutation.mutate(item.model)}
                >
                  {pending ? (
                    <Loader2 className='size-4 animate-spin' />
                  ) : (
                    <RotateCcw className='size-4' />
                  )}
                  {t('Retest and restore now')}
                </Button>
              </div>
            )
          })}
        </div>
      </PopoverContent>
    </Popover>
  )
}
