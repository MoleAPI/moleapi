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
import i18next from 'i18next'
import { useState, useEffect, useCallback } from 'react'
import { toast } from 'sonner'

import { useIsAdmin } from '@/hooks/use-admin'
import { useDebounce } from '@/hooks/use-debounce'

import {
  getUserBillingHistory,
  getAllBillingHistory,
  completeOrder,
  isApiSuccess,
} from '../api'
import type { TopupRecord } from '../types'

// ============================================================================
// Billing History Hook
// ============================================================================

interface UseBillingHistoryOptions {
  /** Initial page number */
  initialPage?: number
  /** Initial page size */
  initialPageSize?: number
  /** Initial admin user search */
  initialUserKeyword?: string
}

export function useBillingHistory(options: UseBillingHistoryOptions = {}) {
  const {
    initialPage = 1,
    initialPageSize = 10,
    initialUserKeyword = '',
  } = options
  const isAdmin = useIsAdmin()

  const [records, setRecords] = useState<TopupRecord[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(initialPage)
  const [pageSize, setPageSize] = useState(initialPageSize)
  const [keyword, setKeyword] = useState('')
  const [userKeyword, setUserKeyword] = useState(initialUserKeyword)
  const [startTime, setStartTime] = useState('')
  const [endTime, setEndTime] = useState('')
  const [loading, setLoading] = useState(false)
  const [completing, setCompleting] = useState(false)
  const debouncedKeyword = useDebounce(keyword, 500)
  const debouncedUserKeyword = useDebounce(userKeyword, 500)

  /**
   * Fetch billing history
   */
  const fetchBillingHistory = useCallback(async () => {
    setLoading(true)
    try {
      const response = isAdmin
        ? await getAllBillingHistory(page, pageSize, {
            keyword: debouncedKeyword,
            userKeyword: debouncedUserKeyword,
            startTimestamp: startTime
              ? Math.floor(new Date(startTime).getTime() / 1000)
              : undefined,
            endTimestamp: endTime
              ? Math.floor(new Date(endTime).getTime() / 1000)
              : undefined,
          })
        : await getUserBillingHistory(page, pageSize, debouncedKeyword)

      if (isApiSuccess(response) && response.data) {
        setRecords(response.data.items || [])
        setTotal(response.data.total || 0)
      } else {
        toast.error(
          response.message || i18next.t('Failed to load billing history')
        )
        setRecords([])
        setTotal(0)
      }
    } catch (error) {
      // eslint-disable-next-line no-console
      console.error('Failed to fetch billing history:', error)
      toast.error(i18next.t('Failed to load billing history'))
      setRecords([])
      setTotal(0)
    } finally {
      setLoading(false)
    }
  }, [
    isAdmin,
    page,
    pageSize,
    debouncedKeyword,
    debouncedUserKeyword,
    startTime,
    endTime,
  ])

  /**
   * Complete a pending order (admin only)
   */
  const handleCompleteOrder = useCallback(
    async (tradeNo: string) => {
      if (!isAdmin) {
        toast.error(i18next.t('Admin access required'))
        return false
      }

      setCompleting(true)
      try {
        const response = await completeOrder({ trade_no: tradeNo })
        if (isApiSuccess(response)) {
          toast.success(i18next.t('Order completed successfully'))
          // Refresh the list
          await fetchBillingHistory()
          return true
        } else {
          toast.error(response.message || i18next.t('Failed to complete order'))
          return false
        }
      } catch (error) {
        // eslint-disable-next-line no-console
        console.error('Failed to complete order:', error)
        toast.error(i18next.t('Failed to complete order'))
        return false
      } finally {
        setCompleting(false)
      }
    },
    [isAdmin, fetchBillingHistory]
  )

  /**
   * Change page
   */
  const handlePageChange = useCallback((newPage: number) => {
    setPage(newPage)
  }, [])

  /**
   * Change page size
   */
  const handlePageSizeChange = useCallback((newPageSize: number) => {
    setPageSize(newPageSize)
    setPage(1) // Reset to first page when changing page size
  }, [])

  /**
   * Search by keyword
   */
  const handleSearch = useCallback((newKeyword: string) => {
    setKeyword(newKeyword)
    setPage(1) // Reset to first page when searching
  }, [])

  const handleUserSearch = useCallback((newKeyword: string) => {
    setUserKeyword(newKeyword)
    setPage(1)
  }, [])

  const handleStartTimeChange = useCallback((value: string) => {
    setStartTime(value)
    setPage(1)
  }, [])

  const handleEndTimeChange = useCallback((value: string) => {
    setEndTime(value)
    setPage(1)
  }, [])

  const resetFilters = useCallback(() => {
    setKeyword('')
    setUserKeyword('')
    setStartTime('')
    setEndTime('')
    setPage(1)
  }, [])

  // Fetch data when dependencies change
  useEffect(() => {
    fetchBillingHistory()
  }, [fetchBillingHistory])

  return {
    records,
    total,
    page,
    pageSize,
    keyword,
    userKeyword,
    startTime,
    endTime,
    loading,
    completing,
    isAdmin,
    handlePageChange,
    handlePageSizeChange,
    handleSearch,
    handleUserSearch,
    handleStartTimeChange,
    handleEndTimeChange,
    resetFilters,
    handleCompleteOrder,
    refresh: fetchBillingHistory,
  }
}
