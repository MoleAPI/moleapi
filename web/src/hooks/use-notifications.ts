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
import { useState, useMemo, useEffect } from 'react'

import { getSupportConfig, getSupportTickets } from '@/features/support/api'
import {
  isSupportWorkingTime,
  useSupportWorkingHours,
} from '@/features/support/hooks/use-ticket-updates'
import { useIsAdmin } from '@/hooks/use-admin'
import { useStatus } from '@/hooks/use-status'
import { getNotice } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'
import { useAuthStore } from '@/stores/auth-store'
import { useNotificationStore } from '@/stores/notification-store'

export type NotificationTab = 'notice' | 'announcements' | 'tickets'

function hashString(input: string): string {
  let hash = 0
  if (!input) return '0'

  for (let i = 0; i < input.length; i += 1) {
    const chr = input.charCodeAt(i)
    hash = (hash << 5) - hash + chr
    hash |= 0
  }

  return hash.toString(36)
}

/**
 * Generate a unique key for an announcement
 * Prefer backend id, fall back to a content hash so edits register
 */
function getAnnouncementKey(item: Record<string, unknown>): string {
  if (!item) return ''

  if (item.id !== undefined && item.id !== null) {
    return `id:${item.id}`
  }

  const fingerprint = JSON.stringify({
    publishDate: (item?.publishDate as string) || '',
    content: ((item?.content as string) || '').trim(),
    extra: ((item?.extra as string) || '').trim(),
    type: (item?.type as string) || '',
    title: ((item?.title as string) || '').trim(),
    link: ((item?.link as string) || '').trim(),
  })
  return `hash:${hashString(fingerprint)}`
}

/**
 * Hook to manage notices, announcements, and ticket updates
 * Provides unread counts and read status management
 */
