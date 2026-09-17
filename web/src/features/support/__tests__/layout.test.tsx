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
import { useState } from 'react'
import { I18nextProvider } from 'react-i18next'
import { describe, expect, test, vi } from 'vitest'

import { sendChatCompletion } from '@/features/playground/api'
import {
  DEFAULT_CONFIG,
  DEFAULT_PARAMETER_ENABLED,
} from '@/features/playground/constants'
import type { usePlaygroundState } from '@/features/playground/hooks'
import zh from '@/i18n/locales/zh.json'

import { CommunityChannels } from '../components/community-channels'
import { SupportAssistant } from '../components/support-assistant'
import { TicketCreateForm } from '../components/ticket-create-form'

vi.mock('@/features/playground/hooks', () => ({
  usePlaygroundOptions: () => ({ isLoadingModels: false }),
}))
vi.mock('@/features/playground/api', () => ({
  sendChatCompletion: vi.fn().mockResolvedValue({
    choices: [
      {
        message: {
          content:
            '{"subject":"接口返回 400 错误","content":"整理后的错误描述，包含请求 ID 和复现步骤。"}',
          role: 'assistant',
        },
      },
    ],
  }),
}))

describe('support page layout', () => {
  test('opens community channels on demand and disables unconfigured ones', async () => {
    const user = userEvent.setup()
    render(<CommunityChannels links={{}} />)

    expect(screen.queryByRole('button', { name: /QQ/ })).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Community' }))

    for (const platform of ['QQ', 'WeChat', 'Telegram', 'Discord']) {
      expect(
        screen.getByRole('button', { name: new RegExp(platform) })
      ).toBeDisabled()
    }
  })

  test('polishes title and description and restores the previous draft on undo', async () => {
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
    const subject = screen.getByLabelText('标题')
    await user.type(subject, '原始标题')
    await user.type(description, '调用接口后持续出现 400 错误，请帮忙排查。')
    await user.click(screen.getByRole('button', { name: 'AI 润色' }))

    await waitFor(() => expect(subject).toHaveValue('接口返回 400 错误'))
    expect(description).toHaveValue(
      '整理后的错误描述，包含请求 ID 和复现步骤。'
    )
    await user.click(screen.getByRole('button', { name: '撤销润色' }))
    expect(subject).toHaveValue('原始标题')
    expect(description).toHaveValue('调用接口后持续出现 400 错误，请帮忙排查。')
  })

  test('sends selected text files with an AI question', async () => {
    Element.prototype.getAnimations ??= () => []
    const user = userEvent.setup()
    let complete: (
      response: Awaited<ReturnType<typeof sendChatCompletion>>
    ) => void = () => undefined
    vi.mocked(sendChatCompletion).mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          complete = resolve
        })
    )
    const queryClient = new QueryClient()
    const updateMessages = vi.fn()
    const onCreateTicket = vi.fn()
    const state = {
      config: DEFAULT_CONFIG,
      parameterEnabled: DEFAULT_PARAMETER_ENABLED,
      messages: [],
      isLoadingMessages: false,
      models: [{ label: 'gpt-4o', value: 'gpt-4o' }],
      groups: [{ label: 'default', value: 'default' }],
      setModels: vi.fn(),
      setGroups: vi.fn(),
      updateConfig: vi.fn(),
      updateMessages,
      createConversation: vi.fn(),
    } as unknown as ReturnType<typeof usePlaygroundState>

    function AssistantHarness() {
      const [files, setFiles] = useState<File[]>([])
      return (
        <SupportAssistant
          state={state}
          files={files}
          onFilesChange={setFiles}
          onCreateTicket={onCreateTicket}
        />
      )
    }

    render(
      <QueryClientProvider client={queryClient}>
        <AssistantHarness />
      </QueryClientProvider>
    )

    await user.upload(
      screen.getByLabelText('Attachments'),
      new File(['HTTP 400: invalid model'], 'error.log', { type: 'text/plain' })
    )
    await user.type(
      screen.getByPlaceholderText(
        'Describe the error, request ID, and what you already tried...'
      ),
      'Why did this fail?'
    )
    await user.click(screen.getByRole('button', { name: 'Send' }))

    expect(screen.getByText('AI is checking...')).toBeInTheDocument()
    complete({
      choices: [
        {
          message: { role: 'assistant', content: 'Check the model name.' },
          index: 0,
          finish_reason: 'stop',
        },
      ],
    } as Awaited<ReturnType<typeof sendChatCompletion>>)
    await waitFor(() => expect(updateMessages).toHaveBeenCalled())
    expect(
      vi.mocked(sendChatCompletion).mock.lastCall?.[0].messages.at(-1)?.content
    ).toContain('error.log:\nHTTP 400: invalid model')
    await waitFor(() =>
      expect(
        screen.getByPlaceholderText(
          'Describe the error, request ID, and what you already tried...'
        )
      ).toHaveValue('')
    )

    await user.type(
      screen.getByPlaceholderText(
        'Describe the error, request ID, and what you already tried...'
      ),
      'Please investigate this manually.'
    )
    await user.click(
      screen.getByRole('button', { name: 'Request human support' })
    )
    expect(onCreateTicket).toHaveBeenCalledWith(
      expect.objectContaining({
        subject: 'Please investigate this manually.',
        content: expect.stringContaining('Please investigate this manually.'),
      })
    )
  })
})
