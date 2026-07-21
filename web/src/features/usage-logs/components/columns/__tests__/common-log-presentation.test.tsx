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

import i18next from 'i18next'
import type { ComponentType } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { I18nextProvider, initReactI18next } from 'react-i18next'

import { usageLogSchema, type UsageLog } from '../../../data/schema'
import { UsageLogsProvider } from '../../usage-logs-provider'
import { useCommonLogsColumns } from '../common-logs-columns'

const log = usageLogSchema.parse({
  id: 1,
  user_id: 1,
  created_at: 1_700_000_000,
  type: 2,
  content: '',
  token_name: 'mole-token',
  model_name: 'gpt-4o-mini',
  quota: 25_000,
  prompt_tokens: 1_200,
  completion_tokens: 300,
  channel: 7,
  channel_name: 'Primary',
  group: 'default',
  other: JSON.stringify({
    cache_tokens: 240,
    cache_creation_tokens: 80,
  }),
})

function TestCell(props: {
  columnId: string
  isAdmin?: boolean
  value: UsageLog
}) {
  const column = useCommonLogsColumns(props.isAdmin ?? false).find(
    (item) =>
      item.id === props.columnId ||
      ('accessorKey' in item && item.accessorKey === props.columnId)
  )
  assert.ok(column && typeof column.cell === 'function')

  const Cell = column.cell as ComponentType<{
    row: {
      original: UsageLog
      getValue: (key: keyof UsageLog) => UsageLog[keyof UsageLog]
    }
  }>

  return (
    <Cell
      row={{
        original: props.value,
        getValue: (key) => props.value[key],
      }}
    />
  )
}

function TestDesktopLayout() {
  const columns = useCommonLogsColumns(true)
  const totalWidth = columns.reduce(
    (total, column) => total + (column.size ?? 0),
    0
  )

  return (
    <output data-all-sized={columns.every((column) => column.size != null)}>
      {totalWidth}
    </output>
  )
}

async function renderCell(columnId: string, isAdmin = false) {
  const i18n = i18next.createInstance()
  await i18n.use(initReactI18next).init({ lng: 'en' })

  return renderToStaticMarkup(
    <I18nextProvider i18n={i18n}>
      <UsageLogsProvider>
        <TestCell columnId={columnId} isAdmin={isAdmin} value={log} />
      </UsageLogsProvider>
    </I18nextProvider>
  )
}

test('channel, token, group, and model use colored rounded labels', async () => {
  const channelHtml = await renderCell('channel', true)
  const tokenHtml = await renderCell('token_name')
  const modelHtml = await renderCell('model_name')

  assert.match(channelHtml, /data-slot="status-badge"/)
  assert.match(channelHtml, /bg-(?:chart|success|warning|info|muted)/)
  assert.equal(tokenHtml.match(/data-slot="status-badge"/g)?.length, 2)
  assert.equal(
    tokenHtml.match(/\bbg-(?:chart|success|warning|info|muted)/g)?.length,
    2
  )
  assert.match(modelHtml, /data-slot="status-badge"/)
  assert.match(modelHtml, /bg-(?:chart|success|warning|info|muted)/)
})

test('cache uses explicit words and numeric cost stays unboxed', async () => {
  const tokensHtml = await renderCell('prompt_tokens')
  const costHtml = await renderCell('quota')

  assert.match(tokensHtml, /Cache Read[^<]*240/)
  assert.match(tokensHtml, /Cache Write[^<]*80/)
  assert.doesNotMatch(tokensHtml, /[↓↑]/)
  assert.match(costHtml, /tabular-nums/)
  assert.doesNotMatch(costHtml, /\b(?:border|rounded|bg-)/)
})

test('desktop common logs keep full values on one horizontally scrollable row', async () => {
  const i18n = i18next.createInstance()
  await i18n.use(initReactI18next).init({ lng: 'en' })

  const layoutHtml = renderToStaticMarkup(
    <I18nextProvider i18n={i18n}>
      <UsageLogsProvider>
        <TestDesktopLayout />
      </UsageLogsProvider>
    </I18nextProvider>
  )
  const timeHtml = await renderCell('created_at', true)
  const tokensHtml = await renderCell('prompt_tokens', true)
  const modelHtml = await renderCell('model_name', true)

  assert.match(layoutHtml, /data-all-sized="true">1700</)
  assert.match(timeHtml, /items-center/)
  assert.doesNotMatch(timeHtml, /font-mono[^"]*truncate/)
  assert.match(tokensHtml, /flex-nowrap/)
  assert.doesNotMatch(tokensHtml, /flex-wrap/)
  assert.doesNotMatch(tokensHtml, /overflow-hidden/)
  assert.doesNotMatch(modelHtml, /truncate whitespace-nowrap/)
})
