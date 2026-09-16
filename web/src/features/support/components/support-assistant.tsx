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
import { AiChat02Icon, ArrowUp02Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useMutation } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Spinner } from '@/components/ui/spinner'
import { Textarea } from '@/components/ui/textarea'
import { sendChatCompletion } from '@/features/playground/api'

import {
  TICKET_TYPES,
  type TicketType,
  parseAssistantReply,
} from '../constants'

type AssistantMessage = {
  id: string
  role: 'user' | 'assistant'
  content: string
}

export type AssistantTicketDraft = {
  type: TicketType
  subject: string
  content: string
}

export function SupportAssistant(props: {
  onCreateTicket: (draft: AssistantTicketDraft) => void
}) {
  const { t } = useTranslation()
  const [input, setInput] = useState('')
  const [messages, setMessages] = useState<AssistantMessage[]>([])
  const [suggestion, setSuggestion] = useState<{
    type: TicketType
    subject: string
  } | null>(null)

  const ask = useMutation({
    mutationFn: async (question: string) => {
      // ponytail: keep the last 10 turns to bound cost; add persistence only if
      // long-running support sessions become a real requirement.
      const recentMessages = [
        ...messages,
        { id: crypto.randomUUID(), role: 'user' as const, content: question },
      ].slice(-10)
      const response = await sendChatCompletion({
        model: 'deepseek-flash',
        stream: false,
        temperature: 0.2,
        max_tokens: 1200,
        messages: [
          {
            role: 'system',
            content: `You are MoleAPI support triage. Help users troubleshoot safely and concisely. Never ask for passwords, complete API keys, or other secrets. Ask for request IDs and redacted errors when useful. If the problem is not resolved, recommend creating a support ticket. Reply in the user's language. End every response with exactly one machine-readable line in this format: <ticket>{"type":"TYPE","subject":"SHORT SUBJECT"}</ticket>. TYPE must be one of: ${TICKET_TYPES.map((item) => item.value).join(', ')}.`,
          },
          ...recentMessages.map(({ role, content }) => ({ role, content })),
        ],
      })
      const content = response.choices?.[0]?.message?.content?.trim()
      if (!content) throw new Error('AI support returned an empty response.')
      return { question, parsed: parseAssistantReply(content) }
    },
    onSuccess: ({ question, parsed }) => {
      setMessages((current) => [
        ...current,
        { id: crypto.randomUUID(), role: 'user', content: question },
        {
          id: crypto.randomUUID(),
          role: 'assistant',
          content: parsed.answer,
        },
      ])
      setSuggestion({ type: parsed.type, subject: parsed.subject })
      setInput('')
    },
  })

  const createTicket = () => {
    const firstQuestion = messages.find(
      (message) => message.role === 'user'
    )?.content
    const subject = suggestion?.subject || firstQuestion?.slice(0, 200) || ''
    props.onCreateTicket({
      type: suggestion?.type ?? 'Other',
      subject:
        subject.trim().length >= 3 ? subject.trim() : t('AI support request'),
      content: [
        t('AI-assisted support conversation'),
        ...messages.map(
          (message) =>
            `${message.role === 'user' ? t('You') : t('AI support')}: ${message.content}`
        ),
      ].join('\n\n'),
    })
  }

  const submit = () => {
    const question = input.trim()
    if (!question || ask.isPending) return
    ask.mutate(question)
  }

  return (
    <div className='flex min-h-0 flex-1 flex-col'>
      <div className='border-b px-5 py-4'>
        <h2 className='font-semibold'>{t('Ask AI first')}</h2>
        <p className='text-muted-foreground mt-1 text-sm'>
          {t(
            'Get troubleshooting help and a suggested ticket category before contacting support.'
          )}
        </p>
      </div>

      <ScrollArea className='min-h-0 flex-1'>
        {messages.length === 0 ? (
          <Empty className='min-h-[25rem]'>
            <EmptyHeader>
              <EmptyMedia variant='icon'>
                <HugeiconsIcon icon={AiChat02Icon} />
              </EmptyMedia>
              <EmptyTitle>
                {t('Describe the problem you are seeing')}
              </EmptyTitle>
              <EmptyDescription>
                {t(
                  'AI can suggest checks, identify the right ticket type, and prepare the details for support.'
                )}
              </EmptyDescription>
            </EmptyHeader>
          </Empty>
        ) : (
          <div className='mx-auto flex w-full max-w-3xl flex-col gap-5 p-5'>
            {messages.map((message) => (
              <article
                key={message.id}
                className={
                  message.role === 'user'
                    ? 'bg-primary text-primary-foreground ml-auto max-w-[85%] rounded-xl px-4 py-3'
                    : 'bg-muted max-w-[92%] rounded-xl px-4 py-3'
                }
              >
                <p className='text-sm leading-relaxed whitespace-pre-wrap'>
                  {message.content}
                </p>
              </article>
            ))}
            {ask.isPending && (
              <div className='bg-muted flex w-fit items-center gap-2 rounded-xl px-4 py-3 text-sm'>
                <Spinner data-icon='inline-start' />
                {t('AI is checking...')}
              </div>
            )}
            {ask.isError && (
              <p className='text-destructive text-sm'>
                {t(
                  'AI support is temporarily unavailable. You can create a ticket directly.'
                )}
              </p>
            )}
          </div>
        )}
      </ScrollArea>

      <div className='shrink-0 border-t p-4'>
        {messages.length > 0 && (
          <div className='mb-3 flex justify-end'>
            <Button variant='outline' size='sm' onClick={createTicket}>
              {t('Create ticket from this conversation')}
            </Button>
          </div>
        )}
        <form
          className='flex items-end gap-2'
          onSubmit={(event) => {
            event.preventDefault()
            submit()
          }}
        >
          <Textarea
            rows={3}
            value={input}
            placeholder={t(
              'Describe the error, request ID, and what you already tried...'
            )}
            onChange={(event) => setInput(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === 'Enter' && !event.shiftKey) {
                event.preventDefault()
                submit()
              }
            }}
          />
          <Button
            type='submit'
            size='icon'
            disabled={!input.trim() || ask.isPending}
          >
            {ask.isPending ? (
              <Spinner />
            ) : (
              <HugeiconsIcon icon={ArrowUp02Icon} />
            )}
            <span className='sr-only'>{t('Send')}</span>
          </Button>
        </form>
        <p className='text-muted-foreground mt-2 text-xs'>
          {t(
            'Powered by deepseek-flash through Playground billing. Do not share secrets.'
          )}
        </p>
      </div>
    </div>
  )
}
