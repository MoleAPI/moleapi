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
import type { StatusBadgeProps } from '@/components/status-badge'
import {
  BILLING_PRICING_VARS,
  normalizeTierLabel,
  parseTiersFromExpr,
  type ParsedTier,
} from '@/features/pricing/lib/billing-expr'
import { formatLogQuota } from '@/lib/format'

import type { UsageLog } from '../data/schema'
import type { LogOtherData } from '../types'

export { normalizeTierLabel }

const PARAM_OVERRIDE_ACTION_MAP: Record<string, string> = {
  set: 'Set',
  delete: 'Delete',
  copy: 'Copy',
  move: 'Move',
  append: 'Append',
  prepend: 'Prepend',
  trim_prefix: 'Trim Prefix',
  trim_suffix: 'Trim Suffix',
  ensure_prefix: 'Ensure Prefix',
  ensure_suffix: 'Ensure Suffix',
  trim_space: 'Trim Space',
  to_lower: 'To Lower',
  to_upper: 'To Upper',
  replace: 'Replace',
  regex_replace: 'Regex Replace',
  set_header: 'Set Header',
  delete_header: 'Delete Header',
  copy_header: 'Copy Header',
  move_header: 'Move Header',
  pass_headers: 'Pass Headers',
  sync_fields: 'Sync Fields',
  return_error: 'Return Error',
}

/**
 * Get localized label for a param override action
 */
export function getParamOverrideActionLabel(
  action: string,
  t: (key: string) => string
): string {
  const key = PARAM_OVERRIDE_ACTION_MAP[action.toLowerCase()]
  return key ? t(key) : action
}

export function getLoginMethodLabel(
  method: string,
  t: (key: string) => string
): string {
  const labels: Record<string, string> = {
    password: 'Password',
    '2fa': 'Two-Factor Authentication',
    passkey: 'Passkey',
    wechat: 'WeChat',
    telegram: 'Telegram',
    oauth: 'OAuth',
    unknown: 'Unknown',
  }
  if (method.startsWith('oauth:')) {
    return `${t('OAuth')}: ${method.slice('oauth:'.length)}`
  }
  const label = labels[method.toLowerCase()]
  return label ? t(label) : method
}

/**
 * Parse a param override audit line into action and content
 */
export function parseAuditLine(
  line: string
): { action: string; content: string } | null {
  if (typeof line !== 'string') return null
  const firstSpace = line.indexOf(' ')
  if (firstSpace <= 0) return { action: line, content: line }
  return {
    action: line.slice(0, firstSpace),
    content: line.slice(firstSpace + 1),
  }
}

/**
 * Check if the log is a violation fee log
 */
export function isViolationFeeLog(other: LogOtherData | null): boolean {
  if (!other) return false
  return (
    other.violation_fee === true ||
    Boolean(other.violation_fee_code) ||
    Boolean(other.violation_fee_marker)
  )
}

function isPositiveFiniteNumber(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value) && value > 0
}

function hasLegacySearchSurcharge(
  enabled: boolean | undefined,
  count: number | undefined,
  price: number | undefined
): boolean {
  return (
    enabled === true &&
    isPositiveFiniteNumber(count) &&
    isPositiveFiniteNumber(price)
  )
}

/**
 * Check whether a consume log includes an actual tool-call surcharge.
 * Structured surcharge items cover current logs, while the legacy fields keep
 * historical Web Search, File Search, and Image Generation logs visible.
 */
export function hasToolSurcharge(other: LogOtherData | null): boolean {
  if (!other) return false

  const hasStructuredSurcharge =
    Array.isArray(other.tool_surcharges) &&
    other.tool_surcharges.some(
      (item) =>
        typeof item?.name === 'string' &&
        item.name.trim() !== '' &&
        isPositiveFiniteNumber(item.count) &&
        isPositiveFiniteNumber(item.price)
    )
  if (hasStructuredSurcharge) return true

  if (
    hasLegacySearchSurcharge(
      other.web_search,
      other.web_search_call_count,
      other.web_search_price
    )
  ) {
    return true
  }

  if (
    hasLegacySearchSurcharge(
      other.file_search,
      other.file_search_call_count,
      other.file_search_price
    )
  ) {
    return true
  }

  return (
    other.image_generation_call === true &&
    isPositiveFiniteNumber(other.image_generation_call_price)
  )
}

