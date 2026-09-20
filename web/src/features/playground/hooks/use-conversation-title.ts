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
  onRename: (title: string) => void
}) {
  const requestedSessions = useRef(new Set<string>())
  const { currentTitle, group, messages, onRename, sessionId } = props

  useEffect(() => {
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
      requestedSessions.current.has(sessionId)
    ) {
      return
    }

    requestedSessions.current.add(sessionId)
    void sendChatCompletion({
      model: DEFAULT_MODEL,
      group,
      stream: false,
      messages: [
        {
          role: 'system',
          content:
            'Summarize the user support conversation as a short title. Reply with only the title, no quotes, markdown, or punctuation. Keep it under 30 characters and use the conversation language.',
        },
        { role: 'user', content: getMessageContent(firstUser) },
      ],
    })
      .then((response) => {
        const title = response.choices?.[0]?.message?.content
          ?.replace(/^title\s*:\s*/i, '')
          .split('\n')[0]
          .replaceAll(/^['"“”「」]+|['"“”「」]+$/g, '')
          .trim()
          .slice(0, 60)
        if (title) onRename(title)
      })
      .catch(() => {
        requestedSessions.current.delete(sessionId)
      })
  }, [
    currentTitle,
    group,
    messages,
    onRename,
    sessionId,
  ])
}
