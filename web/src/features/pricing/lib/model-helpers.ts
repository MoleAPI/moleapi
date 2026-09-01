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
import { EXCLUDED_GROUPS, FILTER_ALL, QUOTA_TYPE_VALUES } from '../constants'
import type { PricingModel } from '../types'

// ----------------------------------------------------------------------------
// Model Helper Utilities
// ----------------------------------------------------------------------------

/**
 * Get available groups for a model
 */
export function getAvailableGroups(
  model: PricingModel,
  usableGroup: Record<string, { desc: string; ratio: number }>
): string[] {
  const modelEnableGroups = Array.isArray(model.enable_groups)
    ? model.enable_groups
    : []

  return Object.keys(usableGroup)
    .filter((g) => !EXCLUDED_GROUPS.includes(g))
    .filter((g) => modelEnableGroups.includes(g))
    .sort(
      (a, b) =>
        getConfiguredGroupRatio(model.group_ratio || {}, a) -
        getConfiguredGroupRatio(model.group_ratio || {}, b)
    )
}

/**
 * Read a configured group ratio while preserving valid zero ratios.
 */
export function getConfiguredGroupRatio(
  groupRatio: Record<string, number>,
  group: string
): number {
  const ratio = groupRatio[group]
  return typeof ratio === 'number' && Number.isFinite(ratio) ? ratio : 1
}

export function getModelEndpoint(
  model: PricingModel,
  fallback: Record<string, { path?: string; method?: string }>,
  endpointType: string
): { path?: string; method?: string } {
  return (
    model.supported_endpoints?.[endpointType] || fallback[endpointType] || {}
  )
}

/**
 * Resolve the group ratio used by model square summary prices.
 *
 * When no specific group is selected, the model square shows the best price
 * available to the viewer. When a group filter is active, it shows that
 * group's price instead.
 */
export function getDisplayGroupRatio(
  model: PricingModel,
  selectedGroup?: string
): number {
  const modelEnableGroups = Array.isArray(model.enable_groups)
    ? model.enable_groups
    : []
  const groupRatio = model.group_ratio || {}

  if (
    selectedGroup &&
    selectedGroup !== FILTER_ALL &&
    modelEnableGroups.includes(selectedGroup)
  ) {
    return getConfiguredGroupRatio(groupRatio, selectedGroup)
  }

  if (modelEnableGroups.length === 0) {
    return 1
  }

  let minRatio = Number.POSITIVE_INFINITY

  for (const group of modelEnableGroups) {
    const ratio = groupRatio[group]
    if (
      typeof ratio === 'number' &&
      Number.isFinite(ratio) &&
      ratio < minRatio
    ) {
      minRatio = ratio
    }
  }

  return minRatio === Number.POSITIVE_INFINITY ? 1 : minRatio
}

export function getDiscountPercent(ratio: number): number | null {
  if (!Number.isFinite(ratio) || ratio < 0 || ratio >= 1) return null

  const percent = Math.round((1 - ratio) * 100)
  return percent > 0 ? percent : null
}

export function getModelPriceDisplay(
  model: PricingModel,
  selectedGroup?: string
): { isStartingAt: boolean; discountPercent: number | null } {
  const groups = Array.isArray(model.enable_groups) ? model.enable_groups : []
  const hasSelectedGroup = Boolean(
    selectedGroup &&
    selectedGroup !== FILTER_ALL &&
    groups.includes(selectedGroup)
  )
  const ratios = groups.map((group) =>
    getConfiguredGroupRatio(model.group_ratio || {}, group)
  )

  return {
    isStartingAt: !hasSelectedGroup && new Set(ratios).size > 1,
    discountPercent: getDiscountPercent(
      getDisplayGroupRatio(model, selectedGroup)
    ),
  }
}

export function getDisplayedPriceGroups(
  model: PricingModel,
  selectedGroup?: string
): Array<{ group: string; index: number; ratio: number; isCurrent: boolean }> {
  const groups =
    Array.isArray(model.enable_groups) && model.enable_groups.length > 0
      ? model.enable_groups
      : ['default']
  const selected =
    selectedGroup &&
    selectedGroup !== FILTER_ALL &&
    groups.includes(selectedGroup)
      ? selectedGroup
      : ''
  const items = groups.map((group, index) => ({
    group,
    index,
    ratio: getConfiguredGroupRatio(model.group_ratio || {}, group),
    isCurrent: group === selected,
  }))
  const defaultGroup = items.find((item) => item.group === 'default')
  const seenRatios = new Set<string>()

  return items
    .filter((item) => {
      if (
        defaultGroup &&
        item.group !== 'default' &&
        item.ratio === defaultGroup.ratio
      ) {
        return false
      }
      const ratioKey = String(item.ratio)
      if (seenRatios.has(ratioKey)) {
        return false
      }
      seenRatios.add(ratioKey)
      return true
    })
    .sort((a, b) => a.ratio - b.ratio || a.index - b.index)
}

function getDescriptionTranslations(
  model: PricingModel
): Record<string, string> {
  const value = model.description_i18n
  if (!value) return {}
  if (typeof value === 'string') {
    try {
      const parsed = JSON.parse(value)
      return parsed && typeof parsed === 'object' && !Array.isArray(parsed)
        ? (parsed as Record<string, string>)
        : {}
    } catch {
      return {}
    }
  }
  return value
}

function getLanguageCandidates(language?: string): string[] {
  const normalized = (language || '').trim()
  const lower = normalized.toLowerCase()
  const candidates = [normalized]

  if (lower.startsWith('zh-hant') || lower.startsWith('zh-tw')) {
    candidates.push('zh-TW', 'zh')
  } else if (lower.startsWith('zh')) {
    candidates.push('zh')
  } else if (lower) {
    candidates.push(lower.split('-')[0])
  }

  candidates.push('en')
  return candidates.filter(Boolean)
}

export function getLocalizedModelDescription(
  model: PricingModel,
  language?: string
): string {
  const translations = getDescriptionTranslations(model)
  for (const locale of getLanguageCandidates(language)) {
    const description = translations[locale]
    if (typeof description === 'string' && description.trim()) {
      return description
    }
  }
  return model.description || ''
}

export function getModelDescriptionSearchText(model: PricingModel): string {
  return [
    model.description,
    ...Object.values(getDescriptionTranslations(model)),
  ]
    .filter(Boolean)
    .join(' ')
}

/**
 * Replace model placeholder in endpoint path
 */
export function replaceModelInPath(path: string, modelName: string): string {
  return path.replaceAll('{model}', modelName)
}

/**
 * Check if model is token-based pricing
 */
export function isTokenBasedModel(model: PricingModel): boolean {
  return model.quota_type === QUOTA_TYPE_VALUES.TOKEN
}