/**
 * Parse the 'other' field from JSON string to object
 */
export function parseLogOther(other: string): LogOtherData | null {
  if (!other) return null
  try {
    return JSON.parse(other) as LogOtherData
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to parse log other field:', error)
    return null
  }
}

export function getReasoningEffortVariant(
  effort: string | undefined
): StatusBadgeProps['variant'] {
  switch (effort?.trim().toLowerCase()) {
    case 'max':
    case 'xhigh':
    case 'high':
      return 'orange'
    case 'medium':
      return 'yellow'
    case 'low':
    case 'minimal':
      return 'green'
    case 'none':
    default:
      return 'grey'
  }
}

/**
 * Resolve image token counts across current and legacy usage-log payloads.
 * Older logs stored image-input tokens in `image_output`.
 */
export function getImageTokenBreakdown(other: LogOtherData): {
  input: number
  output: number
} {
  const explicitInput = other.image_input_tokens
  const legacyInput = other.image_output
  const input = Number(explicitInput ?? legacyInput ?? 0)
  const output = Number(other.image_output_tokens ?? 0)

  return {
    input: Number.isFinite(input) && input > 0 ? input : 0,
    output: Number.isFinite(output) && output > 0 ? output : 0,
  }
}

/**
 * Get time color based on duration (in seconds)
 */
export function getTimeColor(
  seconds: number
): 'success' | 'warning' | 'danger' {
  if (seconds < 10) return 'success'
  if (seconds < 30) return 'warning'
  return 'danger'
}

/**
 * Get first-response-token color based on latency (in seconds)
 */
export function getFirstResponseTimeColor(
  seconds: number
): 'success' | 'warning' | 'danger' {
  if (seconds < 5) return 'success'
  if (seconds < 10) return 'warning'
  return 'danger'
}

/**
 * Get throughput color based on generated tokens per second
 */
export function getThroughputColor(
  tokensPerSecond: number
): 'success' | 'warning' | 'danger' {
  if (tokensPerSecond >= 30) return 'success'
  if (tokensPerSecond >= 15) return 'warning'
  return 'danger'
}

/**
 * Get response color using throughput only when enough output tokens exist.
 */
export function getResponseTimeColor(
  seconds: number,
  completionTokens: number
): 'success' | 'warning' | 'danger' {
  if (completionTokens < 100 || seconds <= 0) return getTimeColor(seconds)
  return getThroughputColor(completionTokens / seconds)
}

/**
 * Format model name with mapping indicator
 */
export function formatModelName(log: UsageLog): {
  name: string
  isMapped: boolean
  actualModel?: string
} {
  const other = parseLogOther(log.other)
  const isMapped = !!(
    other?.is_model_mapped &&
    other?.upstream_model_name &&
    other.upstream_model_name !== ''
  )

  return {
    name: log.model_name,
    isMapped,
    actualModel: isMapped ? other.upstream_model_name : undefined,
  }
}

/**
 * Decode a base64-encoded billing expression. Safely returns an empty string
 * when the input is missing or malformed (e.g. legacy logs without expr_b64).
 */
export function decodeBillingExprB64(exprB64: string | undefined): string {
  if (!exprB64) return ''
  try {
    const binaryString =
      typeof window !== 'undefined'
        ? window.atob(exprB64)
        : Buffer.from(exprB64, 'base64').toString('binary')
    const bytes = new Uint8Array(binaryString.length)

    for (let i = 0; i < binaryString.length; i++) {
      bytes[i] = binaryString.charCodeAt(i)
    }

    if (typeof TextDecoder !== 'undefined') {
      return new TextDecoder().decode(bytes)
    }

    return decodeURIComponent(
      Array.prototype.map
        .call(bytes, (byte: number) => `%${byte.toString(16).padStart(2, '0')}`)
        .join('')
    )
  } catch {
    return ''
  }
}

/**
 * Resolve which parsed tier corresponds to the matched_tier label in a log
 * entry. Missing or unknown labels do not fall back to another tier because
 * that would display guessed unit prices.
 */
