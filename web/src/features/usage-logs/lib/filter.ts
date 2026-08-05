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
/**
 * Utility functions for usage logs filters
 */
import { LOG_CATEGORY_LABELS } from '../constants'
import type {
  LogCategory,
  LogFilters,
  CommonLogFilters,
  DrawingLogFilters,
  TaskLogFilters,
  CommonQuickFilterField,
} from '../types'

// ============================================================================
// Filter Building Functions
// ============================================================================

function cleanTextFilter(value: string | undefined): string | undefined {
  const nextValue = value?.trim()
  return nextValue ? nextValue : undefined
}

export function buildQuickFilterSearch(
  search: Record<string, unknown>,
  field: CommonQuickFilterField,
  value: string
): Record<string, unknown> {
  return {
    ...search,
    page: 1,
    [field]: field === 'type' ? [value] : value,
  }
}

/**
 * Build search params from filters based on log category
 */
export function buildSearchParams(
  filters: LogFilters,
  logCategory: LogCategory
): Record<string, unknown> {
  const channel = cleanTextFilter(filters.channel)
  const baseParams: Record<string, unknown> = {
    ...(filters.startTime && { startTime: filters.startTime.getTime() }),
    ...(filters.endTime && { endTime: filters.endTime.getTime() }),
    ...(channel && { channel }),
  }

  switch (logCategory) {
    case 'common': {
      const commonFilters = filters as CommonLogFilters
      const model = cleanTextFilter(commonFilters.model)
      const token = cleanTextFilter(commonFilters.token)
      const group = cleanTextFilter(commonFilters.group)
      const username = cleanTextFilter(commonFilters.username)
      const requestId =
        cleanTextFilter(commonFilters.requestId) ??
        cleanTextFilter(commonFilters.upstreamRequestId)
      return {
        ...baseParams,
        ...(model && { model }),
        ...(token && { token }),
        ...(group && { group }),
        ...(username && { username }),
        ...(requestId && { requestId }),
      }
    }
    case 'drawing': {
      const drawingFilters = filters as DrawingLogFilters
      const mjId = cleanTextFilter(drawingFilters.mjId)
      return {
        ...baseParams,
        ...(mjId && { filter: mjId }),
      }
    }
    case 'task': {
      const taskFilters = filters as TaskLogFilters
      const taskId = cleanTextFilter(taskFilters.taskId)
      return {
        ...baseParams,
        ...(taskId && { filter: taskId }),
      }
    }
    default:
      return baseParams
  }
}

/**
 * Get log category display name
 */
export function getLogCategoryLabel(category: LogCategory): string {
  return LOG_CATEGORY_LABELS[category]
}
