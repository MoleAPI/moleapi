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
import { ChevronRight, Copy } from 'lucide-react'
import { memo } from 'react'
import { useTranslation } from 'react-i18next'

import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { getLobeIcon } from '@/lib/lobe-icon'
import { cn } from '@/lib/utils'

import { DEFAULT_TOKEN_UNIT, FILTER_ALL } from '../constants'
import {
  getDynamicDisplayGroupRatio,
  getDynamicPricingSummary,
} from '../lib/dynamic-price'
import { parseTags } from '../lib/filters'
import {
  getConfiguredGroupRatio,
  getDiscountPercent,
  getLocalizedModelDescription,
  isTokenBasedModel,
} from '../lib/model-helpers'
import {
  formatFixedPrice,
  formatGroupPrice,
  stripTrailingZeros,
} from '../lib/price'
import type { ModelCapability, PricingModel, TokenUnit } from '../types'
import { ModelBillingModeBadge } from './model-billing-mode-badge'
import { ModelPerfBadge, type ModelPerfBadgeData } from './model-perf-badge'

export interface ModelCardProps {
  model: PricingModel
  onClick: () => void
  priceRate?: number
  usdExchangeRate?: number
  tokenUnit?: TokenUnit
  showRechargePrice?: boolean
  selectedGroup?: string
  perf?: ModelPerfBadgeData
}

type Metric = {
  id: string
  label: string
  value: string
  unit?: string
  tone?: 'default' | 'success' | 'accent'
}

const CAPABILITY_LABEL_KEYS: Record<ModelCapability, string> = {
  function_calling: 'Function calling',
  streaming: 'Streaming',
  vision: 'Vision',
  json_mode: 'JSON mode',
  structured_output: 'Structured output',
  reasoning: 'Reasoning',
  tools: 'Tools',
  system_prompt: 'System prompt',
  web_search: 'Web search',
  code_interpreter: 'Code interpreter',
  caching: 'Prompt caching',
  embeddings: 'Embeddings',
}

function formatRatio(ratio: number): string {
  const value = Number.isInteger(ratio)
    ? ratio.toString()
    : ratio.toFixed(3).replace(/0+$/, '').replace(/\.$/, '')
  return `x${value}`
}

function PriceLine(props: Metric) {
  return (
    <div className='grid min-w-0 grid-cols-[3rem_minmax(0,1fr)] items-baseline gap-2'>
      <div className='text-muted-foreground text-[11px] leading-4'>
        {props.label}
      </div>
      <div
        className={cn(
          'text-foreground flex min-w-0 items-baseline gap-1 font-mono text-[12px] leading-4 font-semibold tabular-nums',
          props.tone === 'success' && 'text-emerald-600 dark:text-emerald-300',
          props.tone === 'accent' && 'text-primary'
        )}
      >
        <span className='min-w-0 truncate'>{props.value}</span>
        {props.unit && (
          <span className='text-muted-foreground shrink-0 text-[10px]'>
            {props.unit}
          </span>
        )}
      </div>
    </div>
  )
}

function PriceGroupColumn(props: {
  group: string
  ratio: number
  isCurrent: boolean
  lines: Metric[]
  t: (key: string) => string
}) {
  const discount = getDiscountPercent(props.ratio)

  return (
    <div className='min-w-0 px-3 py-2 sm:grid sm:grid-cols-[9rem_minmax(0,1fr)] sm:gap-3'>
      <div className='mb-2 flex min-w-0 items-center gap-1.5 sm:mb-0'>
        <span className='text-foreground min-w-0 truncate text-xs font-semibold'>
          {props.group}
        </span>
        <span
          className={cn(
            'rounded-md border px-1.5 py-0.5 text-[10px] leading-3',
            discount
              ? 'border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-300'
              : 'text-muted-foreground bg-muted/60'
          )}
        >
          {formatRatio(props.ratio)}
        </span>
        {props.isCurrent && (
          <span className='bg-primary/10 text-primary rounded-md px-1.5 py-0.5 text-[10px] leading-3'>
            {props.t('Current')}
          </span>
        )}
      </div>
      <div className='space-y-1 sm:grid sm:grid-cols-2 sm:gap-x-4 sm:gap-y-1 sm:space-y-0 lg:grid-cols-5'>
        {props.lines.map((line) => (
          <PriceLine key={`${props.group}-${line.id}`} {...line} />
        ))}
      </div>
    </div>
  )
}

function getDynamicMetricTone(field: string): Metric['tone'] {
  if (field.includes('cache')) return 'success'
  if (field.includes('image') || field.includes('audio')) return 'accent'
  return undefined
}