export function resolveMatchedTier(
  tiers: ParsedTier[],
  matchedLabel: string | undefined
): ParsedTier | null {
  if (tiers.length === 0) return null
  if (!matchedLabel) return null
  const found = tiers.find((tier) => {
    const l1 = normalizeTierLabel(tier.label)
    const l2 = normalizeTierLabel(matchedLabel)
    return l1 === l2 && l1 !== ''
  })
  return found || null
}

/**
 * Tiered pricing summary derived from an `other` log payload using the
 * billing-expression library. Returns null when the entry is not a tiered
 * billing log or the expression failed to parse.
 */
export interface TieredBillingSummary {
  tiers: ParsedTier[]
  tier: ParsedTier
  priceEntries: Array<{ field: string; shortLabel: string; price: number }>
}

/**
 * Whether the request payload reports any cache-related token usage. Used to
 * suppress cache pricing rows from the tiered breakdown when the request did
 * not exercise the cache path.
 */
export function hasAnyCacheTokens(
  other: LogOtherData | null | undefined
): boolean {
  if (!other) return false
  return (
    (other.cache_tokens || 0) > 0 ||
    (other.cache_creation_tokens || 0) > 0 ||
    (other.cache_creation_tokens_5m || 0) > 0 ||
    (other.cache_creation_tokens_1h || 0) > 0 ||
    (other.cache_write_tokens || 0) > 0
  )
}

export function getTieredBillingSummary(
  other: LogOtherData | null
): TieredBillingSummary | null {
  if (!other || other.billing_mode !== 'tiered_expr') return null
  const exprStr = decodeBillingExprB64(other.expr_b64)
  if (!exprStr) return null
  const tiers = parseTiersFromExpr(exprStr)
  const tier = resolveMatchedTier(tiers, other.matched_tier)
  if (!tier) return null

  const cacheTokensPresent = hasAnyCacheTokens(other)

  const priceEntries: TieredBillingSummary['priceEntries'] = []
  for (const v of BILLING_PRICING_VARS) {
    if (!v.field) continue
    if (v.group === 'cache' && !cacheTokensPresent) continue
    const raw = tier[v.field as keyof ParsedTier]
    const price = Number(raw)
    if (Number.isFinite(price) && price > 0) {
      priceEntries.push({
        field: v.field,
        shortLabel: v.shortLabel,
        price,
      })
    }
  }
  return { tiers, tier, priceEntries }
}

/**
 * Calculate duration and return formatted result with color variant
 * @param submitTime - Submit timestamp
 * @param finishTime - Finish timestamp
 * @param unit - Unit of the timestamps ('seconds' or 'milliseconds')
 */
export function formatDuration(
  submitTime?: number,
  finishTime?: number,
  unit: 'seconds' | 'milliseconds' = 'milliseconds'
): { durationSec: number; variant: StatusBadgeProps['variant'] } | null {
  if (!submitTime || !finishTime) return null

  const durationSec =
    unit === 'milliseconds'
      ? (finishTime - submitTime) / 1000
      : finishTime - submitTime

  return { durationSec, variant: durationSec > 60 ? 'red' : 'green' }
}

/**
 * Maps a language-independent log operation `action` to an i18n
 * template string (the template itself is the i18n key, with {{placeholders}}).
 *
 * The backend stores only `action` + structured `params` in `other.op`; the UI
 * renders localized content at display time so business logs are fully
 * translatable instead of being frozen to whatever language was written to DB.
 */
const AUDIT_PARAM_LABELS: Record<string, string> = {
  'user.manage:disable': 'Disable',
  'user.manage:enable': 'Enable',
  'user.manage:delete': 'Delete',
  'user.manage:promote': 'Promote',
  'user.manage:demote': 'Demote',
  'channel.multi_key_manage:disable_key': 'Disable',
  'channel.multi_key_manage:enable_key': 'Enable',
  'channel.multi_key_manage:enable_all_keys': 'Enable All',
  'channel.multi_key_manage:disable_all_keys': 'Disable All',
  'channel.multi_key_manage:delete_key': 'Delete',
  'channel.multi_key_manage:delete_disabled_keys': 'Delete disabled keys',
}

