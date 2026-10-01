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
import { renderHook, waitFor } from '@testing-library/react'
import { describe, expect, test, vi } from 'vitest'

import { sendChatCompletion } from '../api'
import type { Message } from '../types'
import { useConversationTitle } from './use-conversation-title'

vi.mock('../api', () => ({ sendChatCompletion: vi.fn() }))

const loadingMessages: Message[] = [
  {
    key: 'user-1',
    from: 'user',
    versions: [{ id: 'user-version-1', content: 'How do I reset my API key?' }],
    status: 'complete',
  },
  {
    key: 'assistant-1',
    from: 'assistant',
    versions: [{ id: 'assistant-version-1', content: '' }],
    status: 'loading',
  },
]

const completeMessages: Message[] = [
  loadingMessages[0],
  {
    ...loadingMessages[1],
    versions: [
      { id: 'assistant-version-1', content: 'Open the API key page.' },
    ],
    status: 'complete',
  },
]

describe('useConversationTitle', () => {
  test('caps a new title request and does not repeat it after a retry', async () => {
    vi.mocked(sendChatCompletion).mockResolvedValue({
      id: 'completion-1',
      object: 'chat.completion',
      created: 1,
      model: 'deepseek-flash',
      choices: [
        {
          index: 0,
          message: { role: 'assistant', content: 'Resetting API Keys' },
          finish_reason: 'stop',
        },
      ],
    })
    const onRename = vi.fn()
    const { rerender } = renderHook((props) => useConversationTitle(props), {
      initialProps: {
        messages: [] as Message[],
        sessionId: 'session-1',
        currentTitle: '',
        group: 'default',
        onRename,
      },
    })

    rerender({
      messages: loadingMessages,
      sessionId: 'session-1',
      currentTitle: 'How do I reset my API key?',
      group: 'default',
      onRename,
    })
    rerender({
      messages: completeMessages,
      sessionId: 'session-1',
      currentTitle: 'How do I reset my API key?',
      group: 'default',
      onRename,
    })

    await waitFor(() => expect(sendChatCompletion).toHaveBeenCalledOnce())
    expect(vi.mocked(sendChatCompletion).mock.lastCall?.[0]).toMatchObject({
      max_tokens: 256,
      reasoning_effort: 'low',
      temperature: 0.2,
      stream: false,
      messages: [
        {
          role: 'user',
          content: expect.stringContaining('How do I reset my API key?'),
        },
      ],
    })
    await waitFor(() =>
      expect(onRename).toHaveBeenCalledWith('session-1', 'Resetting API Keys')
    )

    rerender({
      messages: loadingMessages,
      sessionId: 'session-1',
      currentTitle: 'Resetting API Keys',
      group: 'default',
      onRename,
    })
    rerender({
      messages: completeMessages,
      sessionId: 'session-1',
      currentTitle: 'Resetting API Keys',
      group: 'default',
      onRename,
    })

    expect(sendChatCompletion).toHaveBeenCalledOnce()
  })

  test('does not generate a title when an existing conversation is opened', () => {
    renderHook(() =>
      useConversationTitle({
        messages: completeMessages,
        sessionId: 'stored-session',
        currentTitle: 'How do I reset my API key?',
        group: 'default',
        onRename: vi.fn(),
      })
    )

    expect(sendChatCompletion).not.toHaveBeenCalled()
  })
})
