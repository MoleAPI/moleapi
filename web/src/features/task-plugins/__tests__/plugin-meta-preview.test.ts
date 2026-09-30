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
import { expect, test } from 'vitest'

import {
  parsePluginMetaPreview,
  resolvePluginMetaPreview,
} from '../lib/plugin-meta-preview'
import type { MarketplacePlugin } from '../types'

const plugin: MarketplacePlugin = {
  key: 'incho',

  name: 'Incho',

  latest: '1.0.1',

  versions: [
    { version: '1.0.1', path: 'new.js' },

    { version: '1.0.0', path: 'old.js', baseUrl: 'https://old.example.com' },
  ],

  models: ['latest-model'],

  protocols: ['openai_video'],

  channelTypes: [61],
}

test('previews TypeSafe model constants used by a read-only validation hook', () => {
  const preview = parsePluginMetaPreview(`

    const MODELS = ['jev-1.13.0', 'jev-latest', 'jev-preview'];

    export const meta = {

      models: MODELS,

      routes: [{method: 'POST', path: '/typesafe/v1/systemone', type: 'submit'}],

    };

    function validate(model) { return MODELS.includes(model); }

  `)

  expect(preview.status).toBe('parsed')

  expect(preview.fields.models).toEqual({
    state: 'value',

    origin: 'source',

    value: ['jev-1.13.0', 'jev-latest', 'jev-preview'],
  })
})

test('resolves earlier local constants and nested aliases without using index hints', () => {
  const preview = parsePluginMetaPreview(`

    const MODEL = 'source-model', MODELS = [MODEL];

    const ALIAS /* comment */ = /* comment */ MODELS;

    const ROUTE = {method: 'POST', path: '/source/submit', type: 'submit', models: ALIAS};

    export const meta = {models: ALIAS, routes: [ROUTE]};

  `)

  expect(preview.status).toBe('parsed')

  const fields = resolvePluginMetaPreview(plugin, plugin.versions[0], preview)

  expect(fields.models).toEqual({
    state: 'value',

    origin: 'source',

    value: ['source-model'],
  })

  expect(fields.routes).toEqual({
    state: 'value',

    origin: 'source',

    value: [
      {
        method: 'POST',

        path: '/source/submit',

        type: 'submit',

        models: ['source-model'],
      },
    ],
  })
})

test.each([
  'let MODELS = ["m"];',

  'export const MODELS = ["m"];',

  'const MODELS = getModels();',

  'const MODELS = LATER; const LATER = ["m"];',

  'const MODELS = ALIAS; const ALIAS = MODELS;',

  'const MODELS = ["m"]; MODELS.push("changed");',

  'const MODELS = ["m"]; MODELS[0] = "changed";',

  'const MODELS = ["m"]; mutate(MODELS);',

  'const MODELS = ["m"]; const ALIAS = MODELS; ALIAS.push("changed");',

  'const MODELS = ["m"]; const BOX = {models: MODELS}; mutate(BOX);',

  'const MODELS = ["m"]; const BOX = {models: MODELS, includes() { this.models.push("changed"); }}; BOX.includes();',

  'const MODELS = ["m"]; export {MODELS};',

  'const MODELS = ["m"]; function expose() { return MODELS; }',

  'const MODELS = ["m"]; function change(MODELS) { MODELS.push("changed"); }',
])('keeps unsafe or non-static model constants unknown: %s', (declaration) => {
  const preview = parsePluginMetaPreview(`

    ${declaration}

    export const meta = {models: MODELS, baseUrl: 'https://example.com'};

  `)

  expect(preview.status).toBe('partial')

  expect(preview.fields.models).toEqual({ state: 'unknown' })

  expect(preview.fields.baseUrl).toEqual({
    state: 'value',

    origin: 'source',

    value: 'https://example.com',
  })
})

test('rejects mutations after metadata initialization through a constant alias', () => {
  const preview = parsePluginMetaPreview(`

    const MODELS = ['m'];

    export const meta = {models: MODELS};

    const ALIAS = MODELS;

    ALIAS.splice(0, 1);

  `)

  expect(preview.status).toBe('partial')

  expect(preview.fields.models).toEqual({ state: 'unknown' })
})

test('bounds repeated constant expansion and keeps unrelated literal fields readable', () => {
  const declarations = ["const LEVEL0 = ['m'];"]

  for (let i = 1; i <= 16; i += 1) {
    declarations.push(`const LEVEL${i} = [LEVEL${i - 1}, LEVEL${i - 1}];`)
  }

  const preview = parsePluginMetaPreview(`

    ${declarations.join('\n')}

    export const meta = {

      routes: [{method: 'POST', path: '/jobs', type: 'submit', extra: LEVEL16}],

      models: ['m'],

    };

  `)

  expect(preview.status).toBe('partial')

  expect(preview.fields.routes).toEqual({ state: 'unknown' })

  expect(preview.fields.models).toEqual({
    state: 'value',

    origin: 'source',

    value: ['m'],
  })
})