const AUDIT_TEMPLATES: Record<string, string> = {
  login: 'Logged in successfully via {{method}}',
  // User management
  'user.create': 'Created user {{username}} (role {{role}})',
  'user.update': 'Updated user {{username}} (ID: {{id}})',
  'user.delete': 'Deleted user {{username}} (ID: {{id}})',
  'user.manage': 'Performed {{action}} on user {{username}} (ID: {{id}})',
  'user.quota_add': 'Increased user quota by {{quota}}',
  'user.quota_subtract': 'Decreased user quota by {{quota}}',
  'user.quota_override': 'Overrode user quota from {{from}} to {{to}}',
  'user.binding_clear': 'Cleared {{bindingType}} binding for user {{username}}',
  'user.2fa_disable': 'Force-disabled two-factor authentication for the user',
  'user.passkey_register': 'Registered a passkey',
  'user.passkey_delete': 'Deleted a passkey',
  'user.topup_complete': 'Completed top-up order for the user',
  'user.reset_passkey': 'Reset the user passkey',
  'user.oauth_unbind': 'Removed an OAuth binding for the user',
  'user.invite_rebate_apply_default':
    'Applied default invite rebate ratio {{ratio}} to {{updated}} users',
  'user.invite_rebate_batch_update':
    'Batch updated invite rebate ratio to {{target_ratio}} for {{updated}} users',
  'user.reward_transfer':
    'Transferred {{quota}} of referral rewards to the balance',
  'user.registration_reward': 'Received a {{quota}} sign-up reward',
  'user.invitee_reward':
    'Received a {{quota}} reward for using an invitation code',
  'user.inviter_reward': 'Received a {{quota}} reward for inviting a user',
  'user.topup_rebate':
    'Received a {{quota}} top-up rebate from user {{related_user}}',
  'user.checkin_reward': 'Checked in and received {{quota}}',
  'user.security_verification':
    'Security verification succeeded via {{method}}',
  'user.2fa_setup_started': 'Started two-factor authentication setup',
  'user.2fa_enabled': 'Enabled two-factor authentication',
  'user.2fa_disabled': 'Disabled two-factor authentication',
  'user.2fa_backup_codes_regenerated':
    'Regenerated two-factor authentication backup codes',
  // System settings
  'option.update': 'Updated system setting {{key}}',
  'option.model_pricing.import':
    'Imported {{updated_options}} model pricing settings',
  'option.payment_compliance': 'Confirmed payment compliance',
  'option.reset_ratio': 'Reset model ratios',
  'option.clear_affinity_cache': 'Cleared channel affinity cache',
  'option.waffo_catalog': 'Fetched the Waffo Pancake catalog',
  'option.waffo_pair': 'Created a Waffo Pancake pairing',
  'option.waffo_save': 'Saved Waffo Pancake settings',
  'option.waffo_subscription_product':
    'Created a Waffo Pancake subscription product',
  // Custom OAuth
  'custom_oauth.discovery': 'Discovered a custom OAuth provider configuration',
  'custom_oauth.create': 'Created a custom OAuth provider',
  'custom_oauth.update': 'Updated a custom OAuth provider',
  'custom_oauth.delete': 'Deleted a custom OAuth provider',
  // Performance / cache
  'performance.clear_disk_cache': 'Cleared disk cache',
  'performance.reset_stats': 'Reset performance statistics',
  'performance.gc': 'Triggered garbage collection',
  'performance.clear_logs': 'Cleared log files',
  'ratio_sync.fetch': 'Fetched upstream model ratios',
  // Channel
  'channel.create': 'Created channel {{name}} (type {{type}}, count {{count}})',
  'channel.update': 'Updated channel {{name}} (ID: {{id}})',
  'channel.delete': 'Deleted channel {{name}} (ID: {{id}})',
  'channel.delete_batch': 'Batch deleted {{count}} channels',
  'channel.delete_disabled': 'Deleted all disabled channels ({{count}})',
  'channel.key_view': 'Viewed channel key {{name}} (ID: {{id}})',
  'channel.tag_disable': 'Disabled channels with tag {{tag}}',
  'channel.tag_enable': 'Enabled channels with tag {{tag}}',
  'channel.tag_edit': 'Edited channels with tag {{tag}}',
  'channel.tag_batch_set': 'Batch set tag for {{count}} channels',
  'channel.copy':
    'Copied channel (source ID: {{sourceId}}) to {{name}} (new ID: {{id}})',
  'channel.multi_key_manage':
    'Multi-key management {{action}} on channel (ID: {{id}})',
  'channel.status_update': 'Updated channel {{id}} status to {{status}}',
  'channel.status_update_batch':
    'Updated {{count}} of {{total}} channels to {{status}}',
  'channel.fix': 'Repaired channel abilities',
  'channel.fetch_models': 'Fetched upstream channel models',
  'channel.codex_refresh': 'Refreshed Codex channel credentials',
  'channel.codex_usage_reset': 'Reset Codex channel usage',
  'channel.ollama_pull': 'Pulled an Ollama model',
  'channel.ollama_delete': 'Deleted an Ollama model',
  'channel.upstream_apply':
    'Applied upstream model changes to channel (ID: {{id}})',
  'channel.upstream_apply_all':
    'Applied upstream model changes to {{count}} channels',
  'channel.upstream_detect':
    'Detected upstream model changes for channel {{name}} (ID: {{id}})',
  'channel.upstream_detect_all':
    'Started upstream model update detection task {{task_id}}',
  // Redemption codes
  'redemption.create':
    'Created {{count}} redemption codes named {{name}} ({{quota}} each)',
  'redemption.update': 'Updated a redemption code',
  'redemption.delete': 'Deleted a redemption code',
  'redemption.delete_invalid': 'Deleted invalid redemption codes',
  // Top-ups
  'topup.redemption': 'Redeemed code {{redemption_id}} and received {{quota}}',
  'topup.completed':
    'Top-up completed via {{provider}}: credited {{quota}}, paid {{money}}',
  'topup.admin_complete':
    'Admin completed the top-up: credited {{quota}}, paid {{money}}',
  'topup.invoice_view':
    'Viewed invoice for top-up {{trade_no}} (ID: {{topup_id}})',
  // Prefill groups
  'prefill_group.create': 'Created a prefill group',
  'prefill_group.update': 'Updated a prefill group',
  'prefill_group.delete': 'Deleted a prefill group',
  // Vendors
  'vendor.create': 'Created a vendor',
  'vendor.update': 'Updated a vendor',
  'vendor.delete': 'Deleted a vendor',
  // Model metadata
  'model.create': 'Created a model',
  'model.update': 'Updated a model',
  'model.delete': 'Deleted a model',
  'model.sync_upstream': 'Synced upstream models',
  // Deployments
  'deployment.create': 'Created a deployment',
  'deployment.update': 'Updated a deployment',
  'deployment.delete': 'Deleted a deployment',
  'deployment.test_connection': 'Tested the deployment connection',
  'deployment.price_estimation': 'Estimated a deployment price',
  'deployment.update_name': 'Renamed a deployment',
  'deployment.extend': 'Extended a deployment',
  // Subscriptions
  'subscription.plan_create': 'Created a subscription plan',
  'subscription.plan_update': 'Updated a subscription plan',
  'subscription.plan_delete':
    'Deleted subscription plan {{plan_title}} (ID: {{plan_id}})',
  'subscription.bind': 'Bound a subscription',
  'subscription.plan_reset':
    'Reset active subscriptions for plan {{plan_title}} (ID: {{plan_id}})',
  'subscription.user_plan_reset':
    'Reset active subscriptions for user {{target_user_id}} under plan {{plan_title}} (ID: {{plan_id}})',
  'subscription.user_bind': 'Bound a subscription to a user',
  'subscription.user_invalidate': 'Invalidated a user subscription',
  'subscription.user_delete': 'Deleted a user subscription',
  'subscription.purchase':
    'Purchased subscription {{plan}} for {{money}} via {{payment_method}}',
  'subscription.balance_purchase':
    'Purchased subscription {{plan}} with balance: paid {{money}}, deducted {{quota}}',
  // Logs
  'log.clear': 'Cleared historical logs',
  'log.cleanup_start': 'Log cleanup task started.',
  // System instances
  'system_info.delete_stale': 'Deleted stale system instances',
  'system_info.delete_instance': 'Deleted a system instance',
  // Model metadata
  'model.descriptions_import': 'Imported model descriptions',
  // Generic middleware fallback
  generic: '{{method}} {{route}}',
}

