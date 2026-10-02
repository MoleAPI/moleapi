import { useEffect, useRef } from 'react'

import { sendChatCompletion } from '../api'
import { DEFAULT_MODEL } from '../constants'
import { getConversationTitle } from '../lib'
import { getMessageContent } from '../lib/message/message-utils'
import type { Message } from '../types'

export function useConversationTitle(props: {
  messages: Message[]
  sessionId: string
  currentTitle: string
  group: string
  onRename: (sessionId: string, title: string) => void
}) {
  const requestedSessions = useRef(new Set<string>())
  const eligibleSessions = useRef(new Set<string>())
  const { currentTitle, group, messages, onRename, sessionId } = props

  useEffect(() => {
    if (sessionId && !currentTitle && messages.length === 0) {
      eligibleSessions.current.add(sessionId)
    }

    const firstUser = messages.find((message) => message.from === 'user')
    const lastMessage = messages.at(-1)
    const fallbackTitle = getConversationTitle(messages)
    if (
      !sessionId ||
      !firstUser ||
      !lastMessage ||
      lastMessage.from !== 'assistant' ||
      lastMessage.status !== 'complete' ||
      !fallbackTitle ||
      currentTitle !== fallbackTitle ||
      !eligibleSessions.current.has(sessionId) ||
      requestedSessions.current.has(sessionId)
    ) {
      return
    }

    // ponytail: try once and keep the first-message fallback if title generation fails.
    requestedSessions.current.add(sessionId)
    void sendChatCompletion({
      model: DEFAULT_MODEL,
      group,
      stream: false,
      max_tokens: 256,
      reasoning_effort: 'low',
      temperature: 0.2,
      messages: [
        {
          role: 'user',
          content: `Write a short title for the conversation below. Reply with only the title, no explanation, quotes, markdown, or punctuation. Keep it under 30 characters and use the conversation language.\n\nConversation:\n${getMessageContent(firstUser)}`,
        },
      ],
    })
      .then((response) => {
        const title = response.choices?.[0]?.message?.content
          ?.replace(/^title\s*:\s*/i, '')
          .split('\n')[0]
          .replaceAll(/^['"“”「」]+|['"“”「」]+$/g, '')
          .trim()
          .slice(0, 60)
        if (title) onRename(sessionId, title)
      })
      .catch(() => undefined)
  }, [currentTitle, group, messages, onRename, sessionId])
}
