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

globalThis.matchMedia ??= () => ({ matches: false }) as MediaQueryList
globalThis.customElements ??= {
  define: () => {},
  get: () => undefined,
} as unknown as CustomElementRegistry

const { InlineLogDetails } = await import('../../dialogs/details-dialog')
const { UsageLogsProvider } = await import('../../usage-logs-provider')
const { useCommonLogsColumns } = await import('../common-logs-columns')

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
  const columnOrder = columns.map((column) =>
    'accessorKey' in column ? column.accessorKey : column.id
  )
  const columnIds = new Set(columnOrder)
  const totalWidth = columns.reduce(
    (total, column) => total + (column.size ?? 0),
    0
  )

  return (
    <output
      data-all-sized={columns.every((column) => column.size != null)}
      data-has-type-column={columnIds.has('type')}
      data-has-stream-column={columnIds.has('is_stream')}
      data-use-time-count={columnOrder.filter((id) => id === 'use_time').length}
      data-column-order={columnOrder.join('|')}
    >
      {totalWidth}
    </output>
  )
}

async function renderCell(columnId: string, isAdmin = false, value = log) {
  const i18n = i18next.createInstance()
  await i18n.use(initReactI18next).init({ lng: 'en' })

  return renderToStaticMarkup(
    <I18nextProvider i18n={i18n}>
      <UsageLogsProvider>
        <TestCell columnId={columnId} isAdmin={isAdmin} value={value} />
      </UsageLogsProvider>
    </I18nextProvider>
  )
}

async function renderInlineDetails(value = log) {
  const i18n = i18next.createInstance()
  await i18n.use(initReactI18next).init({ lng: 'en' })

  return renderToStaticMarkup(
    <I18nextProvider i18n={i18n}>
      <InlineLogDetails log={value} isAdmin />
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
  assert.match(channelHtml, /ring-current\/15/)
  assert.equal(tokenHtml.match(/data-slot="status-badge"/g)?.length, 1)
  assert.match(tokenHtml, /ring-current\/15/)
  assert.match(groupHtml, /data-slot="status-badge"/)
  assert.match(groupHtml, /bg-(?:chart|success|warning|info|muted)/)
  assert.match(groupHtml, /ring-current\/15/)
  assert.match(groupHtml, /truncate/)
  assert.match(groupHtml, /font-normal/)
  assert.match(modelHtml, /data-slot="status-badge"/)
  assert.match(modelHtml, /bg-(?:chart|success|warning|info|muted)/)
  assert.match(modelHtml, /ring-current\/15/)
})

test('cache uses explicit words and numeric cost stays unboxed', async () => {
  const tokensHtml = await renderCell('prompt_tokens')
  const costHtml = await renderCell('quota')

  assert.match(tokensHtml, /Cache Read[^<]*240/)
  assert.match(tokensHtml, /Cache Write[^<]*80/)
  assert.match(tokensHtml, /!\s*text-\[14px\]|!text-\[14px\]/)
  assert.match(tokensHtml, /!\s*text-\[8px\]|!text-\[8px\]/)
  assert.doesNotMatch(tokensHtml, /[↓↑]/)
  assert.match(costHtml, /tabular-nums/)
  assert.doesNotMatch(costHtml, /\b(?:border|rounded|bg-)/)
})

