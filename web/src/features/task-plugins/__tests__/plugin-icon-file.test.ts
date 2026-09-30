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

import { describe, test } from 'vitest'

import { fetchPluginIconDataUri } from '../lib/plugin-icon-file'

describe('fetchPluginIconDataUri', () => {
  const okResponse = (body: string) =>
    new Response(body, {
      status: 200,

      headers: { 'content-type': 'image/svg+xml' },
    })

  test('revalidates with the host so a stale browser-cached logo does not fail the digest', async () => {
    const result = await fetchPluginIconDataUri(
      'https://raw.example/plugins/tasks/incho/icon.svg',

      {
        sha256:
          'd4dc56669143034f31aa309635d4113d9ad76a02b1739da22c965ed2049be9e6',

        fetchImpl: async (_input, init) =>
          okResponse(
            ['no-cache', 'no-store', 'reload'].includes(
              init?.cache ?? 'default'
            )
              ? '<svg/>'
              : '<svg>stale</svg>'
          ),
      }
    )

    assert.equal(result, 'data:image/svg+xml;base64,PHN2Zy8+')
  })
})
