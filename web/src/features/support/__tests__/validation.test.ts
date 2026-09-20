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

import zh from '@/i18n/locales/zh.json'

import { buildTicketDescription, ticketSchema } from '../api'
import { INVOICE_TYPE, TICKET_TYPES, parseAssistantReply } from '../constants'

describe('support ticket validation', () => {
  test('accepts every supported ticket type', () => {
    for (const { value: type } of TICKET_TYPES) {
      expect(
        ticketSchema.safeParse({
          type,
          subject: 'Need help',
          content: 'A clear description of the request.',
          invoiceTitle: type === INVOICE_TYPE ? 'Mole API Ltd.' : '',
        }).success
      ).toBe(true)
    }
  })

  test('rejects unsupported types, short descriptions, and incomplete invoices', () => {
    expect(
      ticketSchema.safeParse({
        type: 'Unknown',
        subject: 'Hi',
        content: 'Too short',
      }).success
    ).toBe(false)
    expect(
      ticketSchema.safeParse({
        type: INVOICE_TYPE,
        subject: 'Invoice request',
        content: 'Please issue an invoice for my order.',
        invoiceTitle: '',
      }).success
    ).toBe(false)
  })

  test('adds invoice and billing information to the submitted description', () => {
    const description = buildTicketDescription(
      {
        type: INVOICE_TYPE,
        subject: 'Invoice request',
        content: 'Please issue an invoice for my order.',
        invoiceTitle: 'Mole API Ltd.',
        taxId: '123456',
        invoiceEmail: 'billing@example.com',
        invoiceAddressPhone: '',
        bankAccount: '',
      },
      [
        {
          id: 1,
          user_id: 1,
          amount: 10,
          money: 10,
          payment_currency: 'USD',
          trade_no: 'ORDER-1',
          payment_method: 'stripe',
          create_time: 1,
          status: 'success',
        },
      ]
    )

    expect(description).toContain('Invoice title: Mole API Ltd.')
    expect(description).toContain('ORDER-1 | USD 10 | stripe | success')
  })

  test('keeps every ticket type and template inside the active translation namespace', () => {
    for (const item of TICKET_TYPES) {
      expect(zh.translation[item.value]).toBeTruthy()
      expect(zh.translation[item.template]).toBeTruthy()
      expect(zh.translation[item.template]).not.toBe(item.template)
    }
  })

  test('extracts a safe ticket suggestion from the AI reply', () => {
    expect(
      parseAssistantReply(
        'Try a new API key.\n<ticket>{"type":"Authentication Issue","subject":"API key rejected"}</ticket>'
      )
    ).toEqual({
      answer: 'Try a new API key.',
      type: 'Authentication Issue',
      subject: 'API key rejected',
    })
    expect(
      parseAssistantReply(
        'More detail is needed.\n<ticket>{"type":"Unsafe Type","subject":42}</ticket>'
      )
    ).toEqual({
      answer: 'More detail is needed.',
      type: 'Other',
      subject: '',
    })
  })
})