export const ModelCard = memo(function ModelCard(props: ModelCardProps) {
  const { t, i18n } = useTranslation()
  const { copyToClipboard } = useCopyToClipboard()
  const tokenUnit = props.tokenUnit ?? DEFAULT_TOKEN_UNIT
  const priceRate = props.priceRate ?? 1
  const usdExchangeRate = props.usdExchangeRate ?? 1
  const showRechargePrice = props.showRechargePrice ?? false
  const isTokenBased = isTokenBasedModel(props.model)
  const tokenUnitLabel = tokenUnit === 'K' ? '1K' : '1M'
  const tokenPriceUnit = `/ ${tokenUnitLabel}`
  const groups = props.model.enable_groups || []
  const capabilities = props.model.capabilities || []
  const tags = parseTags(props.model.tags)
  const modelIconKey = props.model.icon || props.model.vendor_icon
  const modelIcon = modelIconKey ? getLobeIcon(modelIconKey, 28) : null
  const initial = props.model.model_name?.charAt(0).toUpperCase() || '?'
  const isDynamicPricing =
    props.model.billing_mode === 'tiered_expr' &&
    Boolean(props.model.billing_expr)
  const hasCachedPrice = isTokenBased && props.model.cache_ratio != null
  const dynamicSummary = isDynamicPricing
    ? getDynamicPricingSummary(props.model, {
        tokenUnit,
        showRechargePrice,
        priceRate,
        usdExchangeRate,
        groupRatioMultiplier: getDynamicDisplayGroupRatio(
          props.model,
          props.selectedGroup
        ),
      })
    : null
  const description = getLocalizedModelDescription(props.model, i18n.language)

  const capabilityTags = capabilities.map(
    (capability) => CAPABILITY_LABEL_KEYS[capability] ?? capability
  )
  const allChips = [...new Set([...tags, ...capabilityTags])]
  const chips = allChips.slice(0, 5)
  const hiddenChipCount = Math.max(allChips.length - chips.length, 0)
  const groupRatio = props.model.group_ratio || {}
  const selectedGroup =
    props.selectedGroup &&
    props.selectedGroup !== FILTER_ALL &&
    groups.includes(props.selectedGroup)
      ? props.selectedGroup
      : ''
  const sortedGroups = (groups.length > 0 ? groups : ['default'])
    .map((group, index) => ({
      group,
      index,
      ratio: getConfiguredGroupRatio(groupRatio, group),
      isCurrent: group === selectedGroup,
    }))
    .sort((a, b) => {
      if (a.isCurrent !== b.isCurrent) return a.isCurrent ? -1 : 1
      return a.ratio - b.ratio || a.index - b.index
    })
  const visibleGroups = sortedGroups.slice(0, 3)
  const hiddenGroupCount = Math.max(sortedGroups.length - visibleGroups.length, 0)

  const handleCopy = (e: React.MouseEvent) => {
    e.stopPropagation()
    copyToClipboard(props.model.model_name || '')
  }

  const handleDetailsClick = (e: React.MouseEvent) => {
    e.stopPropagation()
    props.onClick()
  }

  let specialPriceSummary: string | null = null
  if (dynamicSummary) {
    if (dynamicSummary.isSpecialExpression) {
      specialPriceSummary = dynamicSummary.rawExpression
    }
  }
  const priceGroups = visibleGroups.map((item) => {
    let lines: Metric[] = []

    if (isDynamicPricing) {
      const summary = getDynamicPricingSummary(props.model, {
        tokenUnit,
        showRechargePrice,
        priceRate,
        usdExchangeRate,
        groupRatioMultiplier: item.ratio,
      })
      lines =
        summary?.entries.slice(0, 4).map((entry) => ({
          id: entry.key,
          label: t(entry.shortLabel),
          value: stripTrailingZeros(entry.formatted),
          unit: tokenPriceUnit,
          tone: getDynamicMetricTone(entry.field),
        })) || []
    } else if (isTokenBased) {
      lines = [
        {
          id: 'input',
          label: t('Input'),
          value: stripTrailingZeros(
            formatGroupPrice(
              props.model,
              item.group,
              'input',
              tokenUnit,
              showRechargePrice,
              priceRate,
              usdExchangeRate,
              groupRatio
            )
          ),
          unit: tokenPriceUnit,
        },
        {
          id: 'output',
          label: t('Output'),
          value: stripTrailingZeros(
            formatGroupPrice(
              props.model,
              item.group,
              'output',
              tokenUnit,
              showRechargePrice,
              priceRate,
              usdExchangeRate,
              groupRatio
            )
          ),
          unit: tokenPriceUnit,
        },
      ]
      if (hasCachedPrice) {
        lines.push({
          id: 'cache',
          label: t('Cached'),
          value: stripTrailingZeros(
            formatGroupPrice(
              props.model,
              item.group,
              'cache',
              tokenUnit,
              showRechargePrice,
              priceRate,
              usdExchangeRate,
              groupRatio
            )
          ),
          unit: tokenPriceUnit,
          tone: 'success',
        })
      }
      if (props.model.image_ratio != null) {
        lines.push({
          id: 'image',
          label: t('Image'),
          value: stripTrailingZeros(
            formatGroupPrice(
              props.model,
              item.group,
              'image',
              tokenUnit,
              showRechargePrice,
              priceRate,
              usdExchangeRate,
              groupRatio
            )
          ),
          unit: tokenPriceUnit,
          tone: 'accent',
        })
      }
      if (props.model.audio_ratio != null) {
        lines.push({
          id: 'audio-input',
          label: t('Audio In'),
          value: stripTrailingZeros(
            formatGroupPrice(
              props.model,
              item.group,
              'audio_input',
              tokenUnit,
              showRechargePrice,
              priceRate,
              usdExchangeRate,
              groupRatio
            )
          ),
          unit: tokenPriceUnit,
          tone: 'accent',
        })
      }
      if (
        props.model.audio_ratio != null &&
        props.model.audio_completion_ratio != null
      ) {
        lines.push({
          id: 'audio-output',
          label: t('Audio Out'),
          value: stripTrailingZeros(
            formatGroupPrice(
              props.model,
              item.group,
              'audio_output',
              tokenUnit,
              showRechargePrice,
              priceRate,
              usdExchangeRate,
              groupRatio
            )
          ),
          unit: tokenPriceUnit,
          tone: 'accent',
        })
      }
    } else {
      lines = [
        {
          id: 'request',
          label: t('Per Request'),
          value: stripTrailingZeros(
            formatFixedPrice(
              props.model,
              item.group,
              showRechargePrice,
              priceRate,
              usdExchangeRate,
              groupRatio
            )
          ),
          unit: `/ ${t('request')}`,
        },
      ]
    }

    return { ...item, lines }
  })

  return (
    <div
      className={cn(
        'group relative w-full max-w-full overflow-hidden rounded-lg border bg-background p-3 transition-colors sm:p-4',
        'hover:border-primary/25 hover:bg-muted/20'
      )}
    >
      <button
        type='button'
        aria-label={`${t('Details')}: ${props.model.model_name}`}
        onClick={props.onClick}
        className='focus-visible:ring-ring absolute inset-0 z-10 cursor-pointer rounded-lg focus-visible:ring-2 focus-visible:outline-none'
      />

      <div className='flex min-w-0 items-start gap-3'>
        <div className='bg-muted/50 flex size-10 shrink-0 items-center justify-center rounded-lg sm:size-11'>
          <span className='[&_svg]:size-6 sm:[&_svg]:size-7'>
            {modelIcon || (
              <span className='text-muted-foreground text-sm font-bold'>
                {initial}
              </span>
            )}
          </span>
        </div>

        <div className='min-w-0 flex-1'>
          <div className='flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1'>
            <h3 className='text-foreground min-w-0 truncate text-sm leading-tight font-semibold sm:text-[15px]'>
              {props.model.model_name}
            </h3>
            <button
              type='button'
              onClick={handleCopy}
              className='text-muted-foreground hover:text-foreground hover:bg-muted relative z-20 rounded-md border p-1 transition-colors'
              title={t('Copy')}
              aria-label={t('Copy')}
            >
              <Copy className='size-3' />
            </button>
          </div>
          <p className='text-muted-foreground mt-1 line-clamp-1 text-[12px] leading-relaxed'>
            {description || t('No description available.')}
          </p>
        </div>

        <div className='relative z-20 hidden shrink-0 items-center gap-1 sm:flex'>
          <ModelPerfBadge perf={props.perf} className='mr-1' />
          <button
            type='button'
            onClick={handleDetailsClick}
            className='text-muted-foreground hover:text-foreground hover:bg-muted hidden items-center gap-1 rounded-md border px-2 py-1 text-xs transition-colors sm:inline-flex'
          >
            {t('Details')}
            <ChevronRight className='size-3.5' />
          </button>
        </div>
      </div>

      <div className='mt-3 overflow-hidden rounded-lg border bg-muted/10'>
        {specialPriceSummary ? (
          <div className='min-w-0 px-3 py-2'>
            <span className='text-amber-700 dark:text-amber-300'>
              {t('Special billing expression')}
            </span>
            <code className='text-muted-foreground/70 mt-0.5 line-clamp-1 block font-mono text-[11px] break-all'>
              {specialPriceSummary}
            </code>
          </div>
        ) : (
          <div className='divide-y'>
            {priceGroups.map((item) => (
              <PriceGroupColumn
                key={item.group}
                group={item.group}
                ratio={item.ratio}
                isCurrent={item.isCurrent}
                lines={item.lines}
                t={t}
              />
            ))}
            {hiddenGroupCount > 0 && (
              <div className='text-muted-foreground flex items-center px-3 py-2 text-xs'>
                +{hiddenGroupCount}
              </div>
            )}
          </div>
        )}
      </div>

      <div className='mt-3 flex min-w-0 items-center justify-between gap-3'>
        <div className='flex min-w-0 flex-wrap items-center gap-1.5'>
          <ModelBillingModeBadge
            model={props.model}
            className='-ml-1.5 shrink-0'
          />
          {chips.map((chip, index) => (
            <span
              key={`tag-${chip}`}
              className={cn(
                'inline-flex max-w-full min-w-0 items-center truncate rounded-md border px-1.5 py-0.5 text-[11px] leading-4',
                index === 0
                  ? 'bg-primary/10 text-primary border-primary/20'
                  : 'bg-muted/60 text-muted-foreground'
              )}
            >
              {chip}
            </span>
          ))}
          {hiddenChipCount > 0 && (
            <span className='text-muted-foreground/40 text-xs'>
              +{hiddenChipCount}
            </span>
          )}
        </div>
      </div>
    </div>
  )
})
