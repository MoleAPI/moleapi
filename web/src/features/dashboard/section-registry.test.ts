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
import assert from 'node:assert/strict'

import type { TFunction } from 'i18next'
import { test } from 'vitest'

import { getDashboardSectionNavItems } from './section-registry'

const translate = ((key: string) => key) as TFunction

test('channel success rate follows model analytics and is admin-only', () => {
  const adminTitles = getDashboardSectionNavItems(translate, {
    isAdmin: true,
  }).map((item) => item.title)
  const userTitles = getDashboardSectionNavItems(translate).map(
    (item) => item.title
  )

  assert.equal(
    adminTitles.indexOf('Channel Success Rate'),
    adminTitles.indexOf('Model Call Analytics') + 1
  )
  assert.equal(userTitles.includes('Channel Success Rate'), false)
})
