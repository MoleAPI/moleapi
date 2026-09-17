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
import {
  AiChat02Icon,
  ArrowUp02Icon,
  PlusSignIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useMutation } from '@tanstack/react-query'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Response } from '@/components/ai-elements/response'
import { ModelGroupSelector } from '@/components/model-group-selector'
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
  usePlaygroundOptions,
  type usePlaygroundState,
} from '@/features/playground/hooks'
import {
  buildChatCompletionPayload,
  getMessageContent,
} from '@/features/playground/lib'

import {
  TICKET_TYPES,
  type TicketType,
  parseAssistantReply,
} from '../constants'
import { AttachmentPicker } from './attachment-picker'

export type AssistantTicketDraft = {
  type: TicketType
  subject: string
  content: string
  files?: File[]
}

export function SupportAssistant(props: {
  onCreateTicket: (draft: AssistantTicketDraft) => void
  state: ReturnType<typeof usePlaygroundState>
  files: File[]
  onFilesChange: (files: File[]) => void
  onBusyChange?: (busy: boolean) => void
}) {
  const { t } = useTranslation()
  const onBusyChange = props.onBusyChange
  const [input, setInput] = useState('')
  const [suggestion, setSuggestion] = useState<{
    type: TicketType
    subject: string
    sessionId: string
  } | null>(null)
  const {
    config,
    parameterEnabled,
    messages,
    activeSessionId,
    isLoadingMessages,
    models,
    groups,
    setModels,
    setGroups,
    updateConfig,
    updateMessages,
    createConversation,
  } = props.state
  const activeSessionRef = useRef(activeSessionId)
  const messageEndRef = useRef<HTMLDivElement>(null)
  activeSessionRef.current = activeSessionId
  const { isLoadingModels } = usePlaygroundOptions({
    currentGroup: config.group,
    currentModel: config.model,
    setGroups,
    setModels,
    updateConfig,
  })
  const hasReadableFiles = props.files.some(
    (file) =>
      file.type.startsWith('image/') ||
      file.type === 'text/plain' ||
      file.name.endsWith('.log')
  )

  const ask = useMutation({
    mutationFn: async (question: string) => {
      const textFiles = await Promise.all(
        props.files
          .filter(
            (file) => file.type === 'text/plain' || file.name.endsWith('.log')
          )
          .map(
            async (file) =>
              `${file.name}:\n${(await file.text()).slice(0, 6000)}`
          )
      )
      const content = [question, ...textFiles].filter(Boolean).join('\n\n')
      const userMessage = {
        key: crypto.randomUUID(),
        from: 'user' as const,
        versions: [{ id: crypto.randomUUID(), content }],
      }
      // ponytail: ten recent messages keep support costs bounded without a
      // separate server-side conversation store.
      const payload = buildChatCompletionPayload(
        [...messages, userMessage].slice(-10),
        { ...config, stream: false },
        parameterEnabled
      )
      const images = await Promise.all(
        props.files
          .filter((file) => file.type.startsWith('image/'))
          .map(
            (file) =>
              new Promise<string>((resolve, reject) => {
                const reader = new FileReader()
                reader.addEventListener(
                  'load',
                  () => resolve(String(reader.result)),
                  { once: true }
                )
                reader.addEventListener('error', () => reject(reader.error), {
                  once: true,
                })
                reader.readAsDataURL(file)
              })
          )
      )
      if (images.length > 0) {
        const last = payload.messages.at(-1)
        if (last) {
          last.content = [
            {
              type: 'text',
              text: content || 'Please inspect the attached image.',
            },
            ...images.map((url) => ({
              type: 'image_url' as const,
              image_url: { url },
            })),
          ]
        }
      }
      payload.messages.unshift({
        role: 'system',
        content: `You are MoleAPI support triage. Help users troubleshoot safely and concisely. Never ask for passwords, complete API keys, or other secrets. Ask for request IDs and redacted errors when useful. If the problem is not resolved, recommend creating a support ticket. Reply in the user's language. End every response with exactly one machine-readable line in this format: <ticket>{"type":"TYPE","subject":"SHORT SUBJECT"}</ticket>. TYPE must be one of: ${TICKET_TYPES.map((item) => item.value).join(', ')}.`,
      })
      const response = await sendChatCompletion(payload)
      const answer = response.choices?.[0]?.message?.content?.trim()
      if (!answer) throw new Error('AI support returned an empty response.')
      return {
        userMessage,
        parsed: parseAssistantReply(answer),
        sessionId: activeSessionId,
      }
    },
    onSuccess: ({ userMessage, parsed, sessionId }) => {
      if (sessionId !== activeSessionRef.current) {
        return
      }
      updateMessages((current) => [
        ...current,
        userMessage,
        {
          key: crypto.randomUUID(),
          from: 'assistant',
          versions: [{ id: crypto.randomUUID(), content: parsed.answer }],
          status: 'complete',
        },
      ])
      setSuggestion({ type: parsed.type, subject: parsed.subject, sessionId })
      setInput('')
    },
    onError: () =>
      toast.error(
        t(
          'AI support is temporarily unavailable. You can create a ticket directly.'
        )
      ),
  })

  useEffect(() => {
    onBusyChange?.(ask.isPending)
    return () => onBusyChange?.(false)
  }, [ask.isPending, onBusyChange])

  useEffect(() => {
    messageEndRef.current?.scrollIntoView?.({ block: 'end' })
  }, [messages.length, ask.isPending])

  const createTicket = () => {
    const firstQuestion = messages.find((message) => message.from === 'user')
    const firstQuestionContent = firstQuestion
      ? getMessageContent(firstQuestion)
      : ''
    const currentSuggestion =
      suggestion?.sessionId === activeSessionId ? suggestion : null
    const subject =
      currentSuggestion?.subject ||
      (firstQuestionContent || input.trim()).slice(0, 200)
    props.onCreateTicket({
      type: currentSuggestion?.type ?? 'Other',
      subject:
        subject.trim().length >= 3 ? subject.trim() : t('AI support request'),
      content: [
        t('AI-assisted support conversation'),
        ...messages.map(
          (message) =>
            `${message.from === 'user' ? t('You') : t('AI support')}: ${getMessageContent(message)}`
        ),
        ...(input.trim() ? [`${t('You')}: ${input.trim()}`] : []),
      ]
        .join('\n\n')
        .slice(0, 10000),
    })
  }

  const submit = () => {
    const question = input.trim()
    if ((!question && !hasReadableFiles) || ask.isPending) return
    ask.mutate(question)
  }

  return (
    <div className='flex min-h-0 flex-1 flex-col'>
      <div className='border-b px-4 py-3 sm:px-5'>
        <div className='flex flex-wrap items-center justify-between gap-3'>
          <div>
            <h2 className='font-semibold'>{t('Ask AI')}</h2>
          </div>
          <div className='flex flex-wrap items-center gap-2'>
            <Button
              variant='outline'
              size='sm'
              onClick={createTicket}
              disabled={
                (messages.length === 0 &&
                  !input.trim() &&
                  props.files.length === 0) ||
                ask.isPending
              }
            >
              {t('Request human support')}
            </Button>
            <Button
              aria-label={t('New chat')}
              disabled={ask.isPending || isLoadingMessages}
              onClick={() => {
                setSuggestion(null)
                props.onFilesChange([])
                createConversation()
              }}
              size='icon-sm'
              variant='outline'
            >
              <HugeiconsIcon icon={PlusSignIcon} />
            </Button>
          </div>
        </div>
      </div>

      <ScrollArea className='min-h-0 flex-1'>
        {messages.length === 0 && !ask.isPending ? (
          <Empty className='min-h-full'>
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
          <div className='mx-auto flex w-full max-w-3xl flex-col gap-5 p-4 sm:p-6'>
            {messages.map((message) => (
              <article
                key={message.key}
                className={
                  message.from === 'user'
                    ? 'bg-primary text-primary-foreground ml-auto max-w-[85%] rounded-xl px-4 py-3'
                    : 'bg-muted max-w-[92%] rounded-xl px-4 py-3'
                }
              >
                {message.from === 'assistant' ? (
                  <Response>{getMessageContent(message)}</Response>
                ) : (
                  <p className='text-sm leading-relaxed whitespace-pre-wrap'>
                    {getMessageContent(message)}
                  </p>
                )}
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
            <div ref={messageEndRef} aria-hidden='true' />
          </div>
        )}
      </ScrollArea>

      <div className='shrink-0 border-t p-4'>
        <div className='mb-3 flex flex-wrap items-center gap-2'>
          <ModelGroupSelector
            selectedModel={config.model}
            models={models}
            onModelChange={(value) => updateConfig('model', value)}
            selectedGroup={config.group}
            groups={groups}
            onGroupChange={(value) => updateConfig('group', value)}
            disabled={ask.isPending || isLoadingModels}
          />
          <span className='text-muted-foreground text-xs'>
            {t('Uses your Playground model and balance. Do not share secrets.')}
          </span>
        </div>
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
              if (
                event.key === 'Enter' &&
                !event.shiftKey &&
                !event.nativeEvent.isComposing
              ) {
                event.preventDefault()
                submit()
              }
            }}
          />
          <Button
            type='submit'
            size='icon'
            disabled={
              (!input.trim() && !hasReadableFiles) ||
              ask.isPending ||
              isLoadingMessages
            }
          >
            {ask.isPending ? (
              <Spinner />
            ) : (
              <HugeiconsIcon icon={ArrowUp02Icon} />
            )}
            <span className='sr-only'>{t('Send')}</span>
          </Button>
        </form>
        <div className='mt-3'>
          <AttachmentPicker
            files={props.files}
            onFilesChange={props.onFilesChange}
            onRejected={(fileName) =>
              toast.error(
                t('{{file}} exceeds the 5 MB attachment limit.', {
                  file: fileName,
                })
              )
            }
            compact
          />
          <p className='text-muted-foreground mt-1 text-xs'>
            {t(
              'AI can read images and text files. Other files will be included with a human support request.'
            )}
          </p>
        </div>
      </div>
    </div>
  )
}
