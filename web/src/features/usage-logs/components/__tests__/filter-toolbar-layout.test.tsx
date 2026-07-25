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
import { test } from 'node:test'

import type { Table } from '@tanstack/react-table'
import i18next from 'i18next'
import type { ReactNode } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { I18nextProvider, initReactI18next } from 'react-i18next'

import { LogsFilterInput, LogsFilterToolbar } from '../logs-filter-toolbar'

async function renderWithI18n(node: ReactNode) {
  const i18n = i18next.createInstance()
  await i18n.use(initReactI18next).init({ lng: 'en' })

  return renderToStaticMarkup(
    <I18nextProvider i18n={i18n}>{node}</I18nextProvider>
  )
}

const emptyTable = {
  getAllColumns: () => [],
} as unknown as Table<unknown>

test('filter input clips caret rendering to the visible input bounds', async () => {
  const html = await renderWithI18n(
    <LogsFilterInput placeholder='Token Name' value='' />
  )

  assert.match(html, /overflow-hidden/)
  assert.match(html, /rounded-lg/)
})

test('compact toolbar keeps actions in a fixed right-side group', async () => {
  const html = await renderWithI18n(
    <LogsFilterToolbar
      table={emptyTable}
      compactDesktop
      primaryFilters={
        <>
          <span data-filter='type' />
          <span data-filter='request-id' />
        </>
      }
      stats={<span data-stats='logs' />}
      hasActiveFilters
      onReset={() => {}}
      onSearch={() => {}}
    />
  )

  assert.match(html, /lg:flex-row/)
  assert.match(html, /shrink-0/)
  assert.match(html, /justify-end/)
  assert.match(html, /data-stats="logs"/)
})
