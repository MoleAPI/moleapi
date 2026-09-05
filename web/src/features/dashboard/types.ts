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
import type { TimeGranularity } from '@/lib/time'

// ============================================================================
// Quota & Usage Data Types
// ============================================================================

export interface QuotaDataItem {
  id?: number
  user_id?: number
  username?: string
  model_name?: string
  created_at: number
  token_used?: number
  count?: number
  quota?: number
}

export interface FlowQuotaDataItem {
  user_id?: number
  username?: string
  node_name?: string
  use_group?: string
  token_id?: number
  token_name?: string
  channel_id?: number
  channel_name?: string
  model_name?: string
  token_used?: number
  count?: number
  quota?: number
}

export interface ChannelSuccessPoint {
  ts: number
  request_count: number
  success_count: number
  success_rate: number
}

export interface ChannelSuccessSummary {
  channel_id: number
  channel_name: string
  request_count: number
  success_count: number
  success_rate: number
  series: ChannelSuccessPoint[]
}

export interface ChannelSuccessData {
  channels: ChannelSuccessSummary[]
  probe_overview: ChannelProbeOverview
}

export interface ChannelProbeOverviewItem {
  channel_id: number
  channel_name: string
  model: string
  level?: 'basic' | 'standard' | 'advanced' | 'custom'
  status: 'pending' | 'healthy' | 'degraded'
  recent_pass: number
  recent_total: number
  last_test_at?: number
}

export interface ChannelProbeOverview {
  enabled: boolean
  mode: 'hi' | 'intelligence' | 'custom'
  enabled_channels: number
  total_models: number
  healthy: number
  degraded: number
  pending: number
  items: ChannelProbeOverviewItem[]
}

export interface AdminBusinessMetrics {
  new_users: number
  new_purchasing_users: number
  new_user_purchasing_users: number
  intent_orders: number
  intent_amounts: AdminBusinessOrderAmount[]
  top_up_intent_orders: number
  top_up_intent_amount_usd: number
  paid_orders: number
  paid_amounts: AdminBusinessOrderAmount[]
  top_up_paid_orders: number
  top_up_paid_amount_usd: number
  usd_exchange_rate: number
  paying_users: number
  payment_success_rate: number
  top_up_ranking: AdminBusinessTopUpUser[]
}

export interface AdminBusinessOrderAmount {
  currency: string
  orders: number
  amount: number
  average_amount: number
}

export interface AdminBusinessTopUpUser {
  rank: number
  user_id: number
  username: string
  currency: string
  orders: number
  amount: number
}

export type FlowMetric = 'quota' | 'tokens' | 'requests'

export type FlowOverflowMode = 'aggregate' | 'hide'

export type FlowRole = 'user' | 'admin' | 'root'

export type FlowNodeKind =
  | 'user'
  | 'node'
  | 'token'
  | 'group'
  | 'model'
  | 'channel'

export interface FlowNodeFilter {
  kind: FlowNodeKind
  id: string
}

export interface FlowLinkSelection {
  source: string
  target: string
}

export interface FlowBuildOptions {
  role?: FlowRole
  selectedUsers?: string[]
  selectedNodes?: FlowNodeFilter[]
  activeNode?: FlowNodeFilter
  activeLink?: FlowLinkSelection
  colorPalette?: readonly string[]
  visibleStages?: FlowNodeKind[]
  topNodeLimit?: number
  overflowMode?: FlowOverflowMode
  // When true, sensitive node labels (users, tokens, nodes, groups, channels)
  // are partially masked in the rendered graph while keeping node identity so
  // the Sankey shape stays intact.
  maskSensitive?: boolean
  // Resolves the label for a token whose record no longer exists (deleted).
  // Lets the caller inject a localized string such as "Deleted (123)".
  deletedTokenLabel?: (tokenId: number) => string
  otherNodeLabel?: (kind: FlowNodeKind) => string
}

export interface DashboardFlowNode {
  id: string
  label: string
  kind: FlowNodeKind
  value: number
  requests: number
  quota: number
  tokens: number
  color: string
  colorKey: string
  highlighted?: boolean
  dimmed?: boolean
}

export interface DashboardFlowLink {
  source: string
  target: string
  value: number
  requests: number
  quota: number
  tokens: number
  sourceLabel: string
  targetLabel: string
  color: string
  linkColor: string
  linkAlpha: number
  hoverColor: string
  colorKey: string
  share: number
  highlighted?: boolean
  dimmed?: boolean
}

export interface DashboardFlowGraph {
  nodes: DashboardFlowNode[]
  links: DashboardFlowLink[]
}

export interface FlowUserFilterOption {
  value: string
  label: string
  valueLabel: string
  valueRaw: number
  color: string
}

export interface FlowNodeFilterOption {
  kind: FlowNodeKind
  value: string
  label: string
  valueLabel: string
  valueRaw: number
  color: string
}

export interface FlowFilterOptions {
  users: FlowUserFilterOption[]
  nodes: FlowNodeFilterOption[]
}

export interface FlowSummary {
  quota: number
  tokens: number
  requests: number
}

export interface ProcessedFlowData {
  summary: FlowSummary
  flow: DashboardFlowGraph
  filterOptions: FlowFilterOptions
}

// ============================================================================
// Uptime Monitoring Types
// ============================================================================

export interface UptimeMonitor {
  name: string
  uptime: number
  status: number
  group?: string
}

export interface UptimeGroupResult {
  categoryName: string
  monitors: UptimeMonitor[]
}

// ============================================================================
// Dashboard Filter Types
// ============================================================================

export interface DashboardFilters {
  start_timestamp?: Date
  end_timestamp?: Date
  time_granularity?: TimeGranularity
  username?: string
}

export type ConsumptionDistributionChartType = 'bar' | 'area'

export type ModelAnalyticsChartTab = 'trend' | 'proportion' | 'top'

export interface DashboardChartPreferences {
  consumptionDistributionChart: ConsumptionDistributionChartType
  modelAnalyticsChart: ModelAnalyticsChartTab
  defaultTimeRangeDays: number
  defaultTimeGranularity: TimeGranularity
}

// User analytics selections are held by the dashboard parent so they survive
// switching between dashboard sub-sections, matching the model/flow filters.
export interface UserChartsFilters {
  timeGranularity: TimeGranularity
  selectedRange: number | null
  customStart?: Date
  customEnd?: Date
  topUserLimit: number
}

// ============================================================================
// API Info Types
// ============================================================================

export interface ApiInfoItem {
  url: string
  route: string
  description: string
  color: string
}

export interface PingStatus {
  latency: number | null
  testing: boolean
  error: boolean
}

export type PingStatusMap = Record<string, PingStatus>

// ============================================================================
// Chart Types
// ============================================================================

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type VChartSpec = Record<string, any>

export interface ProcessedChartData {
  spec_pie: VChartSpec
  spec_line: VChartSpec
  spec_area: VChartSpec
  spec_model_line: VChartSpec
  spec_rank_bar: VChartSpec
  totalQuotaDisplay: string
  totalCountDisplay: string
}

export interface ProcessedUserChartData {
  spec_user_rank: VChartSpec
  spec_user_trend: VChartSpec
}

// ============================================================================
// Announcement Types
// ============================================================================

export interface AnnouncementItem {
  id?: number
  content: string
  publishDate?: string
  type?: 'default' | 'ongoing' | 'success' | 'warning' | 'error'
  extra?: string
}

// ============================================================================
// FAQ Types
// ============================================================================

export interface FAQItem {
  id?: number
  question: string
  answer: string
}
