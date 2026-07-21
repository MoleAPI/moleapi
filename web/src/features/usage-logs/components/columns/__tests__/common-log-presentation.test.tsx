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
import { InlineLogDetails } from '../../dialogs/details-dialog'
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
  use_time: 1,
  is_stream: true,
  channel: 7,
  channel_name: 'Primary',
  group: 'default',
  request_id: '202607211431014826310688268d9d6UJDWU9My',
  other: JSON.stringify({
    admin_info: {
      use_channel: [7, 9],
    },
    cache_tokens: 240,
    cache_creation_tokens: 80,
    model_ratio: 0.07,
    completion_ratio: 4,
    cache_ratio: 0.1,
    cache_creation_ratio: 1.25,
    group_ratio: 1,
    frt: 600,
    request_path: '/v1/chat/completions',
    request_conversion: ['OpenAI Compatible'],
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
  const columnIds = new Set(
    columns.map((column) =>
      'accessorKey' in column ? column.accessorKey : column.id
    )
  )
  const totalWidth = columns.reduce(
    (total, column) => total + (column.size ?? 0),
    0
  )

  return (
    <output
      data-all-sized={columns.every((column) => column.size != null)}
      data-has-type-column={columnIds.has('type')}
      data-has-stream-column={columnIds.has('is_stream')}
      data-column-order={columns
        .map((column) =>
          'accessorKey' in column ? column.accessorKey : column.id
        )
        .join('|')}
    >
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

async function renderInlineDetails() {
  const i18n = i18next.createInstance()
  await i18n.use(initReactI18next).init({ lng: 'en' })

  return renderToStaticMarkup(
    <I18nextProvider i18n={i18n}>
      <InlineLogDetails log={log} isAdmin />
    </I18nextProvider>
  )
}

test('type, channel, token, group, and model use colored rounded labels', async () => {
  const typeHtml = await renderCell('type')
  const channelHtml = await renderCell('channel', true)
  const tokenHtml = await renderCell('token_name')
  const groupHtml = await renderCell('group')
  const modelHtml = await renderCell('model_name')

  assert.match(typeHtml, /data-slot="status-badge"/)
  assert.match(typeHtml, /Consume/)
  assert.match(channelHtml, /data-slot="status-badge"/)
  assert.match(channelHtml, /bg-(?:chart|success|warning|info|muted)/)
  assert.equal(tokenHtml.match(/data-slot="status-badge"/g)?.length, 1)
  assert.match(groupHtml, /data-slot="status-badge"/)
  assert.match(groupHtml, /bg-(?:chart|success|warning|info|muted)/)
  assert.match(groupHtml, /truncate/)
  assert.match(groupHtml, /font-normal/)
  assert.match(modelHtml, /data-slot="status-badge"/)
  assert.match(modelHtml, /bg-(?:chart|success|warning|info|muted)/)
})

test('cache uses explicit words and numeric cost stays unboxed', async () => {
  const tokensHtml = await renderCell('prompt_tokens')
  const costHtml = await renderCell('quota')

  assert.match(tokensHtml, /Cache Read[^<]*240/)
  assert.match(tokensHtml, /Cache Write[^<]*80/)
  assert.match(tokensHtml, /!\s*text-\[12px\]|!text-\[12px\]/)
  assert.match(tokensHtml, /!\s*text-\[9px\]|!text-\[9px\]/)
  assert.doesNotMatch(tokensHtml, /[↓↑]/)
  assert.match(costHtml, /tabular-nums/)
  assert.doesNotMatch(costHtml, /\b(?:border|rounded|bg-)/)
})

test('timing and stream render as separate pill columns', async () => {
  const timingHtml = await renderCell('use_time')
  const streamHtml = await renderCell('is_stream')

  assert.equal(timingHtml.match(/rounded-lg/g)?.length, 2)
  assert.doesNotMatch(timingHtml, /Stream/)
  assert.match(streamHtml, /rounded-lg/)
  assert.match(streamHtml, /Stream/)
  assert.match(streamHtml, /300 t\/s/)
})

test('details preview uses the effective multiplier instead of standard', async () => {
  const detailsHtml = await renderCell('content')

  assert.match(detailsHtml, /1\.0 · \$0\.14 \/ \$0\.56\/M/)
  assert.doesNotMatch(detailsHtml, /Standard/)
})

test('expanded details show request summary, cache tokens, and billing calculation', async () => {
  const detailsHtml = await renderInlineDetails()

  assert.match(detailsHtml, /data-inline-log-details="merged"/)
  assert.match(detailsHtml, /Request ID/)
  assert.match(detailsHtml, /202607211431014826310688268d9d6UJDWU9My/)
  assert.match(detailsHtml, /Channel/)
  assert.match(detailsHtml, /7/)
  assert.match(detailsHtml, /Retry Chain/)
  assert.match(detailsHtml, /7 → 9/)
  assert.match(detailsHtml, /Path/)
  assert.match(detailsHtml, /\/v1\/chat\/completions/)
  assert.match(detailsHtml, /Native format/)
  assert.match(detailsHtml, /Input Tokens/)
  assert.match(detailsHtml, /Output Tokens/)
  assert.match(detailsHtml, /Cache Read/)
  assert.match(detailsHtml, /240 Tokens/)
  assert.match(detailsHtml, /Cache Write/)
  assert.match(detailsHtml, /80 Tokens/)
  assert.match(detailsHtml, /Calculation/)
  assert.match(detailsHtml, /Total Cost/)
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
  const typeHtml = await renderCell('type', true)
  const tokensHtml = await renderCell('prompt_tokens', true)
  const modelHtml = await renderCell('model_name', true)

  assert.match(layoutHtml, /data-all-sized="true"/)
  assert.match(layoutHtml, /data-has-type-column="true"/)
  assert.match(layoutHtml, /data-has-stream-column="true"/)
  assert.match(layoutHtml, /prompt_tokens\|use_time\|is_stream\|quota/)
  assert.match(layoutHtml, />1447</)
  assert.match(timeHtml, /font-mono/)
  assert.doesNotMatch(timeHtml, /font-mono[^"]*truncate/)
  assert.doesNotMatch(timeHtml, /data-slot="status-badge"/)
  assert.match(typeHtml, /data-slot="status-badge"/)
  assert.match(tokensHtml, /flex-col/)
  assert.match(tokensHtml, /flex-nowrap/)
  assert.doesNotMatch(tokensHtml, /flex-wrap/)
  assert.doesNotMatch(tokensHtml, /overflow-hidden/)
  assert.doesNotMatch(modelHtml, /truncate whitespace-nowrap/)
})
