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
import type { ColumnDef } from '@tanstack/react-table'
import { GitBranch, ListFilter, Sparkles, KeyRound } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { GroupBadge } from '@/components/group-badge'
import { StatusBadge, type StatusBadgeProps } from '@/components/status-badge'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { getUserAvatarFallback, getUserAvatarStyle } from '@/lib/avatar'
import { formatBillingCurrencyFromUSD } from '@/lib/currency'
import { formatLogQuota, formatTimestampToDate } from '@/lib/format'
import { cn } from '@/lib/utils'

import { LOG_TYPE_ALL_VALUE } from '../../constants'
import type { UsageLog } from '../../data/schema'
import {
  formatModelName,
  getTieredBillingSummary,
  hasAnyCacheTokens,
  getImageTokenBreakdown,
  parseLogOther,
  isViolationFeeLog,
  renderLogContent,
} from '../../lib/format'
import {
  isDisplayableLogType,
  isTimingLogType,
  getLogTypeConfig,
  isPerCallBilling,
} from '../../lib/utils'
import type { CommonQuickFilterHandler, LogOtherData } from '../../types'
import { LogCostDisplay } from '../log-cost-display'
import { ModelBadge } from '../model-badge'
import { TimingMetricsCell, StreamTpsCell } from '../timing-metrics-cell'
import { useUsageLogsContext } from '../usage-logs-provider'

interface DetailSegment {
  text: string
  muted?: boolean
  danger?: boolean
}

const logPillClassName = 'ring-1 ring-inset ring-current/15'

function formatRatioCompact(ratio: number | undefined): string {
  if (ratio == null || !Number.isFinite(ratio)) return '-'
  return ratio % 1 === 0
    ? ratio.toFixed(1)
    : ratio.toFixed(4).replace(/\.?0+$/, '')
}

function getEffectiveGroupRatio(other: LogOtherData | null): number | null {
  const userGroupRatio = other?.user_group_ratio
  if (
    userGroupRatio != null &&
    userGroupRatio !== -1 &&
    Number.isFinite(userGroupRatio)
  ) {
    return userGroupRatio
  }

  const groupRatio = other?.group_ratio
  if (groupRatio != null && Number.isFinite(groupRatio)) {
    return groupRatio
  }

  return null
}

function getPriceRatioSuffix(other: LogOtherData): string {
  const ratio = getEffectiveGroupRatio(other)
  return ratio == null ? '' : ` · ${formatRatioCompact(ratio)}`
}

function buildDetailSegments(
  log: UsageLog,
  other: LogOtherData | null,
  t: (key: string, opts?: Record<string, unknown>) => string,
  isAdmin: boolean
): DetailSegment[] {
  const upstreamError = isAdmin ? other?.admin_info?.upstream_error : undefined
  const segments: DetailSegment[] = upstreamError
    ? [{ text: upstreamError, danger: true }]
    : buildTypeDetailSegments(log, other, t)
  const adminSegments: DetailSegment[] = []
  // Quota saturation is a rare, admin-only anomaly marker; surface it first
  // and in danger styling so it stands out on the related billing log. The
  // backend already strips admin_info for non-admins; gate on isAdmin too as
  // defense in depth so the marker never leaks if that changes.
  if (isAdmin && other?.admin_info?.quota_saturation) {
    adminSegments.push({ text: t('Quota clamped'), danger: true })
  }
  const plugin = isAdmin ? other?.admin_info?.task_plugin : undefined
  if (plugin) {
    const version = plugin.version ? ` @ ${plugin.version}` : ''
    adminSegments.push({
      text: `${t('Plugin')}: ${plugin.name || plugin.key}${version}`,
    })
  }
  return [...adminSegments, ...segments]
}

