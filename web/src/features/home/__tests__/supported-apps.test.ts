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

import { isValidElement } from 'react'
import { test } from 'vitest'

import {
  SUPPORTED_APPS,
  SUPPORTED_APPS_MORE_KEY,
} from '../components/supported-apps'

test('supported application pills make clear that the list is not exhaustive', () => {
  assert.equal(SUPPORTED_APPS_MORE_KEY, 'More apps…')
})

test('supported application pills open the requested MoleAPI guides', () => {
  assert.deepEqual(
    SUPPORTED_APPS.map((app) => [app.name, app.href]),
    [
      [
        'Cherry Studio',
        'https://docs.moleapi.com/zh-CN/docs/apps/cherry-studio',
      ],
      ['CC Switch', 'https://docs.moleapi.com/zh-CN/docs/apps/cc-switch'],
      ['NextChat', 'https://docs.moleapi.com/zh-CN/docs/apps/nextchat'],
      ['Dify', 'https://docs.moleapi.com/zh-CN/docs/apps/dify'],
      ['LobeHub', 'https://docs.moleapi.com/zh-CN/docs/apps/lobechat'],
      ['AionUI', 'https://docs.moleapi.com/zh-CN/docs/apps/aionui'],
      ['Codex', 'https://docs.moleapi.com/zh-CN/docs/apps/codex-app'],
    ]
  )
})

test('supported application pills use bundled product artwork instead of initials', () => {
  const expectedIcons = new Map([
    ['CC Switch', '/app-icons/cc-switch.svg'],
    ['NextChat', '/app-icons/nextchat.svg'],
    ['AionUI', '/app-icons/aionui.svg'],
  ])

  for (const app of SUPPORTED_APPS) {
    const expectedSrc = expectedIcons.get(app.name)
    if (!expectedSrc) continue

    assert.ok(isValidElement<{ src?: string }>(app.icon))
    assert.equal(app.icon.props.src, expectedSrc)
  }
})
