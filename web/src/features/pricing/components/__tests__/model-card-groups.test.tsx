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
import { test, vi } from 'vitest'

import type { PricingModel } from '../../types'
import { ModelCard } from '../model-card'

vi.mock('@/lib/lobe-icon', () => ({ getLobeIcon: () => null }))

test('model card shows the price for every distinct group', async () => {
  const i18n = i18next.createInstance()
  await i18n.use(initReactI18next).init({ lng: 'en' })
  const model = {
    id: 1,
    model_name: 'group-priced-model',
    quota_type: 0,
    model_ratio: 1,
    completion_ratio: 2,
    enable_groups: ['standard', 'discount'],
    group_ratio: { standard: 1, discount: 0.5 },
  } satisfies PricingModel

  const html = renderToStaticMarkup(
    <I18nextProvider i18n={i18n}>
      <ModelCard model={model} selectedGroup='standard' onClick={() => {}} />
    </I18nextProvider>
  )
  const text = html.replaceAll(/<[^>]*>/g, ' ').replaceAll(/\s+/g, ' ')

  assert.match(text, /discount ×0\.5 Input \$1 \/1M Output \$2 \/1M/)
  assert.match(text, /standard ×1 Current Input \$2 \/1M Output \$4 \/1M/)
})
