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
  userFormSchema,
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
    exclusive_bonus_ratio: 1250,
  } satisfies User

  const defaults = transformUserToFormDefaults(user)
  assert.equal(defaults.invite_rebate_percent, 1.25)

  const payload = transformFormDataToPayload(defaults, user.id)
  assert.equal(payload.invite_rebate_ratio, 125)
  assert.equal(defaults.exclusive_bonus_percent, 12.5)
  assert.equal(payload.exclusive_bonus_ratio, 1250)
  assert.equal(
    transformFormDataToPayload(
      { ...defaults, exclusive_bonus_percent: 0 },
      user.id
    ).exclusive_bonus_ratio,
    0
  )
})

test('exclusive bonus rejects negative, excessive, and invalid percentages', () => {
  for (const exclusive_bonus_percent of [-1, 100.01, Number.NaN]) {
    assert.equal(
      userFormSchema.safeParse({ username: 'alice', exclusive_bonus_percent })
        .success,
      false
    )
  }
})
