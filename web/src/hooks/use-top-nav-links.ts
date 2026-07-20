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
import {
  DashboardSquare01Icon,
  File01Icon,
  Home01Icon,
  InformationCircleIcon,
  RankingIcon,
  SaleTag01Icon,
} from '@hugeicons/core-free-icons'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import type { TopNavLink } from '@/components/layout/types'
import { useStatus } from '@/hooks/use-status'
import { parseHeaderNavModulesFromStatus } from '@/lib/nav-modules'
import { useAuthStore } from '@/stores/auth-store'

export type { TopNavLink } from '@/components/layout/types'

/**
 * Generate top navigation links based on HeaderNavModules configuration from backend /api/status
 * Backend format example (stringified JSON):
 * {
 *   home: true,
 *   console: true,
 *   pricing: { enabled: true, requireAuth: false },
 *   rankings: { enabled: true, requireAuth: false },
 *   docs: true,
 *   about: true
 * }
 */
export function useTopNavLinks(): TopNavLink[] {
  const { t } = useTranslation()
  const { status } = useStatus()
  const { auth } = useAuthStore()

  // Parse HeaderNavModules
  const modules = useMemo(() => {
    return parseHeaderNavModulesFromStatus(
      status as Record<string, unknown> | null
    )
  }, [status])

  // Documentation link (may be external)
  const docsLink: string | undefined = status?.docs_link as string | undefined

  const isAuthed = !!auth?.user

  const links: TopNavLink[] = []

  // Home
  if (modules?.home !== false) {
    links.push({
      title: t('Home'),
      href: '/',
      icon: Home01Icon,
      iconClassName: 'text-blue-500',
    })
  }

  // Console -> /dashboard (new console path)
  if (modules?.console !== false) {
    links.push({
      title: t('Console'),
      href: '/dashboard',
      icon: DashboardSquare01Icon,
      iconClassName: 'text-violet-500',
    })
  }

  // Pricing
  const pricing = modules?.pricing
  if (pricing && typeof pricing === 'object' && pricing.enabled) {
    const requiresAuth = pricing.requireAuth && !isAuthed
    links.push({
      title: t('Model Square'),
      href: '/pricing',
      requiresAuth,
      icon: SaleTag01Icon,
      iconClassName: 'text-emerald-500',
    })
  }

  // Rankings
  const rankings = modules?.rankings
  if (rankings && typeof rankings === 'object' && rankings.enabled) {
    const requiresAuth = rankings.requireAuth && !isAuthed
    links.push({
      title: t('Rankings'),
      href: '/rankings',
      requiresAuth,
      icon: RankingIcon,
      iconClassName: 'text-amber-500',
    })
  }

  // Docs (supports external links)
  if (modules?.docs !== false) {
    if (docsLink) {
      links.push({
        title: t('Docs'),
        href: docsLink,
        external: true,
        icon: File01Icon,
        iconClassName: 'text-sky-500',
      })
    } else {
      links.push({
        title: t('Docs'),
        href: '/docs',
        icon: File01Icon,
        iconClassName: 'text-sky-500',
      })
    }
  }

  // About
  if (modules?.about !== false) {
    links.push({
      title: t('About'),
      href: '/about',
      icon: InformationCircleIcon,
      iconClassName: 'text-rose-500',
    })
  }

  return links
}
