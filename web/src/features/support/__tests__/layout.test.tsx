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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { describe, expect, test, vi } from 'vitest'

import zh from '@/i18n/locales/zh.json'

import { CommunityChannels } from '../components/community-channels'
import { TicketCreateForm } from '../components/ticket-create-form'

vi.mock('@/features/playground/api', () => ({
  sendChatCompletion: vi.fn().mockResolvedValue({
    choices: [
      {
        message: {
          content: '整理后的错误描述，包含请求 ID 和复现步骤。',
          role: 'assistant',
        },
      },
    ],
  }),
}))

describe('support page layout', () => {
  test('always shows four community channels and disables unconfigured ones', () => {
    render(<CommunityChannels links={{}} />)

    for (const platform of ['QQ', 'WeChat', 'Telegram', 'Discord']) {
      expect(
        screen.getByRole('button', { name: new RegExp(platform) })
      ).toBeDisabled()
    }
  })

  test('shows the translated selected ticket type and can polish its description', async () => {
    const user = userEvent.setup()
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    const i18n = createInstance()
    await i18n.init({
      lng: 'zh',
      resources: { zh },
    })

    render(
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={queryClient}>
          <TicketCreateForm
            accountEmail='user@example.com'
            onCreated={() => undefined}
          />
        </QueryClientProvider>
      </I18nextProvider>
    )

    expect(screen.getByLabelText('工单类型')).toHaveTextContent('API 接入')

    const description = screen.getByLabelText('详细描述')
    await user.type(description, '调用接口后持续出现 400 错误，请帮忙排查。')
    await user.click(screen.getByRole('button', { name: 'AI 润色' }))

    await waitFor(() =>
      expect(description).toHaveValue(
        '整理后的错误描述，包含请求 ID 和复现步骤。'
      )
    )
  })
})
