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

import { buildApiBaseUrl } from '../api-endpoint'

describe('API key endpoint guidance', () => {
  test('uses the configured address without duplicating the API version', () => {
    assert.equal(
      buildApiBaseUrl(' https://api.example.com/ ', 'https://fallback.test'),
      'https://api.example.com/v1'
    )
    assert.equal(
      buildApiBaseUrl('https://api.example.com/v1/', 'https://fallback.test'),
      'https://api.example.com/v1'
    )
  })

  test('falls back to the current site when no address is configured', () => {
    assert.equal(
      buildApiBaseUrl('', 'https://console.example.com/'),
      'https://console.example.com/v1'
    )
  })
})
