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

import { buildOAuthPresetEndpoints, OAUTH_PRESETS } from '../types'

describe('custom OAuth presets', () => {
  test('uses fixed Google OpenID Connect endpoints without a base URL', () => {
    const google = OAUTH_PRESETS.find((preset) => preset.key === 'google')

    assert.ok(google)
    assert.equal(google.name, 'Google')
    assert.equal(google.icon, 'FcGoogle')
    assert.equal(google.needsBaseUrl, false)
    assert.equal(google.scopes, 'openid profile email')
    assert.equal(google.user_id_field, 'sub')
    assert.equal(google.username_field, 'preferred_username')
    assert.equal(google.display_name_field, 'name')
    assert.equal(google.email_field, 'email')
    assert.deepEqual(
      buildOAuthPresetEndpoints(google, 'https://ignored.example.com/'),
      {
        authorization_endpoint: 'https://accounts.google.com/o/oauth2/v2/auth',
        token_endpoint: 'https://oauth2.googleapis.com/token',
        user_info_endpoint: 'https://openidconnect.googleapis.com/v1/userinfo',
      }
    )
  })

  test('trims base URL before applying self-hosted preset endpoints', () => {
    const gitlab = OAUTH_PRESETS.find((preset) => preset.key === 'gitlab')

    assert.ok(gitlab)
    assert.deepEqual(
      buildOAuthPresetEndpoints(gitlab, 'https://gitlab.example.com///'),
      {
        authorization_endpoint: 'https://gitlab.example.com/oauth/authorize',
        token_endpoint: 'https://gitlab.example.com/oauth/token',
        user_info_endpoint: 'https://gitlab.example.com/api/v4/user',
      }
    )
  })
})
