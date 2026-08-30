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
import { useQuery } from '@tanstack/react-query'
import { getRouteApi } from '@tanstack/react-router'
import type { ColumnDef } from '@tanstack/react-table'
import { Fragment, useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  DataTablePage,
  DataTableRow,
  useDataTable,
} from '@/components/data-table'
import { TableCell, TableRow } from '@/components/ui/table'
import { useMediaQuery } from '@/hooks'
import { useTableUrlState } from '@/hooks/use-table-url-state'
import { cn } from '@/lib/utils'

import {
  DEFAULT_LOGS_DATA,
  LOG_TYPE_ALL_VALUE,
  LOG_TYPE_ENUM,
} from '../constants'
import type { UsageLog } from '../data/schema'
import { useColumnsByCategory } from '../lib/columns'
import { buildQuickFilterSearch } from '../lib/filter'
import { parseLogOther } from '../lib/format'
import { fetchLogsByCategory } from '../lib/utils'
import type { CommonQuickFilterField, LogCategory } from '../types'
import { CommonLogsFilterBar } from './common-logs-filter-bar'
import { InlineLogDetails } from './dialogs/details-dialog'
import { TaskLogsFilterBar } from './task-logs-filter-bar'
import { UsageLogsMobileList } from './usage-logs-mobile-card'
import { useLogsViewScope, type LogsViewAccess } from './usage-logs-provider'

const route = getRouteApi('/_authenticated/usage-logs/$section')

const logTypeRowTint: Record<number, string> = {
  [LOG_TYPE_ENUM.ERROR]: 'bg-rose-50/40 dark:bg-rose-950/20',
  [LOG_TYPE_ENUM.REFUND]: 'bg-blue-50/30 dark:bg-blue-950/15',
}

// Warning tint for logs where a quota conversion saturated (admin-only marker).
// Takes precedence over the per-type tint since it flags a billing anomaly.
const quotaSaturationRowTint = 'bg-amber-50/60 dark:bg-amber-950/25'

function getColumnVisibilityStorageKey(
  logCategory: LogCategory,
  viewAccess: LogsViewAccess
): string {
  return `usage-logs:${logCategory}:${viewAccess}:column-visibility`
}

function deserializeLogTypeFilter(value: unknown): unknown[] {
  let values: unknown[] = []
  if (Array.isArray(value)) {
    values = value
  } else if (value) {
    values = [value]
  }
  return values.filter((item) => String(item) !== LOG_TYPE_ALL_VALUE)
}

interface UsageLogsTableProps {
  logCategory: LogCategory
}

