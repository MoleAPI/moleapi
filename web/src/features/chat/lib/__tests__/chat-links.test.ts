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
import { describe, test } from 'node:test'

import { resolveChatServerAddress, resolveChatUrl } from '../chat-links'

describe('chat links', () => {
  test('uses the MoleAPI API host instead of the home host', () => {
    const serverAddress = resolveChatServerAddress(
      { server_address: 'https://home.moleapi.com' },
      'https://home.moleapi.com'
    )

    assert.equal(serverAddress, 'https://api.moleapi.com')
    assert.equal(
      resolveChatUrl({
        template:
          'https://b.nextapi.fun/#/?settings={"key":"{key}","url":"{address}"}',
        apiKey: 'sk-test',
        serverAddress,
      }),
      'https://b.nextapi.fun/#/?settings={"key":"sk-test","url":"https%3A%2F%2Fapi.moleapi.com"}'
    )
  })

  test('prefers configured API routes for chat links', () => {
    assert.equal(
      resolveChatServerAddress(
        {
          server_address: 'https://home.moleapi.com',
          api_info: [
            { url: 'https://jp.moleapi.com', route: '日本专线' },
            { url: 'https://api.moleapi.com', route: '默认线路' },
          ],
        },
        'https://home.moleapi.com'
      ),
      'https://api.moleapi.com'
    )
  })
})