function buildTypeDetailSegments(
  log: UsageLog,
  other: LogOtherData | null,
  t: (key: string, opts?: Record<string, unknown>) => string
): DetailSegment[] {
  const localizedContent = renderLogContent(log, other, t)
  if (localizedContent) return [{ text: localizedContent }]

  if (log.type === 6) {
    return [{ text: t('Async task refund') }]
  }

  if (log.type !== 2) return []

  const isViolation = isViolationFeeLog(other)
  if (isViolation) {
    const segments: DetailSegment[] = []
    segments.push({ text: t('Violation Fee'), danger: true })
    if (other?.violation_fee_code) {
      segments.push({
        text: other.violation_fee_code,
        muted: true,
      })
    }
    segments.push({
      text: `${t('Fee')}: ${formatLogQuota(other?.fee_quota ?? log.quota)}`,
      muted: true,
    })
    return segments
  }

  if (!other) return []

  const segments: DetailSegment[] = []

  const priceOpts = { digitsLarge: 4, digitsSmall: 6, abbreviate: false }
  const formatPrice = (price: number) =>
    `${formatBillingCurrencyFromUSD(price, priceOpts)}/M`
  const formatPriceCompact = (price: number) =>
    formatBillingCurrencyFromUSD(price, priceOpts)
  const formatPriceList = (prices: string[], showUnit: boolean) => {
    const text = prices.join(' / ')
    return showUnit ? `${text}/M` : text
  }
  const isTieredExpr = other.billing_mode === 'tiered_expr'
  const tieredSummary = getTieredBillingSummary(other)
  if (isTieredExpr) {
    if (tieredSummary) {
      const baseEntries = tieredSummary.priceEntries
        .filter((entry) => ['inputPrice', 'outputPrice'].includes(entry.field))
        .map((entry) => formatPriceCompact(entry.price))
      if (baseEntries.length > 0) {
        const tierLabel = tieredSummary.tier.label || t('Default')
        segments.push({
          text: `${tierLabel} · ${formatPriceList(baseEntries, true)}${getPriceRatioSuffix(other)}`,
        })
      }

      const cacheEntries = tieredSummary.priceEntries
        .filter((entry) =>
          ['cacheReadPrice', 'cacheCreatePrice', 'cacheCreate1hPrice'].includes(
            entry.field
          )
        )
        .map((entry) => {
          return formatPriceCompact(entry.price)
        })
      if (cacheEntries.length > 0) {
        segments.push({
          text: `${t('Cache')} ${formatPriceList(cacheEntries, false)}${getPriceRatioSuffix(other)}`,
          muted: true,
        })
      }

      const otherEntries = tieredSummary.priceEntries
        .filter(
          (entry) =>
            ![
              'inputPrice',
              'outputPrice',
              'cacheReadPrice',
              'cacheCreatePrice',
              'cacheCreate1hPrice',
            ].includes(entry.field)
        )
        .map((entry) => ({
          text: `${t(entry.shortLabel)} ${formatPrice(entry.price)}${getPriceRatioSuffix(other)}`,
          muted: true,
        }))
      for (const entry of otherEntries) {
        segments.push({
          text: entry.text,
          muted: entry.muted,
        })
      }
    } else {
      segments.push({
        text: `${t('Dynamic Pricing')} · ${t('No matching results')}`,
        muted: true,
      })
    }
  } else {
    const modelPrice = other.model_price
    const isPerCall = isPerCallBilling(modelPrice)
    if (isPerCall && modelPrice != null) {
      segments.push({
        text: `${t('Per-call')} · ${formatBillingCurrencyFromUSD(modelPrice, priceOpts)}${getPriceRatioSuffix(other)}`,
      })
    } else if (other.model_ratio != null) {
      const inputPriceUSD = other.model_ratio * 2.0
      const baseEntries = [formatPriceCompact(inputPriceUSD)]
      if (other.completion_ratio != null) {
        baseEntries.push(
          formatPriceCompact(inputPriceUSD * other.completion_ratio)
        )
      }
      segments.push({
        text: `${formatPriceList(baseEntries, true)}${getPriceRatioSuffix(other)}`,
      })

      if (hasAnyCacheTokens(other)) {
        const cacheEntries = [
          other.cache_ratio != null && other.cache_ratio !== 1
            ? formatPriceCompact(inputPriceUSD * other.cache_ratio)
            : null,
          other.cache_creation_ratio != null && other.cache_creation_ratio !== 1
            ? formatPriceCompact(inputPriceUSD * other.cache_creation_ratio)
            : null,
          other.cache_creation_ratio_1h != null &&
          other.cache_creation_ratio_1h !== 0
            ? formatPriceCompact(inputPriceUSD * other.cache_creation_ratio_1h)
            : null,
        ].filter(Boolean) as string[]

        if (cacheEntries.length > 0) {
          segments.push({
            text: `${t('Cache')} ${formatPriceList(cacheEntries, false)}${getPriceRatioSuffix(other)}`,
            muted: true,
          })
        }
      }

      const imageBreakdown = getImageTokenBreakdown(other)
      if (imageBreakdown.input > 0 || imageBreakdown.output > 0) {
        const imagePrice = formatPrice(inputPriceUSD * (other.image_ratio ?? 1))
        const imageOutputPrice = formatPrice(
          inputPriceUSD *
            (other.image_output_ratio ?? other.completion_ratio ?? 1)
        )
        if (imageBreakdown.input > 0) {
          segments.push({
            text: `${t('Image input')} ${imagePrice}${getPriceRatioSuffix(other)}`,
            muted: true,
          })
        }
        if (imageBreakdown.output > 0) {
          segments.push({
            text: `${t('Image Out')} ${imageOutputPrice}${getPriceRatioSuffix(other)}`,
            muted: true,
          })
        }
      }
    } else {
      const userGroupRatio = other.user_group_ratio
      const groupRatio = other.group_ratio
      const isUserGroup =
        userGroupRatio != null &&
        Number.isFinite(userGroupRatio) &&
        userGroupRatio !== -1
      const effectiveRatio = isUserGroup ? userGroupRatio : groupRatio
      const ratioLabel = isUserGroup
        ? t('User Exclusive Ratio')
        : t('Group Ratio')

      if (effectiveRatio != null && Number.isFinite(effectiveRatio)) {
        segments.push({
          text: `${ratioLabel} ${formatRatioCompact(effectiveRatio)}x`,
        })
      }
    }
  }

  if (other.is_system_prompt_overwritten) {
    segments.push({
      text: t('System Prompt Override'),
      danger: true,
    })
  }

  return segments
}