export function UsageLogsTable({ logCategory }: UsageLogsTableProps) {
  const { t } = useTranslation()
  const {
    isAdminView: isAdmin,
    isRootView: isRoot,
    viewAccess,
  } = useLogsViewScope()
  const isMobile = useMediaQuery('(max-width: 640px)')
  const searchParams = route.useSearch()
  const navigate = route.useNavigate()
  const [expandedLogId, setExpandedLogId] = useState<string | null>(null)

  const {
    columnFilters,
    onColumnFiltersChange,
    pagination,
    onPaginationChange,
    ensurePageInRange,
  } = useTableUrlState({
    search: searchParams,
    navigate,
    pagination: { defaultPage: 1, defaultPageSize: isMobile ? 20 : 100 },
    globalFilter: { enabled: false },
    columnFilters: [
      {
        columnId: 'type',
        searchKey: 'type',
        type: 'array' as const,
        deserialize: deserializeLogTypeFilter,
      },
      { columnId: 'model_name', searchKey: 'model', type: 'string' as const },
      { columnId: 'token_name', searchKey: 'token', type: 'string' as const },
      { columnId: 'group', searchKey: 'group', type: 'string' as const },
      ...(isAdmin
        ? [
            {
              columnId: 'channel',
              searchKey: 'channel',
              type: 'string' as const,
            },
            {
              columnId: 'username',
              searchKey: 'username',
              type: 'string' as const,
            },
          ]
        : []),
    ],
  })

  const { data, isLoading, isFetching } = useQuery({
    queryKey: [
      'logs',
      logCategory,
      viewAccess,
      pagination.pageIndex + 1,
      pagination.pageSize,
      columnFilters,
      searchParams,
      t,
    ],
    queryFn: async () => {
      const result = await fetchLogsByCategory({
        logCategory,
        isAdmin,
        page: pagination.pageIndex + 1,
        pageSize: pagination.pageSize,
        searchParams,
        columnFilters,
      })

      if (!result?.success) {
        toast.error(result?.message || t('Failed to load logs'))
        return DEFAULT_LOGS_DATA
      }

      return result.data || DEFAULT_LOGS_DATA
    },
    placeholderData: (previousData, previousQuery) => {
      if (
        previousQuery?.queryKey[1] === logCategory &&
        previousQuery.queryKey[2] === viewAccess
      ) {
        return previousData
      }
      return undefined
    },
  })

  const logs = data?.items || []
  const handleQuickFilter = useCallback(
    (field: CommonQuickFilterField, value: string) => {
      void navigate({
        search: (current) =>
          buildQuickFilterSearch(
            current as Record<string, unknown>,
            field,
            value
          ),
      })
    },
    [navigate]
  )
  const columns = useColumnsByCategory(
    logCategory,
    isAdmin,
    isRoot,
    logCategory === 'common' ? handleQuickFilter : undefined
  )
  const isLoadingData = isLoading || (isFetching && !data)

  const { table } = useDataTable({
    data: logs as Record<string, unknown>[],
    columns: columns as ColumnDef<Record<string, unknown>>[],
    columnFilters,
    columnVisibilityStorageKey: getColumnVisibilityStorageKey(
      logCategory,
      viewAccess
    ),
    pagination,
    enableRowSelection: false,
    enableSorting: true,
    // ponytail: sort the loaded page only; add API sorting if cross-page order is needed.
    withSortedRowModel: true,
    getRowId: (row) => String(row.id),
    onPaginationChange,
    onColumnFiltersChange,
    manualPagination: true,
    manualFiltering: true,
    totalCount: data?.total || 0,
    ensurePageInRange,
  })

  const isCommon = logCategory === 'common'

  return (
    <DataTablePage
      table={table}
      columns={columns as ColumnDef<Record<string, unknown>>[]}
      isLoading={isLoadingData}
      isFetching={isFetching}
      emptyTitle={t('No Logs Found')}
      emptyDescription={t(
        'No usage logs available. Logs will appear here once API calls are made.'
      )}
      skeletonKeyPrefix='usage-log-skeleton'
      applyHeaderSize
      tableClassName={cn(
        '[&_[data-slot=table]]:text-xs [&_[data-slot=table]_td]:text-xs [&_[data-slot=table]_td_*]:text-xs [&_[data-slot=table]_th]:text-xs [&_[data-slot=table]_th_*]:text-xs'
      )}
      mobile={
        <UsageLogsMobileList
          table={table}
          isLoading={isLoadingData}
          logCategory={logCategory}
          onQuickFilter={handleQuickFilter}
        />
      }
      toolbar={
        isCommon ? (
          <CommonLogsFilterBar table={table} />
        ) : (
          <TaskLogsFilterBar table={table} logCategory={logCategory} />
        )
      }
      renderRow={(row) => {
        const log = row.original as unknown as UsageLog
        const logType = log.type as number | undefined
        const rowExpanded = isCommon && expandedLogId === String(log.id)
        let tintClass =
          isCommon && logType != null ? (logTypeRowTint[logType] ?? '') : ''
        if (isCommon && isAdmin) {
          const other = parseLogOther(log.other ?? '')
          if (other?.admin_info?.quota_saturation) {
            tintClass = quotaSaturationRowTint
          }
        }

        const toggleExpanded = () => {
          if (!isCommon) return
          const logId = String(log.id)
          setExpandedLogId((current) => (current === logId ? null : logId))
        }

        return (
          <Fragment key={row.id}>
            <DataTableRow
              row={row}
              className={cn(
                'transition-colors',
                isCommon &&
                  'focus-visible:ring-ring cursor-pointer outline-none focus-visible:ring-2 focus-visible:ring-inset',
                rowExpanded && 'bg-muted/40',
                tintClass
              )}
              getColumnClassName={() => (isCommon ? 'py-1.5' : 'py-3.5')}
              onClick={(event) => {
                if (
                  (event.target as HTMLElement).closest(
                    'button, a, input, select, textarea, [role="button"]'
                  )
                ) {
                  return
                }
                toggleExpanded()
              }}
              onKeyDown={(event) => {
                if (event.target !== event.currentTarget) return
                if (event.key === 'Enter' || event.key === ' ') {
                  event.preventDefault()
                  toggleExpanded()
                }
              }}
              tabIndex={isCommon ? 0 : undefined}
              aria-expanded={isCommon ? rowExpanded : undefined}
              title={isCommon ? t('Click to view full details') : undefined}
            />
            {rowExpanded && (
              <TableRow className='bg-muted/20 hover:bg-muted/20'>
                <TableCell
                  colSpan={row.getVisibleCells().length}
                  className='px-4 py-3'
                >
                  <InlineLogDetails log={log} isAdmin={isAdmin} />
                </TableCell>
              </TableRow>
            )}
          </Fragment>
        )
      }}
    />
  )
}
