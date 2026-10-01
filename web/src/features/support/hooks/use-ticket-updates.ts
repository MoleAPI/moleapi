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
import { useEffect, useState } from 'react'

import { getSupportTicketUpdates, type SupportTicket } from '../api'

export function isSupportWorkingTime(now = Date.now()) {
  // ponytail: Taipei uses UTC+8 year-round; use an IANA formatter if the schedule becomes configurable.
  const time = new Date(now + 8 * 60 * 60_000)
  return (
    time.getUTCDay() > 0 &&
    time.getUTCDay() < 6 &&
    time.getUTCHours() >= 9 &&
    time.getUTCHours() < 18
  )
}

export function useSupportWorkingHours() {
  const [now, setNow] = useState(Date.now)
  const [visible, setVisible] = useState(
    () => document.visibilityState !== 'hidden'
  )
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 60_000)
    const onVisibility = () => {
      setVisible(document.visibilityState !== 'hidden')
      setNow(Date.now())
    }
    document.addEventListener('visibilitychange', onVisibility)
    return () => {
      window.clearInterval(timer)
      document.removeEventListener('visibilitychange', onVisibility)
    }
  }, [])
  return visible && isSupportWorkingTime(now)
}

export function useTicketUpdates(
  ticket: SupportTicket | undefined,
  isAdmin: boolean,
  hasStaffReply: boolean,
  userId: number | undefined,
  refresh: () => Promise<unknown>
) {
  const workingHours = useSupportWorkingHours()
  const createdDay = Math.floor(
    (Date.parse(ticket?.createdTime ?? '') + 8 * 60 * 60_000) / 86_400_000
  )
  const today = Math.floor((Date.now() + 8 * 60 * 60_000) / 86_400_000)
  const eligible = Boolean(
    ticket &&
    userId &&
    !ticket.isArchived &&
    (ticket.statusType || ticket.status) !== 'Closed' &&
    (isAdmin || (!hasStaffReply && createdDay === today))
  )
  const interval = (isAdmin ? 30 : 5) * 60_000
  const updates = useQuery({
    queryKey: ['support', 'ticket-updates', userId, isAdmin, ticket?.id],
    queryFn: () => {
      if (!ticket) throw new Error('Ticket ID is required')
      if (!isSupportWorkingTime() || document.visibilityState === 'hidden') {
        return { modifiedTime: ticket.modifiedTime }
      }
      return getSupportTicketUpdates(ticket.id)
    },
    initialData: ticket ? { modifiedTime: ticket.modifiedTime } : undefined,
    enabled: eligible && workingHours,
    staleTime: interval,
    refetchInterval: eligible && workingHours ? interval : false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
    retry: false,
    meta: { errorToast: false },
  })
  useEffect(() => {
    if (
      eligible &&
      workingHours &&
      updates.data?.modifiedTime &&
      updates.data.modifiedTime !== ticket?.modifiedTime
    ) {
      void refresh()
    }
  }, [
    eligible,
    workingHours,
    updates.data,
    updates.dataUpdatedAt,
    ticket?.modifiedTime,
    refresh,
  ])
}