export function useCommonLogsColumns(
  isAdmin: boolean,
  onQuickFilter?: CommonQuickFilterHandler
): ColumnDef<UsageLog>[] {
  const { t } = useTranslation()
  const columns: ColumnDef<UsageLog>[] = [
    {
      accessorKey: 'created_at',
      header: t('Time'),
      cell: ({ row }) => {
        const timestamp = row.getValue('created_at') as number

        return (
          <span className='font-mono text-xs whitespace-nowrap tabular-nums'>
            {formatTimestampToDate(timestamp)}
          </span>
        )
      },
      enableHiding: false,
      size: 150,
    },
    {
      accessorKey: 'type',
      header: t('Type'),
      cell: ({ row }) => {
        const config = getLogTypeConfig(row.original.type)

        return (
          <StatusBadge
            label={t(config.label)}
            variant={config.color as StatusBadgeProps['variant']}
            size='sm'
            copyable={false}
            onClick={(event) => {
              event.stopPropagation()
              onQuickFilter?.('type', String(row.original.type))
            }}
            title={t('Filter by this value')}
            className={cn(
              logPillClassName,
              onQuickFilter && 'cursor-pointer',
              'shrink-0 !text-xs [&_span]:!text-xs'
            )}
          />
        )
      },
      filterFn: (row, _id, value) => {
        if (!Array.isArray(value) || value.length === 0) return true
        if (value.includes(LOG_TYPE_ALL_VALUE)) return true
        return value.includes(String(row.original.type))
      },
      enableHiding: false,
      size: 70,
    },
  ]

  if (isAdmin) {
    columns.push(
      {
        id: 'channel',
        header: t('Channel'),
        accessorFn: (row) => row.channel,
        cell: function ChannelCell({ row }) {
          const { sensitiveVisible, setAffinityTarget, setAffinityDialogOpen } =
            useUsageLogsContext()
          const log = row.original

          if (!isDisplayableLogType(log.type)) return null

          const other = parseLogOther(log.other)
          const affinity = other?.admin_info?.channel_affinity
          const rawUseChannel = other?.admin_info?.use_channel ?? []
          const useChannel = Array.isArray(rawUseChannel)
            ? rawUseChannel.map(String).filter(Boolean)
            : []
          const hasRetryChain = useChannel.length > 1
          const channelChain = hasRetryChain
            ? useChannel.join(' → ')
            : undefined
          const channelDisplay = log.channel_name
            ? `${log.channel_name} #${log.channel}`
            : `#${log.channel}`
          const channelIdDisplay = `#${log.channel}`
          const multiKeyIndex = other?.admin_info?.multi_key_index
          const showMultiKeyIndex =
            other?.admin_info?.is_multi_key === true &&
            typeof multiKeyIndex === 'number' &&
            Number.isFinite(multiKeyIndex)

          return (
            <TooltipProvider>
              <Tooltip>
                <TooltipTrigger
                  render={
                    <div className='flex max-w-none items-center gap-1' />
                  }
                >
                  <div className='relative inline-flex items-center gap-1'>
                    <StatusBadge
                      label={channelIdDisplay}
                      autoColor={String(log.channel)}
                      copyText={String(log.channel)}
                      size='sm'
                      showDot={false}
                      onClick={() =>
                        onQuickFilter?.('channel', String(log.channel))
                      }
                      title={t('Copy and filter')}
                      className={cn(logPillClassName, 'font-mono')}
                    />
                    {showMultiKeyIndex && (
                      <StatusBadge
                        label={String(multiKeyIndex)}
                        size='sm'
                        showDot={false}
                        copyable={false}
                        variant='neutral'
                        className={cn(
                          logPillClassName,
                          'h-5 min-w-5 justify-center rounded-full px-1 font-mono text-xs'
                        )}
                        aria-label={`${t('Key')} ${multiKeyIndex}`}
                      />
                    )}
                    {hasRetryChain && (
                      <Popover>
                        <PopoverTrigger
                          render={
                            <button
                              type='button'
                              className='text-muted-foreground hover:text-foreground focus-visible:ring-ring inline-flex size-5 shrink-0 items-center justify-center rounded-full transition-colors focus-visible:ring-2 focus-visible:outline-none'
                              aria-label={t('Retry Chain')}
                              onClick={(e) => e.stopPropagation()}
                            />
                          }
                        >
                          <GitBranch
                            className='size-3.5 text-amber-500'
                            aria-hidden='true'
                          />
                        </PopoverTrigger>
                        <PopoverContent
                          side='top'
                          align='start'
                          className='w-64 text-xs'
                        >
                          <div className='flex flex-col gap-1'>
                            <p className='font-medium'>{t('Retry Chain')}</p>
                            <p className='text-muted-foreground font-mono break-all'>
                              {channelChain}
                            </p>
                          </div>
                        </PopoverContent>
                      </Popover>
                    )}
                    {affinity && (
                      <button
                        type='button'
                        className='absolute -top-1 -right-1 leading-none text-amber-500'
                        onClick={(e) => {
                          e.stopPropagation()
                          setAffinityTarget({
                            rule_name: affinity.rule_name || '',
                            using_group:
                              affinity.using_group ||
                              affinity.selected_group ||
                              '',
                            key_hint: affinity.key_hint || '',
                            key_fp: affinity.key_fp || '',
                          })
                          setAffinityDialogOpen(true)
                        }}
                      >
                        <Sparkles className='size-3 fill-current' />
                      </button>
                    )}
                  </div>
                </TooltipTrigger>
                <TooltipContent>
                  <div className='space-y-1'>
                    <p>
                      {sensitiveVisible ? channelDisplay : channelIdDisplay}
                    </p>
                    {channelChain && (
                      <p className='text-muted-foreground text-xs'>
                        {t('Chain')}: {channelChain}
                      </p>
                    )}
                    {showMultiKeyIndex && (
                      <p className='text-muted-foreground text-xs'>
                        {t('Key')}: {multiKeyIndex}
                      </p>
                    )}
                    {affinity && (
                      <div className='border-t pt-1 text-xs'>
                        <p className='font-medium'>{t('Channel Affinity')}</p>
                        <p>
                          {t('Rule')}: {affinity.rule_name || '-'}
                        </p>
                        <p>
                          {t('Group')}:{' '}
                          {sensitiveVisible
                            ? affinity.using_group ||
                              affinity.selected_group ||
                              '-'
                            : '••••'}
                        </p>
                      </div>
                    )}
                  </div>
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>
          )
        },
        size: 86,
      },
      {
        id: 'user',
        header: t('User'),
        accessorFn: (row) => row.username,
        cell: function UserCell({ row }) {
          const { sensitiveVisible, setSelectedUserId, setUserInfoDialogOpen } =
            useUsageLogsContext()
          const log = row.original

          if (!log.username) return null

          return (
            <div className='flex items-center gap-1'>
              <button
                type='button'
                className='flex items-center gap-1.5 text-left'
                onClick={(e) => {
                  e.stopPropagation()
                  setSelectedUserId(log.user_id)
                  setUserInfoDialogOpen(true)
                }}
              >
                <Avatar className='ring-border/60 size-6 ring-1 max-sm:hidden'>
                  <AvatarFallback
                    className={cn(
                      'text-[11px] font-semibold',
                      !sensitiveVisible && 'bg-muted text-muted-foreground'
                    )}
                    style={
                      sensitiveVisible
                        ? getUserAvatarStyle(log.username)
                        : undefined
                    }
                  >
                    {sensitiveVisible
                      ? getUserAvatarFallback(log.username)
                      : '•'}
                  </AvatarFallback>
                </Avatar>
                <TooltipProvider delay={300}>
                  <Tooltip>
                    <TooltipTrigger
                      render={
                        <span className='text-muted-foreground text-sm whitespace-nowrap hover:underline' />
                      }
                    >
                      {sensitiveVisible ? log.username : '••••'}
                    </TooltipTrigger>
                    {sensitiveVisible && log.username.length > 12 && (
                      <TooltipContent side='top'>{log.username}</TooltipContent>
                    )}
                  </Tooltip>
                </TooltipProvider>
              </button>
              {sensitiveVisible && onQuickFilter && (
                <button
                  type='button'
                  className='text-muted-foreground hover:text-foreground focus-visible:ring-ring inline-flex size-5 shrink-0 items-center justify-center rounded transition-colors focus-visible:ring-2 focus-visible:outline-none'
                  aria-label={t('Filter logs by {{value}}', {
                    value: log.username,
                  })}
                  title={t('Filter by this value')}
                  onClick={(event) => {
                    event.stopPropagation()
                    onQuickFilter('username', log.username)
                  }}
                >
                  <ListFilter className='size-3' />
                </button>
              )}
            </div>
          )
        },
        size: 112,
      }
    )
  }

  columns.push({
    accessorKey: 'token_name',
    header: t('Token'),
    cell: function TokenNameCell({ row }) {
      const { sensitiveVisible } = useUsageLogsContext()
      const log = row.original
      if (!isDisplayableLogType(log.type)) return null

      const tokenName = log.token_name
      if (!tokenName) return null

      const displayName = sensitiveVisible ? tokenName : '••••'

      return (
        <div className='flex max-w-none items-center gap-1'>
          <TooltipProvider delay={300}>
            <Tooltip>
              <TooltipTrigger render={<div className='max-w-none' />}>
                <StatusBadge
                  icon={KeyRound}
                  copyable={sensitiveVisible}
                  copyText={sensitiveVisible ? tokenName : undefined}
                  onClick={() =>
                    sensitiveVisible && onQuickFilter?.('token', tokenName)
                  }
                  title={sensitiveVisible ? t('Copy and filter') : undefined}
                  size='sm'
                  showDot={false}
                  className={cn(
                    logPillClassName,
                    'text-foreground h-6 max-w-none gap-1.5 rounded-md px-2 py-0.5 [font-family:var(--font-body)] !text-[11px] font-normal [&_span]:font-normal'
                  )}
                >
                  <span className='!text-[11px] whitespace-nowrap'>
                    {displayName}
                  </span>
                </StatusBadge>
              </TooltipTrigger>
              {sensitiveVisible && tokenName.length > 16 && (
                <TooltipContent side='top' className='max-w-xs break-all'>
                  {tokenName}
                </TooltipContent>
              )}
            </Tooltip>
          </TooltipProvider>
        </div>
      )
    },
    size: 132,
  })
  columns.push(
    {
      id: 'group',
      header: t('Group'),
      accessorFn: (row) => row.group || parseLogOther(row.other)?.group || '',
      cell: function GroupCell({ row }) {
        const { sensitiveVisible } = useUsageLogsContext()
        const log = row.original
        if (!isDisplayableLogType(log.type)) return null

        const other = parseLogOther(log.other)
        const group = log.group || other?.group || ''
        if (!group) return null

        return (
          <span className='inline-flex items-center text-xs leading-none whitespace-nowrap'>
            <GroupBadge
              group={group}
              label={sensitiveVisible ? undefined : '••••'}
              copyable={sensitiveVisible}
              copyText={sensitiveVisible ? group : undefined}
              onClick={() =>
                sensitiveVisible && onQuickFilter?.('group', group)
              }
              title={sensitiveVisible ? t('Copy and filter') : undefined}
              size='sm'
              className={cn(
                logPillClassName,
                'max-w-full align-baseline !text-[11px] leading-none font-normal [&>span]:!text-[11px] [&>span]:leading-none [&>span]:font-normal'
              )}
            />
          </span>
        )
      },
      size: 86,
    },
    {
      accessorKey: 'model_name',
      header: t('Model'),
      cell: function ModelCell({ row }) {
        const log = row.original
        if (!isDisplayableLogType(log.type)) return null

        const modelInfo = formatModelName(log)

        return (
          <div className='flex w-fit'>
            <ModelBadge
              modelName={modelInfo.name}
              actualModel={modelInfo.actualModel}
              className={cn(
                logPillClassName,
                '!text-[11px] font-normal [&_span]:!text-[11px] [&_span]:font-normal'
              )}
            />
          </div>
        )
      },
      meta: { mobileTitle: true },
      size: 190,
    },
    {
      accessorKey: 'prompt_tokens',
      header: 'Tokens',
      cell: ({ row }) => {
        const log = row.original
        if (!isDisplayableLogType(log.type)) return null

        const other = parseLogOther(log.other)

        const promptTokens = log.prompt_tokens || 0
        const completionTokens = log.completion_tokens || 0
        const cacheReadTokens = other?.cache_tokens || 0
        const cacheWrite5m = other?.cache_creation_tokens_5m || 0
        const cacheWrite1h = other?.cache_creation_tokens_1h || 0
        const hasSplitCache = cacheWrite5m > 0 || cacheWrite1h > 0
        const cacheWriteTokens = hasSplitCache
          ? cacheWrite5m + cacheWrite1h
          : other?.cache_write_tokens || other?.cache_creation_tokens || 0
        if (
          promptTokens === 0 &&
          completionTokens === 0 &&
          cacheReadTokens === 0 &&
          cacheWriteTokens === 0
        ) {
          return <span className='text-muted-foreground text-xs'>-</span>
        }

        return (
          <div className='flex flex-col gap-0.5 pl-1'>
            <span className='shrink-0 font-mono !text-[14px] leading-5 font-semibold tabular-nums'>
              {promptTokens.toLocaleString()} /{' '}
              {completionTokens.toLocaleString()}
            </span>
            {cacheReadTokens > 0 || cacheWriteTokens > 0 ? (
              <div className='text-muted-foreground/60 flex flex-nowrap items-center gap-1.5 !text-[8px] leading-none'>
                {cacheReadTokens > 0 ? (
                  <span className='shrink-0'>
                    {t('Cache Read')} {cacheReadTokens.toLocaleString()}
                  </span>
                ) : null}
                {cacheWriteTokens > 0 ? (
                  <span className='shrink-0'>
                    {t('Cache Write')} {cacheWriteTokens.toLocaleString()}
                  </span>
                ) : null}
              </div>
            ) : null}
          </div>
        )
      },
      size: 145,
    },
    {
      accessorKey: 'use_time',
      header: t('Timing'),
      cell: ({ row }) => {
        const log = row.original
        if (!isTimingLogType(log.type)) return null

        const useTime = row.getValue('use_time') as number
        const other = parseLogOther(log.other)

        return (
          <TimingMetricsCell
            useTimeSec={useTime}
            completionTokens={log.completion_tokens}
            frtMs={other?.frt}
            isStream={log.is_stream}
            presentation='pills'
          />
        )
      },
      size: 92,
    },
    {
      id: 'is_stream',
      header: t('Stream'),
      accessorFn: (row) => row.is_stream,
      cell: ({ row }) => {
        const log = row.original
        if (!isTimingLogType(log.type)) return null

        const useTime = log.use_time || 0
        const other = parseLogOther(log.other)
        const tokensPerSecond =
          useTime > 0 && log.completion_tokens > 0
            ? log.completion_tokens / useTime
            : null

        return (
          <div className='inline-flex rounded-lg border border-sky-500/20 bg-sky-500/5 px-2 py-1 dark:border-sky-400/20 dark:bg-sky-400/5'>
            <StreamTpsCell
              isStream={log.is_stream}
              tokensPerSecond={tokensPerSecond}
              streamStatus={other?.stream_status}
              inline
            />
          </div>
        )
      },
      size: 104,
    },
    {
      accessorKey: 'quota',
      header: t('Cost'),
      cell: ({ row }) => {
        const log = row.original
        if (!isDisplayableLogType(log.type)) return null

        const quota = row.getValue('quota') as number
        const other = parseLogOther(log.other)
        return <LogCostDisplay quota={quota} other={other} />
      },
      size: 90,
    },

    {
      accessorKey: 'content',
      header: t('Details'),
      cell: function DetailsCell({ row }) {
        const log = row.original
        const other = parseLogOther(log.other)

        const segments = buildDetailSegments(log, other, t, isAdmin)
        let detailPreview = <span className='text-muted-foreground/40'>—</span>
        if (segments.length > 0) {
          detailPreview = (
            <span className='flex max-w-full min-w-0 flex-col gap-0.5 leading-snug'>
              {segments.map((segment) => (
                <span
                  key={`${segment.text}-${segment.muted ? 'muted' : ''}-${segment.danger ? 'danger' : ''}`}
                  className={cn(
                    'min-w-0 break-all whitespace-normal sm:wrap-break-word',
                    segments.length > 1 ? 'line-clamp-1' : 'line-clamp-2',
                    segment.muted && 'text-muted-foreground/60',
                    segment.danger && 'text-red-600 dark:text-red-400'
                  )}
                >
                  {segment.text}
                </span>
              ))}
            </span>
          )
        } else if (log.content) {
          detailPreview = (
            <span className='text-muted-foreground line-clamp-2 break-all whitespace-normal sm:wrap-break-word'>
              {log.content}
            </span>
          )
        }

        return (
          <span className='block max-w-full min-w-0 text-left leading-snug'>
            {detailPreview}
          </span>
        )
      },
      enableSorting: false,
      size: 190,
    }
  )

  return columns
}
