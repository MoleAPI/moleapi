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
import { test } from 'vitest'

import i18next from 'i18next'
import type { ReactNode } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { I18nextProvider, initReactI18next } from 'react-i18next'

import { HeroTerminalDemo } from '../components/hero-terminal-demo'
import {
  API_DEMOS,
  PUBLIC_API_BASE_URL,
} from '../components/hero-terminal-demo-data'
import { HeroTypewriter } from '../components/hero-typewriter'
import { Stats } from '../components/sections/stats'

async function renderWithEnglish(element: ReactNode) {
  const i18n = i18next.createInstance()
  await i18n.use(initReactI18next).init({ lng: 'en' })

  return renderToStaticMarkup(
    <I18nextProvider i18n={i18n}>{element}</I18nextProvider>
  )
}

test('hero restores the simple-setup typewriter message', async () => {
  const html = await renderWithEnglish(<HeroTypewriter />)

  assert.match(html, /Simple setup, ready in moments/)
})

test('hero API demo shows the MoleAPI base URL and cycles common routes', async () => {
  const html = await renderWithEnglish(<HeroTerminalDemo />)

  assert.match(html, /https:\/\/api\.moleapi\.com/)
  assert.match(html, /\/v1\/chat\/completions/)
  assert.match(html, /cURL/)
  assert.match(html, /Python/)
  assert.match(html, /Node\.js/)
  assert.match(html, /Copy to clipboard/)
  assert.doesNotMatch(html, />Response</)
  assert.doesNotMatch(html, /200 ok/i)
  assert.doesNotMatch(html, /tokens · cost/i)
  assert.equal(PUBLIC_API_BASE_URL, 'https://api.moleapi.com')
  assert.deepEqual(
    API_DEMOS.map((demo) => demo.endpoint),
    [
      '/v1/chat/completions',
      '/v1/responses',
      '/v1/embeddings',
      '/v1/rerank',
      '/v1/moderations',
      '/v1/images/generations',
      '/v1/messages',
      '/v1beta/models/{model}:generateContent',
    ]
  )
})

test('model providers follow the four gateway capability totals', async () => {
  const html = await renderWithEnglish(<Stats />)

  assert.ok(
    html.indexOf('scheduling controls') < html.indexOf('Model Providers')
  )
  assert.match(html, /Model Providers/)
  assert.match(html, /OpenAI/)
})
