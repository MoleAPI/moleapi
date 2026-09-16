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
import { describe, expect, test } from 'vitest'

import { ticketSchema } from '../api'

describe('support ticket validation', () => {
  test('accepts support and invoice tickets with useful descriptions', () => {
    for (const type of ['Support', 'Billing & Invoice']) {
      expect(
        ticketSchema.safeParse({
          type,
          subject: 'Need help',
          content: 'A clear description of the request.',
        }).success
      ).toBe(true)
    }
  })

  test('rejects unsupported types and short descriptions', () => {
    expect(
      ticketSchema.safeParse({
        type: 'Other',
        subject: 'Hi',
        content: 'Too short',
      }).success
    ).toBe(false)
  })
})
