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

import i18next from 'i18next'
import { renderToStaticMarkup } from 'react-dom/server'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { test } from 'vitest'

import { EmptyAboutState } from '..'

test('empty about page shows the centered compliance notice without provider reference links', async () => {
  const i18n = i18next.createInstance()
  await i18n.use(initReactI18next).init({ lng: 'en' })

  const html = renderToStaticMarkup(
    <I18nextProvider i18n={i18n}>
      <EmptyAboutState />
    </I18nextProvider>
  )

  assert.match(html, /Service Compliance Notice/)
  assert.match(html, /<section[^>]*class="[^"]*text-center/)
  assert.match(
    html,
    /MoleAPI does not provide services in any country or region where the relevant upstream provider, including OpenAI or Anthropic, has not made the corresponding service available\./
  )
  assert.doesNotMatch(html, /China|United States/)
  assert.doesNotMatch(html, /No About Content Set/)
  assert.doesNotMatch(html, /Current availability references/)
  assert.doesNotMatch(html, /supported-countries/)
})