test('group cell hides the multiplier', async () => {
  const groupRatioLog = usageLogSchema.parse({
    ...log,
    group: 'relay',
    other: JSON.stringify({
      group_ratio: 0.3,
    }),
  })
  const groupHtml = await renderCell('group', false, groupRatioLog)

  assert.match(groupHtml, /relay/)
  assert.doesNotMatch(groupHtml, /0\.3x/)
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

test('details preview keeps pricing first and omits neutral multiplier', async () => {
  const detailsHtml = await renderCell('content')

  assert.match(detailsHtml, /\$0\.14 \/ \$0\.56\/M · 1\.0/)
  assert.match(detailsHtml, /flex-col/)
  assert.match(detailsHtml, /Cache \$0\.014 \/ \$0\.175 · 1\.0/)
  assert.match(detailsHtml, /line-clamp-1/)
  assert.doesNotMatch(detailsHtml, /1\.0x/)
  assert.doesNotMatch(detailsHtml, /whitespace-nowrap/)
})

test('details preview appends group multiplier to price summary', async () => {
  const discountLog = usageLogSchema.parse({
    ...log,
    other: JSON.stringify({
      model_ratio: 0.07,
      completion_ratio: 4,
      group_ratio: 0.3,
    }),
  })
  const detailsHtml = await renderCell('content', false, discountLog)

  assert.match(detailsHtml, /\$0\.14 \/ \$0\.56\/M · 0\.3/)
  assert.doesNotMatch(detailsHtml, /0\.3x ·/)
})

test('per-call details append group multiplier after the price', async () => {
  const perCallLog = usageLogSchema.parse({
    ...log,
    other: JSON.stringify({
      model_price: 0.02,
      group_ratio: 0.3,
    }),
  })
  const detailsHtml = await renderCell('content', false, perCallLog)

  assert.match(detailsHtml, /Per-call · \$0\.02 · 0\.3/)
})

test('details column wraps long raw content', async () => {
  const longErrorLog = usageLogSchema.parse({
    ...log,
    type: 0,
    content: `provider_error_${'x'.repeat(160)}`,
    other: '',
  })
  const detailsHtml = await renderCell('content', false, longErrorLog)

  assert.match(detailsHtml, /provider_error_/)
  assert.match(detailsHtml, /break-all/)
  assert.match(detailsHtml, /line-clamp-2/)
  assert.match(detailsHtml, /whitespace-normal/)
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
  assert.match(
    detailsHtml,
    /\(Input .* \+ Output .* \+ Cache Read .* \+ Cache Write .*\) × 1\.0 default Group/
  )
  assert.match(detailsHtml, /1\.0x/)
  assert.doesNotMatch(detailsHtml, /1\.0000x/)
  assert.match(detailsHtml, /Total Cost/)
})

test('cache calculation follows OpenAI and Anthropic token semantics', async () => {
  const openAILog = usageLogSchema.parse({
    ...log,
    quota: 1188,
    prompt_tokens: 4_392,
    completion_tokens: 42,
    other: JSON.stringify({
      model_ratio: 0.5,
      completion_ratio: 6,
      cache_tokens: 3_840,
      cache_ratio: 0.1,
      group_ratio: 1,
      usage_semantic: 'openai',
    }),
  })
  const anthropicLog = usageLogSchema.parse({
    ...openAILog,
    other: JSON.stringify({
      model_ratio: 0.5,
      completion_ratio: 6,
      cache_tokens: 3_840,
      cache_ratio: 0.1,
      group_ratio: 1,
      usage_semantic: 'anthropic',
    }),
  })

  const openAIHtml = await renderInlineDetails(openAILog)
  const anthropicHtml = await renderInlineDetails(anthropicLog)

  assert.match(openAIHtml, /Input 552 × \$1\/M/)
  assert.match(openAIHtml, /Cache Read 3,840 × \$0\.1\/M/)
  assert.doesNotMatch(openAIHtml, /Input 4,392 × \$1\/M/)
  assert.match(anthropicHtml, /Input 4,392 × \$1\/M/)
  assert.match(anthropicHtml, /Cache Read 3,840 × \$0\.1\/M/)
})

test('ratio billing details separate image output from text output', async () => {
  const imageLog = usageLogSchema.parse({
    ...log,
    quota: 11_196,
    prompt_tokens: 9,
    completion_tokens: 186,
    other: JSON.stringify({
      model_ratio: 4,
      completion_ratio: 3.75,
      image_ratio: 15,
      image_output_ratio: 15,
      group_ratio: 1,
      image: true,
      image_output_tokens: 186,
      request_path: '/pg/chat/completions',
      request_conversion: ['OpenAI Compatible', 'openai_image'],
      admin_info: {
        use_channel: [74],
      },
    }),
  })

  const detailsHtml = await renderInlineDetails(imageLog)

  assert.match(detailsHtml, /Image Out/)
  assert.match(detailsHtml, /Image Output Tokens/)
  assert.match(detailsHtml, /Image Out 186 × \$120\/M/)
  assert.doesNotMatch(detailsHtml, /Image Out 186 × \$30\/M/)
  assert.doesNotMatch(detailsHtml, /Output 186/)
})

test('dynamic billing details use compact ratio formatting for media pricing logs', async () => {
  const tieredLog = usageLogSchema.parse({
    ...log,
    other: JSON.stringify({
      billing_mode: 'tiered_expr',
      expr_b64: Buffer.from(
        'tier("standard", p * 2 + c * 8 + img * 3 + ai * 10 + ao * 40)'
      ).toString('base64'),
      matched_tier: 'standard',
      group_ratio: 0.3,
      request_path: '/v1/chat/completions',
      request_conversion: ['OpenAI Compatible'],
      image_input_tokens: 20,
      audio_input_token_count: 30,
      admin_info: {
        use_channel: [7],
      },
    }),
  })

  const detailsHtml = await renderInlineDetails(tieredLog)
  const previewHtml = await renderCell('content', true, tieredLog)

  assert.match(detailsHtml, /Dynamic Pricing/)
  assert.match(detailsHtml, /Image In/)
  assert.match(detailsHtml, /Audio In/)
  assert.match(detailsHtml, /Audio Out/)
  assert.match(detailsHtml, /0\.3x/)
  assert.doesNotMatch(detailsHtml, /1\.0000x/)
  assert.match(previewHtml, /standard · \$2 \/ \$8\/M · 0\.3/)
  assert.match(previewHtml, /Image In \$3\/M · 0\.3/)
  assert.match(previewHtml, /Audio In \$10\/M · 0\.3/)
  assert.match(previewHtml, /flex-col/)
  assert.doesNotMatch(previewHtml, /1\.0x/)
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
  assert.match(layoutHtml, /data-use-time-count="1"/)
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