export function useNotifications() {
  const [popoverOpen, setPopoverOpen] = useState(false)
  const [activeTab, setActiveTab] = useState<NotificationTab>('notice')
  const user = useAuthStore((state) => state.auth.user)
  const isAdmin = useIsAdmin()
  const workingHours = useSupportWorkingHours()
  const supportConfig = useQuery({
    queryKey: ['support', 'config'],
    queryFn: getSupportConfig,
    enabled: Boolean(user),
    staleTime: 60_000,
    meta: { errorToast: false },
  })
  const supportEnabled = Boolean(
    user && supportConfig.data?.enabled && (isAdmin || user.email)
  )
  const support = useQuery({
    queryKey: [
      'support',
      'tickets',
      'notifications',
      user?.id,
      user?.role,
      user?.email,
    ],
    queryFn: () => getSupportTickets(0, 'notifications'),
    enabled:
      supportEnabled &&
      ((isAdmin && workingHours) || (popoverOpen && activeTab === 'tickets')),
    // ponytail: admins poll one recent page; users fetch notifications on demand.
    staleTime: (isAdmin ? 30 : 5) * 60_000,
    gcTime: 30 * 60_000,
    retry: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
    meta: { errorToast: false },
  })

  const refetchSupport = support.refetch
  useEffect(() => {
    if (!isAdmin || !supportEnabled || !workingHours) return
    const timer = window.setInterval(() => {
      // Recheck at execution time, including timers delayed while the tab slept.
      if (isSupportWorkingTime() && document.visibilityState !== 'hidden') {
        void refetchSupport()
      }
    }, 30 * 60_000)
    return () => window.clearInterval(timer)
  }, [isAdmin, supportEnabled, workingHours, refetchSupport])

  // Fetch Notice from API
  const {
    data: noticeResponse,
    isLoading: noticeLoading,
    refetch: refetchNotice,
  } = useQuery({
    queryKey: ['notice'],
    queryFn: async () => requireServerSuccess(await getNotice()),
    staleTime: 1000 * 60 * 5, // 5 minutes
  })

  // Fetch Announcements from status
  const { status, loading: statusLoading } = useStatus()
  const announcementsEnabled = status?.announcements_enabled ?? false
  const announcements = useMemo<Record<string, unknown>[]>(() => {
    if (!announcementsEnabled) return []
    return ((status?.announcements || []) as Record<string, unknown>[]).slice(
      0,
      20
    )
  }, [announcementsEnabled, status?.announcements])

  // Notification store
  const {
    lastReadNotice,
    markNoticeRead,
    markAnnouncementsRead,
    isAnnouncementRead,
    readSupportTickets,
  } = useNotificationStore()

  const ticketNotifications = supportEnabled
    ? (support.data?.tickets ?? [])
        .filter(
          (ticket) =>
            !ticket.isArchived &&
            (isAdmin
              ? ticket.activity === 'new' || ticket.activity === 'customer'
              : ticket.activity === 'agent')
        )
        .map((ticket) => ({
          ...ticket,
          unread:
            readSupportTickets[`${user?.id}:${ticket.id}`] !==
            (ticket.modifiedTime || ticket.createdTime),
        }))
    : []
  const unreadTicketCount = ticketNotifications.filter(
    (ticket) => ticket.unread
  ).length

  // Extract notice content
  const noticeContent = noticeResponse?.success
    ? (noticeResponse.data || '').trim()
    : ''

  // Calculate unread counts
  const unreadCounts = useMemo(() => {
    const noticeUnread =
      noticeContent && noticeContent !== lastReadNotice ? 1 : 0

    const announcementsUnread = announcements.filter(
      (item: Record<string, unknown>) => {
        const key = getAnnouncementKey(item)
        return !isAnnouncementRead(key)
      }
    ).length

    return {
      notice: noticeUnread,
      announcements: announcementsUnread,
      total: noticeUnread + announcementsUnread,
    }
  }, [noticeContent, lastReadNotice, announcements, isAnnouncementRead])

  const markAnnouncementsAsRead = () => {
    if (announcements.length > 0) {
      const allKeys = announcements.map((item: Record<string, unknown>) =>
        getAnnouncementKey(item)
      )
      markAnnouncementsRead(allKeys)
    }
  }

  // Handle popover open
  const handleOpenPopover = (tab?: NotificationTab) => {
    const nextTab = tab || (unreadTicketCount > 0 ? 'tickets' : activeTab)

    // Mark currently visible content as read when opening the notification center
    if (nextTab === 'notice' && noticeContent) {
      markNoticeRead(noticeContent)
    }
    if (nextTab === 'announcements') {
      markAnnouncementsAsRead()
    }

    setActiveTab(nextTab)
    setPopoverOpen(true)
  }

  const handlePopoverOpenChange = (open: boolean) => {
    if (open) {
      handleOpenPopover()
      return
    }

    setPopoverOpen(false)
  }

  // Handle tab change - mark announcements as read when switching to that tab
  const handleTabChange = (tab: NotificationTab) => {
    setActiveTab(tab)

    if (tab === 'notice' && noticeContent) {
      markNoticeRead(noticeContent)
    }
    if (tab === 'announcements') {
      markAnnouncementsAsRead()
    }
  }

  return {
    // Data
    notice: noticeContent,
    announcements,
    loading: noticeLoading || statusLoading,

    // Unread counts
    unreadCount: unreadCounts.total + unreadTicketCount,
    support: {
      enabled: supportEnabled,
      tickets: ticketNotifications,
      loading: support.isLoading,
      error: support.isError,
      retry: () => {
        void support.refetch()
      },
      unreadCount: unreadTicketCount,
    },
    unreadNoticeCount: unreadCounts.notice,
    unreadAnnouncementsCount: unreadCounts.announcements,

    // Popover state
    popoverOpen,
    setPopoverOpen: handlePopoverOpenChange,
    activeTab,
    setActiveTab: handleTabChange,

    // Actions
    openPopover: handleOpenPopover,
    closePopover: () => setPopoverOpen(false),
    refetchNotice,
  }
}
