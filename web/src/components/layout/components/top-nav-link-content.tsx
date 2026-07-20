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

import { cn } from '@/lib/utils'

import type { TopNavLink } from '../types'

export function TopNavLinkContent(props: {
  link: TopNavLink
  label: string
  iconClassName?: string
}) {
  return (
    <span className='inline-flex items-center gap-1.5'>
      {props.link.icon && (
        <HugeiconsIcon
          icon={props.link.icon}
          aria-hidden='true'
          className={cn('size-3.5 shrink-0', props.link.iconClassName)}
          strokeWidth={2}
        />
      )}
      <span>{props.label}</span>
    </span>
  )
}
