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
import { memo, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { getLobeIcon } from '@/lib/lobe-icon'
import { cn } from '@/lib/utils'

import { DEFAULT_TOKEN_UNIT } from '../constants'
import {
  getCardExamplePrice,
  getDynamicPriceUnitLabelKey,
  getDynamicPricingSummary,
  isUnconfiguredTaskUsageModel,
} from '../lib/dynamic-price'
import { parseTags } from '../lib/filters'
import {
  getDisplayedPriceGroups,
  getLocalizedModelDescription,
  isTokenBasedModel,
} from '../lib/model-helpers'
import {
  formatFixedPrice,
  formatGroupPrice,
  stripTrailingZeros,
} from '../lib/price'
import { getTaskNumberFields } from '../lib/task-expr'
import type { PricingModel, TokenUnit } from '../types'
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

export const ModelCard = memo(function ModelCard(props: ModelCardProps) {
  const { t, i18n } = useTranslation()
  const { copyToClipboard } = useCopyToClipboard()
  const tokenUnit = props.tokenUnit ?? DEFAULT_TOKEN_UNIT
  const priceRate = props.priceRate ?? 1
  const usdExchangeRate = props.usdExchangeRate ?? 1
  const showRechargePrice = props.showRechargePrice ?? false
  const isTokenBased = isTokenBasedModel(props.model)
  const tokenUnitLabel = tokenUnit === 'K' ? '1K' : '1M'
  const tags = parseTags(props.model.tags)
  const endpoints = props.model.supported_endpoint_types || []
  const modelIconKey = props.model.icon || props.model.vendor_icon
  const modelIcon = modelIconKey ? getLobeIcon(modelIconKey, 28) : null
  const initial = props.model.model_name?.charAt(0).toUpperCase() || '?'
  const isDynamicPricing =
    props.model.billing_mode === 'tiered_expr' &&
    Boolean(props.model.billing_expr)
  const isUnconfiguredTaskUsage = isUnconfiguredTaskUsageModel(props.model)
  const hasCachedPrice = isTokenBased && props.model.cache_ratio != null
  const showTaskFieldLabels =
    getTaskNumberFields(props.model.billing_usage_schema).length > 1
  const groupRatio = props.model.group_ratio || {}
  const visibleGroups = getDisplayedPriceGroups(
    props.model,
    props.selectedGroup
  )
  const description = getLocalizedModelDescription(props.model, i18n.language)
  const bottomTags = [...endpoints.slice(0, 2), ...tags.slice(0, 2)]
  const hiddenCount =
    Math.max(endpoints.length - 2, 0) + Math.max(tags.length - 2, 0)

  const handleCopy = (e: React.MouseEvent) => {
    e.stopPropagation()
    copyToClipboard(props.model.model_name || '')
  }

  return (
    <div
      className={cn(
        'group relative flex flex-col rounded-xl border p-3 transition-colors sm:p-5',
        'hover:bg-muted/20'
      )}
    >
      {/* Header: icon + name + price + actions */}
      <div className='flex items-start justify-between gap-2.5 sm:gap-3'>
        <div className='flex min-w-0 items-start gap-2.5 sm:gap-3'>
          <div className='bg-muted/40 flex size-9 shrink-0 items-center justify-center rounded-lg sm:size-10 sm:rounded-xl'>
            {modelIcon || (
              <span className='text-muted-foreground text-sm font-bold'>
                {initial}
              </span>
            )}
          </div>
          <div className='min-w-0'>
            <h3 className='text-foreground truncate font-mono text-[15px] leading-tight font-bold'>
              {props.model.model_name}
            </h3>
          </div>
        </div>

        <div className='flex shrink-0 items-center gap-1.5'>
          <button
            type='button'
            onClick={props.onClick}
            className='text-muted-foreground hover:text-foreground hover:bg-muted inline-flex items-center gap-1 rounded-md border px-2 py-1 text-xs transition-colors sm:px-2.5 sm:py-1.5'
          >
            {t('Details')}
            <ChevronRight className='size-3.5' />
          </button>
          <button
            type='button'
            onClick={handleCopy}
            className='text-muted-foreground hover:text-foreground hover:bg-muted rounded-md border p-1.5 transition-colors'
            title={t('Copy')}
          >
            <Copy className='size-3.5' />
          </button>
        </div>
      </div>

      {/* Description */}
      <p className='text-muted-foreground mt-2 line-clamp-1 flex-1 text-[13px] leading-relaxed sm:mt-4 sm:line-clamp-2 sm:min-h-[2.5rem]'>
        {description || t('No description available.')}
      </p>

      <div className='mt-3 divide-y rounded-lg border'>
        {visibleGroups.map((item) => {
          const dynamicOptions = {
            tokenUnit,
            showRechargePrice,
            priceRate,
            usdExchangeRate,
            groupRatioMultiplier: item.ratio,
          }
          const dynamicSummary = isDynamicPricing
            ? getDynamicPricingSummary(props.model, dynamicOptions)
            : null
          const examplePrice = isDynamicPricing
            ? getCardExamplePrice(props.model, dynamicOptions)
            : null
          let priceContent: ReactNode
          if (dynamicSummary?.isSpecialExpression) {
            priceContent = (
              <code className='text-muted-foreground line-clamp-1 break-all'>
                {dynamicSummary.rawExpression}
              </code>
            )
          } else if (dynamicSummary?.primaryEntries.length) {
            priceContent = (
              <>
                {dynamicSummary.primaryEntries.map((entry) => {
                  const unitLabelKey = getDynamicPriceUnitLabelKey(entry)
                  let entryLabel = ''
                  if (entry.labelKind !== 'schema') {
                    entryLabel = `${t(entry.shortLabel)} `
                  } else if (showTaskFieldLabels) {
                    entryLabel = `${entry.shortLabel} `
                  }
                  return (
                    <span
                      key={entry.key}
                      className='text-muted-foreground whitespace-nowrap'
                    >
                      {entryLabel}
                      <span className='text-foreground font-mono font-semibold'>
                        {entry.formattedRange ?? entry.formatted}
                        {unitLabelKey ? `/${t(unitLabelKey)}` : ''}
                      </span>
                    </span>
                  )
                })}
                {examplePrice ? (
                  <span className='text-muted-foreground/70 min-w-0 truncate'>
                    {examplePrice.label} ≈ {examplePrice.formatted}
                  </span>
                ) : null}
              </>
            )
          } else if (isUnconfiguredTaskUsage) {
            priceContent = (
              <span className='text-muted-foreground'>
                {t('Usage-based billing · price not configured')}
              </span>
            )
          } else if (isTokenBased) {
            priceContent = (
              <>
                {(['input', 'output'] as const).map((type) => (
                  <span
                    key={type}
                    className='text-muted-foreground whitespace-nowrap'
                  >
                    {t(type === 'input' ? 'Input' : 'Output')}{' '}
                    <span className='text-foreground font-mono font-semibold'>
                      {stripTrailingZeros(
                        formatGroupPrice(
                          props.model,
                          item.group,
                          type,
                          tokenUnit,
                          showRechargePrice,
                          priceRate,
                          usdExchangeRate,
                          groupRatio
                        )
                      )}
                    </span>
                    /{tokenUnitLabel}
                  </span>
                ))}
                {hasCachedPrice ? (
                  <span className='text-muted-foreground whitespace-nowrap'>
                    {t('Cached')}{' '}
                    <span className='text-foreground font-mono font-semibold'>
                      {stripTrailingZeros(
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
                      )}
                    </span>
                    /{tokenUnitLabel}
                  </span>
                ) : null}
              </>
            )
          } else {
            priceContent = (
              <span className='text-muted-foreground whitespace-nowrap'>
                <span className='text-foreground font-mono font-semibold'>
                  {stripTrailingZeros(
                    formatFixedPrice(
                      props.model,
                      item.group,
                      showRechargePrice,
                      priceRate,
                      usdExchangeRate,
                      groupRatio
                    )
                  )}
                </span>{' '}
                / {t('request')}
              </span>
            )
          }

          return (
            <div
              key={item.group}
              className='grid min-w-0 gap-1.5 px-2.5 py-2 sm:grid-cols-[7rem_minmax(0,1fr)] sm:items-baseline sm:gap-3'
            >
              <div className='flex min-w-0 items-center gap-1.5'>
                <span className='truncate text-xs font-semibold'>
                  {item.group}
                </span>
                <span className='text-muted-foreground rounded border px-1 py-0.5 font-mono text-[10px]'>
                  ×{item.ratio}
                </span>
                {item.isCurrent ? (
                  <span className='bg-primary/10 text-primary rounded px-1 py-0.5 text-[10px]'>
                    {t('Current')}
                  </span>
                ) : null}
              </div>
              <div className='flex min-w-0 flex-wrap items-baseline gap-x-3 gap-y-1 text-xs'>
                {priceContent}
              </div>
            </div>
          )
        })}
      </div>

      {/* Footer: left metadata and right performance summary share row alignment */}
      <div className='mt-2 grid grid-cols-[minmax(0,1fr)_auto] items-start gap-x-2 gap-y-1 sm:mt-4'>
        <div className='flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1'>
          <ModelBillingModeBadge model={props.model} />
        </div>
        <ModelPerfBadge perf={props.perf} className='row-span-2 self-start' />

        <div className='flex min-w-0 flex-wrap items-center gap-x-2.5 gap-y-0.5 sm:gap-x-3 sm:gap-y-1'>
          {bottomTags.map((item) => (
            <span key={item} className='text-muted-foreground/70 text-xs'>
              {item}
            </span>
          ))}
          {hiddenCount > 0 && (
            <span className='text-muted-foreground/40 text-xs'>
              +{hiddenCount}
            </span>
          )}
        </div>
      </div>
    </div>
  )
})
