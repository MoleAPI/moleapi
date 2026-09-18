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
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { useState } from 'react'
import { I18nextProvider } from 'react-i18next'
import { beforeAll, describe, expect, test, vi } from 'vitest'

import { sendChatCompletion } from '@/features/playground/api'
import {
  DEFAULT_CONFIG,
  DEFAULT_PARAMETER_ENABLED,
} from '@/features/playground/constants'
import type { usePlaygroundState } from '@/features/playground/hooks'
import { useIsAdmin } from '@/hooks/use-admin'
import zh from '@/i18n/locales/zh.json'

import { TicketBrowser, TicketDetail } from '..'
import { updateSupportTicketStatus, type SupportTicket } from '../api'
import { CommunityChannels } from '../components/community-channels'
import { SupportAssistant } from '../components/support-assistant'
import { TicketCreateForm } from '../components/ticket-create-form'

vi.mock('@/features/playground/hooks', () => ({
  usePlaygroundOptions: () => ({ isLoadingModels: false }),
}))
vi.mock('@/hooks/use-admin', () => ({ useIsAdmin: vi.fn(() => false) }))
vi.mock('../api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api')>()),
  updateSupportTicketStatus: vi.fn().mockResolvedValue(null),
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
  beforeAll(() => {
    Element.prototype.getAnimations ??= () => []
  })
  const ticket: SupportTicket = {
    id: '42',
    ticketNumber: '1042',
    subject: 'API request failed',
    description:
      '<p>Please investigate</p><img src="https://tracker.example/pixel"><div style="background:url(https://tracker.example/bg)">Request details</div>',
    status: 'Open',
    category: 'API Integration',
    priority: 'Medium',
    email: 'alice@example.com',
    createdTime: '2026-09-18T08:00:00Z',
    modifiedTime: '2026-09-18T09:00:00Z',
    activity: 'customer',
    user: { id: 11, username: 'alice & co' },
  }

  test('renders ticket and message dates with the Chinese interface locale', async () => {
    vi.mocked(useIsAdmin).mockReturnValue(false)
    const i18n = createInstance()
    await i18n.init({ lng: 'zhCN', resources: { zhCN: zh } })
    render(
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={new QueryClient()}>
          <TicketBrowser
            loading={false}
            tickets={[ticket]}
            selectedTicket={ticket.id}
            onSelect={vi.fn()}
            detail={{
              ticket,
              conversations: [
                {
                  id: '1',
                  type: 'thread',
                  direction: 'out',
                  visibility: 'public',
                  summary: 'Support response',
                  content: '',
                  createdTime: ticket.modifiedTime,
                  fromEmailAddress: 'support@example.com',
                },
              ],
              attachments: [],
            }}
            detailLoading={false}
            detailError={false}
            onRetry={vi.fn()}
            reply=''
            onReplyChange={vi.fn()}
            onSend={vi.fn()}
            sending={false}
          />
        </QueryClientProvider>
      </I18nextProvider>
    )
    expect(screen.getByText('Support response')).toBeVisible()
    expect(screen.getAllByText(ticket.subject)).toHaveLength(2)
  })

  test('confirms closing, supports reopening, and blocks external message media', async () => {
    vi.mocked(useIsAdmin).mockReturnValue(false)
    const user = userEvent.setup()
    const queryClient = new QueryClient()
    const props = {
      data: { ticket, conversations: [], attachments: [] },
      reply: '',
      onReplyChange: vi.fn(),
      onBack: vi.fn(),
      onSend: vi.fn(),
      sending: false,
    }
    const view = render(
      <QueryClientProvider client={queryClient}>
        <TicketDetail {...props} />
      </QueryClientProvider>
    )
    expect(view.container.querySelector('img')).toBeNull()
    expect(
      view.container.querySelector('[style*="tracker.example"]')
    ).toBeNull()
    expect(screen.getByText('Please investigate')).toBeVisible()
    await user.click(screen.getByRole('button', { name: 'Close ticket' }))
    expect(updateSupportTicketStatus).not.toHaveBeenCalled()
    await user.click(
      within(screen.getByRole('alertdialog')).getByRole('button', {
        name: 'Close ticket',
      })
    )
    await waitFor(() =>
      expect(updateSupportTicketStatus).toHaveBeenCalledWith('42', 'Closed')
    )
    view.rerender(
      <QueryClientProvider client={queryClient}>
        <TicketDetail
          {...props}
          data={{ ...props.data, ticket: { ...ticket, status: 'Closed' } }}
        />
      </QueryClientProvider>
    )
    expect(
      screen.queryByRole('textbox', { name: 'Reply' })
    ).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Reopen ticket' }))
    await waitFor(() =>
      expect(updateSupportTicketStatus).toHaveBeenCalledWith('42', 'Open')
    )
  })

  test('prioritizes customer replies, filters tickets and links admin to user logs', async () => {
    vi.mocked(useIsAdmin).mockReturnValue(true)
    const user = userEvent.setup()
    const queryClient = new QueryClient()
    render(
      <QueryClientProvider client={queryClient}>
        <TicketBrowser
          loading={false}
          tickets={[
            {
              ...ticket,
              id: '43',
              subject: 'Answered request',
              activity: 'agent',
              modifiedTime: '2026-09-18T12:00:00Z',
            },
            {
              ...ticket,
              id: '44',
              subject: 'Resolved request',
              status: 'Closed',
            },
            ticket,
          ]}
          selectedTicket='42'
          onSelect={vi.fn()}
          detail={{ ticket, conversations: [], attachments: [] }}
          detailLoading={false}
          detailError={false}
          onRetry={vi.fn()}
          reply=''
          onReplyChange={vi.fn()}
          onSend={vi.fn()}
          sending={false}
        />
      </QueryClientProvider>
    )
    const rows = screen.getAllByRole('button', {
      name: /API request failed|Answered request/,
    })
    expect(rows[0]).toHaveTextContent('API request failed')
    expect(rows[0]).toHaveTextContent('alice & co · ID 11')
    expect(
      screen.queryByRole('button', { name: /Resolved request/ })
    ).not.toBeInTheDocument()
    const logs = screen.getByRole('link', { name: 'User logs' })
    expect(logs).toHaveAttribute(
      'href',
      '/usage-logs/common?username=alice%20%26%20co'
    )
    expect(logs).toHaveAttribute('target', '_blank')
    await user.click(screen.getByRole('tab', { name: 'Needs reply' }))
    expect(
      screen.queryByRole('button', { name: /Answered request/ })
    ).not.toBeInTheDocument()
    await user.type(
      screen.getByRole('textbox', { name: 'Search tickets' }),
      'no-match'
    )
    expect(screen.getByText('No matching tickets')).toBeVisible()
    await user.clear(screen.getByRole('textbox', { name: 'Search tickets' }))
    await user.click(screen.getByRole('tab', { name: 'Closed' }))
    expect(
      screen.getByRole('button', { name: /Resolved request/ })
    ).toBeVisible()
    await user.click(screen.getByRole('combobox', { name: 'Ticket status' }))
    await user.click(screen.getByRole('option', { name: 'Waiting' }))
    await waitFor(() =>
      expect(updateSupportTicketStatus).toHaveBeenCalledWith('42', 'On Hold')
    )
  })

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
