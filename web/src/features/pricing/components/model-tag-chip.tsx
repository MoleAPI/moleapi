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
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

import { getModelTagIcon, getModelTagLabelKey } from '../lib/model-tags'

export function ModelTagIcon(props: { tag: string; className?: string }) {
  const icon = getModelTagIcon(props.tag)
  if (!icon) return null

  return (
    <HugeiconsIcon
      icon={icon}
      size={13}
      strokeWidth={2}
      aria-hidden='true'
      className={cn('shrink-0', props.className)}
    />
  )
}

function tagChipClassName(tag: string, index: number): string {
  const labelKey = getModelTagLabelKey(tag)
  if (index === 0) {
    return 'border-sky-300/60 bg-sky-50 text-sky-600 dark:border-sky-400/30 dark:bg-sky-400/10 dark:text-sky-300'
  }
  if (labelKey === 'Prompt caching') {
    return 'border-emerald-300/70 bg-emerald-50 text-emerald-600 dark:border-emerald-400/30 dark:bg-emerald-400/10 dark:text-emerald-300'
  }
  return 'border-border bg-muted/35 text-muted-foreground'
}

export function ModelTagChip(props: {
  tag: string
  index: number
  className?: string
}) {
  const { t } = useTranslation()
  const label = t(getModelTagLabelKey(props.tag))

  return (
    <span
      title={label}
      className={cn(
        'inline-flex h-[22px] max-w-36 min-w-0 items-center gap-1 overflow-hidden rounded-full border px-2 text-[13px] leading-none font-medium whitespace-nowrap',
        tagChipClassName(props.tag, props.index),
        props.className
      )}
    >
      <ModelTagIcon tag={props.tag} />
      <span className='min-w-0 truncate'>{label}</span>
    </span>
  )
}