/**
 * Render localized content from a log's structured
 * `other.op` descriptor. Returns null when the log has no recognized action,
 * letting callers fall back to the raw `content` field.
 */
export function renderAuditContent(
  other: LogOtherData | null | undefined,
  t: (key: string, opts?: Record<string, unknown>) => string
): string | null {
  const op = other?.op
  if (!op?.action) return null
  const template = AUDIT_TEMPLATES[op.action]
  if (!template) return null
  const params = { ...op.params } as Record<string, unknown>
  if (typeof params.quota_raw === 'number') {
    params.quota = formatLogQuota(params.quota_raw)
  }
  if (typeof params.from_raw === 'number') {
    params.from = formatLogQuota(params.from_raw)
  }
  if (typeof params.to_raw === 'number') {
    params.to = formatLogQuota(params.to_raw)
  }
  if (typeof params.action === 'string') {
    const label = AUDIT_PARAM_LABELS[`${op.action}:${params.action}`]
    if (label) params.action = t(label)
  }
  if (op.action === 'login' && typeof params.method === 'string') {
    params.method = getLoginMethodLabel(params.method, t)
  }
  if (op.action === 'user.create' && typeof params.role === 'number') {
    if (params.role === 1) params.role = t('User')
    if (params.role === 10) params.role = t('Admin')
    if (params.role === 100) params.role = t('Root')
  }
  if (
    (op.action === 'channel.status_update' ||
      op.action === 'channel.status_update_batch') &&
    typeof params.status === 'number'
  ) {
    if (params.status === 1) params.status = t('Enabled')
    if (params.status === 2) params.status = t('Disabled')
  }
  return t(template, params)
}

