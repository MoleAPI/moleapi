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

import type { User } from '../../types'
import {
  transformFormDataToPayload,
  transformUserToFormDefaults,
} from '../user-form'

test('user form converts invite rebate percent to backend basis points', () => {
  const user = {
    id: 1,
    username: 'alice',
    display_name: 'Alice',
    quota: 0,
    used_quota: 0,
    request_count: 0,
    group: 'default',
    status: 1,
    role: 1,
    invite_rebate_ratio: 125,
  } satisfies User

  const defaults = transformUserToFormDefaults(user)
  assert.equal(defaults.invite_rebate_percent, 1.25)

  const payload = transformFormDataToPayload(defaults, user.id)
  assert.equal(payload.invite_rebate_ratio, 125)
})
