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
import { CoinsDollarIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

import { isTokenBasedModel } from '../lib/model-helpers'
import type { PricingModel } from '../types'

interface ModelBillingModeBadgeProps {
  model: PricingModel
  className?: string
}

export function ModelBillingModeBadge(props: ModelBillingModeBadgeProps) {
  const { t } = useTranslation()
  const isTokenBased = isTokenBasedModel(props.model)
  const label = isTokenBased ? t('Pay as you go') : t('Pay per call')

  return (
    <span
      className={cn(
        'inline-flex h-[22px] max-w-full min-w-0 items-center gap-1 rounded-full border border-orange-300/60 bg-orange-50 px-2 text-[13px] leading-none font-medium whitespace-nowrap text-orange-600 dark:border-orange-400/30 dark:bg-orange-400/10 dark:text-orange-300',
        props.className
      )}
    >
      <HugeiconsIcon
        icon={CoinsDollarIcon}
        size={13}
        strokeWidth={2}
        aria-hidden='true'
      />
      <span className='min-w-0 truncate'>{label}</span>
    </span>
  )
}