function normalizeLegacyQuota(value: string): string {
  const tokenQuota = /^(-?\d+) 点额度$/.exec(value.trim())
  if (tokenQuota) return formatLogQuota(Number(tokenQuota[1]))
  return value.trim().replace(/\s*额度$/, '')
}

/**
 * Render current structured operations and the finite set of historical
 * business-log sentences that predate operation descriptors.
 */
export function renderLogContent(
  log: UsageLog,
  other: LogOtherData | null | undefined,
  t: (key: string, opts?: Record<string, unknown>) => string
): string | null {
  const structured = renderAuditContent(other, t)
  if (structured) return structured

  const content = log.content?.trim()
  if (!content) return null

  // ponytail: keep this finite legacy parser until old natural-language rows age out;
  // new log types must use other.op instead of adding another stored sentence.
  let match: RegExpExecArray | null
  if (log.type === 3) {
    match = /^管理员重置订阅套餐 (.+)（ID: (\d+)）额度$/.exec(content)
    if (match) {
      return t(
        'Reset active subscriptions for plan {{plan_title}} (ID: {{plan_id}})',
        {
          plan_title: match[1],
          plan_id: match[2],
        }
      )
    }
  }

  if (log.type === 7) {
    match = /^Logged in successfully via (.+)$/.exec(content)
    if (match) {
      return t('Logged in successfully via {{method}}', {
        method: getLoginMethodLabel(match[1], t),
      })
    }
  }

  if (log.type === 4) {
    if (content === '开始设置两步验证') {
      return t('Started two-factor authentication setup')
    }
    if (content === '成功启用两步验证') {
      return t('Enabled two-factor authentication')
    }
    if (content === '禁用两步验证') {
      return t('Disabled two-factor authentication')
    }
    if (content === '重新生成两步验证备用码') {
      return t('Regenerated two-factor authentication backup codes')
    }
    if (content === '通用安全验证成功 (验证方式: 2FA)') {
      return t('Security verification succeeded via {{method}}', {
        method: '2FA',
      })
    }

    match = /^管理员调整额度 (.+)$/.exec(content)
    if (match) {
      const quota =
        log.quota === 0
          ? normalizeLegacyQuota(match[1])
          : formatLogQuota(Math.abs(log.quota))
      if (log.quota > 0) {
        return t('Increased user quota by {{quota}}', { quota })
      }
      if (log.quota < 0) {
        return t('Decreased user quota by {{quota}}', { quota })
      }
    }

    match = /^邀请好友充值返利 (.+)，受邀用户 (.+)$/.exec(content)
    if (match) {
      return t(
        'Received a {{quota}} top-up rebate from user {{related_user}}',
        {
          quota:
            log.quota === 0
              ? normalizeLegacyQuota(match[1])
              : formatLogQuota(Math.abs(log.quota)),
          related_user: match[2],
        }
      )
    }

    const rewardPatterns: Array<[RegExp, string]> = [
      [
        /^转移邀请奖励 (.+) 到余额$/,
        'Transferred {{quota}} of referral rewards to the balance',
      ],
      [/^新用户注册赠送 (.+)$/, 'Received a {{quota}} sign-up reward'],
      [
        /^使用邀请码赠送 (.+)$/,
        'Received a {{quota}} reward for using an invitation code',
      ],
      [
        /^邀请用户赠送 (.+)$/,
        'Received a {{quota}} reward for inviting a user',
      ],
      [/^用户签到，获得额度 (.+)$/, 'Checked in and received {{quota}}'],
    ]
    for (const [pattern, template] of rewardPatterns) {
      match = pattern.exec(content)
      if (match) {
        return t(template, {
          quota:
            log.quota === 0
              ? normalizeLegacyQuota(match[1])
              : formatLogQuota(Math.abs(log.quota)),
        })
      }
    }
  }

  if (log.type === 1) {
    match = /^通过兑换码充值 (.+)，兑换码ID (\d+)$/.exec(content)
    if (match) {
      return t('Redeemed code {{redemption_id}} and received {{quota}}', {
        quota: normalizeLegacyQuota(match[1]),
        redemption_id: match[2],
      })
    }

    match =
      /^管理员补单成功，充值金额:\s*(.+?)[，,]\s*支付金额[：:]\s*([0-9.]+)$/.exec(
        content
      )
    if (match) {
      return t(
        'Admin completed the top-up: credited {{quota}}, paid {{money}}',
        {
          quota: normalizeLegacyQuota(match[1]),
          money: match[2],
        }
      )
    }

    match =
      /^(使用在线|使用Creem|蓝兔支付|NOWPayments|Waffo Pancake|Waffo)充值成功，充值(?:金额|额度):\s*(.+?)[，,]\s*支付金额[：:]\s*([0-9.]+)$/.exec(
        content
      )
    if (match) {
      const providers: Record<string, string> = {
        使用在线: other?.admin_info?.callback_payment_method ?? t('Online'),
        使用Creem: 'Creem',
        蓝兔支付: 'LanTu',
        NOWPayments: 'NOWPayments',
        Waffo: 'Waffo',
        'Waffo Pancake': 'Waffo Pancake',
      }
      return t(
        'Top-up completed via {{provider}}: credited {{quota}}, paid {{money}}',
        {
          provider: providers[match[1]],
          quota: normalizeLegacyQuota(match[2]),
          money: match[3],
        }
      )
    }

    match =
      /^订阅购买成功，套餐:\s*(.+?)[，,]\s*支付金额:\s*([^，,]+)[，,]\s*支付方式:\s*(.+)$/.exec(
        content
      )
    if (match) {
      return t(
        'Purchased subscription {{plan}} for {{money}} via {{payment_method}}',
        {
          plan: match[1],
          money: match[2],
          payment_method: match[3],
        }
      )
    }

    match =
      /^使用余额购买订阅成功，套餐:\s*(.+?)[，,]\s*支付金额:\s*([^，,]+)[，,]\s*扣除额度:\s*(-?\d+)$/.exec(
        content
      )
    if (match) {
      return t(
        'Purchased subscription {{plan}} with balance: paid {{money}}, deducted {{quota}}',
        {
          plan: match[1],
          money: match[2],
          quota: formatLogQuota(Number(match[3])),
        }
      )
    }
  }

  return null
}
